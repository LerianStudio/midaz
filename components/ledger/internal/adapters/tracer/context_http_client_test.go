// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextHTTPClientReserve(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"allow", "deny", "review", "wrong transaction", "missing controls", "unavailable", "duplicate", "invalid request"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

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
			client, err := NewContextHTTPClient(server.URL, config, staticTokens("test-token"))
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
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		body        string
		unavailable bool
		rejected    bool
		cause       error
	}{
		{name: "internal", status: http.StatusInternalServerError, unavailable: true},
		{name: "bad gateway", status: http.StatusBadGateway, unavailable: true},
		{name: "request timeout", status: http.StatusRequestTimeout, unavailable: true},
		{name: "tenant service unavailable", status: http.StatusServiceUnavailable, body: `{"code":"0161"}`, unavailable: true},
		{name: "saturated canonical response", status: http.StatusTooManyRequests, body: `{"code":"0537"}`, unavailable: true},
		{name: "missing policy", status: http.StatusServiceUnavailable, body: `{"code":"0537"}`, rejected: true, cause: constant.ErrContextPolicyUnavailable},
		{name: "invalid request", status: http.StatusBadRequest, body: `{"code":"0094"}`, cause: constant.ErrInvalidRequestBody},
		{name: "operation conflict", status: http.StatusConflict, body: `{"code":"0540"}`, cause: constant.ErrReserveOperationConflict},
		{name: "contract unavailable", status: http.StatusUnprocessableEntity, body: `{"code":"0534"}`, cause: constant.ErrTracerContractUnavailable},
		{name: "rejected token", status: http.StatusUnauthorized, body: `{"code":"0094"}`, unavailable: true, cause: constant.ErrTracerTokenUnavailable},
		{name: "unauthorized producer", status: http.StatusForbidden, body: `{"code":"0043"}`, rejected: true, cause: constant.ErrInsufficientPrivileges},
		{name: "unauthorized producer problem document", status: http.StatusForbidden, body: `{"type":"about:blank","title":"Forbidden","status":403,"code":"0043"}`, rejected: true, cause: constant.ErrInsufficientPrivileges},
		{name: "missing tenant", status: http.StatusBadRequest, body: `{"code":"0487"}`, rejected: true, cause: constant.ErrReservationTenantRequired},
		{name: "forbidden without code", status: http.StatusForbidden, unavailable: true},
		{name: "forbidden with unrecognized code", status: http.StatusForbidden, body: `{"code":"RBAC_DENIED"}`, unavailable: true},
		{name: "forbidden with non-JSON body", status: http.StatusForbidden, body: `RBAC: access denied`, unavailable: true},
		{name: "bad request without code", status: http.StatusBadRequest, unavailable: true},
		{name: "not found without code", status: http.StatusNotFound, unavailable: true},
		{name: "method not allowed", status: http.StatusMethodNotAllowed, unavailable: true},
		{name: "conflict without code", status: http.StatusConflict, unavailable: true},
		{name: "unsupported media type", status: http.StatusUnsupportedMediaType, unavailable: true},
		{name: "unprocessable without code", status: http.StatusUnprocessableEntity, unavailable: true},
		{name: "redirect", status: http.StatusTemporaryRedirect, unavailable: true},
		{name: "moved permanently", status: http.StatusMovedPermanently, unavailable: true},
		{name: "unexpected success", status: http.StatusAccepted},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := contextHTTPResponseError(test.status, []byte(test.body))
			require.Equal(t, test.unavailable, errors.Is(err, ErrTracerUnavailable))
			require.Equal(t, test.rejected, errors.Is(err, ErrTracerRequestRejected))
			if test.cause != nil {
				require.ErrorIs(t, err, test.cause)
			}
			if test.status == http.StatusForbidden {
				require.NotErrorIs(t, err, constant.ErrTracerTokenUnavailable)
			}
		})
	}
}

