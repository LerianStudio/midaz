// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracercontext"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracerobligation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtest "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// The mounted Ledger route, official facts, journal and accounting engine are
// real. The HTTP peer is a contract fixture, not a deployed Tracer or mTLS proof.
func TestIntegrationContextTracerMountedLedger(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	h.seedEnforceClosedTracer(t)
	_, err := h.db.Exec(`UPDATE ledger SET settings='{"tracer":{"mode":"enforce","failPosture":"closed","validationMode":"rules-and-limits","timeoutMs":5000}}'::jsonb WHERE id=$1`, h.ledgerID)
	require.NoError(t, err)
	assetID := pgtest.CreateTestAsset(t, h.db, h.orgID, h.ledgerID, "USD")
	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	config := tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 65536}
	received := make(chan tracercontract.ReserveRequest, 1)
	completed := make(chan uuid.UUID, 1)
	evaluationID := uuid.MustParse("9db0acb6-b304-43a7-af12-7a46e1c5667d")
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/reservations" {
			raw, err := io.ReadAll(io.LimitReader(r.Body, int64(config.MaxBodyBytes)+1))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			request, err := tracercontract.DecodeReserveJSON(r.Context(), raw, config.MaxBodyBytes, bounds)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			select {
			case received <- request:
			default:
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: evaluationID, Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonRuleAllow}, ReservationIDs: []uuid.UUID{}})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/reservations/transaction/")
		if !strings.HasSuffix(path, "/confirm") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		transactionID, err := uuid.Parse(strings.TrimSuffix(path, "/confirm"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case completed <- transactionID:
		default:
			w.WriteHeader(http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: "CONFIRMED", EvaluationID: &evaluationID})
	}))
	t.Cleanup(peer.Close)
	facts, err := tracercontext.NewRepository(h.pgConn, bounds, false)
	require.NoError(t, err)
	loader, err := tracer.NewOfficialContextLoader(facts, "origin-a", bounds)
	require.NoError(t, err)
	journal, err := tracerobligation.NewRepository(h.pgConn, config, false, 10)
	require.NoError(t, err)
	client, err := tracer.NewContextHTTPClient(peer.URL, tracer.ContextClientConfig{Namespace: "origin-a", Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100}, tracer.WithOperationTimeout(5*time.Second))
	require.NoError(t, err)
	instant := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	recovery, err := command.NewTracerRecoveryProcessor(journal, client, journal, command.TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", SingleTenant: true, MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: 5 * time.Second}, func() time.Time { return instant })
	require.NoError(t, err)
	coordinator, err := command.NewContextTracerCoordinator(recovery, loader, command.ContextTracerConfig{Facts: config, MaxReservations: 100})
	require.NoError(t, err)
	h.handler.Command.ContextTracer = coordinator
	h.handler.Command.TracerReserver = &forbiddenReserver{t: t}
	response := h.createV2Direct(t, h.newV2App(), h.v2Body("context integration", "USD", "10.125", []string{h.v2Leg("@payer", "10.125")}, []string{h.v2Leg("@receiver", "10.125")}), nil)
	require.Equal(t, http.StatusCreated, response.status, string(response.rawBody))
	transactionID := mustTxID(t, response)
	require.Len(t, received, 1)
	request := <-received
	require.Equal(t, transactionID, request.TransactionID)
	require.Equal(t, tracercontract.AssetRef{Namespace: "origin-a", ID: assetID.String(), Code: "USD"}, request.Asset)
	require.Len(t, request.Context.Accounts, 2)
	require.Len(t, request.Context.Entries, 2)
	require.Equal(t, "10.125", string(request.Amount))
	key := tracerreservation.Key{OrganizationID: h.orgID, LedgerID: h.ledgerID, TransactionID: transactionID}
	record, err := journal.Find(t.Context(), key)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, tracerreservation.Confirmed, record.State)
	assertBalances := func() {
		t.Helper()
		for alias, expected := range map[string]string{"@payer": "89.875", "@receiver": "10.125"} {
			raw, err := h.redisContainer.Client.Get(t.Context(), utils.BalanceInternalKey(h.orgID, h.ledgerID, alias+"#default")).Bytes()
			require.NoError(t, err)
			balance, err := balancecache.Decode(raw)
			require.NoError(t, err)
			require.Equal(t, expected, balance.Available.String())
			require.True(t, balance.OnHold.IsZero())
		}
	}
	assertBalances()
	instant = instant.Add(time.Second)
	summary, err := recovery.RunOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, summary.Delivered)
	require.Len(t, completed, 1)
	require.Equal(t, transactionID, <-completed)
	summary, err = recovery.RunOnce(t.Context())
	require.NoError(t, err)
	require.Zero(t, summary.Claimed)
	assertBalances()
}
