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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtest "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// The mounted Ledger route, official facts and accounting engine are real. The HTTP peer is a contract fixture, not a deployed Tracer or mTLS proof.
func TestIntegrationContextTracerMountedLedger(t *testing.T) {
	for _, decision := range []tracercontract.Decision{tracercontract.DecisionAllow, tracercontract.DecisionDeny, tracercontract.DecisionReview} {
		t.Run(string(decision), func(t *testing.T) { testMountedContextDecision(t, decision, mmodel.TracerModeEnforce, nil) })
	}
}

func TestIntegrationContextTracerLimitDenial(t *testing.T) {
	testMountedContextDecision(t, tracercontract.DecisionDeny, mmodel.TracerModeEnforce, nil, tracercontract.ReasonLimitExceeded)
}

func TestIntegrationContextTracerUnavailableClosed(t *testing.T) {
	testMountedContextDecision(t, tracercontract.DecisionAllow, mmodel.TracerModeEnforce, tracer.ErrTracerUnavailable)
}

func TestIntegrationContextTracerPolicyFailureNeverPosts(t *testing.T) {
	for _, mode := range []string{mmodel.TracerModeEnforce, mmodel.TracerModeAdvisory} {
		for _, cause := range []error{constant.ErrContextPolicyUnavailable, constant.ErrExpressionCostExceeded} {
			t.Run(mode+"/"+cause.Error(), func(t *testing.T) { testMountedContextDecision(t, tracercontract.DecisionAllow, mode, cause) })
		}
	}
}

