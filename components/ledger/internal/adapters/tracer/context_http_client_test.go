// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextHTTPClientReserve(t *testing.T) {
	for _, scenario := range []string{"allow", "deny", "review", "wrong transaction", "missing controls", "unavailable", "duplicate", "invalid request"} {
		t.Run(scenario, func(t *testing.T) {
			request, config := contextClientFixture(t)
			result := contextResultFixture(request)
			if scenario == "deny" {
				result.Decision = tracercontract.DecisionDeny
				result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}
			}
			if scenario == "review" {
				result.Decision = tracercontract.DecisionReview
				result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonRuleReview}
			}
			received := make(chan tracercontract.ReserveRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/reservations" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				actual, err := tracercontract.DecodeReserveJSON(r.Context(), raw, config.MaxBodyBytes, config.Bounds)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				received <- actual
				if scenario == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				response := *result
				if scenario == "wrong transaction" {
					response.TransactionID = result.EvaluationID
				}
				if scenario == "missing controls" {
					response.Controls.Rules = tracercontract.RulesNotRequested
				}
				data, _ := json.Marshal(response)
				if scenario == "duplicate" {
					data = append(data[:len(data)-1], []byte(`,"decision":"DENY"}`)...)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write(data)
			}))
			t.Cleanup(server.Close)
			client, err := NewContextHTTPClient(server.URL, config)
			require.NoError(t, err)
			if scenario == "invalid request" {
				request.LongLived = nil
			}
			actual, err := client.Reserve(t.Context(), request)
			switch scenario {
			case "allow", "deny", "review":
				require.NoError(t, err)
				require.Equal(t, result, actual)
			default:
				require.Error(t, err)
				require.Nil(t, actual)
			}
			if scenario == "unavailable" {
				require.ErrorIs(t, err, ErrTracerUnavailable)
			}
			if scenario == "oversized" {
				require.ErrorIs(t, err, ErrTracerUnavailable)
			}
			if scenario == "invalid request" {
				require.Empty(t, received)
			} else {
				require.Equal(t, request, <-received)
			}
		})
	}
}

func TestContextHTTPResponseErrorClassifiesAvailability(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		unavailable bool
		cause       error
	}{
		{name: "internal", status: http.StatusInternalServerError, unavailable: true},
		{name: "bad gateway", status: http.StatusBadGateway, unavailable: true},
		{name: "request timeout", status: http.StatusRequestTimeout, unavailable: true},
		{name: "saturated canonical response", status: http.StatusTooManyRequests, body: `{"code":"0518"}`, unavailable: true},
		{name: "missing policy", status: http.StatusServiceUnavailable, body: `{"code":"0518"}`, cause: constant.ErrContextPolicyUnavailable},
		{name: "invalid request", status: http.StatusBadRequest, body: `{"code":"0094"}`, cause: constant.ErrInvalidRequestBody},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := contextHTTPResponseError(test.status, []byte(test.body))
			require.Equal(t, test.unavailable, errors.Is(err, ErrTracerUnavailable))
			if test.cause != nil {
				require.ErrorIs(t, err, test.cause)
			}
		})
	}
}

func TestContextHTTPClientCompletion(t *testing.T) {
	for _, scenario := range []string{"confirm", "release", "before admission", "legacy reply", "wrong status", "wrong transaction", "oversized", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			request, config := contextClientFixture(t)
			evaluation := contextResultFixture(request).EvaluationID
			expectedStatus, action := "CONFIRMED", "confirm"
			if scenario == "release" {
				expectedStatus, action = "RELEASED", "release"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/reservations/transaction/"+request.TransactionID.String()+"/"+action {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if _, err := tracercontract.DecodeCompletionJSON(r.Context(), raw, config.MaxBodyBytes); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if scenario == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				result := tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: request.TransactionID, Status: expectedStatus, EvaluationID: &evaluation}
				if scenario == "before admission" {
					result.EvaluationID = nil
				}
				if scenario == "wrong status" {
					result.Status = "RELEASED"
				}
				if scenario == "wrong transaction" {
					result.TransactionID = evaluation
				}
				if scenario == "legacy reply" {
					_, _ = io.WriteString(w, `{"status":"CONFIRMED","flipped":0}`)
					return
				}
				if scenario == "oversized" {
					_, _ = io.WriteString(w, strings.Repeat(" ", config.MaxBodyBytes+1))
					return
				}
				_ = json.NewEncoder(w).Encode(result)
			}))
			t.Cleanup(server.Close)
			client, err := NewContextHTTPClient(server.URL, config)
			require.NoError(t, err)
			var result *tracercontract.TransactionCompletionResult
			if scenario == "release" {
				result, err = client.ReleaseByTransaction(t.Context(), request.TransactionID)
			} else {
				result, err = client.ConfirmByTransaction(t.Context(), request.TransactionID)
			}
			switch scenario {
			case "confirm", "release", "before admission":
				require.NoError(t, err)
				require.Equal(t, expectedStatus, result.Status)
				require.Equal(t, request.TransactionID, result.TransactionID)
			default:
				require.Error(t, err)
				require.Nil(t, result)
			}
			if scenario == "before admission" {
				require.Nil(t, result.EvaluationID)
			}
			if scenario == "unavailable" {
				require.ErrorIs(t, err, ErrTracerUnavailable)
			}
		})
	}
}

func TestContextHTTPClientDoesNotTreatPolicyFailuresAsAvailability(t *testing.T) {
	for _, cause := range []error{constant.ErrContextPolicyUnavailable, constant.ErrContextLimitsUnavailable, constant.ErrExpressionCostExceeded, constant.ErrExpressionEvaluation} {
		t.Run(cause.Error(), func(t *testing.T) {
			request, config := contextClientFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": cause.Error()})
			}))
			t.Cleanup(server.Close)
			client, err := NewContextHTTPClient(server.URL, config)
			require.NoError(t, err)
			_, err = client.Reserve(t.Context(), request)
			require.ErrorIs(t, err, cause)
			require.NotErrorIs(t, err, ErrTracerUnavailable)
		})
	}
}
