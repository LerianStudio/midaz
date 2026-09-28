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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtest "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// tracerCall is one request received by the fake Tracer peer, in arrival order.
type tracerCall struct {
	op            string
	transactionID uuid.UUID
}

const (
	tracerCallReserve = "reserve"
	tracerCallConfirm = "confirm"
	tracerCallRelease = "release"
)

// drainTracerCalls returns every call recorded so far without waiting. The
// completion seams run inline before the HTTP response, so a finished request
// has already recorded all of its calls.
func drainTracerCalls(calls chan tracerCall) []tracerCall {
	recorded := []tracerCall{}
	for {
		select {
		case call := <-calls:
			recorded = append(recorded, call)
		default:
			return recorded
		}
	}
}

type mountedContextLifecycle struct {
	requests chan tracercontract.ReserveRequest
	calls    chan tracerCall
	instant  time.Time
}

// Uses the mounted Ledger, actual fee engine, databases and accounting engine.
// The HTTP Tracer peer records contract traffic; it does not prove Tracer capacity.
func attachContextLifecycle(t *testing.T, h *feeHarness) *mountedContextLifecycle {
	t.Helper()
	h.enableAccountingEngine(t)
	h.queryUC.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}
	h.seedEnforceClosedTracer(t)
	_, err := h.db.ExecContext(t.Context(), `UPDATE ledger SET settings='{"tracer":{"mode":"enforce","failPosture":"closed","validationMode":"rules-and-limits","timeoutMs":5000}}'::jsonb WHERE id=$1`, h.ledgerID)
	require.NoError(t, err)
	pgtest.CreateTestAsset(t, h.db, h.orgID, h.ledgerID, "USD")
	fixture := &mountedContextLifecycle{requests: make(chan tracercontract.ReserveRequest, 8), calls: make(chan tracerCall, 16), instant: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	bounds := tracercontract.DefaultResourceProfile().Facts
	const maxBodyBytes = 1048576
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/v1/reservations" {
			raw, err := io.ReadAll(io.LimitReader(r.Body, int64(maxBodyBytes)+1))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			request, err := tracercontract.DecodeReserveJSON(r.Context(), raw, maxBodyBytes, bounds)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fixture.requests <- request
			fixture.calls <- tracerCall{op: tracerCallReserve, transactionID: request.TransactionID}
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
		status := "CONFIRMED"
		if parts[1] == tracerCallRelease {
			status = "RELEASED"
		} else if parts[1] != tracerCallConfirm {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		evaluation := uuid.NewSHA1(id, []byte("evaluation"))
		result := tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: id, Status: status, EvaluationID: &evaluation}
		fixture.calls <- tracerCall{op: parts[1], transactionID: id}
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(peer.Close)
	facts, err := tracercontext.NewRepository(h.pgConn, bounds, false)
	require.NoError(t, err)
	loader, err := tracer.NewOfficialContextLoader(facts, bounds)
	require.NoError(t, err)
	client, err := tracer.NewContextHTTPClient(peer.URL, tracer.ContextClientConfig{Bounds: bounds, MaxBodyBytes: maxBodyBytes, MaxReservations: 100}, tracer.WithOperationTimeout(5*time.Second))
	require.NoError(t, err)
	h.handler.Command.ContextTracer, err = command.NewContextTracerCoordinator(client, loader, command.ContextTracerConfig{Bounds: bounds, MaxReservations: 100, AdmissionTimeout: 5 * time.Second}, func() time.Time { return fixture.instant })
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
	require.Equal(t, []tracerCall{
		{op: tracerCallReserve, transactionID: forward.TransactionID},
		{op: tracerCallConfirm, transactionID: forward.TransactionID},
		{op: tracerCallReserve, transactionID: reverse.TransactionID},
		{op: tracerCallConfirm, transactionID: reverse.TransactionID},
	}, drainTracerCalls(fixture.calls),
		"the original reservation stays consumed and the revert consumes its own instead of refunding the original")
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
			require.Equal(t, id, request.TransactionID)
			require.Equal(t, []tracerCall{{op: tracerCallReserve, transactionID: id}}, drainTracerCalls(fixture.calls),
				"PENDING must not consume or release the reservation")
			completed := h.post(t, app, h.v2StatePath(id, action), "", nil)
			require.Contains(t, []int{http.StatusOK, http.StatusCreated}, completed.status, string(completed.rawBody))
			require.Empty(t, fixture.requests, "completion must not reserve again")
			expected := tracerCallConfirm
			if action == "cancel" {
				expected = tracerCallRelease
			}
			require.Equal(t, []tracerCall{{op: expected, transactionID: id}}, drainTracerCalls(fixture.calls))
			if action == "commit" {
				assertLiveBalance(t, h, "@payer", "default", "89.875")
				assertLiveBalance(t, h, "@receiver", "default", "10.125")
			} else {
				assertLiveBalance(t, h, "@payer", "default", "100")
				assertLiveBalance(t, h, "@receiver", "default", "0")
			}
		})
	}
}

