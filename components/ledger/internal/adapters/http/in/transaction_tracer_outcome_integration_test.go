//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	nethttp "net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type outcomeV2Reserver struct {
	reserveErr error
	requests   atomic.Int64
	last       tracerclient.ReserveRequest
}

var ledgerTracerOutcomeIntegrationFuture = time.Unix(4102444800, 0).UTC()

func (r *outcomeV2Reserver) Reserve(_ context.Context, req tracerclient.ReserveRequest) (*tracerclient.ReserveResult, error) {
	r.requests.Add(1)
	r.last = req
	if r.reserveErr != nil {
		return nil, r.reserveErr
	}
	return &tracerclient.ReserveResult{
		TransactionID: req.TransactionID,
		DeliveryMode:  tracerclient.DeliveryModeLedgerOutcomeV2,
	}, nil
}
func (*outcomeV2Reserver) Confirm(context.Context, uuid.UUID) error              { return nil }
func (*outcomeV2Reserver) Release(context.Context, uuid.UUID) error              { return nil }
func (*outcomeV2Reserver) ConfirmByTransaction(context.Context, uuid.UUID) error { return nil }
func (*outcomeV2Reserver) ReleaseByTransaction(context.Context, uuid.UUID) error { return nil }

func configureOutcomeV2Ledger(t *testing.T, infra *testInfra, reserver *outcomeV2Reserver) {
	t.Helper()
	_, err := infra.pgContainer.DB.Exec(`UPDATE ledger SET settings =
		'{"tracer":{"mode":"enforce","failPosture":"closed","timeoutMs":250}}'::jsonb
		WHERE id = $1 AND organization_id = $2`, infra.ledgerID, infra.orgID)
	require.NoError(t, err)
	infra.handler.TracerReserver = reserver
	infra.handler.TracerOutcomeV2 = true
}

func TestIntegration_LedgerTracerOutcomeV2_DurableAcrossDispatcherDowntime(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	infra := setupTestInfra(t)
	reserver := &outcomeV2Reserver{}
	configureOutcomeV2Ledger(t, infra, reserver)
	sourceID, _ := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	app := buildHumaV2DirectApp(t, infra.handler)

	response := decodeTxResponse(t, postV2Create(t, app, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, "outcome-v2-dispatcher-down"), nethttp.StatusCreated)
	transactionID := uuid.MustParse(response["id"].(string))
	record, err := infra.redisRepo.ReadTracerOutcome(context.Background(), infra.orgID, infra.ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, mmodel.TracerOutcomeCommitted, record.State)
	require.NotNil(t, record.EconomicOutcome)
	assert.Equal(t, tracerclient.DeliveryModeLedgerOutcomeV2, reserver.last.DeliveryMode)
	assert.Equal(t, utils.TransactionTracerOutcomeID(transactionID), record.OutcomeID)
	assert.Equal(t, 1, int(reserver.requests.Load()))
	drainBalanceSync(t, context.Background(), infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.True(t, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceID).Equal(
		decimal.NewFromInt(900)), "money moved while the undelivered outcome remained durable")
	due, err := infra.redisRepo.ListDueTracerOutcomes(context.Background(), ledgerTracerOutcomeIntegrationFuture, 10)
	require.NoError(t, err)
	assert.Contains(t, due, utils.TransactionTracerOutcomeKey(infra.orgID, infra.ledgerID, transactionID))
}

func TestIntegration_LedgerTracerOutcomeV2_LostReserveResponseAbortsWithoutMoney(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	infra := setupTestInfra(t)
	reserver := &outcomeV2Reserver{reserveErr: tracerclient.ErrTracerUnavailable}
	configureOutcomeV2Ledger(t, infra, reserver)
	sourceID, _ := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	app := buildHumaV2DirectApp(t, infra.handler)

	resp := postV2Create(t, app, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, "outcome-v2-reserve-lost")
	defer resp.Body.Close()
	assert.Equal(t, nethttp.StatusServiceUnavailable, resp.StatusCode)
	assert.True(t, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceID).Equal(
		decimal.NewFromInt(1000)), "an ambiguous Reserve response must not move money")
	due, err := infra.redisRepo.ListDueTracerOutcomes(context.Background(), ledgerTracerOutcomeIntegrationFuture, 10)
	require.NoError(t, err)
	require.Len(t, due, 1, "PREPARED must remain scheduled for CAS-abort recovery")
	record, err := infra.redisRepo.ReadTracerOutcomeByKey(context.Background(), due[0])
	require.NoError(t, err)
	assert.Equal(t, mmodel.TracerOutcomePrepared, record.State)
}

