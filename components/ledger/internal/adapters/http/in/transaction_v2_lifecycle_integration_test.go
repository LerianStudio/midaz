// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// This file is the v2 transaction LIFECYCLE integration + parity proof: it exercises the v2
// `commit`, `cancel`, and `revert` ops end-to-end against the SAME real-repo handler backed by
// testcontainers as the sibling create/hold/block file. The split from
// transaction_v2_handler_integration_test.go is topical, not budgetary (this repo enforces no
// file-size limit): that file covers the CREATE surface (direct/hold/block/unblock), this one
// covers what happens to a transaction AFTER it exists. Both files share the same `in` package
// and reuse the shared request/decode/DB helpers, `seedTransfer`, `setupTestInfra`, and the app
// builders defined in the sibling files.
//
// NOT PARALLEL: the app builders call libProblem.Install() (process-global huma.NewError hook)
// and Huma validation uses process-global sync.Pools; concurrent builds cross-contaminate.
// Every test here stays sequential (see the create/hold file header for the full rationale).

// v2CommitURL / v2CancelURL build the concrete v2 lifecycle paths for a pending transaction.
// Both ops are mounted by the SAME RegisterTransactionV2RoutesToApp seam as direct/hold, so
// the v2 app built by buildHumaV2DirectApp serves them alongside the create ops. They are
// bodiless (POST with an empty body, 201 on success).
func v2CommitURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/commit"
}

func v2CancelURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/cancel"
}

// v1CancelURL builds the concrete v1 cancel path for a pending transaction. It is the cancel
// counterpart to v1CommitURL, mounted by both buildHumaTransactionApp (v1App, Huma) and the
// raw infra.app v1 chain over the SAME cancel core the v2 op reuses.
func v1CancelURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/cancel"
}

// v2RevertURL / v1RevertURL build the concrete revert paths for a transaction. The v2 op is
// mounted by the SAME RegisterTransactionV2RoutesToApp seam as direct/hold/commit/cancel and
// reuses the v1 RevertTransactionHuma shell verbatim, so both surfaces enter the SAME
// revertTransaction eligibility gate and createRevertTransaction core.
func v2RevertURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/revert"
}

func v1RevertURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/revert"
}

func patchV1Transaction(t *testing.T, app *fiber.App, orgID, ledgerID, txID uuid.UUID, body string) *nethttp.Response {
	t.Helper()

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String()
	req := httptest.NewRequest(nethttp.MethodPatch, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

func prepareRevertUpdateFreeze(t *testing.T, infra *testInfra) {
	t.Helper()
	client := infra.redisContainer.Client
	ctx := context.Background()
	// setupTestInfra starts at the normal finalized serving target. Rollout
	// compatibility cases need a separate first-install fixture, so reset the
	// isolated test certificate together with its Redis witnesses. Production
	// has no such transition: PREPARED and later are deliberately forward-only.
	_, err := infra.pgContainer.DB.ExecContext(ctx, `DELETE FROM transaction_revert_rollout_initialization`)
	require.NoError(t, err)
	require.NoError(t, client.Del(ctx, transactionredis.RevertUpdateFreezeKey,
		transactionredis.RevertRolloutGenerationKey, transactionredis.FinancialDatasetGenerationKey).Err())
	connection := redistestutil.CreateConnection(t, infra.redisContainer.Addr)
	rolloutWitness := revertclaim.NewPostgreSQLRepository(infra.pgConn)
	initializer := transactionredis.NewRevertUpdateFreezeGuard(connection,
		transactionredis.RevertUpdateFreezeInitialize, integrationRedisDatasetGeneration).
		WithRolloutInitializationWitness(rolloutWitness, integrationRolloutInitializationID)
	require.NoError(t, initializer.InitializeFinancialDatasetGeneration(ctx))
	prepared := transactionredis.NewRevertUpdateFreezeGuard(connection,
		transactionredis.RevertUpdateFreezePrepared, integrationRedisDatasetGeneration).
		WithRolloutInitializationWitness(rolloutWitness, "")
	require.NoError(t, prepared.ValidatePrepared(ctx))
	infra.revertFreeze = prepared
	infra.handler.RevertUpdateFreeze = prepared
	infra.handler.Command.RevertRolloutLease = prepared
}

func activateRevertUpdateFreeze(t *testing.T, infra *testInfra) {
	t.Helper()
	prepareRevertUpdateFreeze(t, infra)
	ctx := context.Background()
	connection := redistestutil.CreateConnection(t, infra.redisContainer.Addr)
	rolloutWitness := revertclaim.NewPostgreSQLRepository(infra.pgConn)
	active := transactionredis.NewRevertUpdateFreezeGuard(connection,
		transactionredis.RevertUpdateFreezeActive, integrationRedisDatasetGeneration).
		WithRolloutInitializationWitness(rolloutWitness, "")
	require.NoError(t, active.Activate(ctx))
	infra.revertFreeze = active
	infra.handler.RevertUpdateFreeze = active
	infra.handler.Command.RevertRolloutLease = active
}

// =============================================================================
// 12. HOLD COMMIT VIA V2 (lifecycle parity, core): a v2-held transaction committed through
//     the v2 commit op settles IDENTICALLY to a v1-held transaction committed through the v1
//     commit op — the source on-hold releases, the destination credit applies, the status
//     flips to APPROVED, and the persisted operation set plus the commit response are
//     indistinguishable between the two surfaces (ignoring IDs/timestamps). Both surfaces
//     reuse the SAME commitTransaction core, so this proves the v2 lifecycle surface reaches
//     that core with the same effect the v1 surface has.
// =============================================================================

func TestIntegration_TransactionV2Hold_CommitViaV2_ParityWithV1(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org so both flows use IDENTICAL aliases and starting
	// balances; the only legitimate difference is then the ledger id (stripped) plus per-row
	// IDs/timestamps.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// v1 lane: hold -> commit, each fully processed (create -> drain) BEFORE the v2 lane,
	// because the balance-sync schedule ZSET is GLOBAL, not per-ledger (see the direct parity
	// test for the same discipline).
	v1HoldResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	v1TxID := uuid.MustParse(v1HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v1CommitResp := decodeTxResponse(t, postTransaction(t, v1App, v1CommitURL(infra.orgID, ledgerV1, v1TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be APPROVED after v1 commit")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: hold -> commit through the v2 commit op.
	v2HoldResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerV2, holdParityV2Body, ""), nethttp.StatusCreated)
	v2TxID := uuid.MustParse(v2HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2CommitResp := decodeTxResponse(t, postTransaction(t, v2App, v2CommitURL(infra.orgID, ledgerV2, v2TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be APPROVED after v2 commit")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Settled balances: source on-hold released (available 900, on-hold 0), destination
	// credited (available 100, on-hold 0), IDENTICAL on both surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available after commit")
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold released after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold released after commit")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest credited after commit")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest credited after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold after commit")

	// Full lifecycle operation set: 1 ON_HOLD (hold) + 1 DEBIT (release) + 1 CREDIT (apply)
	// on each, with the economic projection IDENTICAL between the two surfaces.
	v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
	v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)
	require.Len(t, v1Ops, 3, "committed v1 hold should carry 3 operations (ON_HOLD + DEBIT + CREDIT)")
	require.Len(t, v2Ops, 3, "committed v2 hold should carry 3 operations (ON_HOLD + DEBIT + CREDIT)")
	assertOperationSetsEqual(t, v1Ops, v2Ops)

	// Commit response deep-equal, ignoring IDs/timestamps: the v2 commit response (with the
	// settled operations embedded) is indistinguishable from the v1 commit response.
	require.Equal(t, stripVolatile(v1CommitResp), stripVolatile(renameV2LegKeys(v2CommitResp)),
		"v2 commit response must be indistinguishable from the v1 commit equivalent (ignoring IDs/timestamps)")
}

// =============================================================================
// 13. HOLD CANCEL VIA V2 (lifecycle parity, core): a v2-held transaction cancelled through
//     the v2 cancel op reverts IDENTICALLY to a v1-held transaction cancelled through the v1
//     cancel op — the source on-hold is released back to available (no destination credit),
//     the status flips to CANCELED, and the persisted operations plus the cancel response are
//     indistinguishable between the two surfaces (ignoring IDs/timestamps).
// =============================================================================

func TestIntegration_TransactionV2Hold_CancelViaV2_ParityWithV1(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// v1 lane: hold -> cancel, fully processed BEFORE the v2 lane (global ZSET discipline).
	v1HoldResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	v1TxID := uuid.MustParse(v1HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v1CancelResp := decodeTxResponse(t, postTransaction(t, v1App, v1CancelURL(infra.orgID, ledgerV1, v1TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be CANCELED after v1 cancel")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: hold -> cancel through the v2 cancel op.
	v2HoldResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerV2, holdParityV2Body, ""), nethttp.StatusCreated)
	v2TxID := uuid.MustParse(v2HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2CancelResp := decodeTxResponse(t, postTransaction(t, v2App, v2CancelURL(infra.orgID, ledgerV2, v2TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be CANCELED after v2 cancel")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Reverted balances: the on-hold reservation is released back to available (source
	// 1000 / 0), and the destination is never credited (0 / 0), IDENTICAL on both surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available restored after cancel")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available restored after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold released after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold released after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest never credited on cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest never credited on cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold after cancel")

	// Cancel response deep-equal, ignoring IDs/timestamps: the v2 cancel response (with its
	// persisted operations embedded) is indistinguishable from the v1 cancel response, which
	// proves cross-surface operation-level parity without pinning the release leg's Type.
	require.Equal(t, stripVolatile(v1CancelResp), stripVolatile(renameV2LegKeys(v2CancelResp)),
		"v2 cancel response must be indistinguishable from the v1 cancel equivalent (ignoring IDs/timestamps)")
}

// =============================================================================
// 14. CROSS-SURFACE LIFECYCLE (core reuse both ways): the hold and its lifecycle transition
//     are decoupled across surfaces because both share the SAME commitTransaction core.
//     (a) a v2-held transaction is committable through the EXISTING v1 commit endpoint, and
//     (b) a v1-held transaction is committable through the v2 commit op — both settle to
//     APPROVED with the identical ledger effect (source hold released, destination credited).
// =============================================================================

func TestIntegration_TransactionV2Hold_CrossSurfaceCommit(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org, identical seed. Ledger A carries the v2-hold -> v1-commit
	// direction; ledger B carries the v1-hold -> v2-commit direction.
	ledgerA := infra.ledgerID
	ledgerB := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerB)

	aSrc, aDst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerA, "@src", "@dst", 1000)
	bSrc, bDst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerB, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// (a) v2-held, committed through the EXISTING v1 commit endpoint (v1App). Fully processed
	// BEFORE ledger B (global ZSET discipline).
	aHoldResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerA, holdParityV2Body, ""), nethttp.StatusCreated)
	aTxID := uuid.MustParse(aHoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, aTxID), "v2 hold on ledger A should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerA)

	_ = decodeTxResponse(t, postTransaction(t, v1App, v1CommitURL(infra.orgID, ledgerA, aTxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, aTxID), "a v2-held transaction must commit through the v1 commit endpoint")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerA)

	// (b) v1-held (pending JSON), committed through the v2 commit op (v2App).
	bHoldResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerB), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	bTxID := uuid.MustParse(bHoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, bTxID), "v1 hold on ledger B should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerB)

	_ = decodeTxResponse(t, postTransaction(t, v2App, v2CommitURL(infra.orgID, ledgerB, bTxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, bTxID), "a v1-held transaction must commit through the v2 commit op")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerB)

	// Both directions settle IDENTICALLY: source hold released (900 / 0), destination
	// credited (100 / 0) — the transition surface does not change the ledger effect.
	for _, bal := range []struct {
		src, dst uuid.UUID
		lane     string
	}{
		{aSrc, aDst, "ledger A (v2 hold -> v1 commit)"},
		{bSrc, bDst, "ledger B (v1 hold -> v2 commit)"},
	} {
		requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, bal.src), "%s source available after commit", bal.lane)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, bal.src), "%s source on-hold released after commit", bal.lane)
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, bal.dst), "%s dest credited after commit", bal.lane)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, bal.dst), "%s dest on-hold after commit", bal.lane)
	}
}

// =============================================================================
// 15. LIFECYCLE STATE + ID ERRORS: the v2 commit/cancel ops surface the SAME state and
//     path errors the v1 ops do, because they reuse the SAME commitTransaction core and the
//     SAME ParseUUIDPathParameters Fiber guard:
//       - commit of a non-PENDING (already-APPROVED direct) tx -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - cancel of the same already-APPROVED tx              -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - commit of an unknown (well-formed) transaction_id   -> 404 ErrEntityNotFound (0007, generic query not-found)
//       - cancel of an unknown (well-formed) transaction_id   -> 404 ErrEntityNotFound (0007, generic query not-found)
//       - commit with a malformed transaction_id              -> 400 ErrInvalidPathParameter (0065, blocked on the chain)
//       - cancel with a malformed transaction_id              -> 400 ErrInvalidPathParameter (0065, blocked on the chain)
//
//     The unknown-id rows here are the CORRECT contract for a missing entity (404/0007).
//     Revert diverges on that same input class and answers 409/0099 — a known defect recorded
//     in section 17's unknown-id case, which names the root cause and the fix contract.
// =============================================================================

