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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	pgtest "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type mountedContextLifecycle struct {
	requests    chan tracercontract.ReserveRequest
	completions chan tracercontract.TransactionCompletionResult
	journal     *tracerobligation.Repository
	recovery    *command.TracerRecoveryProcessor
	instant     time.Time
}

// Uses the mounted Ledger, actual fee engine, databases and accounting engine.
// The HTTP Tracer peer records contract traffic; it does not prove Tracer capacity.
func attachContextLifecycle(t *testing.T, h *feeHarness) *mountedContextLifecycle {
	t.Helper()
	h.enableAccountingEngine(t)
	h.seedEnforceClosedTracer(t)
	_, err := h.db.ExecContext(t.Context(), `UPDATE ledger SET settings='{"tracer":{"mode":"enforce","failPosture":"closed","validationMode":"rules-and-limits","timeoutMs":5000}}'::jsonb WHERE id=$1`, h.ledgerID)
	require.NoError(t, err)
	pgtest.CreateTestAsset(t, h.db, h.orgID, h.ledgerID, "USD")
	fixture := &mountedContextLifecycle{requests: make(chan tracercontract.ReserveRequest, 8), completions: make(chan tracercontract.TransactionCompletionResult, 8), instant: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	bounds := tracercontract.DefaultResourceProfile().Facts
	cfg := tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 1048576}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/v1/reservations" {
			raw, err := io.ReadAll(io.LimitReader(r.Body, int64(cfg.MaxBodyBytes)+1))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			request, err := tracercontract.DecodeReserveJSON(r.Context(), raw, cfg.MaxBodyBytes, bounds)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fixture.requests <- request
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: uuid.NewSHA1(request.TransactionID, []byte("evaluation")), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonRuleAllow}, ReservationIDs: []uuid.UUID{uuid.NewSHA1(request.TransactionID, []byte("reservation"))}})
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/reservations/transaction/"), "/")
		if len(parts) != 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		id, err := uuid.Parse(parts[0])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		status := string(tracerreservation.Confirmed)
		if parts[1] == "release" {
			status = string(tracerreservation.Released)
		} else if parts[1] != "confirm" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		evaluation := uuid.NewSHA1(id, []byte("evaluation"))
		result := tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: id, Status: status, EvaluationID: &evaluation}
		fixture.completions <- result
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(peer.Close)
	facts, err := tracercontext.NewRepository(h.pgConn, bounds, false)
	require.NoError(t, err)
	loader, err := tracer.NewOfficialContextLoader(facts, "origin-a", bounds)
	require.NoError(t, err)
	fixture.journal, err = tracerobligation.NewRepository(h.pgConn, cfg, false, 10)
	require.NoError(t, err)
	client, err := tracer.NewContextHTTPClient(peer.URL, tracer.ContextClientConfig{Namespace: "origin-a", Bounds: bounds, MaxBodyBytes: cfg.MaxBodyBytes, MaxReservations: 100}, tracer.WithOperationTimeout(5*time.Second))
	require.NoError(t, err)
	fixture.recovery, err = command.NewTracerRecoveryProcessor(fixture.journal, client, fixture.journal, command.TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", SingleTenant: true, MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: 5 * time.Second}, func() time.Time { return fixture.instant })
	require.NoError(t, err)
	h.handler.Command.ContextTracer, err = command.NewContextTracerCoordinator(fixture.recovery, loader, command.ContextTracerConfig{Facts: cfg, MaxReservations: 100, AdmissionTimeout: 5 * time.Second})
	require.NoError(t, err)
	h.handler.Command.TracerReserver = &forbiddenReserver{t: t}
	return fixture
}

func contextGrossDebit(request tracercontract.ReserveRequest, account uuid.UUID) decimal.Decimal {
	total := decimal.Zero
	for _, entry := range request.Context.Entries {
		if entry.AccountID == account && entry.Direction == tracercontract.Debit {
			total = total.Add(decimal.RequireFromString(string(entry.Amount)))
		}
	}
	return total
}

