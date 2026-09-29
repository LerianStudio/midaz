//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type revertOnceHarness struct {
	*feeHarness
	engineRedis *redistransaction.RedisConsumerRepository
	v1, v2      *fiber.App
}

// setupRevertOnceHarness wires the real engine, completer, evidence resolver and
// protected recovery acknowledgement, as bootstrap does.
func setupRevertOnceHarness(t *testing.T) *revertOnceHarness {
	t.Helper()

	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)

	engineRedis, ok := h.redisRepo.(*redistransaction.RedisConsumerRepository)
	require.True(t, ok)

	h.queryUC.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}
	h.commandUC.TransactionEvidenceResolver = testEngineEvidenceResolver{repository: engineRedis}
	h.commandUC.EngineRecoveryAcknowledger = &atomicBatchHTTPRecoveryAcknowledger{repository: engineRedis, completedAt: time.Now()}

	return &revertOnceHarness{feeHarness: h, engineRedis: engineRedis, v1: h.newApp(), v2: h.newV2App()}
}

// expectBalances compares the live (Redis-first) balances and, after draining the
// balance sync schedule, the PostgreSQL rows.
func (h *revertOnceHarness) expectBalances(t *testing.T, label, src, dst string, srcID, dstID uuid.UUID, wantSrc, wantDst int64) {
	t.Helper()

	live := func(alias string) decimal.Decimal {
		got, err := h.queryUC.GetBalances(h.ctx(), h.orgID, h.ledgerID, []string{mtransaction.AliasKey(alias, "default")})
		require.NoError(t, err)
		require.Len(t, got, 1)

		return got[0].Available
	}

	srcRedis, dstRedis := live(src), live(dst)
	drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)
	srcPG, dstPG := postgrestestutil.GetBalanceAvailable(t, h.db, srcID), postgrestestutil.GetBalanceAvailable(t, h.db, dstID)

	assert.Truef(t, srcRedis.Equal(decimal.NewFromInt(wantSrc)) && dstRedis.Equal(decimal.NewFromInt(wantDst)),
		"%s: Redis src=%s dst=%s", label, srcRedis, dstRedis)
	assert.Truef(t, srcPG.Equal(decimal.NewFromInt(wantSrc)) && dstPG.Equal(decimal.NewFromInt(wantDst)),
		"%s: PostgreSQL src=%s dst=%s", label, srcPG, dstPG)
}

func (h *revertOnceHarness) hasRecoveryRecord(t *testing.T, txID uuid.UUID) bool {
	t.Helper()

	records, err := h.engineRedis.ReadAllRecoveryMessages(h.ctx(), redistransaction.RecoveryQueueSourceEngineRecover)
	require.NoError(t, err)

	for field := range records {
		if strings.HasPrefix(field, txID.String()+":") {
			return true
		}
	}

	return false
}

func (h *revertOnceHarness) countReverts(t *testing.T, originID uuid.UUID) int {
	t.Helper()

	var count int
	require.NoError(t, h.db.QueryRow(`SELECT COUNT(*) FROM transaction WHERE parent_transaction_id = $1`, originID).Scan(&count))

	return count
}

func (h *revertOnceHarness) revertMarkerExists(t *testing.T, originID uuid.UUID) bool {
	t.Helper()

	guards := "engine:" + cachepolicy.HashTag + ":guards:" + h.orgID.String() + ":" + h.ledgerID.String()
	exists, err := h.redisContainer.Client.HExists(h.ctx(), guards, originID.String()+":reverted").Result()
	require.NoError(t, err)

	return exists
}

// expireRevertIdempotencySlot deletes the idempotency slot caching revertID,
// standing in for its 300s TTL.
func (h *revertOnceHarness) expireRevertIdempotencySlot(t *testing.T, revertID uuid.UUID) {
	t.Helper()

	client := h.redisContainer.Client
	prefix := "idempotency:{" + h.orgID.String() + ":" + h.ledgerID.String() + ":"

	require.Eventually(t, func() bool {
		keys, err := client.Keys(h.ctx(), prefix+"*").Result()
		require.NoError(t, err)

		for _, key := range keys {
			if value, err := client.Get(h.ctx(), key).Result(); err == nil && strings.Contains(value, revertID.String()) {
				require.NoError(t, client.Del(h.ctx(), key).Err())

				return true
			}
		}

		return false
	}, 5*time.Second, 20*time.Millisecond, "idempotency slot of %s", revertID)
}

// unavailableCompleter stands in for a PostgreSQL or MongoDB outage after the
// engine applied a result: the projection is deferred to recovery.
type unavailableCompleter struct{}