func TestIntegration_TransactionV2Lifecycle_StateAndIDErrors(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// A v2 `direct` transfer settles immediately (APPROVED): it is the non-PENDING subject for
	// the commit/cancel state-error cases below.
	directResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	approvedTxID := uuid.MustParse(directResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, approvedTxID), "the v2 direct transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	unknownTxID := uuid.Must(libCommons.GenerateUUIDv7())

	cases := []struct {
		name       string
		url        string
		wantStatus int
		wantCode   string
	}{
		{
			// A non-PENDING transaction is an invalid-status conflict: ErrCommitTransactionNotPending
			// maps to EntityConflictError -> 409 (see pkg/errors.go), not 422.
			name:       "commit of an already-APPROVED transaction is not pending",
			url:        v2CommitURL(infra.orgID, infra.ledgerID, approvedTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			name:       "cancel of an already-APPROVED transaction is not pending",
			url:        v2CancelURL(infra.orgID, infra.ledgerID, approvedTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			// The query not-found path (write-behind miss -> GetTransactionByID miss) returns the
			// generic ErrEntityNotFound (0007) -> 404, entityType "Transaction".
			name:       "commit of an unknown transaction id is not found",
			url:        v2CommitURL(infra.orgID, infra.ledgerID, unknownTxID),
			wantStatus: nethttp.StatusNotFound,
			wantCode:   cn.ErrEntityNotFound.Error(),
		},
		{
			// Cancel mirrors commit on the same unknown (well-formed) id: the query not-found
			// path returns the generic ErrEntityNotFound (0007) -> 404.
			name:       "cancel of an unknown transaction id is not found",
			url:        v2CancelURL(infra.orgID, infra.ledgerID, unknownTxID),
			wantStatus: nethttp.StatusNotFound,
			wantCode:   cn.ErrEntityNotFound.Error(),
		},
		{
			// Malformed transaction_id is rejected by ParseUUIDPathParameters on the Fiber
			// chain BEFORE the Huma terminal — canonical 400, never a native Huma 422.
			name:       "commit with a malformed transaction id is a bad request",
			url:        "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/not-a-uuid/commit",
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   cn.ErrInvalidPathParameter.Error(),
		},
		{
			// Cancel mirrors commit on a malformed transaction_id: ParseUUIDPathParameters blocks
			// it on the Fiber chain BEFORE the Huma terminal — canonical 400, never a Huma 422.
			name:       "cancel with a malformed transaction id is a bad request",
			url:        "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/not-a-uuid/cancel",
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   cn.ErrInvalidPathParameter.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertProblemCode(t, postTransaction(t, v2App, tc.url, "", ""), tc.wantStatus, tc.wantCode)
		})
	}

	// The rejected lifecycle calls left the direct transaction APPROVED and created no extra
	// transaction: exactly the one direct transfer is persisted.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, approvedTxID), "the direct transaction must stay APPROVED after the rejected commit/cancel")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "rejected lifecycle calls must not persist new transactions")
}

// =============================================================================
// 16. REVERT VIA V2 (lifecycle parity, core): an APPROVED transaction reverted through the
//     v2 revert op produces the SAME reverse transaction a v1 revert of the equivalent
//     origin produces — the reverse is linked to its origin via ParentTransactionID, the
//     accounting direction is inverted leg-for-leg (the origin's debited source becomes the
//     reverse's credited destination and vice versa), the net balances return to the
//     pre-transaction state, and the persisted operation set plus the revert response are
//     indistinguishable between the two surfaces (ignoring IDs/timestamps).
// =============================================================================

func TestIntegration_TransactionV2Revert_ParityWithV1(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org so both flows use IDENTICAL aliases and starting
	// balances; the only legitimate difference is then the ledger id (stripped) plus per-row
	// IDs/timestamps.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// v1 lane: transfer -> revert, each step fully processed (create -> drain) BEFORE the v2
	// lane, because the balance-sync schedule ZSET is GLOBAL, not per-ledger (see the direct
	// parity test for the same discipline).
	v1OriginResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), equivalentV1Body, ""), nethttp.StatusCreated)
	v1OriginID := uuid.MustParse(v1OriginResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1OriginID), "the v1 origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v1RevertRaw := postTransaction(t, v1App, v1RevertURL(infra.orgID, ledgerV1, v1OriginID), "", "")
	assert.Equal(t, "false", v1RevertRaw.Header.Get("X-Idempotency-Replayed"),
		"a fresh v1 revert must advertise X-Idempotency-Replayed: false — the header is what distinguishes it from a replay")

	v1RevertResp := decodeTxResponse(t, v1RevertRaw, nethttp.StatusCreated)
	v1RevertID := uuid.MustParse(v1RevertResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1RevertID), "the v1 reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: transfer -> revert through the v2 revert op.
	v2OriginResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, ledgerV2, equivalentV2Body, ""), nethttp.StatusCreated)
	v2OriginID := uuid.MustParse(v2OriginResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2OriginID), "the v2 origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2RevertRaw := postTransaction(t, v2App, v2RevertURL(infra.orgID, ledgerV2, v2OriginID), "", "")
	assert.Equal(t, "false", v2RevertRaw.Header.Get("X-Idempotency-Replayed"),
		"a fresh v2 revert must advertise X-Idempotency-Replayed: false, byte-identically to the v1 surface")

	v2RevertResp := decodeTxResponse(t, v2RevertRaw, nethttp.StatusCreated)
	v2RevertID := uuid.MustParse(v2RevertResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2RevertID), "the v2 reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Parent link — the core assertion of the v2 revert surface. Read from the response
	// BEFORE stripVolatile (which deletes parentTransactionId in place) AND from the
	// persisted row, on BOTH surfaces.
	assert.Equal(t, v1OriginID.String(), v1RevertResp["parentTransactionId"], "the v1 reverse response must link back to its origin")
	assert.Equal(t, v2OriginID.String(), v2RevertResp["parentTransactionId"], "the v2 reverse response must link back to its origin")

	v1Parent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v1RevertID)
	require.NotNil(t, v1Parent, "the persisted v1 reverse transaction must carry a parent")
	assert.Equal(t, v1OriginID, *v1Parent, "the persisted v1 reverse must point at the reverted transaction")

	v2Parent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v2RevertID)
	require.NotNil(t, v2Parent, "the persisted v2 reverse transaction must carry a parent")
	assert.Equal(t, v2OriginID, *v2Parent, "the persisted v2 reverse must point at the reverted transaction")

	// The link is one-directional: reverting does not stamp a parent onto the origin, so the
	// origin stays revertable-once rather than becoming a revert itself.
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v1OriginID), "the v1 origin must not acquire a parent")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v2OriginID), "the v2 origin must not acquire a parent")

	// Net balances back to the pre-transaction state: source restored to its full 1000, the
	// destination emptied back to 0, no on-hold anywhere, IDENTICAL on both surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available restored after revert")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available restored after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest drained back after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest drained back after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold after revert")

	// Inverted accounting direction: the origin debits @src and credits @dst; each reverse
	// credits @src and debits @dst, for the same amount, landing the balance-after back on the
	// seeded state. Both legs of a reverse carry distinct aliases, so the alias join lines them
	// up across the origin/reverse pair.
	v1OriginOps := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, v1OriginID))
	v1RevertOps := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, v1RevertID))
	v2RevertOps := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, v2RevertID))

	require.Len(t, v1OriginOps, 2, "the origin transfer should carry 2 operations")
	require.Len(t, v1RevertOps, 2, "the v1 reverse should carry 2 operations")
	require.Len(t, v2RevertOps, 2, "the v2 reverse should carry 2 operations")

	assert.Equal(t, cn.DEBIT, v1OriginOps["@src"].Type, "the origin debits the source")
	assert.Equal(t, cn.CREDIT, v1OriginOps["@dst"].Type, "the origin credits the destination")

	for lane, revertOps := range map[string]map[string]operationEconomicRow{"v1": v1RevertOps, "v2": v2RevertOps} {
		assert.Equal(t, cn.CREDIT, revertOps["@src"].Type, "%s reverse must CREDIT the originally debited source", lane)
		assert.Equal(t, cn.DEBIT, revertOps["@dst"].Type, "%s reverse must DEBIT the originally credited destination", lane)
		requireDecimalEqual(t, v1OriginOps["@src"].Amount, revertOps["@src"].Amount, "%s reverse source amount mirrors the origin", lane)
		requireDecimalEqual(t, v1OriginOps["@dst"].Amount, revertOps["@dst"].Amount, "%s reverse destination amount mirrors the origin", lane)
		requireDecimalEqual(t, decimal.NewFromInt(1000), revertOps["@src"].AvailableAfter, "%s reverse restores the source available balance", lane)
		requireDecimalEqual(t, decimal.Zero, revertOps["@dst"].AvailableAfter, "%s reverse drains the destination available balance", lane)
	}

	// Full economic parity of the two reverse operation sets.
	assertOperationSetsEqual(t,
		fetchOperationRows(t, infra.pgContainer.DB, v1RevertID),
		fetchOperationRows(t, infra.pgContainer.DB, v2RevertID))

	// Revert response deep-equal, ignoring IDs/timestamps: the v2 revert response (with the
	// reverse operations embedded) is indistinguishable from the v1 revert response.
	require.Equal(t, stripVolatile(v1RevertResp), stripVolatile(renameV2LegKeys(v2RevertResp)),
		"v2 revert response must be indistinguishable from the v1 revert equivalent (ignoring IDs/timestamps)")

	// Exactly one origin + one reverse per ledger: neither surface produced a stray extra
	// transaction while reverting.
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, ledgerV1), "the v1 lane should hold exactly the origin and its reverse")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, ledgerV2), "the v2 lane should hold exactly the origin and its reverse")
}

// seedEmptyReversalTransaction persists an APPROVED transaction whose reversal is degenerate
// — zero send value and no asset code — plus the single operation row the revert read path
// inner-joins on. The create path cannot produce this shape, so the row is seeded directly to
// reach the empty-reversal branch of the eligibility gate.
func seedEmptyReversalTransaction(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID) uuid.UUID {
	t.Helper()

	params := postgrestestutil.DefaultTransactionParams()
	params.Status = cn.APPROVED
	params.Amount = decimal.Zero
	params.AssetCode = ""
	// The default params carry a 100 USD send body, which would contradict the zero/no-asset
	// columns above. The revert gate reads the columns and the operations, never the body, so
	// the row is left body-less to stay self-consistent (the column is nullable).
	params.Body = nil

	txID := postgrestestutil.CreateTestTransaction(t, db, orgID, ledgerID, params)

	postgrestestutil.CreateTestOperation(t, db, orgID, ledgerID, postgrestestutil.OperationParams{
		TransactionID: txID,
		Description:   "degenerate reversal leg",
		Type:          cn.DEBIT,
		AccountID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AccountAlias:  "@degenerate",
		BalanceID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AssetCode:     "",
		Amount:        decimal.Zero,
		Status:        cn.APPROVED,
	})

	return txID
}

// assertProblemCode drains an error response and asserts its HTTP status plus the `code`
// field of its RFC 9457 problem+json envelope. The code is compared as a FIELD, not as a
// whole-body substring, so an incidental occurrence of the same digits anywhere else in the
// payload cannot make the assertion pass. The failing body is dumped on every mismatch.
func assertProblemCode(t *testing.T, resp *nethttp.Response, wantStatus int, wantCode string) {
	t.Helper()

	body := drainBody(t, resp)

	assert.Equalf(t, wantStatus, resp.StatusCode, "unexpected status; body: %s", string(body))

	var problem struct {
		Code string `json:"code"`
	}

	require.NoErrorf(t, json.Unmarshal(body, &problem), "response body should be valid problem+json; body: %s", string(body))
	assert.Equalf(t, wantCode, problem.Code, "problem+json code field should equal %s; body: %s", wantCode, string(body))
}

// =============================================================================
// 17. REVERT INELIGIBILITY + ID ERRORS: the v2 revert op surfaces the SAME eligibility and
//     path errors the v1 op does, because it reuses the SAME revertTransaction gate and the
//     SAME ParseUUIDPathParameters Fiber guard. Each gate has its own sentinel and its own
//     status CLASS:
//       - origin that was ALREADY reverted (a child revert exists)
//             -> 201 with the exact durable reverse and X-Idempotency-Replayed=true
//       - the reverse transaction itself (carries a ParentTransactionID)
//             -> 409 ErrTransactionIDIsAlreadyARevert (0088, EntityConflictError)
//       - a non-APPROVED transaction (PENDING or CANCELED)
//             -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - a degenerate transaction whose reversal is empty
//             -> 422 ErrTransactionCantRevert (0089, UnprocessableOperationError)
//       - a transaction whose operations point at a NON-bidirectional operation route
//             -> 422 ErrRouteNotBidirectional (0150, UnprocessableOperationError)
//       - an unknown (well-formed) transaction_id
//             -> 409 ErrCommitTransactionNotPending (0099) — a KNOWN DEFECT, not the intended
//                contract. See the case body for the root cause and the fix contract; commit
//                and cancel correctly return 404/0007 for the same input class (section 15).
//       - a malformed transaction_id
//             -> 400 ErrInvalidPathParameter (0065, blocked on the Fiber chain)
// =============================================================================