func TestIntegrationContextTracerEngineRejectionReleases(t *testing.T) {
	h := setupFeeHarness(t)
	fixture := attachContextLifecycle(t, h)
	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(5), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	result := h.createV2Direct(t, h.newV2App(), h.v2Body("insufficient", "USD", "10", []string{h.v2Leg("@payer", "10")}, []string{h.v2Leg("@receiver", "10")}), nil)
	require.Equal(t, http.StatusUnprocessableEntity, result.status, string(result.rawBody))
	require.Len(t, fixture.requests, 1)
	request := <-fixture.requests
	require.Equal(t, []tracerCall{
		{op: tracerCallReserve, transactionID: request.TransactionID},
		{op: tracerCallRelease, transactionID: request.TransactionID},
	}, drainTracerCalls(fixture.calls), "an accounting rejection releases the admitted reservation")
	assertLiveBalance(t, h, "@payer", "default", "5")
}

func TestIntegrationContextTracerNotParticipating(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		settings string
		skip     bool
	}{
		{name: "mode off", settings: `{"tracer":{"mode":"off"}}`},
		{name: "honored skip", settings: `{"tracer":{"mode":"enforce","failPosture":"closed","validationMode":"rules-and-limits","timeoutMs":5000},"overrides":{"allowTracerSkip":true}}`, skip: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			h := setupFeeHarness(t)
			fixture := attachContextLifecycle(t, h)
			_, err := h.db.ExecContext(t.Context(), `UPDATE ledger SET settings=$1::jsonb WHERE id=$2`, scenario.settings, h.ledgerID)
			require.NoError(t, err)
			require.NoError(t, h.redisRepo.Del(h.ctx(), utils.LedgerSettingsInternalKey(h.orgID, h.ledgerID)))
			h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100), "deposit")
			h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
			app := h.newV2App()
			body := h.v2Body("not participating", "USD", "10", []string{h.v2Leg("@payer", "10")}, []string{h.v2Leg("@receiver", "10")})
			if scenario.skip {
				body = strings.TrimSuffix(body, "}") + `,"skip":{"tracer":true}}`
			}
			direct := h.createV2Direct(t, app, body, nil)
			require.Equal(t, http.StatusCreated, direct.status, string(direct.rawBody))
			held := h.createV2Hold(t, app, body, nil)
			require.Equal(t, http.StatusCreated, held.status, string(held.rawBody))
			committed := h.post(t, app, h.v2StatePath(mustTxID(t, held), "commit"), "", nil)
			require.Contains(t, []int{http.StatusOK, http.StatusCreated}, committed.status, string(committed.rawBody))
			require.Empty(t, drainTracerCalls(fixture.calls), "a non-participating transaction never dials the Tracer")
			assertLiveBalance(t, h, "@payer", "default", "80")
		})
	}
}