func (unavailableCompleter) Complete(context.Context, *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	return command.TransactionCompletionResult{}, errors.New("projection store unavailable")
}

func requireAlreadyReverted(t *testing.T, label string, resp txResponse) {
	t.Helper()

	require.Equalf(t, 409, resp.status, "%s: %s", label, resp.rawBody)
	assert.Equalf(t, cn.ErrTransactionIDHasAlreadyParentTransaction.Error(), resp.body["code"], "%s: %s", label, resp.rawBody)
}

// TestIntegration_RevertOnceAfterStrandedRevert reverts an origin whose first
// revert moved money but never reached PostgreSQL (its projection store was
// unavailable). Every later revert is refused by the engine.
func TestIntegration_RevertOnceAfterStrandedRevert(t *testing.T) {
	h := setupRevertOnceHarness(t)

	src, dst, funder := "@ro-src", "@ro-dst", "@ro-funder"
	srcID := h.seedBalance(t, src, "USD", decimal.NewFromInt(1000), "deposit")
	dstID := h.seedBalance(t, dst, "USD", decimal.Zero, "deposit")
	h.seedBalance(t, funder, "USD", decimal.NewFromInt(1000), "deposit")

	fundDestination := func() {
		t.Helper()

		body := h.v2Body("funding", "USD", "100", []string{h.v2Leg(funder, "100")}, []string{h.v2Leg(dst, "100")})
		resp := h.createV2Direct(t, h.v2, body, map[string]string{"X-Idempotency": uuid.NewString()})
		require.Equalf(t, 201, resp.status, "funding: %s", resp.rawBody)
	}

	// X-TTL 3600 keeps the origin's engine index after the revert's 300s slot expires.
	body := h.v2Body("origin", "USD", "100", []string{h.v2Leg(src, "100")}, []string{h.v2Leg(dst, "100")})
	created := h.createV2Direct(t, h.v2, body, map[string]string{"X-TTL": "3600"})
	require.Equalf(t, 201, created.status, "create: %s", created.rawBody)
	origin := mustTxID(t, created)
	h.expectBalances(t, "after origin", src, dst, srcID, dstID, 900, 100)

	completer := h.commandUC.AppliedTransactionCompleter
	h.commandUC.AppliedTransactionCompleter = unavailableCompleter{}
	first := h.post(t, h.v2, h.v2StatePath(origin, "revert"), "", nil)
	h.commandUC.AppliedTransactionCompleter = completer
	require.Equalf(t, 201, first.status, "first revert: %s", first.rawBody)
	firstID := mustTxID(t, first)
	h.expectBalances(t, "after first revert (stranded)", src, dst, srcID, dstID, 1000, 0)
	require.Zero(t, h.countReverts(t, origin), "the first revert strands: no PostgreSQL row")
	require.True(t, h.hasRecoveryRecord(t, firstID))
	require.True(t, h.revertMarkerExists(t, origin))

	// The revert's slot expires while its record is still missing from PostgreSQL.
	h.expireRevertIdempotencySlot(t, firstID)
	fundDestination()
	h.expectBalances(t, "after funding destination", src, dst, srcID, dstID, 1000, 100)

	requireAlreadyReverted(t, "v2 second revert", h.post(t, h.v2, h.v2StatePath(origin, "revert"), "", nil))
	requireAlreadyReverted(t, "v1 second revert", h.post(t, h.v1, h.statePath(origin, "revert"), "", nil))
	h.expectBalances(t, "after refused second reverts", src, dst, srcID, dstID, 1000, 100)

	// The marker outlives the origin's own engine index.
	_, err := h.engineRedis.CleanupEngineRecovery(h.ctx(), time.Now().Add(3601*time.Second), 100)
	require.NoError(t, err)
	_, err = h.engineRedis.GetEngineTransactionIndex(h.ctx(), h.orgID, h.ledgerID, origin)
	require.ErrorIs(t, err, redistransaction.ErrEngineWriteBehindNotFound)

	requireAlreadyReverted(t, "revert after origin index reaped", h.post(t, h.v2, h.v2StatePath(origin, "revert"), "", nil))
	h.expectBalances(t, "after refused third revert", src, dst, srcID, dstID, 1000, 100)
	require.Zero(t, h.countReverts(t, origin))
	require.True(t, h.revertMarkerExists(t, origin), "a stranded revert keeps its origin marked")
}