func TestIntegration_TransactionV2Revert_IneligibilityAndIDErrors(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	// Both apps are built up-front, before any request runs: the builders call the
	// process-global libProblem.Install() (see file header), so building one mid-test would
	// re-install that hook underneath a surface already in use.
	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Subject pair 1: an APPROVED direct transfer and its reverse. After this pair the origin
	// MUST replay its durable reverse while the reverse itself remains ineligible.
	originResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originTxID := uuid.MustParse(originResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originTxID), "the origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	revertResp := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originTxID), "", ""), nethttp.StatusCreated)
	revertTxID := uuid.MustParse(revertResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, revertTxID), "the reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 2: a hold left PENDING. Its description differs from the cancel subject below so
	// the two no-key holds hash into DISTINCT idempotency slots instead of replaying.
	pendingResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
		`{"description":"revert gate pending subject","asset":"USD","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`, ""), nethttp.StatusCreated)
	pendingTxID := uuid.MustParse(pendingResp["id"].(string))
	require.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingTxID), "the hold subject should be PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 3: a hold cancelled through the v2 cancel op -> CANCELED.
	cancelResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
		`{"description":"revert gate cancel subject","asset":"USD","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`, ""), nethttp.StatusCreated)
	canceledTxID := uuid.MustParse(cancelResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	_ = decodeTxResponse(t, postTransaction(t, v2App, v2CancelURL(infra.orgID, infra.ledgerID, canceledTxID), "", ""), nethttp.StatusCreated)
	require.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, canceledTxID), "the cancel subject should be CANCELED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 4: an APPROVED transaction whose reversal is empty.
	emptyReversalTxID := seedEmptyReversalTransaction(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID)

	// Subject 5: an APPROVED direct transfer whose operations are stamped with a
	// NON-bidirectional operation route, which is the only input that reaches the
	// bidirectional-route gate. The route is created and stamped AFTER the transfer settles,
	// so nothing about the create path changes; the gate reads only op.RouteID off the
	// transaction's operations and resolves the route by id, so the ledger's route-validation
	// setting (off in this harness) is irrelevant to reaching it.
	nonBidiResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		`{"description":"revert gate non bidirectional subject","asset":"USD","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`, ""), nethttp.StatusCreated)
	nonBidiTxID := uuid.MustParse(nonBidiResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, nonBidiTxID), "the non-bidirectional subject should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	sourceRouteID := postgrestestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "Revert Gate Source Route", "source")
	postgrestestutil.StampOperationRoute(t, infra.pgContainer.DB, nonBidiTxID, sourceRouteID)

	unknownTxID := uuid.Must(libCommons.GenerateUUIDv7())

	cases := []struct {
		name       string
		url        string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "revert of a reverse transaction conflicts because it is already a revert",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, revertTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrTransactionIDIsAlreadyARevert.Error(),
		},
		{
			name:       "revert of a PENDING transaction conflicts on the non-APPROVED status",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, pendingTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			name:       "revert of a CANCELED transaction conflicts on the non-APPROVED status",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, canceledTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			name:       "revert of a transaction with an empty reversal is unprocessable",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, emptyReversalTxID),
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   cn.ErrTransactionCantRevert.Error(),
		},
		{
			// Only routes whose OperationType is "bidirectional" may be reverted:
			// ErrRouteNotBidirectional is an UnprocessableOperationError (see pkg/errors.go)
			// -> 422. Reached because subject 5's operations carry a route_id pointing at a
			// "source" route.
			name:       "revert of a transaction on a non-bidirectional route is unprocessable",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, nonBidiTxID),
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   cn.ErrRouteNotBidirectional.Error(),
		},
		{
			// KNOWN DEFECT (tracked externally) — this expectation records the
			// CURRENT behavior, not the intended contract. A missing entity must render 404
			// (error-handling standard E3/E5), and
			// commit/cancel do exactly that for the same input class (404/0007, section 15).
			// Revert diverges because its read path FindWithOperations
			// (adapters/postgres/transaction/transaction.postgresql.go) has no sql.ErrNoRows
			// arm — unlike FindByParentID in the same file — so zero join rows return the
			// zero-value &Transaction{} with a nil error. The empty status "" then trips the
			// non-APPROVED gate, and the client sees 409/0099 instead of 404/0007.
			//
			// If this starts returning 404/0007 the not-found gap was fixed — update this
			// expectation, do NOT revert the fix.
			//
			// Fixing the read path is OUT OF SCOPE here, and whoever does it must land the
			// caller guard in the SAME change: revertTransaction dereferences the result
			// (tran.ParentTransactionID) immediately after the error check, so a fix that
			// returns (nil, nil) — or any nil transaction with a nil error — converts this
			// wrong status into a nil-pointer panic.
			name:       "revert of an unknown transaction id currently conflicts (0099) instead of 404 — KNOWN DEFECT",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, unknownTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			// Malformed transaction_id is rejected by ParseUUIDPathParameters on the Fiber
			// chain BEFORE the Huma terminal — canonical 400, never a native Huma 422.
			name:       "revert with a malformed transaction id is a bad request",
			url:        "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/not-a-uuid/revert",
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   cn.ErrInvalidPathParameter.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertProblemCode(t, postTransaction(t, v2App, tc.url, "", ""), tc.wantStatus, tc.wantCode)
		})
	}

	v2ReplayResponse := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originTxID), "", "")
	assert.Equal(t, "true", v2ReplayResponse.Header.Get("X-Idempotency-Replayed"))
	v2Replay := decodeTxResponse(t, v2ReplayResponse, nethttp.StatusCreated)
	assert.Equal(t, revertTxID.String(), v2Replay["id"], "same-origin v2 retry must return the durable reverse")
	v1ReplayResponse := postTransaction(t, v1App, v1RevertURL(infra.orgID, infra.ledgerID, originTxID), "", "")
	assert.Equal(t, "true", v1ReplayResponse.Header.Get("X-Idempotency-Replayed"))
	v1Replay := decodeTxResponse(t, v1ReplayResponse, nethttp.StatusCreated)
	assert.Equal(t, revertTxID.String(), v1Replay["id"], "same-origin v1 retry must return the durable reverse")

	// v1↔v2 ineligibility parity, sampled on the two gates whose contract is easiest to get
	// wrong: the same unknown subject rejected through the EXISTING v1 revert endpoint returns the
	// byte-identical problem envelope, so the v2 surface adds no divergent error behavior.
	for _, tc := range []struct {
		name string
		txID uuid.UUID
	}{
		{"unknown transaction id", unknownTxID},
	} {
		v1Resp := postTransaction(t, v1App, v1RevertURL(infra.orgID, infra.ledgerID, tc.txID), "", "")
		v1Body := drainBody(t, v1Resp)

		v2Resp := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, tc.txID), "", "")
		v2Body := drainBody(t, v2Resp)

		assert.Equal(t, v1Resp.StatusCode, v2Resp.StatusCode, "%s: v1 and v2 revert must reject with the same status", tc.name)
		assert.JSONEq(t, string(v1Body), string(v2Body), "%s: v1 and v2 revert must reject with the same problem envelope", tc.name)
	}

	// Every rejected or replayed revert left its subject exactly as it was and persisted nothing:
	// the five created transactions plus the seeded degenerate one, and no extra reverse.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originTxID), "the origin must stay APPROVED after the rejected reverts")
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, revertTxID), "the reverse must stay APPROVED after the rejected reverts")
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingTxID), "the PENDING subject must stay PENDING")
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, canceledTxID), "the CANCELED subject must stay CANCELED")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, emptyReversalTxID), "the degenerate subject must not acquire a reverse")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, nonBidiTxID), "the non-bidirectional subject must not acquire a reverse")
	assert.Equal(t, 6, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "rejected reverts must not persist new transactions")
}

// =============================================================================
// 18. REVERT IDEMPOTENCY IS SCOPED BY ORIGIN.
//
//     Two economically identical origins receive distinct, correctly-parented
//     reverses, while a retry of one origin returns its one durable reverse and
//     never persists or moves funds again.
// =============================================================================

func TestIntegration_TransactionV2Revert_IdempotencyScopedByOrigin(t *testing.T) {

	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// 1000 available covers both 100-unit origins and both reversals.
	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Two economically-IDENTICAL origins in the SAME ledger: same description, asset, amount,
	// from and to. The distinct explicit X-Idempotency keys apply to the CREATE calls only —
	// they are what keeps the two origins two transactions instead of one create replay. The
	// reversals derived from them are byte-identical payloads, which is precisely the input
	// class that collides when the revert slot is keyed on the reversal payload alone.
	const identicalOriginBody = `{"description":"cross origin revert subject","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

	originAResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, identicalOriginBody, "cross-origin-create-a"), nethttp.StatusCreated)
	originAID := uuid.MustParse(originAResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originAID), "origin A should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	originBResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, identicalOriginBody, "cross-origin-create-b"), nethttp.StatusCreated)
	originBID := uuid.MustParse(originBResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originBID), "origin B should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	require.NotEqual(t, originAID, originBID, "the two origins must be DISTINCT transactions for this test to mean anything")
	require.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "the two identical creates must both persist")

	// Revert BOTH origins with NO X-Idempotency header, so each revert's slot is derived
	// entirely by the create core — the collision path under test.
	revertAResp := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originAID), "", ""), nethttp.StatusCreated)
	revertAID := uuid.MustParse(revertAResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	revertBResp := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originBID), "", ""), nethttp.StatusCreated)
	revertBID := uuid.MustParse(revertBResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// (a) Two DISTINCT reverses, each linked to its OWN origin. An equal id — or origin A's id
	// showing up as origin B's reverse parent — is the replay this test exists to catch.
	assert.NotEqual(t, revertAID, revertBID,
		"each origin must get its OWN reverse; an equal id means origin B's revert replayed origin A's cached reverse")
	assert.Equal(t, originAID.String(), revertAResp["parentTransactionId"], "origin A's reverse must link back to origin A")
	assert.Equal(t, originBID.String(), revertBResp["parentTransactionId"], "origin B's reverse must link back to origin B, NOT origin A")

	revertAParent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, revertAID)
	require.NotNil(t, revertAParent, "the persisted reverse of origin A must carry a parent")
	assert.Equal(t, originAID, *revertAParent, "the persisted reverse of origin A must point at origin A")

	revertBParent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, revertBID)
	require.NotNil(t, revertBParent, "the persisted reverse of origin B must carry a parent")
	assert.Equal(t, originBID, *revertBParent, "the persisted reverse of origin B must point at origin B")

	// Both origins really were reverted: 2 origins + 2 reverses.
	assert.Equal(t, 4, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"both origins must be reverted: 2 origins + 2 distinct reverses")

	// Net balances back to the seeded state, which is only true if BOTH origins were reverted
	// (a single applied reverse would leave the source at 900 and the destination at 100).
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available restored after both reverts")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination drained back after both reverts")

	// (b) A repeat revert of the SAME origin replays the durable reverse and persists nothing.
	// The replay is resolved from PostgreSQL primary and must echo the reserved reverse ID.
	repeat := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originAID), "", "")
	assert.Equal(t, "true", repeat.Header.Get("X-Idempotency-Replayed"))
	repeatBody := decodeTxResponse(t, repeat, nethttp.StatusCreated)
	assert.Equal(t, revertAID.String(), repeatBody["id"])

	assert.Equal(t, 4, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"a repeat revert of the SAME origin must not persist a third transaction")
}

func TestIntegration_TransactionV2Revert_RolloutOldBridgeFinalNeverReplaysAnotherOrigin(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	const identicalOriginBody = `{"description":"rollout collision","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`
	originA := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, identicalOriginBody, "rollout-a"), nethttp.StatusCreated)
	originAID := uuid.MustParse(originA["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	originB := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, identicalOriginBody, "rollout-b"), nethttp.StatusCreated)
	originBID := uuid.MustParse(originB["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	reverseA := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originAID), "", ""), nethttp.StatusCreated)
	reverseAID := uuid.MustParse(reverseA["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	originATransaction, err := infra.handler.Query.GetTransactionWithOperationsByID(ctx, infra.orgID, infra.ledgerID, originAID)
	require.NoError(t, err)
	legacyHash, err := legacyRevertIdempotencyHash(originATransaction.TransactionRevert())
	require.NoError(t, err)
	persistedReverseA, err := infra.handler.Query.GetTransactionByID(ctx, infra.orgID, infra.ledgerID, reverseAID)
	require.NoError(t, err)
	legacyValue, err := json.Marshal(persistedReverseA)
	require.NoError(t, err)
	require.NoError(t, infra.redisRepo.Set(ctx, utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, legacyHash), string(legacyValue), 300))

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	phaseZeroResponse := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originBID), "", "")
	assertProblemCode(t, phaseZeroResponse, nethttp.StatusConflict, cn.ErrIdempotencyKey.Error())
	assert.Equal(t, 3, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"phase zero must reject a colliding cached reverse instead of returning origin A for origin B")

	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	bridgeResponse := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originBID), "", "")
	assertProblemCode(t, bridgeResponse, nethttp.StatusConflict, cn.ErrIdempotencyKey.Error())
	assert.Equal(t, 3, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"bridge must fence the colliding legacy slot rather than return origin A's reverse for origin B")

	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx),
		"final cannot coexist directly with phase zero; bridge must first prove the old generation drained")
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeFinal
	reverseB := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originBID), "", ""), nethttp.StatusCreated)
	assert.NotEqual(t, reverseAID.String(), reverseB["id"])
	assert.Equal(t, originBID.String(), reverseB["parentTransactionId"])
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored by final after bridge fence")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored by final after bridge fence")
}

func TestIntegration_TransactionV2Revert_RolloutOldInFlightAndBridgeShareLegacyBarrier(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	originTransaction, err := infra.handler.Query.GetTransactionWithOperationsByID(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	legacyHash, err := legacyRevertIdempotencyHash(originTransaction.TransactionRevert())
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, legacyHash)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	blockedUpdate := patchV1Transaction(t, v1App, infra.orgID, infra.ledgerID, originID, `{"description":"must not change during rollout"}`)
	assertProblemCode(t, blockedUpdate, nethttp.StatusUnprocessableEntity, cn.ErrActionNotPermitted.Error())
	originAfterBlockedUpdate, err := infra.handler.Query.GetTransactionWithOperationsByID(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	legacyHashAfterBlockedUpdate, err := legacyRevertIdempotencyHash(originAfterBlockedUpdate.TransactionRevert())
	require.NoError(t, err)
	assert.Equal(t, legacyHash, legacyHashAfterBlockedUpdate,
		"the shared freeze must keep the old payload-scoped barrier stable throughout coexistence")

	// An old pod starts first and owns the released payload-hash barrier. Its
	// empty value means in-flight, before it has a reverse response to cache.
	acquired, err := infra.redisRepo.SetNX(ctx, legacyKey, "", 300)
	require.NoError(t, err)
	require.True(t, acquired)

	phaseZeroBlocked := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, phaseZeroBlocked, nethttp.StatusConflict, cn.ErrIdempotencyKey.Error())
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"phase zero must retain the old payload-scoped revert algorithm")

	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	blocked := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, blocked, nethttp.StatusConflict, cn.ErrIdempotencyKey.Error())
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"bridge must not move funds while an old request owns the shared barrier")
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "blocked bridge leaves source unchanged")
	requireDecimalEqual(t, decimal.NewFromInt(1100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "blocked bridge leaves destination unchanged")

	// A proven pre-movement old abort releases its own fence. The bridge must
	// also have released its fresh PostgreSQL claim, so the retry can perform
	// exactly one reversal.
	require.NoError(t, infra.redisRepo.Del(ctx, legacyKey))
	reverse := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, originID.String(), reverse["parentTransactionId"])
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored exactly once")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored exactly once")

	infra.handler.RevertIdempotencyMode = revertIdempotencyModeFinal
	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx))
	require.NoError(t, infra.revertFreeze.Finalize(ctx))
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	downgrade := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, downgrade, nethttp.StatusServiceUnavailable, cn.ErrRevertRolloutFreezeRequired.Error())
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"a bridge downgrade after finalization must fail closed without another reverse")

	infra.handler.RevertIdempotencyMode = revertIdempotencyModeFinal
	updated := decodeTxResponse(t, patchV1Transaction(t, v1App, infra.orgID, infra.ledgerID, originID,
		`{"description":"updates restored after finalization"}`), nethttp.StatusOK)
	assert.Equal(t, "updates restored after finalization", updated["description"],
		"finalized rollout must restore the approved transaction update contract")
}

func TestIntegration_TransactionUpdateFreeze_ApprovedBlockedPendingAllowedAndFinalizedRestores(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 2000, 2000)
	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	approved := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body,
		"freeze-approved"), nethttp.StatusCreated)
	approvedID := uuid.MustParse(approved["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, holdParityV2Body,
		"freeze-pending"), nethttp.StatusCreated)
	pendingID := uuid.MustParse(pending["id"].(string))

	activateRevertUpdateFreeze(t, infra)
	blocked := patchV1Transaction(t, v1App, infra.orgID, infra.ledgerID, approvedID, `{"description":"blocked approved update"}`)
	assertProblemCode(t, blocked, nethttp.StatusUnprocessableEntity, cn.ErrActionNotPermitted.Error())
	pendingUpdate := decodeTxResponse(t, patchV1Transaction(t, v1App, infra.orgID, infra.ledgerID, pendingID,
		`{"description":"pending update remains allowed"}`), nethttp.StatusOK)
	assert.Equal(t, "pending update remains allowed", pendingUpdate["description"])

	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx))
	require.NoError(t, infra.revertFreeze.Finalize(ctx))
	approvedUpdate := decodeTxResponse(t, patchV1Transaction(t, v1App, infra.orgID, infra.ledgerID, approvedID,
		`{"description":"approved update restored"}`), nethttp.StatusOK)
	assert.Equal(t, "approved update restored", approvedUpdate["description"])
}