func TestContextHTTPClientCompletion(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"confirm", "release", "before admission", "legacy reply", "wrong status", "wrong transaction", "oversized", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

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
			client, err := NewContextHTTPClient(server.URL, config, staticTokens("test-token"))
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
	t.Parallel()

	for _, cause := range []error{constant.ErrContextPolicyUnavailable, constant.ErrContextLimitsUnavailable, constant.ErrExpressionCostExceeded, constant.ErrExpressionEvaluation} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()

			request, config := contextClientFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": cause.Error()})
			}))
			t.Cleanup(server.Close)
			client, err := NewContextHTTPClient(server.URL, config, staticTokens("test-token"))
			require.NoError(t, err)
			_, err = client.Reserve(t.Context(), request)
			require.ErrorIs(t, err, cause)
			require.NotErrorIs(t, err, ErrTracerUnavailable)
		})
	}
}

// tokenSourceFunc adapts a function to TokenSource.
type tokenSourceFunc func(ctx context.Context) (string, error)

func (f tokenSourceFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

func staticTokens(token string) TokenSource {
	return tokenSourceFunc(func(context.Context) (string, error) { return token, nil })
}

func TestContextHTTPClientSendsBearerAndTenant(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)
	evaluation := contextResultFixture(request).EvaluationID

	type seen struct{ path, authorization, tenant string }

	received := make(chan seen, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- seen{path: r.URL.Path, authorization: r.Header.Get(AuthorizationHeader), tenant: r.Header.Get(TenantHeader)}

		if r.URL.Path == "/v1/reservations" {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(contextResultFixture(request))

			return
		}

		status := "CONFIRMED"
		if strings.HasSuffix(r.URL.Path, "/release") {
			status = "RELEASED"
		}

		_ = json.NewEncoder(w).Encode(tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: request.TransactionID, Status: status, EvaluationID: &evaluation})
	}))
	t.Cleanup(server.Close)

	client, err := NewContextHTTPClient(server.URL, config, staticTokens("m2m-token"))
	require.NoError(t, err)

	ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-007")

	_, err = client.Reserve(ctx, request)
	require.NoError(t, err)

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)

	_, err = client.ReleaseByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)

	for _, suffix := range []string{"/v1/reservations", "/confirm", "/release"} {
		call := <-received
		require.True(t, strings.HasSuffix(call.path, suffix), call.path)
		require.Equal(t, "Bearer m2m-token", call.authorization)
		require.Equal(t, "tenant-007", call.tenant)
	}
}

func TestContextHTTPClientTokenFailureIsUnavailability(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	for name, tokens := range map[string]TokenSource{
		"source error": tokenSourceFunc(func(context.Context) (string, error) {
			return "", fmt.Errorf("%w: identity provider unreachable", constant.ErrTracerTokenUnavailable)
		}),
		"empty token": staticTokens(""),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, err := NewContextHTTPClient(server.URL, config, tokens)
			require.NoError(t, err)

			_, err = client.Reserve(t.Context(), request)
			require.ErrorIs(t, err, ErrTracerUnavailable)
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)

			_, err = client.ConfirmByTransaction(t.Context(), request.TransactionID)
			require.ErrorIs(t, err, ErrTracerUnavailable)
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		})
	}

	require.Zero(t, calls.Load())
}

func TestNewContextHTTPClientRequiresTokenSource(t *testing.T) {
	t.Parallel()

	_, config := contextClientFixture(t)

	client, err := NewContextHTTPClient("https://tracer.example", config, nil)
	require.Error(t, err)
	require.Nil(t, client)
}