// TestIntegration_RevertOnceMarkerReleasedAtCleanup locks the marker's lifetime:
// a durable revert keeps it until its own retention cleanup, then PostgreSQL answers.
func TestIntegration_RevertOnceMarkerReleasedAtCleanup(t *testing.T) {
	h := setupRevertOnceHarness(t)

	src, dst := "@rc-src", "@rc-dst"
	srcID := h.seedBalance(t, src, "USD", decimal.NewFromInt(1000), "deposit")
	dstID := h.seedBalance(t, dst, "USD", decimal.Zero, "deposit")

	created := h.createV2Direct(t, h.v2, h.v2Body("origin", "USD", "100", []string{h.v2Leg(src, "100")}, []string{h.v2Leg(dst, "100")}), nil)
	require.Equalf(t, 201, created.status, "create: %s", created.rawBody)
	origin := mustTxID(t, created)

	reverted := h.post(t, h.v1, h.statePath(origin, "revert"), "", nil)
	require.Equalf(t, 201, reverted.status, "revert: %s", reverted.rawBody)
	require.Equal(t, 1, h.countReverts(t, origin))
	require.True(t, h.revertMarkerExists(t, origin), "the marker outlives the acknowledgement")

	_, err := h.engineRedis.CleanupEngineRecovery(h.ctx(), time.Now().Add(301*time.Second), 100)
	require.NoError(t, err)
	require.False(t, h.revertMarkerExists(t, origin), "cleanup releases the marker with the revert's guard")

	h.expireRevertIdempotencySlot(t, mustTxID(t, reverted))
	requireAlreadyReverted(t, "revert after cleanup", h.post(t, h.v2, h.v2StatePath(origin, "revert"), "", nil))
	h.expectBalances(t, "after refused revert", src, dst, srcID, dstID, 1000, 0)
}

// TestIntegration_RevertOnceCrossLedgerGroupBeforeProjection reverts a cross-ledger
// group again after its revert's idempotency record expired but before any member
// reached PostgreSQL. Each member origin carries its own marker, so the group
// revert is refused whole.
func TestIntegration_RevertOnceCrossLedgerGroupBeforeProjection(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	dispatcher := &heldWriteBehindDispatcher{}
	fixture.infra.handler.Command.TransactionWriteBehindAsync = true
	fixture.infra.handler.Command.TransactionWriteBehindDispatcher = dispatcher
	db, org := fixture.infra.pgContainer.DB, fixture.infra.orgID

	ledgerA, ledgerB := fixture.newLedger(t), fixture.newLedger(t)
	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)
	// Funded credit sides keep balances from refusing a second reversal.
	seedFundedTransfer(t, db, org, ledgerA, "@xl-source", "@external/USD", 100, 100)
	seedFundedTransfer(t, db, org, ledgerB, "@external/USD", "@xl-destination", 100, 100)

	request := atomicBatchTransfer(org, ledgerA, "revert once group", "@xl-source", "@xl-destination", 100)
	request.Credits[0].LedgerID = ledgerB.String()
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	createdResp := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "xl-revert-once")
	createdBody := drainBody(t, createdResp)
	require.Equalf(t, nethttp.StatusCreated, createdResp.StatusCode, "create: %s", createdBody)

	var direct CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(createdBody, &direct))
	require.Len(t, direct.Transactions, 2)
	member := direct.Transactions[1]
	revertURL := v2RevertURL(org, uuid.MustParse(member.LedgerID), uuid.MustParse(member.ID))

	first := postTransaction(t, fixture.app, revertURL, "", "")
	firstBody := drainBody(t, first)
	require.Equalf(t, nethttp.StatusCreated, first.StatusCode, "first group revert: %s", firstBody)
	requireCachedAvailable(t, fixture, ledgerA, "@xl-source", 100)
	requireCachedAvailable(t, fixture, ledgerB, "@xl-destination", 100)
	require.Zero(t, countTransactionsInLedger(t, db, ledgerA), "the scenario runs before any projection")

	// Stand-in for the revert's idempotency record expiring before the projection.
	key := "revert-group:" + *direct.GroupID
	deleted, err := fixture.infra.redisContainer.Client.Del(context.Background(),
		utils.AtomicTransactionBatchIdempotencyInternalKey(org, ledgerA, key),
		utils.AtomicTransactionBatchIdempotencyInternalKey(org, ledgerB, key)).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	second := postTransaction(t, fixture.app, revertURL, "", "")
	secondBody := drainBody(t, second)
	require.Equalf(t, nethttp.StatusConflict, second.StatusCode, "second group revert: %s", secondBody)
	require.Contains(t, string(secondBody), `"code":"`+cn.ErrTransactionIDHasAlreadyParentTransaction.Error()+`"`)
	requireCachedAvailable(t, fixture, ledgerA, "@xl-source", 100)
	requireCachedAvailable(t, fixture, ledgerB, "@xl-destination", 100)
}