func TestIntegration_TransactionV2Revert_RolloutBridgeAndFinalSharePostgresClaim(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	activateRevertUpdateFreeze(t, infra)
	bridge := &TransactionHandler{Query: infra.handler.Query, Command: infra.handler.Command, RevertIdempotencyMode: revertIdempotencyModeBridge, RevertUpdateFreeze: infra.revertFreeze}
	final := &TransactionHandler{Query: infra.handler.Query, Command: infra.handler.Command, RevertIdempotencyMode: revertIdempotencyModeFinal, RevertUpdateFreeze: infra.revertFreeze}
	type result struct {
		tran *transaction.Transaction
		err  error
	}
	results := make(chan result, 2)
	start := make(chan struct{})

	for _, handler := range []*TransactionHandler{bridge, final} {
		go func(h *TransactionHandler) {
			<-start
			tran, _, err := h.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
			results <- result{tran: tran, err: err}
		}(handler)
	}
	close(start)
	first, second := <-results, <-results

	reverseIDs := make(map[string]struct{})
	for _, got := range []result{first, second} {
		if got.err == nil && got.tran != nil {
			reverseIDs[got.tran.ID] = struct{}{}
		}
	}
	require.Len(t, reverseIDs, 1,
		"bridge and final may both return the durable replay, but must expose exactly one reserved reverse")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored exactly once")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored exactly once")
}

type lostBalanceResponseRepository struct {
	transactionredis.RedisRepository
	balanceCalls atomic.Int32
}

type lostLifecycleOutcomeResponseRepository struct {
	transactionredis.RedisRepository
	movementCommitted atomic.Bool
	hiddenOutcomeRead atomic.Bool
}

type lostBackupEnrichmentResponseRepository struct {
	transactionredis.RedisRepository
	lost atomic.Bool
}

type lostSeedResponseRepository struct {
	transactionredis.RedisRepository
	seedCalls atomic.Int32
}

type failFirstOriginCompletionRepository struct {
	transactionredis.RedisRepository
	completionCalls atomic.Int32
}

type failBeforeFirstCompleteRevertLease struct {
	delegate interface {
		CompleteRevert(context.Context, string, string) error
	}
	calls atomic.Int32
}

func (l *failBeforeFirstCompleteRevertLease) CompleteRevert(ctx context.Context, mode, token string) error {
	if l.calls.Add(1) == 1 {
		return errors.New("simulated rollout generation completion failure")
	}

	return l.delegate.CompleteRevert(ctx, mode, token)
}

type pausedTransactionUpdateRepository struct {
	transaction.Repository
	started chan struct{}
	release chan struct{}
}

type pausedTransactionWriteRepository struct {
	transaction.Repository
	started chan struct{}
	release chan struct{}
}

type pausedLegacyRevertBalanceRepository struct {
	transactionredis.RedisRepository
	started chan struct{}
	release chan struct{}
}

type pausedLegacyReplayRepository struct {
	transactionredis.RedisRepository
	started chan struct{}
	release chan struct{}
}

type observedBalanceRepository struct {
	transactionredis.RedisRepository
	started chan struct{}
}

func (r *lostLifecycleOutcomeResponseRepository) AcquireOwnedKey(
	ctx context.Context,
	key, owner string,
	ttl time.Duration,
) (bool, error) {
	if strings.HasPrefix(key, "pending_transaction:") {
		ttl = 1
	}

	return r.RedisRepository.AcquireOwnedKey(ctx, key, owner, ttl)
}

func (r *pausedLegacyReplayRepository) CompleteUnownedKey(
	ctx context.Context,
	key, value string,
	ttl time.Duration,
) (bool, error) {
	if strings.HasPrefix(key, "idempotency:") && value != "" {
		r.started <- struct{}{}
		<-r.release
	}

	return r.RedisRepository.CompleteUnownedKey(ctx, key, value, ttl)
}

func (r *pausedLegacyReplayRepository) CompleteOwnedKey(
	ctx context.Context,
	key, owner, value string,
	ttl time.Duration,
) (bool, error) {
	if strings.HasPrefix(key, "idempotency:") && value != "" {
		r.started <- struct{}{}
		<-r.release
	}

	return r.RedisRepository.CompleteOwnedKey(ctx, key, owner, value, ttl)
}

func (r *lostLifecycleOutcomeResponseRepository) ProcessOutcomeBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
	attempt mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceAtomicResult, error) {
	result, err := r.RedisRepository.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID,
		transactionID, transactionStatus, pending, balances, attempt)
	if err != nil {
		return result, err
	}
	r.movementCommitted.Store(true)

	return nil, errors.New("simulated lost response after immutable economic outcome")
}

func (r *lostLifecycleOutcomeResponseRepository) Get(ctx context.Context, key string) (string, error) {
	if strings.HasSuffix(key, ":balance-outcome") && r.movementCommitted.Load() &&
		r.hiddenOutcomeRead.CompareAndSwap(false, true) {
		return "", errors.New("simulated outcome read outage after lost Lua response")
	}

	return r.RedisRepository.Get(ctx, key)
}

func (r *lostBackupEnrichmentResponseRepository) AcquireOwnedKey(
	ctx context.Context,
	key, owner string,
	ttl time.Duration,
) (bool, error) {
	if strings.HasPrefix(key, "pending_transaction:") {
		ttl = 1
	}

	return r.RedisRepository.AcquireOwnedKey(ctx, key, owner, ttl)
}

func (r *lostBackupEnrichmentResponseRepository) EnrichTransactionBackup(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	operations []mmodel.OperationRedis,
	action string,
	attempt *mmodel.BalanceExecutionAttempt,
) ([]mmodel.OperationRedis, []mmodel.BalanceRedis, bool, error) {
	canonical, balancesAfter, terminal, err := r.RedisRepository.EnrichTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		operations, action, attempt)
	if err != nil {
		return nil, nil, false, err
	}
	if r.lost.CompareAndSwap(false, true) {
		return nil, nil, false, errors.New("simulated lost response after authoritative backup enrichment")
	}

	return canonical, balancesAfter, terminal, nil
}

func (r *pausedLegacyRevertBalanceRepository) ProcessBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
) (*mmodel.BalanceAtomicResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.release

	return r.RedisRepository.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		transactionStatus, pending, balances)
}

func (r *pausedLegacyRevertBalanceRepository) ProcessOutcomeBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
	attempt mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceAtomicResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.release

	return r.RedisRepository.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		transactionStatus, pending, balances, attempt)
}

func (r *observedBalanceRepository) ProcessBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
) (*mmodel.BalanceAtomicResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}

	return r.RedisRepository.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		transactionStatus, pending, balances)
}

func (r *observedBalanceRepository) ProcessOutcomeBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
	attempt mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceAtomicResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}

	return r.RedisRepository.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		transactionStatus, pending, balances, attempt)
}

func (r *pausedTransactionUpdateRepository) FindForUpdate(
	ctx context.Context,
	tx repository.DBExecutor,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*transaction.Transaction, error) {
	tran, err := r.Repository.FindForUpdate(ctx, tx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.release

	return tran, nil
}

func (r *pausedTransactionWriteRepository) UpdateTx(
	ctx context.Context,
	tx repository.DBExecutor,
	organizationID, ledgerID, transactionID uuid.UUID,
	tran *transaction.Transaction,
) (*transaction.Transaction, error) {
	r.started <- struct{}{}
	<-r.release

	return r.Repository.UpdateTx(ctx, tx, organizationID, ledgerID, transactionID, tran)
}

func (r *failFirstOriginCompletionRepository) CompleteOwnedKey(
	ctx context.Context,
	key, owner, value string,
	ttl time.Duration,
) (bool, error) {
	if r.completionCalls.Add(1) == 1 {
		return false, errors.New("simulated crash before origin replay completion")
	}

	return r.RedisRepository.CompleteOwnedKey(ctx, key, owner, value, ttl)
}

func (r *lostSeedResponseRepository) SeedTransactionBackup(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	message []byte,
	attempt mmodel.BalanceExecutionAttempt,
) error {
	call := r.seedCalls.Add(1)
	if err := r.RedisRepository.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, message, attempt); err != nil {
		return err
	}
	if call == 1 {
		return errors.New("simulated connection loss after revert seed was written")
	}

	return nil
}

func (r *lostBalanceResponseRepository) ProcessBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
) (*mmodel.BalanceAtomicResult, error) {
	call := r.balanceCalls.Add(1)
	result, err := r.RedisRepository.ProcessBalanceAtomicOperation(
		ctx, organizationID, ledgerID, transactionID, transactionStatus, pending, balances,
	)
	if err != nil {
		return result, err
	}

	if call == 1 {
		return nil, errors.New("simulated connection loss after balance Lua committed")
	}

	return result, nil
}

func (r *lostBalanceResponseRepository) ProcessOutcomeBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
	attempt mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceAtomicResult, error) {
	call := r.balanceCalls.Add(1)
	result, err := r.RedisRepository.ProcessOutcomeBalanceAtomicOperation(
		ctx, organizationID, ledgerID, transactionID, transactionStatus, pending, balances, attempt,
	)
	if err != nil {
		return result, err
	}
	if call == 1 {
		return nil, errors.New("simulated connection loss after outcome balance Lua committed")
	}

	return result, nil
}

func TestIntegration_TransactionV2Revert_CrashAfterPostgresPersistenceCompletesReplayOnRetry(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	faultRepo := &failFirstOriginCompletionRepository{RedisRepository: infra.redisRepo}
	infra.handler.Command.TransactionRedisRepo = faultRepo
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeFinal

	first := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, first, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, revertclaim.StateCompleted, claim.State,
		"the backup consumer may complete the durable transaction before the HTTP replay fence is materialized")
	assert.Nil(t, claim.FailureReason, "a replay-fence retry cannot downgrade a completed money-path claim")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"the reserved reverse is already durable when replay completion crashes")

	originKey := originRevertIdempotencyKey(claim)
	originOwner, err := infra.redisRepo.Get(ctx, originKey+":owner")
	require.NoError(t, err)
	assert.Equal(t, claim.ReverseTransactionID.String(), originOwner,
		"an ambiguous completion retains the exact reserved owner")

	retry := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, claim.ReverseTransactionID.String(), retry["id"])
	assert.Equal(t, originID.String(), retry["parentTransactionId"])
	assert.Equal(t, int32(2), faultRepo.completionCalls.Load(),
		"retry completes the persisted reverse instead of attempting another movement")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	completed, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, revertclaim.StateCompleted, completed.State)
	originOwner, err = infra.redisRepo.Get(ctx, originKey+":owner")
	require.NoError(t, err)
	assert.Empty(t, originOwner, "completion removes the in-flight owner token")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored once")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored once")
}

func TestIntegration_TransactionV2Revert_BridgeRetryUsesClaimedLegacyFenceAfterPayloadChanges(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	originTransaction, err := infra.handler.Query.GetTransactionWithOperationsByID(
		readrouting.WithPrimaryRead(ctx), infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	originalHash, err := legacyRevertIdempotencyHash(originTransaction.TransactionRevert())
	require.NoError(t, err)
	originalLegacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, originalHash)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	faultRepo := &failFirstOriginCompletionRepository{RedisRepository: infra.redisRepo}
	infra.handler.Command.TransactionRedisRepo = faultRepo

	first := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, first, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())
	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	require.NotNil(t, claim.LegacyFenceKey)
	assert.Equal(t, originalLegacyKey, *claim.LegacyFenceKey)
	assert.Equal(t, revertclaim.StateCompleted, claim.State)

	_, err = infra.pgContainer.DB.ExecContext(ctx, `
		UPDATE transaction SET description = $1
		WHERE organization_id = $2 AND ledger_id = $3 AND id = $4`,
		"payload changed after bridge claimed its fence", infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	mutatedOrigin, err := infra.handler.Query.GetTransactionWithOperationsByID(
		readrouting.WithPrimaryRead(ctx), infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	mutatedHash, err := legacyRevertIdempotencyHash(mutatedOrigin.TransactionRevert())
	require.NoError(t, err)
	mutatedLegacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, mutatedHash)
	require.NotEqual(t, originalLegacyKey, mutatedLegacyKey)

	retry := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, claim.ReverseTransactionID.String(), retry["id"])
	assert.Equal(t, originID.String(), retry["parentTransactionId"])
	originalReplay, err := infra.redisRepo.Get(ctx, originalLegacyKey)
	require.NoError(t, err)
	assert.NotEmpty(t, originalReplay, "retry must complete the exact fence persisted at claim acquisition")
	mutatedFence, err := infra.redisRepo.MGet(ctx, []string{mutatedLegacyKey, mutatedLegacyKey + ":owner"})
	require.NoError(t, err)
	assert.Empty(t, mutatedFence, "retry must never create or complete a key recalculated from mutable payload")
}

func TestIntegration_TransactionV2Revert_FailedRolloutCompletionRetriesExactPersistedGeneration(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		"@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	failedCompletion := &failBeforeFirstCompleteRevertLease{delegate: infra.revertFreeze}
	infra.handler.Command.RevertRolloutLease = failedCompletion

	first := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, first, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())
	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	require.Equal(t, revertclaim.StateCompleted, claim.State)
	require.NotNil(t, claim.RolloutMode)
	require.NotNil(t, claim.RolloutToken)
	assert.Equal(t, revertIdempotencyModeBridge, *claim.RolloutMode)
	require.NotNil(t, claim.RedisGeneration)
	evidence, generationMatches, err := infra.redisRepo.TransactionEconomicEvidenceExists(ctx, infra.orgID, infra.ledgerID,
		claim.ReverseTransactionID, *claim.RedisGeneration)
	require.NoError(t, err)
	assert.False(t, evidence, "transaction and operations are durable before rollout completion is retried")
	assert.True(t, generationMatches)
	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx))
	require.Error(t, infra.revertFreeze.Finalize(ctx),
		"the surviving exact bridge generation must prevent a false rollout drain")

	retry := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, claim.ReverseTransactionID.String(), retry["id"])
	require.NoError(t, infra.revertFreeze.Finalize(ctx),
		"HTTP adoption must seal the exact generation persisted before movement")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.Equal(t, int32(1), failedCompletion.calls.Load(),
		"retry uses the durable claim through the HTTP handoff, not a second consumer mutation")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t,
		infra.pgContainer.DB, srcID), "failed rollout completion cannot restore source twice")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t,
		infra.pgContainer.DB, dstID), "failed rollout completion cannot debit destination twice")
}

func TestIntegration_TransactionApprovedUpdateLeaseBlocksFreezeActivationUntilWriteCompletes(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	prepareRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	pausedRepo := &pausedTransactionWriteRepository{
		Repository: infra.handler.Command.TransactionRepo,
		started:    make(chan struct{}, 1),
		release:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRepo = pausedRepo

	type updateResult struct {
		transaction *transaction.Transaction
		err         error
	}
	result := make(chan updateResult, 1)
	go func() {
		updated, err := infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, originID,
			&transaction.UpdateTransactionInput{Description: "write admitted before activation"})
		result <- updateResult{transaction: updated, err: err}
	}()

	<-pausedRepo.started
	require.Error(t, infra.revertFreeze.Activate(ctx),
		"activation must be serialized after the PostgreSQL write admitted under the absent marker")
	close(pausedRepo.release)
	updated := <-result
	require.NoError(t, updated.err)
	require.NotNil(t, updated.transaction)
	assert.Equal(t, "write admitted before activation", updated.transaction.Description)

	require.NoError(t, infra.revertFreeze.Activate(ctx))
	_, err := infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, originID,
		&transaction.UpdateTransactionInput{Description: "must remain blocked"})
	var frozen pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &frozen)
	assert.Equal(t, cn.ErrActionNotPermitted.Error(), frozen.Code)
}

