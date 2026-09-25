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
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtest "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// The mounted Ledger route, official facts, journal and accounting engine are
// real. The HTTP peer is a contract fixture, not a deployed Tracer or mTLS proof.
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
	expectedState := tracerreservation.Confirmed
	completionPath := "/confirm"
	reason := tracercontract.ReasonRuleAllow
	if !allowed {
		expectedState = tracerreservation.Released
		completionPath = "/release"
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
		if !strings.HasSuffix(path, completionPath) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		transactionID, err := uuid.Parse(strings.TrimSuffix(path, completionPath))
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
		_ = json.NewEncoder(w).Encode(tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: string(expectedState), EvaluationID: &evaluationID})
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
	recoveryConfig := command.TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", SingleTenant: true, MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: 5 * time.Second}
	recovery, err := command.NewTracerRecoveryProcessor(journal, client, journal, recoveryConfig, func() time.Time { return instant })
	require.NoError(t, err)
	coordinator, err := command.NewContextTracerCoordinator(recovery, loader, command.ContextTracerConfig{Facts: config, MaxReservations: 100, AdmissionTimeout: 5 * time.Second})
	require.NoError(t, err)
	h.handler.Command.ContextTracer = coordinator
	h.handler.Command.TracerReserver = &forbiddenReserver{t: t}
	response := h.createV2Direct(t, h.newV2App(), h.v2Body("context integration", "USD", "10.125", []string{h.v2Leg("@payer", "10.125")}, []string{h.v2Leg("@receiver", "10.125")}), nil)
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
			code = "0526"
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
	require.Equal(t, tracercontract.AssetRef{Namespace: "origin-a", ID: assetID.String(), Code: "USD"}, request.Asset)
	require.Len(t, request.Context.Accounts, 2)
	require.Len(t, request.Context.Entries, 2)
	require.Equal(t, "10.125", string(request.Amount))
	key := tracerreservation.Key{OrganizationID: h.orgID, LedgerID: h.ledgerID, TransactionID: transactionID}
	record, err := journal.Find(t.Context(), key)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, expectedState, record.State)
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
	// Reconstruct every recovery collaborator and turn off new admission. The
	// stored terminal obligation must suffice without the original request,
	// facts loader, coordinator or today's ledger settings.
	_, err = h.db.ExecContext(t.Context(), `UPDATE ledger SET settings=jsonb_set(settings,'{tracer,mode}','"off"'::jsonb) WHERE id=$1`, h.ledgerID)
	require.NoError(t, err)
	journal, err = tracerobligation.NewRepository(h.pgConn, config, false, 10)
	require.NoError(t, err)
	client, err = tracer.NewContextHTTPClient(peer.URL, tracer.ContextClientConfig{Namespace: "origin-a", Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100}, tracer.WithOperationTimeout(5*time.Second))
	require.NoError(t, err)
	recovery, err = command.NewTracerRecoveryProcessor(journal, client, journal, recoveryConfig, func() time.Time { return instant })
	require.NoError(t, err)
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