// TestContextHTTPClientRejectedToken pins the 401/403 split on the REST seam
// and the mint bound: a 401 on a token older than tokenRenewBackoff discards
// it and retries once with a freshly minted one; a 401 on a younger token is
// answered as unavailability without a mint; a 403 keeps the token, and is
// final only when it carries 0043.
func TestContextHTTPClientRejectedToken(t *testing.T) {
	t.Parallel()

	exp := tokenSourceEpoch.Add(10 * time.Minute)
	tokens := []string{signedToken(t, exp, "first"), signedToken(t, exp, "second"), signedToken(t, exp, "third"), signedToken(t, exp, "fourth")}

	newSource := func(t *testing.T, mint func(call int32) (string, error)) (*M2MTokenSource, *stubMinter, *testClock) {
		t.Helper()

		if mint == nil {
			mint = func(call int32) (string, error) { return tokens[call-1], nil }
		}

		clk := &testClock{now: tokenSourceEpoch}
		minter := &stubMinter{mintFn: mint}

		return newTestTokenSource(t, minter, clk), minter, clk
	}

	// agedSource returns a source whose first token is already cached and older
	// than the backoff, as a long-lived token the Tracer later rejects is.
	agedSource := func(t *testing.T) (*M2MTokenSource, *stubMinter, *testClock) {
		t.Helper()

		source, minter, clk := newSource(t, nil)

		_, err := source.Token(t.Context())
		require.NoError(t, err)

		clk.Advance(tokenRenewBackoff)

		return source, minter, clk
	}

	serveWithBody := func(t *testing.T, request tracercontract.ReserveRequest, status func(r *http.Request) int, body string) (*atomic.Int32, string) {
		t.Helper()

		var hits atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)

			if code := status(r); code != http.StatusCreated {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(code)
				_, _ = io.WriteString(w, body)

				return
			}

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(contextResultFixture(request))
		}))
		t.Cleanup(server.Close)

		return &hits, server.URL
	}

	serve := func(t *testing.T, request tracercontract.ReserveRequest, status func(r *http.Request) int) (*atomic.Int32, string) {
		t.Helper()

		return serveWithBody(t, request, status, "")
	}

	rejectAll := func(*http.Request) int { return http.StatusUnauthorized }

	t.Run("401 on an aged token renews it and retries once", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serve(t, request, func(r *http.Request) int {
			if r.Header.Get(AuthorizationHeader) == "Bearer "+tokens[0] {
				return http.StatusUnauthorized
			}

			return http.StatusCreated
		})
		source, minter, _ := agedSource(t)

		client, err := NewContextHTTPClient(url, config, source)
		require.NoError(t, err)

		result, err := client.Reserve(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, tracercontract.DecisionAllow, result.Decision)
		require.Equal(t, int32(2), hits.Load())
		require.Equal(t, int32(2), minter.calls.Load())
	})

	t.Run("401 on a just-minted token is not retried", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serve(t, request, rejectAll)
		source, minter, _ := newSource(t, nil)

		client, err := NewContextHTTPClient(url, config, source)
		require.NoError(t, err)

		_, err = client.Reserve(t.Context(), request)
		require.ErrorIs(t, err, ErrTracerUnavailable)
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.NotErrorIs(t, err, ErrTracerRequestRejected)
		require.Equal(t, int32(1), hits.Load())
		require.Equal(t, int32(1), minter.calls.Load())
	})

	t.Run("persistent 401 mints at most once per backoff window", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serve(t, request, rejectAll)
		source, minter, clk := agedSource(t)

		client, err := NewContextHTTPClient(url, config, source)
		require.NoError(t, err)

		const requests = 10

		for range requests {
			_, err = client.Reserve(t.Context(), request)
			require.ErrorIs(t, err, ErrTracerUnavailable)
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)

			_, err = client.ConfirmByTransaction(t.Context(), request.TransactionID)
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		}

		require.Equal(t, int32(2), minter.calls.Load(), "the aged token is replaced once; its replacement is kept")
		require.Equal(t, int32(2*requests+1), hits.Load(), "only the aged token earns a retry")

		clk.Advance(tokenRenewBackoff)

		_, err = client.Reserve(t.Context(), request)
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.Equal(t, int32(3), minter.calls.Load(), "one more mint once the window passes")
	})

	t.Run("identity provider down without a cached token mints once per backoff window", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serve(t, request, func(*http.Request) int { return http.StatusCreated })
		source, minter, clk := newSource(t, func(int32) (string, error) { return "", errors.New("identity provider unreachable") })

		client, err := NewContextHTTPClient(url, config, source)
		require.NoError(t, err)

		for range 10 {
			_, err = client.Reserve(t.Context(), request)
			require.ErrorIs(t, err, ErrTracerUnavailable)
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		}

		require.Equal(t, int32(1), minter.calls.Load())

		clk.Advance(tokenRenewBackoff)

		for range 10 {
			_, err = client.Reserve(t.Context(), request)
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		}

		require.Equal(t, int32(2), minter.calls.Load())
		require.Zero(t, hits.Load(), "no request reaches the Tracer without a token")
	})

	t.Run("a retry that misses the deadline keeps the 401 cause", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serve(t, request, func(r *http.Request) int {
			if r.Header.Get(AuthorizationHeader) == "Bearer "+tokens[0] {
				return http.StatusUnauthorized
			}

			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}

			return http.StatusCreated
		})
		source, minter, _ := agedSource(t)

		client, err := NewContextHTTPClient(url, config, source, WithOperationTimeout(100*time.Millisecond))
		require.NoError(t, err)

		_, err = client.Reserve(t.Context(), request)
		require.ErrorIs(t, err, ErrTracerUnavailable)
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.ErrorIs(t, err, context.DeadlineExceeded, "the retry failed on the operation deadline")
		require.Equal(t, int32(2), hits.Load(), "the renewed token was presented once")
		require.Equal(t, int32(2), minter.calls.Load(), "the rejected token was replaced once")
	})

	t.Run("403 with 0043 is final and keeps the token", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serveWithBody(t, request, func(*http.Request) int { return http.StatusForbidden },
			`{"title":"Insufficient Privileges","status":403,"code":"0043"}`)
		source, minter, _ := agedSource(t)

		client, err := NewContextHTTPClient(url, config, source)
		require.NoError(t, err)

		_, err = client.Reserve(t.Context(), request)
		require.ErrorIs(t, err, ErrTracerRequestRejected)
		require.ErrorIs(t, err, constant.ErrInsufficientPrivileges)
		require.NotErrorIs(t, err, ErrTracerUnavailable)
		require.NotErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.Equal(t, int32(1), hits.Load())
		require.Equal(t, int32(1), minter.calls.Load())
	})

	t.Run("403 without a recognized code is unavailability and keeps the token", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serveWithBody(t, request, func(*http.Request) int { return http.StatusForbidden }, "RBAC: access denied")
		source, minter, _ := agedSource(t)

		client, err := NewContextHTTPClient(url, config, source)
		require.NoError(t, err)

		_, err = client.Reserve(t.Context(), request)
		require.ErrorIs(t, err, ErrTracerUnavailable)
		require.NotErrorIs(t, err, ErrTracerRequestRejected)
		require.NotErrorIs(t, err, constant.ErrInsufficientPrivileges)
		require.NotErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.Equal(t, int32(1), hits.Load())
		require.Equal(t, int32(1), minter.calls.Load())
	})

	t.Run("a source that cannot renew is not retried", func(t *testing.T) {
		t.Parallel()

		request, config := contextClientFixture(t)
		hits, url := serve(t, request, rejectAll)

		client, err := NewContextHTTPClient(url, config, staticTokens("static-token"))
		require.NoError(t, err)

		_, err = client.Reserve(t.Context(), request)
		require.ErrorIs(t, err, ErrTracerUnavailable)
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.Equal(t, int32(1), hits.Load())
	})
}

// TestContextHTTPClientDoesNotFollowRedirects pins that a redirect is an
// availability failure and the bearer token never reaches the redirect target.
func TestContextHTTPClientDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)

	var leaked atomic.Int32

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(AuthorizationHeader) != "" {
			leaked.Add(1)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(target.Close)

	tracer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(tracer.Close)

	client, err := NewContextHTTPClient(tracer.URL, config, staticTokens("m2m-token"))
	require.NoError(t, err)

	_, err = client.Reserve(t.Context(), request)
	require.ErrorIs(t, err, ErrTracerUnavailable)
	require.NotErrorIs(t, err, ErrTracerRequestRejected)

	_, err = client.ConfirmByTransaction(t.Context(), request.TransactionID)
	require.ErrorIs(t, err, ErrTracerUnavailable)
	require.NotErrorIs(t, err, ErrTracerRequestRejected)

	require.Zero(t, leaked.Load(), "the bearer token must not follow a redirect")
}