func TestIntegration_TransactionPendingUpdateSerializesCommitAcrossFreezeActivation(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, holdParityV2Body, ""), nethttp.StatusCreated)
	pendingID := uuid.MustParse(pending["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	prepareRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	pausedRepo := &pausedTransactionUpdateRepository{
		Repository: infra.handler.Command.TransactionRepo,
		started:    make(chan struct{}, 2),
		release:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRepo = pausedRepo
	observedBalance := &observedBalanceRepository{
		RedisRepository: infra.handler.Command.TransactionRedisRepo,
		started:         make(chan struct{}, 1),
	}
	infra.handler.Command.TransactionRedisRepo = observedBalance

	type updateResult struct {
		transaction *transaction.Transaction
		err         error
	}
	result := make(chan updateResult, 1)
	go func() {
		updated, err := infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, pendingID,
			&transaction.UpdateTransactionInput{Description: "pending write serialized before approval"})
		result <- updateResult{transaction: updated, err: err}
	}()

	<-pausedRepo.started
	commitResult := make(chan updateResult, 1)
	go func() {
		committed, err := infra.handler.commitTransaction(ctx, infra.orgID, infra.ledgerID, pendingID, cn.APPROVED)
		commitResult <- updateResult{transaction: committed, err: err}
	}()
	balanceMovedBeforePATCH := false
	select {
	case <-observedBalance.started:
		balanceMovedBeforePATCH = true
	case <-time.After(200 * time.Millisecond):
	}
	select {
	case early := <-commitResult:
		close(pausedRepo.release)
		require.Failf(t, "commit bypassed row lock", "result=%v err=%v", early.transaction, early.err)
	default:
	}
	require.NoError(t, infra.revertFreeze.Activate(ctx),
		"a PENDING update may finish while its row lock prevents partial promotion")

	close(pausedRepo.release)
	updated := <-result
	require.NoError(t, updated.err)
	require.NotNil(t, updated.transaction)
	assert.Equal(t, "pending write serialized before approval", updated.transaction.Description)
	committed := <-commitResult
	require.NoError(t, committed.err)
	require.NotNil(t, committed.transaction)
	assert.Equal(t, cn.APPROVED, committed.transaction.Status.Code)
	var persistedStatus, persistedDescription string
	require.NoError(t, infra.pgContainer.DB.QueryRowContext(ctx,
		`SELECT status, description FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = $3`,
		infra.orgID, infra.ledgerID, pendingID).Scan(&persistedStatus, &persistedDescription))
	require.Equal(t, cn.APPROVED, persistedStatus)
	require.Equal(t, "pending write serialized before approval", persistedDescription)
	require.False(t, balanceMovedBeforePATCH,
		"commit must not seed or move balances while PATCH owns the transaction row")
	phase, err := infra.revertFreeze.Phase(ctx)
	require.NoError(t, err)
	require.Equal(t, transactionredis.RevertUpdateFreezeActive, phase)

	_, err = infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, pendingID,
		&transaction.UpdateTransactionInput{Description: "must remain frozen"})
	var frozen pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &frozen)
	assert.Equal(t, cn.ErrActionNotPermitted.Error(), frozen.Code)

	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx))
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeFinal
	require.NoError(t, infra.revertFreeze.Finalize(ctx))
	finalConnection := redistestutil.CreateConnection(t, infra.redisContainer.Addr)
	finalGuard := transactionredis.NewRevertUpdateFreezeGuard(finalConnection,
		transactionredis.RevertUpdateFreezeFinalized, integrationRedisDatasetGeneration).
		WithRolloutInitializationWitness(revertclaim.NewPostgreSQLRepository(infra.pgConn), "")
	infra.revertFreeze = finalGuard
	infra.handler.RevertUpdateFreeze = finalGuard
	infra.handler.Command.RevertRolloutLease = finalGuard
	finalizedUpdate, err := infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, pendingID,
		&transaction.UpdateTransactionInput{Description: "approved updates restored immediately"})
	require.NoError(t, err)
	require.NotNil(t, finalizedUpdate)
	assert.Equal(t, "approved updates restored immediately", finalizedUpdate.Description,
		"the successful commit's Redis processing lock must not block finalized PATCH")
}

func TestIntegration_TransactionPendingUpdateSerializesCancelMovement(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, holdParityV2Body, ""), nethttp.StatusCreated)
	pendingID := uuid.MustParse(pending["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	pausedRepo := &pausedTransactionUpdateRepository{
		Repository: infra.handler.Command.TransactionRepo,
		started:    make(chan struct{}, 2),
		release:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRepo = pausedRepo
	observedBalance := &observedBalanceRepository{
		RedisRepository: infra.handler.Command.TransactionRedisRepo,
		started:         make(chan struct{}, 1),
	}
	infra.handler.Command.TransactionRedisRepo = observedBalance

	type updateResult struct {
		transaction *transaction.Transaction
		err         error
	}
	patchResult := make(chan updateResult, 1)
	go func() {
		updated, err := infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, pendingID,
			&transaction.UpdateTransactionInput{Description: "pending write serialized before cancel"})
		patchResult <- updateResult{transaction: updated, err: err}
	}()
	<-pausedRepo.started

	cancelResult := make(chan updateResult, 1)
	go func() {
		canceled, err := infra.handler.commitTransaction(ctx, infra.orgID, infra.ledgerID, pendingID, cn.CANCELED)
		cancelResult <- updateResult{transaction: canceled, err: err}
	}()
	select {
	case <-observedBalance.started:
		require.Fail(t, "cancel moved balances while PATCH owned the transaction row")
	case <-time.After(200 * time.Millisecond):
	}
	select {
	case early := <-cancelResult:
		require.Failf(t, "cancel bypassed row lock", "result=%v err=%v", early.transaction, early.err)
	default:
	}

	close(pausedRepo.release)
	updated := <-patchResult
	require.NoError(t, updated.err)
	require.Equal(t, "pending write serialized before cancel", updated.transaction.Description)
	canceled := <-cancelResult
	require.NoError(t, canceled.err)
	require.Equal(t, cn.CANCELED, canceled.transaction.Status.Code)

	var status, description string
	require.NoError(t, infra.pgContainer.DB.QueryRowContext(ctx,
		`SELECT status, description FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = $3`,
		infra.orgID, infra.ledgerID, pendingID).Scan(&status, &description))
	assert.Equal(t, cn.CANCELED, status)
	assert.Equal(t, "pending write serialized before cancel", description)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID),
		"cancel restores the held source exactly once")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID),
		"cancel releases the source hold exactly once")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID),
		"cancel never credits the destination")
	mutationKey := utils.PendingTransactionLockKey(infra.orgID, infra.ledgerID, pendingID.String())
	mutationValues, err := infra.redisRepo.MGet(ctx, []string{mutationKey, mutationKey + ":owner"})
	require.NoError(t, err)
	assert.Empty(t, mutationValues,
		"a terminal status with a durable handoff must release the request serialization immediately")
}

func TestIntegration_TransactionLifecycleLostOutcomeRejectsOppositeAfterProcessingLeaseExpires(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		firstStatus     string
		oppositeURL     func(uuid.UUID, uuid.UUID, uuid.UUID) string
		persistedStatus string
		sourceAvailable int64
		destination     int64
		outcome         string
	}{
		{
			name:            "committed outcome rejects delayed cancel",
			firstStatus:     cn.APPROVED,
			oppositeURL:     v2CancelURL,
			persistedStatus: cn.APPROVED,
			sourceAvailable: 900,
			destination:     1100,
			outcome:         mmodel.TransactionOutcomeCommitted,
		},
		{
			name:            "aborted outcome rejects delayed commit",
			firstStatus:     cn.CANCELED,
			oppositeURL:     v2CommitURL,
			persistedStatus: cn.CANCELED,
			sourceAvailable: 1000,
			destination:     1000,
			outcome:         mmodel.TransactionOutcomeAborted,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("ALLOW_INSECURE_TLS", "true")
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			infra := setupTestInfra(t)
			ctx := context.Background()
			srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
				"@src", "@dst", 1000, 1000)
			v2App := buildHumaV2DirectApp(t, infra.handler)
			pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
				holdParityV2Body, ""), nethttp.StatusCreated)
			pendingID := uuid.MustParse(pending["id"].(string))
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

			faultRepo := &lostLifecycleOutcomeResponseRepository{RedisRepository: infra.redisRepo}
			infra.handler.Command.TransactionRedisRepo = faultRepo
			_, err := infra.handler.commitTransaction(ctx, infra.orgID, infra.ledgerID, pendingID, scenario.firstStatus)
			var unavailable pkg.ServiceUnavailableError
			require.ErrorAs(t, err, &unavailable)
			assert.Equal(t, cn.ErrTransactionOutcomeReconciliationRequired.Error(), unavailable.Code)
			assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID),
				"lost response keeps PostgreSQL pending until exact outcome recovery")

			outcomeKey := utils.TransactionBalanceOutcomeKey(infra.orgID, infra.ledgerID, pendingID)
			rawOutcome, err := infra.redisRepo.Get(ctx, outcomeKey)
			require.NoError(t, err)
			var outcome mmodel.BalanceExecutionOutcome
			require.NoError(t, json.Unmarshal([]byte(rawOutcome), &outcome))
			assert.Equal(t, scenario.outcome, outcome.Outcome)

			processingOwnerKey := utils.PendingTransactionLockKey(infra.orgID, infra.ledgerID, pendingID.String()) + ":owner"
			require.Eventually(t, func() bool {
				owner, readErr := infra.redisRepo.Get(ctx, processingOwnerKey)
				return readErr == nil && owner == ""
			}, 3*time.Second, 25*time.Millisecond,
				"the test-shortened processing lease must expire while the durable outcome survives")

			opposite := postTransaction(t, v2App,
				scenario.oppositeURL(infra.orgID, infra.ledgerID, pendingID), "", "")
			assertProblemCode(t, opposite, nethttp.StatusConflict, cn.ErrCommitTransactionNotPending.Error())
			assert.Equal(t, scenario.persistedStatus,
				postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID),
				"opposite retry must first persist the immutable winning outcome")
			processingKey := utils.PendingTransactionLockKey(infra.orgID, infra.ledgerID, pendingID.String())
			processingValues, err := infra.redisRepo.MGet(ctx, []string{processingKey, processingKey + ":owner"})
			require.NoError(t, err)
			assert.Empty(t, processingValues,
				"a resolved opposite outcome must not retain the request serialization lease")

			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
			requireDecimalEqual(t, decimal.NewFromInt(scenario.sourceAvailable),
				postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID),
				"source reflects exactly one terminal movement")
			requireDecimalEqual(t, decimal.Zero,
				postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID),
				"source hold is released exactly once")
			requireDecimalEqual(t, decimal.NewFromInt(scenario.destination),
				postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID),
				"destination reflects exactly one terminal movement")
			assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
		})
	}
}

func TestIntegration_TransactionLifecycleLostEnrichmentResponsePreservesPostLuaEnvelope(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		"@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
		holdParityV2Body, ""), nethttp.StatusCreated)
	pendingID := uuid.MustParse(pending["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	faultRepo := &lostBackupEnrichmentResponseRepository{RedisRepository: infra.redisRepo}
	infra.handler.Command.TransactionRedisRepo = faultRepo
	result, err := infra.handler.commitTransaction(ctx, infra.orgID, infra.ledgerID, pendingID, cn.APPROVED)
	assert.Nil(t, result)
	require.ErrorContains(t, err, "simulated lost response after authoritative backup enrichment")
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID),
		"the terminal SQL CAS is durable before an enrichment response can be lost")

	backupKey := utils.TransactionInternalKey(infra.orgID, infra.ledgerID, pendingID.String())
	rawBackup, err := infra.redisRepo.ReadMessageFromQueue(ctx, backupKey)
	require.NoError(t, err)
	envelope := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(rawBackup, &envelope))
	assert.Equal(t, pendingID, envelope.TransactionID)
	assert.NotEmpty(t, envelope.AttemptOwner)
	assert.Equal(t, mmodel.TransactionOutcomeCommitted, envelope.ExpectedOutcome)
	require.NotEmpty(t, envelope.BalancesAfter, "the lost response must not erase the Lua-authored after state")
	require.NotEmpty(t, envelope.Operations, "the lost response must retain the exact materialized operation IDs")
	outcomeValue, err := infra.redisRepo.Get(ctx,
		utils.TransactionBalanceOutcomeKey(infra.orgID, infra.ledgerID, pendingID))
	require.NoError(t, err)
	require.NotEmpty(t, outcomeValue, "cleanup is forbidden until delayed persistence proves all operations durable")

	processingOwnerKey := utils.PendingTransactionLockKey(infra.orgID, infra.ledgerID, pendingID.String()) + ":owner"
	require.Eventually(t, func() bool {
		owner, readErr := infra.redisRepo.Get(ctx, processingOwnerKey)
		return readErr == nil && owner == ""
	}, 3*time.Second, 25*time.Millisecond)
	infra.handler.Command.TransactionRedisRepo = infra.redisRepo
	opposite, oppositeErr := infra.handler.commitTransaction(ctx, infra.orgID, infra.ledgerID, pendingID, cn.CANCELED)
	assert.Nil(t, opposite)
	var terminalConflict pkg.EntityConflictError
	require.ErrorAs(t, oppositeErr, &terminalConflict)
	assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), terminalConflict.Code)

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	requireDecimalEqual(t, decimal.NewFromInt(900),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "commit movement occurs exactly once")
	requireDecimalEqual(t, decimal.NewFromInt(1100),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "opposite retry never moves balances")
}