func contextAccountID(t *testing.T, h *feeHarness, alias string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT id FROM account WHERE organization_id=$1 AND ledger_id=$2 AND alias=$3`, h.orgID, h.ledgerID, alias).Scan(&id))
	return id
}

func TestIntegrationContextTracerFeesAndRevert(t *testing.T) {
	h := setupFeeHarness(t)
	fixture := attachContextLifecycle(t, h)
	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
	h.seedPackage(t, packageSpec{label: "shared-fees", fees: []feeSpec{flatFee("fee", "@fee_rev", "0.125", false)}})
	app := h.newV2App()
	result := h.createV2Direct(t, app, h.v2Body("shared fees", "USD", "10", []string{h.v2Leg("@payer", "10")}, []string{h.v2Leg("@receiver", "10")}), nil)
	require.Equal(t, http.StatusCreated, result.status, string(result.rawBody))
	require.Len(t, fixture.requests, 1)
	forward := <-fixture.requests
	require.True(t, contextGrossDebit(forward, contextAccountID(t, h, "@payer")).Equal(decimal.RequireFromString("10.125")))
	assertLiveBalance(t, h, "@payer", "default", "89.875")
	assertLiveBalance(t, h, "@receiver", "default", "10")
	assertLiveBalance(t, h, "@fee_rev", "default", "0.125")
	reversed := h.post(t, app, h.v2StatePath(mustTxID(t, result), "revert"), "", nil)
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, reversed.status, string(reversed.rawBody))
	require.Len(t, fixture.requests, 1, "revert must undergo a new admission")
	reverse := <-fixture.requests
	require.NotEqual(t, forward.TransactionID, reverse.TransactionID)
	require.True(t, contextGrossDebit(reverse, contextAccountID(t, h, "@receiver")).Equal(decimal.NewFromInt(10)))
	require.True(t, contextGrossDebit(reverse, contextAccountID(t, h, "@fee_rev")).Equal(decimal.RequireFromString("0.125")))
	require.True(t, contextGrossDebit(reverse, contextAccountID(t, h, "@payer")).IsZero())
	assertLiveBalance(t, h, "@payer", "default", "100")
	assertLiveBalance(t, h, "@receiver", "default", "0")
	assertLiveBalance(t, h, "@fee_rev", "default", "0")
	summary, err := fixture.recovery.RunOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, summary.Delivered)
	require.Len(t, fixture.completions, 2)
}

func TestIntegrationContextTracerPendingLifecycle(t *testing.T) {
	for _, action := range []string{"commit", "cancel"} {
		t.Run(action, func(t *testing.T) {
			h := setupFeeHarness(t)
			fixture := attachContextLifecycle(t, h)
			h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100), "deposit")
			h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
			app := h.newV2App()
			result := h.createV2Hold(t, app, h.v2Body("shared pending", "USD", "10.125", []string{h.v2Leg("@payer", "10.125")}, []string{h.v2Leg("@receiver", "10.125")}), nil)
			require.Equal(t, http.StatusCreated, result.status, string(result.rawBody))
			id := mustTxID(t, result)
			require.Equal(t, constant.PENDING, dbTxStatus(t, h.db, id))
			require.Len(t, fixture.requests, 1)
			request := <-fixture.requests
			require.NotNil(t, request.LongLived)
			require.True(t, *request.LongLived)
			record, err := fixture.journal.Find(t.Context(), tracerreservation.Key{OrganizationID: h.orgID, LedgerID: h.ledgerID, TransactionID: id})
			require.NoError(t, err)
			require.Equal(t, tracerreservation.Executing, record.State)
			fixture.instant = record.PrepareDeadline.Add(time.Second)
			summary, err := fixture.recovery.RunOnce(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, summary.Unresolved)
			require.Empty(t, fixture.completions, "PENDING must not consume or release the reservation")
			completed := h.post(t, app, h.v2StatePath(id, action), "", nil)
			require.Contains(t, []int{http.StatusOK, http.StatusCreated}, completed.status, string(completed.rawBody))
			fixture.instant = fixture.instant.Add(time.Second)
			summary, err = fixture.recovery.RunOnce(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, summary.Delivered)
			require.Empty(t, fixture.requests, "completion must not reserve again")
			require.Len(t, fixture.completions, 1)
			outcome := <-fixture.completions
			require.Equal(t, id, outcome.TransactionID)
			if action == "commit" {
				require.Equal(t, string(tracerreservation.Confirmed), outcome.Status)
				assertLiveBalance(t, h, "@payer", "default", "89.875")
				assertLiveBalance(t, h, "@receiver", "default", "10.125")
			} else {
				require.Equal(t, string(tracerreservation.Released), outcome.Status)
				assertLiveBalance(t, h, "@payer", "default", "100")
				assertLiveBalance(t, h, "@receiver", "default", "0")
			}
		})
	}
}