func TestIntegration_LedgerTracerOutcomeV2_LostLuaResponseReplaysEconomicFact(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	infra := setupTestInfra(t)
	reserver := &outcomeV2Reserver{}
	configureOutcomeV2Ledger(t, infra, reserver)
	sourceID, _ := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	faultRepo := &lostBalanceResponseRepository{RedisRepository: infra.redisRepo}
	infra.handler.Command.TransactionRedisRepo = faultRepo
	app := buildHumaV2DirectApp(t, infra.handler)

	response := decodeTxResponse(t, postV2Create(t, app, "direct", infra.orgID, infra.ledgerID,
		equivalentV2Body, "outcome-v2-lua-lost"), nethttp.StatusCreated)
	transactionID := uuid.MustParse(response["id"].(string))
	require.Equal(t, int32(1), faultRepo.balanceCalls.Load(), "the HTTP retry path must read the Lua outcome, not execute balances again")
	drainBalanceSync(t, context.Background(), infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
	assert.True(t, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceID).Equal(
		decimal.NewFromInt(900)))
	record, err := infra.redisRepo.ReadTracerOutcome(context.Background(), infra.orgID, infra.ledgerID, transactionID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.TracerOutcomeCommitted, record.State)
}

func TestIntegration_LedgerTracerOutcomeV2_PendingTransitionsWithLifecycleLua(t *testing.T) {
	tests := []struct {
		name          string
		url           func(uuid.UUID, uuid.UUID, uuid.UUID) string
		wantState     string
		wantAvailable decimal.Decimal
	}{
		{name: "commit", url: v2CommitURL, wantState: mmodel.TracerOutcomeCommitted, wantAvailable: decimal.NewFromInt(900)},
		{name: "cancel", url: v2CancelURL, wantState: mmodel.TracerOutcomeAborted, wantAvailable: decimal.NewFromInt(1000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ALLOW_INSECURE_TLS", "true")
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
			infra := setupTestInfra(t)
			reserver := &outcomeV2Reserver{}
			configureOutcomeV2Ledger(t, infra, reserver)
			sourceID, _ := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
			app := buildHumaV2DirectApp(t, infra.handler)

			pending := decodeTxResponse(t, postV2Create(t, app, "hold", infra.orgID, infra.ledgerID,
				holdParityV2Body, "outcome-v2-pending-"+tt.name), nethttp.StatusCreated)
			transactionID := uuid.MustParse(pending["id"].(string))
			record, err := infra.redisRepo.ReadTracerOutcome(context.Background(), infra.orgID, infra.ledgerID, transactionID)
			require.NoError(t, err)
			require.Equal(t, mmodel.TracerOutcomePendingHeld, record.State)
			due, err := infra.redisRepo.ListDueTracerOutcomes(context.Background(), ledgerTracerOutcomeIntegrationFuture, 10)
			require.NoError(t, err)
			assert.NotContains(t, due, utils.TransactionTracerOutcomeKey(infra.orgID, infra.ledgerID, transactionID),
				"PENDING_HELD must never enter stale-PREPARED recovery")

			_ = decodeTxResponse(t, postTransaction(t, app, tt.url(infra.orgID, infra.ledgerID, transactionID), "", ""),
				nethttp.StatusCreated)
			record, err = infra.redisRepo.ReadTracerOutcome(context.Background(), infra.orgID, infra.ledgerID, transactionID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, record.State)
			due, err = infra.redisRepo.ListDueTracerOutcomes(context.Background(), ledgerTracerOutcomeIntegrationFuture, 10)
			require.NoError(t, err)
			assert.Contains(t, due, utils.TransactionTracerOutcomeKey(infra.orgID, infra.ledgerID, transactionID))
			drainBalanceSync(t, context.Background(), infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
			assert.True(t, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceID).Equal(tt.wantAvailable))
		})
	}
}