func TestIntegration_TransactionPendingUpdateSerializesBackupConsumerPromotion(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, holdParityV2Body, ""), nethttp.StatusCreated)
	pendingID := uuid.MustParse(pending["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	prepareRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	pausedRepo := &pausedTransactionUpdateRepository{
		Repository: infra.handler.Command.TransactionRepo,
		started:    make(chan struct{}, 1),
		release:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRepo = pausedRepo

	updateDone := make(chan error, 1)
	go func() {
		_, err := infra.handler.updateTransaction(ctx, infra.orgID, infra.ledgerID, pendingID,
			&transaction.UpdateTransactionInput{Description: "completed before consumer promotion"})
		updateDone <- err
	}()
	<-pausedRepo.started

	approved := cn.APPROVED
	promotionDone := make(chan error, 1)
	go func() {
		_, err := infra.handler.Command.UpdateTransactionStatus(ctx, &transaction.Transaction{
			ID:             pendingID.String(),
			OrganizationID: infra.orgID.String(),
			LedgerID:       infra.ledgerID.String(),
			Status: transaction.Status{
				Code:        approved,
				Description: &approved,
			},
		})
		promotionDone <- err
	}()
	select {
	case err := <-promotionDone:
		require.Failf(t, "consumer bypassed row lock", "err=%v", err)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, infra.revertFreeze.Activate(ctx))
	close(pausedRepo.release)
	require.NoError(t, <-updateDone)
	require.NoError(t, <-promotionDone)

	var persistedStatus, persistedDescription string
	require.NoError(t, infra.pgContainer.DB.QueryRowContext(ctx,
		`SELECT status, description FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = $3`,
		infra.orgID, infra.ledgerID, pendingID).Scan(&persistedStatus, &persistedDescription))
	assert.Equal(t, cn.APPROVED, persistedStatus)
	assert.Equal(t, "completed before consumer promotion", persistedDescription)
}

func TestIntegration_TransactionPendingUpdateCrashReleasesPostgresRowLock(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	pending := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, holdParityV2Body, ""), nethttp.StatusCreated)
	pendingID := uuid.MustParse(pending["id"].(string))

	dbTx, err := infra.handler.Command.TransactionRepo.BeginTx(ctx)
	require.NoError(t, err)
	_, err = infra.handler.Command.TransactionRepo.FindForUpdate(ctx, dbTx, infra.orgID, infra.ledgerID, pendingID)
	require.NoError(t, err)

	approved := cn.APPROVED
	promotionDone := make(chan error, 1)
	go func() {
		_, updateErr := infra.handler.Command.UpdateTransactionStatus(ctx, &transaction.Transaction{
			ID:             pendingID.String(),
			OrganizationID: infra.orgID.String(),
			LedgerID:       infra.ledgerID.String(),
			Status: transaction.Status{
				Code:        approved,
				Description: &approved,
			},
		})
		promotionDone <- updateErr
	}()
	select {
	case updateErr := <-promotionDone:
		require.Failf(t, "promotion bypassed crashed request row lock", "err=%v", updateErr)
	case <-time.After(200 * time.Millisecond):
	}

	// PostgreSQL releases row locks when the crashed request's connection rolls
	// back. No persistent Redis lease exists to strand the PENDING transaction.
	require.NoError(t, dbTx.Rollback())
	select {
	case updateErr := <-promotionDone:
		require.NoError(t, updateErr)
	case <-time.After(5 * time.Second):
		require.Fail(t, "promotion remained blocked after PostgreSQL rollback")
	}
	mutationKey := utils.PendingTransactionLockKey(infra.orgID, infra.ledgerID, pendingID.String())
	values, err := infra.redisRepo.MGet(ctx, []string{mutationKey, mutationKey + ":owner"})
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestIntegration_TransactionPhaseZeroRevertLeaseBlocksDrainUntilRequestCompletes(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	pausedRepo := &pausedLegacyRevertBalanceRepository{
		RedisRepository: infra.redisRepo,
		started:         make(chan struct{}, 1),
		release:         make(chan struct{}),
	}
	infra.handler.Command.TransactionRedisRepo = pausedRepo

	type revertResult struct {
		transaction *transaction.Transaction
		err         error
	}
	result := make(chan revertResult, 1)
	go func() {
		reverse, _, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
		result <- revertResult{transaction: reverse, err: err}
	}()

	<-pausedRepo.started
	require.Error(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx),
		"phase-zero drain must be serialized after every admitted legacy revert")
	loser, _, loserErr := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	require.Error(t, loserErr)
	require.Nil(t, loser)
	require.Error(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx),
		"a same-origin loser shares the durable origin admission and cannot release it underneath the paused winner")
	close(pausedRepo.release)
	reverse := <-result
	require.NoError(t, reverse.err)
	require.NotNil(t, reverse.transaction)
	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx))
}

func TestIntegration_TransactionPhaseZeroDrainWaitsForLegacyReplayPublication(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	pausedRepo := &pausedLegacyReplayRepository{
		RedisRepository: infra.redisRepo,
		started:         make(chan struct{}, 1),
		release:         make(chan struct{}),
	}
	infra.handler.Command.TransactionRedisRepo = pausedRepo

	type revertResult struct {
		transaction *transaction.Transaction
		err         error
	}
	result := make(chan revertResult, 1)
	go func() {
		reverse, _, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
		result <- revertResult{transaction: reverse, err: err}
	}()

	<-pausedRepo.started
	originTransaction, err := infra.handler.Query.GetTransactionWithOperationsByID(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	legacyHash, err := legacyRevertIdempotencyHash(originTransaction.TransactionRevert())
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, legacyHash)
	fenceTTL, err := infra.redisContainer.Client.TTL(ctx, legacyKey).Result()
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), fenceTTL, "phase-zero request fence must not expire before replay publication")
	require.Error(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx),
		"drain must remain blocked while the admitted request publishes its replay")
	select {
	case early := <-result:
		require.Failf(t, "phase-zero request returned before replay publication", "result=%+v", early)
	default:
	}

	close(pausedRepo.release)
	reverse := <-result
	require.NoError(t, reverse.err)
	require.NotNil(t, reverse.transaction)
	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx))

	encodedReplay, err := infra.redisRepo.Get(ctx, legacyKey)
	require.NoError(t, err)
	persistedReplay := &transaction.Transaction{}
	require.NoError(t, json.Unmarshal([]byte(encodedReplay), persistedReplay))
	require.Equal(t, reverse.transaction.ID, persistedReplay.ID)
	require.NotNil(t, persistedReplay.ParentTransactionID)
	require.Equal(t, originID.String(), *persistedReplay.ParentTransactionID)
}

func TestIntegration_TransactionBridgeRevertLeaseBlocksFinalizationUntilRequestCompletes(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	pausedRepo := &pausedFencedBalanceRepository{
		RedisRepository: infra.redisRepo,
		firstStarted:    make(chan uuid.UUID, 1),
		releaseFirst:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRedisRepo = pausedRepo

	type revertResult struct {
		transaction *transaction.Transaction
		err         error
	}
	result := make(chan revertResult, 1)
	go func() {
		reverse, _, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
		result <- revertResult{transaction: reverse, err: err}
	}()

	<-pausedRepo.firstStarted
	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx),
		"a bridge request may safely continue after phase zero is drained")
	require.Error(t, infra.revertFreeze.Finalize(ctx),
		"finalization must be serialized after every admitted bridge revert")
	close(pausedRepo.releaseFirst)
	reverse := <-result
	require.NoError(t, reverse.err)
	require.NotNil(t, reverse.transaction)
	require.NoError(t, infra.revertFreeze.Finalize(ctx))
}

func TestIntegration_TransactionV2Revert_LostSeedResponsePreservesRecoveryEvidence(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	faultRepo := &lostSeedResponseRepository{RedisRepository: infra.redisRepo}
	infra.handler.Command.TransactionRedisRepo = faultRepo
	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge

	first := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, first, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, revertclaim.StateReconciliationRequired, claim.State)
	require.NotNil(t, claim.FailureReason)
	assert.Equal(t, "revert_seed_write_outcome_ambiguous", *claim.FailureReason)

	backup, err := infra.redisRepo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID.String()))
	require.NoError(t, err)
	queued := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(backup, &queued))
	assert.Empty(t, queued.BalancesAfter, "a lost seed response occurs before any balance mutation")
	require.NotNil(t, queued.ParentTransactionID)
	assert.Equal(t, originID, *queued.ParentTransactionID)

	originKey := originRevertIdempotencyKey(claim)
	originOwner, err := infra.redisRepo.Get(ctx, originKey+":owner")
	require.NoError(t, err)
	assert.Equal(t, claim.ReverseTransactionID.String(), originOwner)
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "lost seed response does not restore source")
	requireDecimalEqual(t, decimal.NewFromInt(1100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "lost seed response does not debit destination")

	retry := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, retry, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())
	assert.Equal(t, int32(1), faultRepo.seedCalls.Load(), "reconciliation fencing must prevent a second seed or balance attempt")
}

func TestIntegration_TransactionV2Revert_LostBalanceResponseStaysFenced(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	originTransaction, err := infra.handler.Query.GetTransactionWithOperationsByID(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	legacyHash, err := legacyRevertIdempotencyHash(originTransaction.TransactionRevert())
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, legacyHash)
	originKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))

	faultRepo := &lostBalanceResponseRepository{RedisRepository: infra.redisRepo}
	infra.handler.Command.TransactionRedisRepo = faultRepo
	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge

	first := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, first, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, revertclaim.StateReconciliationRequired, claim.State)
	require.NotNil(t, claim.FailureReason)
	assert.Equal(t, "balance_commit_outcome_ambiguous", *claim.FailureReason)

	legacyOwner, err := infra.redisRepo.Get(ctx, legacyKey+":owner")
	require.NoError(t, err)
	assert.Equal(t, claim.ReverseTransactionID.String(), legacyOwner)
	legacyTTL, err := infra.redisContainer.Client.TTL(ctx, legacyKey).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), legacyTTL, "an ambiguous bridge fence must not expire")
	legacyOwnerTTL, err := infra.redisContainer.Client.TTL(ctx, legacyKey+":owner").Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), legacyOwnerTTL, "an ambiguous owner token must not expire")
	legacyAvailable, err := infra.redisRepo.SetNX(ctx, legacyKey, "late-owner", 300)
	require.NoError(t, err)
	assert.False(t, legacyAvailable, "an ambiguous response must retain the legacy bridge fence")
	originAvailable, err := infra.redisRepo.SetNX(ctx, originKey, "late-origin", 300)
	require.NoError(t, err)
	assert.False(t, originAvailable, "an ambiguous response must retain the origin fence")

	backup, err := infra.redisRepo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID.String()))
	require.NoError(t, err)
	require.NotEmpty(t, backup, "the committed Lua movement must leave the reserved reverse recoverable")
	backupEnvelope := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(backup, &backupEnvelope))
	require.NotNil(t, backupEnvelope.ParentTransactionID)
	assert.Equal(t, originID, *backupEnvelope.ParentTransactionID,
		"recovery must carry the exact origin independently of the economic payload")

	sourceAfterFirst := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@src", cn.DefaultBalanceKey)
	destinationAfterFirst := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dst", cn.DefaultBalanceKey)
	require.NotNil(t, sourceAfterFirst)
	require.NotNil(t, destinationAfterFirst)
	requireDecimalEqual(t, decimal.NewFromInt(1000), sourceAfterFirst.Available, "lost response still committed one source restoration")
	requireDecimalEqual(t, decimal.NewFromInt(1000), destinationAfterFirst.Available, "lost response still committed one destination restoration")

	// Recreate the durable state a hard process crash leaves when Redis commits
	// but the process dies before it can mark the claim MUTATED/reconciliation.
	// ARMED was committed on PostgreSQL primary before Lua could run. The retry
	// must infer the commit only from Lua's atomic BalancesAfter outcome, never
	// from process memory.
	_, err = infra.pgContainer.DB.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET state = 'ARMED', failure_reason = NULL
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)

	retry := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, retry, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())
	assert.Equal(t, int32(1), faultRepo.balanceCalls.Load(), "retry must not dispatch a second balance mutation")
	hardCrashClaim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, hardCrashClaim)
	assert.Equal(t, revertclaim.StateArmed, hardCrashClaim.State,
		"a retry must not overwrite the shared phase while the original owner may still be completing persistence")
	assert.Nil(t, hardCrashClaim.FailureReason,
		"the HTTP 0505 response exposes reconciliation while ARMED remains the durable no-retry fence")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"ambiguous movement stays unpersisted and fenced until backup reconciliation")

	sourceAfterRetry := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@src", cn.DefaultBalanceKey)
	destinationAfterRetry := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dst", cn.DefaultBalanceKey)
	requireDecimalEqual(t, sourceAfterFirst.Available, sourceAfterRetry.Available, "retry cannot restore the source twice")
	requireDecimalEqual(t, destinationAfterFirst.Available, destinationAfterRetry.Available, "retry cannot debit the destination twice")

	// Selective loss of every per-transaction Redis record while the global
	// financial generation survives is not proof that Lua never moved money.
	// ARMED on PostgreSQL is the durable witness that makes this fail closed.
	_, err = infra.pgContainer.DB.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET state = 'ARMED', failure_reason = NULL
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID.String())
	executionKey := utils.TransactionBalanceExecutionKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID)
	outcomeKey := utils.TransactionBalanceOutcomeKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID)
	require.NoError(t, infra.redisContainer.Client.HDel(ctx, transactionredis.TransactionBackupQueue, backupKey).Err())
	require.NoError(t, infra.redisContainer.Client.Del(ctx, outcomeKey, executionKey, executionKey+":owner").Err())

	lostEvidenceRetry := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", "")
	assertProblemCode(t, lostEvidenceRetry, nethttp.StatusServiceUnavailable, cn.ErrRevertReconciliationRequired.Error())
	assert.Equal(t, int32(1), faultRepo.balanceCalls.Load(), "selective Redis loss cannot authorize a second balance command")
	lostEvidenceClaim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, lostEvidenceClaim)
	assert.Equal(t, revertclaim.StateReconciliationRequired, lostEvidenceClaim.State)
	require.NotNil(t, lostEvidenceClaim.FailureReason)
	assert.Equal(t, "armed_revert_economic_evidence_missing", *lostEvidenceClaim.FailureReason)

	sourceAfterLoss := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@src", cn.DefaultBalanceKey)
	destinationAfterLoss := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dst", cn.DefaultBalanceKey)
	requireDecimalEqual(t, sourceAfterFirst.Available, sourceAfterLoss.Available, "selective loss cannot restore the source twice")
	requireDecimalEqual(t, destinationAfterFirst.Available, destinationAfterLoss.Available, "selective loss cannot debit the destination twice")
}