func testMountedContextDecision(t *testing.T, decision tracercontract.Decision, mode string, peerError error, reasonOverride ...tracercontract.ReserveReason) {
	t.Helper()
	allowed := decision == tracercontract.DecisionAllow && peerError == nil
	expectedStatus, completionOp := "CONFIRMED", tracerCallConfirm
	reason := tracercontract.ReasonRuleAllow
	if !allowed {
		expectedStatus, completionOp = "RELEASED", tracerCallRelease
		reason = tracercontract.ReasonRuleDeny
		if decision == tracercontract.DecisionReview {
			reason = tracercontract.ReasonRuleReview
		}
	}
	if len(reasonOverride) > 0 {
		reason = reasonOverride[0]
	}
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	h.seedEnforceClosedTracer(t)
	_, err := h.db.Exec(`UPDATE ledger SET settings='{"tracer":{"mode":"enforce","failPosture":"closed","validationMode":"rules-and-limits","timeoutMs":5000}}'::jsonb WHERE id=$1`, h.ledgerID)
	require.NoError(t, err)
	if peerError != nil {
		posture := mmodel.TracerFailPostureOpen
		if peerError == tracer.ErrTracerUnavailable {
			posture = mmodel.TracerFailPostureClosed
		}
		settings, err := json.Marshal(mmodel.LedgerSettings{Tracer: mmodel.TracerSettings{Mode: mode, FailPosture: posture, ValidationMode: string(tracercontract.ValidationRulesAndLimits), TimeoutMs: 5000}})
		require.NoError(t, err)
		_, err = h.db.ExecContext(t.Context(), `UPDATE ledger SET settings=$1::jsonb WHERE id=$2`, string(settings), h.ledgerID)
		require.NoError(t, err)
	}
	pgtest.CreateTestAsset(t, h.db, h.orgID, h.ledgerID, "BTC")
	h.seedBalance(t, "@payer", "BTC", decimal.NewFromInt(100), "deposit")
	h.seedBalance(t, "@receiver", "BTC", decimal.Zero, "deposit")
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	const maxBodyBytes = 65536
	instant := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	received := make(chan tracercontract.ReserveRequest, 1)
	calls := make(chan tracerCall, 4)
	evaluationID := uuid.MustParse("9db0acb6-b304-43a7-af12-7a46e1c5667d")
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
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
			select {
			case received <- request:
			default:
				w.WriteHeader(http.StatusConflict)
				return
			}
			calls <- tracerCall{op: tracerCallReserve, transactionID: request.TransactionID}
			if peerError != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": peerError.Error()})
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: evaluationID, Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, Reasons: []tracercontract.ReserveReason{reason}, ReservationIDs: []uuid.UUID{}})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/reservations/transaction/")
		rawID, op, found := strings.Cut(path, "/")
		transactionID, err := uuid.Parse(rawID)
		if !found || err != nil || (op != tracerCallConfirm && op != tracerCallRelease) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calls <- tracerCall{op: op, transactionID: transactionID}
		if op != completionOp {
			w.WriteHeader(http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: expectedStatus, EvaluationID: &evaluationID})
	}))
	t.Cleanup(peer.Close)
	facts, err := tracercontext.NewRepository(h.pgConn, bounds, false)
	require.NoError(t, err)
	loader, err := tracer.NewOfficialContextLoader(facts, bounds)
	require.NoError(t, err)
	client, err := tracer.NewContextHTTPClient(peer.URL, tracer.ContextClientConfig{Bounds: bounds, MaxBodyBytes: maxBodyBytes, MaxReservations: 100}, fixedIntegrationToken{}, tracer.WithOperationTimeout(5*time.Second))
	require.NoError(t, err)
	coordinator, err := command.NewContextTracerCoordinator(client, loader, command.ContextTracerConfig{Bounds: bounds, MaxReservations: 100, AdmissionTimeout: 5 * time.Second}, func() time.Time { return instant })
	require.NoError(t, err)
	h.handler.Command.ContextTracer = coordinator
	response := h.createV2Direct(t, h.newV2App(), h.v2Body("context integration", "BTC", "10.125", []string{h.v2Leg("@payer", "10.125")}, []string{h.v2Leg("@receiver", "10.125")}), nil)
	if allowed {
		require.Equal(t, http.StatusCreated, response.status, string(response.rawBody))
	} else if peerError != nil {
		expectedStatus := http.StatusServiceUnavailable
		if peerError == constant.ErrExpressionCostExceeded {
			expectedStatus = http.StatusUnprocessableEntity
		}
		require.Equal(t, expectedStatus, response.status, string(response.rawBody))
		code := peerError.Error()
		if peerError == tracer.ErrTracerUnavailable {
			code = "0178"
		}
		require.Equal(t, code, response.body["code"])
	} else {
		require.Equal(t, http.StatusUnprocessableEntity, response.status, string(response.rawBody))
		code := "0177"
		if decision == tracercontract.DecisionReview {
			code = "0535"
		}
		require.Equal(t, code, response.body["code"])
	}
	require.Len(t, received, 1)
	request := <-received
	require.Equal(t, instant, request.TransactionTimestamp, "freshness comes from the admission clock")
	transactionID := request.TransactionID
	if allowed {
		require.Equal(t, mustTxID(t, response), transactionID)
	}
	var transactionCount, operationCount int
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT count(*) FROM transaction WHERE id=$1`, transactionID).Scan(&transactionCount))
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT count(*) FROM operation WHERE transaction_id=$1`, transactionID).Scan(&operationCount))
	if allowed {
		require.Equal(t, 1, transactionCount)
		require.Equal(t, 2, operationCount)
	} else {
		require.Zero(t, transactionCount, "rejected admission must not create a PENDING transaction")
		require.Zero(t, operationCount)
	}
	require.Equal(t, "BTC", request.Asset)
	for _, entry := range request.Context.Entries {
		require.Equal(t, "BTC", entry.Asset)
	}
	for _, account := range request.Context.Accounts {
		require.Equal(t, "BTC", account.Asset)
	}
	require.Len(t, request.Context.Accounts, 2)
	require.Len(t, request.Context.Entries, 2)
	require.Equal(t, "10.125", string(request.Amount))
	recorded := drainTracerCalls(calls)
	if peerError == tracer.ErrTracerUnavailable {
		// An admission that failed for availability is completed by the retrier, off the request path.
		require.Eventually(t, func() bool {
			recorded = append(recorded, drainTracerCalls(calls)...)
			return len(recorded) >= 2
		}, 10*time.Second, 20*time.Millisecond)
	}
	if peerError == constant.ErrContextPolicyUnavailable {
		// A missing policy is a refusal before evaluation: the Tracer holds nothing to release.
		require.Equal(t, []tracerCall{{op: tracerCallReserve, transactionID: transactionID}}, recorded, "a refusal before evaluation is not released")
	} else {
		require.Equal(t, []tracerCall{
			{op: tracerCallReserve, transactionID: transactionID},
			{op: completionOp, transactionID: transactionID},
		}, recorded, "the known accounting outcome is delivered by transaction")
	}
	assertBalances := func() {
		t.Helper()
		expectedBalances := map[string]string{"@payer": "89.875", "@receiver": "10.125"}
		if !allowed {
			expectedBalances = map[string]string{"@payer": "100", "@receiver": "0"}
		}
		for alias, expected := range expectedBalances {
			if !allowed {
				require.Equal(t, expected, postgresBalanceTotal(t, h, alias).String())
				exists, err := h.redisContainer.Client.Exists(t.Context(), utils.BalanceInternalKey(h.orgID, h.ledgerID, alias+"#default")).Result()
				require.NoError(t, err)
				if exists == 0 {
					continue
				}
			}
			raw, err := h.redisContainer.Client.Get(t.Context(), utils.BalanceInternalKey(h.orgID, h.ledgerID, alias+"#default")).Bytes()
			require.NoError(t, err)
			balance, err := balancecache.Decode(raw)
			require.NoError(t, err)
			require.Equal(t, expected, balance.Available.String())
			require.True(t, balance.OnHold.IsZero())
		}
	}
	assertBalances()
}