func TestIntegration_TransactionV2Revert_FinalRecoversBridgeCrashBeforeLuaThenMovesOnce(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	originTransaction, err := infra.handler.Query.GetTransactionWithOperationsByID(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, originTransaction)
	legacyHash, err := legacyRevertIdempotencyHash(originTransaction.TransactionRevert())
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, legacyHash)

	staleReverseID := uuid.New()
	legacyOwner := staleReverseID.String()
	rolloutMode := "bridge"
	rolloutToken := "bridge-crash-token"
	redisGeneration, err := infra.revertFreeze.FinancialDatasetGeneration(ctx)
	require.NoError(t, err)
	claim, acquired, err := infra.handler.Command.ClaimRevert(ctx, infra.orgID, infra.ledgerID, originID,
		staleReverseID, &legacyKey, &legacyOwner, &rolloutMode, &rolloutToken, &redisGeneration)
	require.NoError(t, err)
	require.True(t, acquired)
	originKey := originRevertIdempotencyKey(claim)
	originAcquired, err := infra.redisRepo.AcquireOwnedKey(ctx, originKey, staleReverseID.String(), 0)
	require.NoError(t, err)
	require.True(t, originAcquired)

	legacyAcquired, err := infra.redisRepo.AcquireOwnedKey(ctx, legacyKey, staleReverseID.String(), 0)
	require.NoError(t, err)
	require.True(t, legacyAcquired)
	seed, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       staleReverseID,
		ParentTransactionID: &originID,
		OrganizationID:      infra.orgID,
		LedgerID:            infra.ledgerID,
		TransactionStatus:   cn.CREATED,
		Action:              cn.ActionRevert,
		AttemptOwner:        staleReverseID.String(),
		ExpectedOutcome:     mmodel.TransactionOutcomeCommitted,
		RedisGeneration:     redisGeneration,
	})
	require.NoError(t, err)
	staleBackupKey := utils.TransactionInternalKey(infra.orgID, infra.ledgerID, staleReverseID.String())
	require.NoError(t, infra.redisRepo.AddMessageToQueue(ctx, staleBackupKey, seed))

	_, err = infra.pgContainer.DB.ExecContext(ctx, `
		UPDATE transaction SET description = $1
		WHERE organization_id = $2 AND ledger_id = $3 AND id = $4`,
		"payload changed after bridge acquired its fence", infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	mutatedOrigin, err := infra.handler.Query.GetTransactionWithOperationsByID(readrouting.WithPrimaryRead(ctx),
		infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	mutatedHash, err := legacyRevertIdempotencyHash(mutatedOrigin.TransactionRevert())
	require.NoError(t, err)
	mutatedLegacyKey := utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, mutatedHash)
	require.NotEqual(t, legacyKey, mutatedLegacyKey,
		"the test must prove final recovery cannot recalculate the bridge fence from mutable payload")

	infra.handler.RevertIdempotencyMode = revertIdempotencyModeFinal
	reverse := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	reverseID := uuid.MustParse(reverse["id"].(string))
	assert.NotEqual(t, staleReverseID, reverseID, "recovery releases the abandoned reservation before retrying")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	claim, err = infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, reverseID, claim.ReverseTransactionID)
	assert.Equal(t, revertclaim.StateCompleted, claim.State)
	legacyValues, err := infra.redisRepo.MGet(ctx, []string{legacyKey, legacyKey + ":owner"})
	require.NoError(t, err)
	assert.Empty(t, legacyValues, "final recovery must clear the exact persistent fence left by bridge")
	mutatedLegacyValues, err := infra.redisRepo.MGet(ctx, []string{mutatedLegacyKey, mutatedLegacyKey + ":owner"})
	require.NoError(t, err)
	assert.Empty(t, mutatedLegacyValues, "recovery must not create or clean a recalculated legacy fence")
	_, err = infra.redisRepo.ReadMessageFromQueue(ctx, staleBackupKey)
	assert.ErrorIs(t, err, redislib.Nil)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "pre-Lua recovery restores source once")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "pre-Lua recovery restores destination once")
}

func TestIntegration_TransactionV2Revert_CrashBeforeSeedRecoversThenMovesOnce(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	staleReverseID := uuid.New()
	redisGeneration, err := infra.revertFreeze.FinancialDatasetGeneration(ctx)
	require.NoError(t, err)
	claim, acquired, err := infra.handler.Command.ClaimRevert(ctx, infra.orgID, infra.ledgerID, originID,
		staleReverseID, nil, nil, nil, nil, &redisGeneration)
	require.NoError(t, err)
	require.True(t, acquired)
	originKey := originRevertIdempotencyKey(claim)
	originAcquired, err := infra.redisRepo.SetNX(ctx, originKey, "", 300)
	require.NoError(t, err)
	require.True(t, originAcquired)
	originOwnerAcquired, err := infra.redisRepo.SetNX(ctx, originKey+":owner", staleReverseID.String(), 0)
	require.NoError(t, err)
	require.True(t, originOwnerAcquired)

	// No execution lease and no backup remain after the simulated crash. On
	// Redis primary, that absence proves balance Lua was never dispatched.
	reverse := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	reverseID := uuid.MustParse(reverse["id"].(string))
	assert.NotEqual(t, staleReverseID, reverseID)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	completed, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, reverseID, completed.ReverseTransactionID)
	assert.Equal(t, revertclaim.StateCompleted, completed.State)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored once after crash-before-seed recovery")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored once after crash-before-seed recovery")
}

type panicOnceLedgerSettingsRepository struct {
	ledger.Repository
	panicNext atomic.Bool
}

func (r *panicOnceLedgerSettingsRepository) GetSettings(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
) (map[string]any, error) {
	if r.panicNext.Swap(false) {
		panic("simulated process crash after persistent H1 and before queue seed")
	}

	return r.Repository.GetSettings(ctx, organizationID, ledgerID)
}

func TestIntegration_TransactionPhaseZeroCrashBeforeSeedHasDurableRecoverableH1Owner(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		"@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, "phase-zero-crash-before-seed"), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	prepareRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeLegacy
	originalLedgerRepo := infra.handler.Query.LedgerRepo
	crashingLedgerRepo := &panicOnceLedgerSettingsRepository{Repository: originalLedgerRepo}
	crashingLedgerRepo.panicNext.Store(true)
	infra.handler.Query.LedgerRepo = crashingLedgerRepo
	infra.handler.Query.OnboardingRedisRepo = nil

	require.Panics(t, func() {
		_, _, _ = infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	})
	infra.handler.Query.LedgerRepo = originalLedgerRepo

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim, "persistent phase-zero H1 must never exist without a durable origin claim")
	require.NotNil(t, claim.LegacyFenceKey)
	require.NotNil(t, claim.LegacyFenceOwner)
	require.Equal(t, claim.ReverseTransactionID.String(), *claim.LegacyFenceOwner)

	values, err := infra.redisRepo.MGet(ctx, []string{*claim.LegacyFenceKey, *claim.LegacyFenceKey + ":owner"})
	require.NoError(t, err)
	require.Contains(t, values, *claim.LegacyFenceKey, "old-compatible H1 main fence must exist")
	require.Equal(t, claim.ReverseTransactionID.String(), values[*claim.LegacyFenceKey+":owner"])
	backupKey := utils.TransactionInternalKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID.String())
	_, err = infra.redisRepo.ReadMessageFromQueue(ctx, backupKey)
	require.ErrorIs(t, err, redislib.Nil, "the crash point is proven before queue seed")
	require.Error(t, infra.revertFreeze.Activate(ctx),
		"a crash after claim and H1 must keep marker activation blocked until the same origin is recovered")

	executionKey := utils.TransactionBalanceExecutionKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID)
	require.NoError(t, infra.redisContainer.Client.Del(ctx, executionKey, executionKey+":owner").Err(),
		"simulate the crashed process losing the owner checked execution lease")

	reverse, replayed, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, reverse)
	require.False(t, replayed, "the recovered pre-seed attempt creates one fresh reverse")
	require.Equal(t, originID.String(), *reverse.ParentTransactionID)
	require.NotEqual(t, claim.ReverseTransactionID.String(), reverse.ID,
		"pre-movement recovery releases the abandoned reserved ID before retry")
	originRolloutToken := libCommons.HashSHA256(strings.Join([]string{
		infra.orgID.String(), infra.ledgerID.String(), originID.String(),
	}, ":"))
	terminalAdmitted, terminalLease, _, terminalErr := infra.revertFreeze.AcquireRevert(ctx,
		revertIdempotencyModeLegacy, originRolloutToken, "post-terminal-probe")
	require.NoError(t, terminalErr)
	require.True(t, terminalAdmitted)
	require.False(t, terminalLease,
		"terminal handoff must seal the origin before removing its rollout attempts")
	require.NoError(t, infra.revertFreeze.Activate(ctx),
		"the retry releases the deterministic origin admission only after its terminal handoff")
	replayedReverse, replayed, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, reverse.ID, replayedReverse.ID,
		"a completed phase-zero claim must replay after its terminal backup has been cleaned")

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	require.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored once")
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored once")
}

func TestIntegration_TransactionBridgeCrashPreservesExactGenerationDrainUntilRecovery(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		"@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, "bridge-crash-before-seed"), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge
	originalLedgerRepo := infra.handler.Query.LedgerRepo
	crashingLedgerRepo := &panicOnceLedgerSettingsRepository{Repository: originalLedgerRepo}
	crashingLedgerRepo.panicNext.Store(true)
	infra.handler.Query.LedgerRepo = crashingLedgerRepo
	infra.handler.Query.OnboardingRedisRepo = nil

	require.Panics(t, func() {
		_, _, _ = infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	})
	infra.handler.Query.LedgerRepo = originalLedgerRepo

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	require.NoError(t, infra.revertFreeze.MarkPhaseZeroDrained(ctx),
		"phase-zero drain is independent from an admitted bridge origin")
	require.Error(t, infra.revertFreeze.Finalize(ctx),
		"a bridge crash after its durable claim must keep the bridge generation fenced")

	executionKey := utils.TransactionBalanceExecutionKey(infra.orgID, infra.ledgerID, claim.ReverseTransactionID)
	require.NoError(t, infra.redisContainer.Client.Del(ctx, executionKey, executionKey+":owner").Err())
	reverse, replayed, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, reverse)
	require.False(t, replayed)
	require.Equal(t, originID.String(), *reverse.ParentTransactionID)
	require.NoError(t, infra.revertFreeze.Finalize(ctx),
		"terminal recovery must release the bridge set, not the phase-zero set")

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	require.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored once")
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored once")
}

type pausedRevertClaimRepository struct {
	revertclaim.Repository
	claimed chan *revertclaim.Claim
	release chan struct{}
}

func (r *pausedRevertClaimRepository) Claim(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
	legacyFenceKey, legacyFenceOwner, rolloutMode, rolloutToken, redisGeneration *string,
) (*revertclaim.Claim, bool, error) {
	claim, acquired, err := r.Repository.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		legacyFenceKey, legacyFenceOwner, rolloutMode, rolloutToken, redisGeneration)
	if err == nil && acquired {
		r.claimed <- claim
		<-r.release
	}

	return claim, acquired, err
}

func TestIntegration_TransactionV2Revert_VisibleClaimAlreadyHasExecutionAttempt(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		"@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	pausedClaims := &pausedRevertClaimRepository{
		Repository: infra.handler.Command.RevertClaimRepo,
		claimed:    make(chan *revertclaim.Claim, 1),
		release:    make(chan struct{}),
	}
	infra.handler.Command.RevertClaimRepo = pausedClaims

	type revertResult struct {
		transaction *transaction.Transaction
		err         error
	}
	firstResult := make(chan revertResult, 1)
	go func() {
		reverse, _, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
		firstResult <- revertResult{transaction: reverse, err: err}
	}()

	claim := <-pausedClaims.claimed
	executionOwner, err := infra.redisRepo.Get(ctx, revertExecutionFenceKey(claim)+":owner")
	require.NoError(t, err)
	assert.Equal(t, claim.ReverseTransactionID.String(), executionOwner,
		"a durable claim must never become visible before its stale-winner execution fence")

	retry, _, retryErr := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	assert.Nil(t, retry)
	require.Error(t, retryErr)
	var activeConflict pkg.EntityConflictError
	require.ErrorAs(t, retryErr, &activeConflict)
	assert.Equal(t, cn.ErrIdempotencyKey.Error(), activeConflict.Code)
	select {
	case early := <-firstResult:
		require.Failf(t, "visible claim was recovered while its owner was live",
			"transaction=%v err=%v", early.transaction, early.err)
	default:
	}

	close(pausedClaims.release)
	winner := <-firstResult
	require.NoError(t, winner.err)
	require.NotNil(t, winner.transaction)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored once")
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored once")
}

type pausedFencedBalanceRepository struct {
	transactionredis.RedisRepository
	firstStarted chan uuid.UUID
	releaseFirst chan struct{}
	calls        atomic.Int32
	movements    atomic.Int32
}

type pausedSeedResponseRepository struct {
	transactionredis.RedisRepository
	firstStarted chan uuid.UUID
	releaseFirst chan struct{}
	seedCalls    atomic.Int32
}

func (r *pausedSeedResponseRepository) SeedTransactionBackup(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	message []byte,
	attempt mmodel.BalanceExecutionAttempt,
) error {
	call := r.seedCalls.Add(1)
	if err := r.RedisRepository.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, message, attempt); err != nil {
		return err
	}
	if call != 1 {
		return nil
	}

	queued := mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(message, &queued); err != nil {
		return err
	}
	r.firstStarted <- transactionID
	<-r.releaseFirst

	return errors.New("simulated stale seed response after successor takeover")
}

func (r *pausedFencedBalanceRepository) ProcessOutcomeBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balances []mmodel.BalanceOperation,
	attempt mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceAtomicResult, error) {
	if r.calls.Add(1) == 1 {
		r.firstStarted <- transactionID
		<-r.releaseFirst
	}

	result, err := r.RedisRepository.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID,
		transactionID, transactionStatus, pending, balances, attempt)
	if err == nil {
		r.movements.Add(1)
	}

	return result, err
}

func TestIntegration_TransactionV2Revert_ExpiredExecutionLeaseFencesPausedWinner(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	pausedRepo := &pausedFencedBalanceRepository{
		RedisRepository: infra.redisRepo,
		firstStarted:    make(chan uuid.UUID, 1),
		releaseFirst:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRedisRepo = pausedRepo

	type revertResult struct {
		transaction *transaction.Transaction
		err         error
	}
	firstResult := make(chan revertResult, 1)
	go func() {
		tran, _, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
		firstResult <- revertResult{transaction: tran, err: err}
	}()

	var pausedReverseID uuid.UUID
	select {
	case pausedReverseID = <-pausedRepo.firstStarted:
	case early := <-firstResult:
		require.NoError(t, early.err, "first revert returned before reaching the paused seed")
		require.FailNow(t, "first revert returned before reaching the paused seed")
	case <-time.After(10 * time.Second):
		require.FailNow(t, "first revert did not reach the paused seed")
	}
	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, pausedReverseID, claim.ReverseTransactionID)
	fenceKey := revertExecutionFenceKey(claim)
	fenceValue, err := infra.redisRepo.Get(ctx, fenceKey+":owner")
	require.NoError(t, err)
	assert.Equal(t, pausedReverseID.String(), fenceValue)

	activeRetry, _, activeRetryErr := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	assert.Nil(t, activeRetry)
	require.Error(t, activeRetryErr)
	assert.Equal(t, int32(1), pausedRepo.calls.Load(), "an active execution lease must not be mistaken for a crashed winner")

	// Expiration is simulated by releasing both same-slot lease records with
	// the old reverse's owner token. Because PostgreSQL was already ARMED before
	// ProcessBalanceOperations could be invoked, the missing lease is ambiguous:
	// it may be a pre-Lua crash or selective Redis loss after movement. Recovery
	// must preserve the claim and refuse a successor forever.
	released, err := infra.redisRepo.ReleaseOwnedKey(ctx, fenceKey, pausedReverseID.String())
	require.NoError(t, err)
	require.True(t, released)
	takeover, _, takeoverErr := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	assert.Nil(t, takeover)
	require.Error(t, takeoverErr)
	var reconciliation pkg.ServiceUnavailableError
	require.ErrorAs(t, takeoverErr, &reconciliation)
	assert.Equal(t, cn.ErrRevertReconciliationRequired.Error(), reconciliation.Code)
	claim, err = infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, revertclaim.StateReconciliationRequired, claim.State)
	assert.Equal(t, pausedReverseID, claim.ReverseTransactionID)

	close(pausedRepo.releaseFirst)
	stale := <-firstResult
	assert.Nil(t, stale.transaction)
	require.Error(t, stale.err)
	var staleConflict pkg.EntityConflictError
	require.ErrorAs(t, stale.err, &staleConflict)
	assert.Equal(t, cn.ErrIdempotencyKey.Error(), staleConflict.Code)
	assert.Equal(t, int32(0), pausedRepo.movements.Load(), "neither the stale executor nor a successor may move balances")

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source remains at the origin result")
	requireDecimalEqual(t, decimal.NewFromInt(1100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination remains at the origin result")
}

func TestIntegration_TransactionV2Revert_PausedSeedWriterCannotDeleteSuccessorReplay(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	activateRevertUpdateFreeze(t, infra)
	infra.handler.RevertIdempotencyMode = revertIdempotencyModeBridge

	pausedRepo := &pausedSeedResponseRepository{
		RedisRepository: infra.redisRepo,
		firstStarted:    make(chan uuid.UUID, 1),
		releaseFirst:    make(chan struct{}),
	}
	infra.handler.Command.TransactionRedisRepo = pausedRepo

	type revertResult struct {
		transaction *transaction.Transaction
		err         error
	}
	firstResult := make(chan revertResult, 1)
	go func() {
		tran, _, err := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
		firstResult <- revertResult{transaction: tran, err: err}
	}()

	var pausedReverseID uuid.UUID
	select {
	case pausedReverseID = <-pausedRepo.firstStarted:
	case early := <-firstResult:
		require.NoError(t, early.err, "first revert returned before reaching the paused seed")
		require.FailNow(t, "first revert returned before reaching the paused seed")
	case <-time.After(10 * time.Second):
		require.FailNow(t, "first revert did not reach the paused seed")
	}
	oldClaim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, oldClaim)
	assert.Equal(t, pausedReverseID, oldClaim.ReverseTransactionID)

	activeRetry, _, activeRetryErr := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	assert.Nil(t, activeRetry)
	require.Error(t, activeRetryErr)
	assert.Equal(t, int32(1), pausedRepo.seedCalls.Load(), "the live execution lease must fence takeover")

	released, err := infra.redisRepo.ReleaseOwnedKey(ctx, revertExecutionFenceKey(oldClaim), pausedReverseID.String())
	require.NoError(t, err)
	require.True(t, released)
	takeover, _, takeoverErr := infra.handler.revertTransaction(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, takeoverErr)
	require.NotNil(t, takeover)
	assert.NotEqual(t, pausedReverseID.String(), takeover.ID)

	close(pausedRepo.releaseFirst)
	stale := <-firstResult
	assert.Nil(t, stale.transaction)
	require.Error(t, stale.err)

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, uuid.MustParse(takeover.ID), claim.ReverseTransactionID)
	assert.Equal(t, revertclaim.StateCompleted, claim.State)

	replayJSON, err := infra.redisRepo.Get(ctx, originRevertIdempotencyKey(claim))
	require.NoError(t, err)
	replay := transaction.Transaction{}
	require.NoError(t, json.Unmarshal([]byte(replayJSON), &replay))
	assert.Equal(t, takeover.ID, replay.ID, "the stale winner cannot delete or replace the successor replay")
	require.NotNil(t, replay.ParentTransactionID)
	assert.Equal(t, originID.String(), *replay.ParentTransactionID)

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored exactly once")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination restored exactly once")
}

// =============================================================================
// 19. CONCURRENT REVERT OF ONE ORIGIN — EXACTLY ONE MUTATION (money path):
//     concurrent requests share the PostgreSQL claim and origin Redis barrier.
//     Callers racing after persistence may receive the same successful replay;
//     only one reverse transaction and one balance mutation may exist.
//
//     N racers revert one APPROVED origin from a common start barrier. The invariant asserted
//     is "exactly one 201, every other answer a 4xx" rather than one specific sentinel: the
//     loser's code depends on the interleaving (409/0084 when it loses the SetNX before the
//     winner's response is cached, 409/0087 once the winner's child is visible to the gate).
//     The observed codes are logged for the record.
//
//     CONCURRENCY NOTE: only the REQUESTS race. The app is built once, before the barrier —
//     the file header's sequential rule is about libProblem.Install() and the app builders,
//     not about serving; fiber's App.Test guards startupProcess with app.mutex and each call
//     serves its own connection.
// =============================================================================

// concurrentRevertRacers is the number of goroutines that revert the single origin at once.
// Small enough to stay fast, large enough that at least one racer loses the claim rather than
// the gate.
const concurrentRevertRacers = 8

// revertRaceResult is one racer's outcome, captured WITHOUT touching *testing.T: require/assert
// call t.FailNow, which is illegal outside the test goroutine, so every racer records its raw
// outcome and the assertions all run on the test goroutine after the WaitGroup drains.
type revertRaceResult struct {
	transportErr error
	status       int
	replayed     string
	txID         string
	problemCode  string
	body         string
}

// fireRevert issues one revert request and decodes whichever envelope came back (a transaction
// on success, an RFC 9457 problem on rejection). Goroutine-safe by construction: it reports
// failures as data instead of failing the test from a non-test goroutine.
func fireRevert(app *fiber.App, url string) revertRaceResult {
	req := httptest.NewRequest(nethttp.MethodPost, url, nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		return revertRaceResult{transportErr: err}
	}

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return revertRaceResult{transportErr: err, status: resp.StatusCode}
	}

	out := revertRaceResult{
		status:   resp.StatusCode,
		replayed: resp.Header.Get("X-Idempotency-Replayed"),
		body:     string(raw),
	}

	var envelope struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}

	if err := json.Unmarshal(raw, &envelope); err != nil {
		return out
	}

	out.txID = envelope.ID
	out.problemCode = envelope.Code

	return out
}

func TestIntegration_TransactionV2Revert_ConcurrentSingleWinner(t *testing.T) {
	// NOT parallel: process-global huma state (see file header). The requests inside race; the
	// test itself does not run alongside other tests.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// BOTH sides start funded, and that is load-bearing. A reverse DEBITS the destination, so
	// an empty destination (seedTransfer's default) can only afford ONE reversal: the atomic
	// balance commit would then reject every duplicate with 0018/insufficient-funds and the
	// "exactly one" assertion below would hold even with the idempotency claim removed — the
	// test would be measuring the destination's balance, not the claim. Funding the destination
	// with headroom makes a SECOND reversal affordable, so if the claim ever stops serializing,
	// the duplicate reverse really does commit and this test fails (verified by temporarily
	// forcing a per-call-unique idempotency preimage on the revert path).
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	originResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(originResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originID), "the origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available after the origin transfer")
	requireDecimalEqual(t, decimal.NewFromInt(1100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination available after the origin transfer")

	// Race: every goroutine is parked on the same closed-channel barrier so they enter the
	// revert core together rather than in the order they were spawned.
	revertURL := v2RevertURL(infra.orgID, infra.ledgerID, originID)
	results := make([]revertRaceResult, concurrentRevertRacers)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := range results {
		wg.Add(1)

		go func(slot int) {
			defer wg.Done()

			<-start

			results[slot] = fireRevert(v2App, revertURL)
		}(i)
	}

	close(start)
	wg.Wait()

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// The create core has THREE terminal outcomes here, not two, so status alone cannot
	// classify a racer:
	//
	//   winner  — 201 + X-Idempotency-Replayed: false. Created the one reverse, moved money.
	//   replay  — 201 + X-Idempotency-Replayed: true. Lost the claim, then read the winner's
	//             persisted reverse from PostgreSQL primary and echoed it. Creates nothing, moves
	//             nothing, and MUST NOT be counted against the exactly-one-create invariant.
	//   loser   — 4xx while the claim is active, or 503/0505 after Redis has
	//             committed but before PostgreSQL persistence is visible.
	//
	// Which of the two losing shapes a racer gets is a pure timing coin-flip against the
	// winner's async cache write, so folding replays into winners would make the
	// single-winner assertion flaky AND invert its message: it would report "reverted more
	// than once" for a request that reverted nothing.
	var (
		winners    []revertRaceResult
		replays    []revertRaceResult
		loserCodes []string
	)

	// Losing reverts may only be rejected for LOSING THE CLAIM. 0018/insufficient-funds in
	// particular must never appear: that would mean the destination ran out of headroom and
	// the balance engine — not the idempotency claim — is what kept the count at one, which is
	// exactly the accidental backstop seedFundedTransfer above exists to remove.
	allowedLoserCodes := []string{
		cn.ErrIdempotencyKey.Error(),
		cn.ErrTransactionIDHasAlreadyParentTransaction.Error(),
		cn.ErrRevertReconciliationRequired.Error(),
	}

	for i, res := range results {
		require.NoErrorf(t, res.transportErr, "racer %d failed at the transport layer", i)

		if res.status == nethttp.StatusCreated {
			if res.replayed == "true" {
				replays = append(replays, res)
				continue
			}

			winners = append(winners, res)

			continue
		}

		assert.GreaterOrEqualf(t, res.status, 400, "racer %d: a losing revert must be fenced; body: %s", i, res.body)
		if res.problemCode == cn.ErrRevertReconciliationRequired.Error() {
			assert.Equalf(t, nethttp.StatusServiceUnavailable, res.status,
				"racer %d: post-movement reconciliation must surface as 503; body: %s", i, res.body)
		} else {
			assert.Lessf(t, res.status, 500,
				"racer %d: an active pre-movement claim is a client conflict; body: %s", i, res.body)
		}
		assert.Containsf(t, allowedLoserCodes, res.problemCode,
			"racer %d: a losing revert may only be rejected for losing the idempotency claim (%v); body: %s",
			i, allowedLoserCodes, res.body)

		loserCodes = append(loserCodes, res.problemCode)
	}

	t.Logf("concurrent revert of one origin: %d/%d created, %d replayed, loser problem codes %v",
		len(winners), concurrentRevertRacers, len(replays), loserCodes)

	require.Lenf(t, winners, 1,
		"exactly ONE concurrent revert of a single origin may CREATE a reverse; %d did, which means the shared idempotency claim did NOT serialize the read-then-act race and the origin was reverted more than once",
		len(winners))

	assert.Equal(t, "false", winners[0].replayed,
		"the winning revert is a fresh create, so it must advertise X-Idempotency-Replayed: false explicitly — an absent header is indistinguishable from a transport that forgot to project the flag")

	// One reverse persisted, linked to the origin — and the origin itself never acquired a parent.
	require.NotEmpty(t, winners[0].txID, "the winning revert must return the reverse transaction id")
	reverseID := uuid.MustParse(winners[0].txID)

	// A replay must hand back the WINNER's reverse. Nothing else in the suite proves this: if
	// the slot ever answered with some other cached value, the replaying caller would receive a
	// 201 naming a transaction that is not the reversal of their origin.
	for i, res := range replays {
		assert.Equalf(t, winners[0].txID, res.txID,
			"replay %d must echo the winning reverse (%s), not any other cached transaction; body: %s",
			i, winners[0].txID, res.body)
	}

	reverseParent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, reverseID)
	require.NotNil(t, reverseParent, "the persisted reverse must carry a parent")
	assert.Equal(t, originID, *reverseParent, "the persisted reverse must point at the raced origin")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, originID), "the origin must not acquire a parent")

	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"the ledger must hold exactly the origin and ONE reverse; a third transaction is a double revert")

	// Balances reverted exactly ONCE: back to the seeded state, not past it. Both sides have the
	// headroom to absorb a second reversal, so these two numbers are the money-path proof — a
	// double revert reads 1100 / 900.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available restored exactly once (a double revert would read 1100)")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination drained back exactly once (a double revert would read 900)")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold after the raced revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold after the raced revert")
}

func TestIntegration_TransactionV2Revert_ConcurrentSingleWinnerWithFundsForOnlyOne(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	requireDecimalEqual(t, decimal.NewFromInt(100),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID),
		"destination must have funds for exactly one reversal")

	results := make([]revertRaceResult, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			results[slot] = fireRevert(v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID))
		}(i)
	}
	close(start)
	wg.Wait()

	freshIDs := make(map[string]struct{})
	for i, result := range results {
		require.NoErrorf(t, result.transportErr, "racer %d transport", i)
		assert.NotEqualf(t, cn.ErrInsufficientFunds.Error(), result.problemCode,
			"racer %d reached the balance engine instead of losing the origin claim: %s", i, result.body)
		if result.status == nethttp.StatusCreated && result.replayed != "true" {
			freshIDs[result.txID] = struct{}{}
		}
	}
	require.Len(t, freshIDs, 1, "exactly one concurrent request may create the reverse")

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	requireDecimalEqual(t, decimal.NewFromInt(1000),
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source restored once")
	requireDecimalEqual(t, decimal.Zero,
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination debited once")
}

func TestIntegration_TransactionV2Revert_ConcurrentReplayRecoveryNeverDowngradesCompletedClaim(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	_, _ = seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	origin := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(origin["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	reverse := decodeTxResponse(t, postTransaction(t, v2App,
		v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	reverseID := reverse["id"].(string)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	claim, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, revertclaim.StateCompleted, claim.State)
	require.NoError(t, infra.redisRepo.Del(ctx, originRevertIdempotencyKey(claim)))

	results := make([]revertRaceResult, concurrentRevertRacers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			results[slot] = fireRevert(v2App, v2RevertURL(infra.orgID, infra.ledgerID, originID))
		}(i)
	}
	close(start)
	wg.Wait()

	for i, result := range results {
		require.NoErrorf(t, result.transportErr, "replay racer %d failed at the transport layer", i)
		assert.Equalf(t, nethttp.StatusCreated, result.status, "replay racer %d returned %s", i, result.body)
		assert.Equal(t, "true", result.replayed)
		assert.Equal(t, reverseID, result.txID)
	}
	completed, err := infra.handler.Command.GetRevertClaim(ctx, infra.orgID, infra.ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, revertclaim.StateCompleted, completed.State)
	assert.Nil(t, completed.FailureReason)
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
}
