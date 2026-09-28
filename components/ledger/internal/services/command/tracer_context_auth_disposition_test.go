// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// restSeamError returns the error the real REST client produces when the
// Tracer answers every call with status.
func restSeamError(t *testing.T, status int) error {
	t.Helper()

	return restSeamErrorWithBody(t, status, "")
}

// restSeamErrorWithBody is restSeamError with a response body, such as a
// canonical {"code": ...} envelope.
func restSeamErrorWithBody(t *testing.T, status int, body string) error {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	client, err := traceradapter.NewContextHTTPClient(server.URL, traceradapter.ContextClientConfig{Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100}, fixedTestToken{})
	require.NoError(t, err)

	_, err = client.ConfirmByTransaction(t.Context(), uuid.MustParse("77777777-7777-4777-8777-777777777777"))
	require.Error(t, err)

	return fmt.Errorf("reserve tracer context: %w", err)
}

// TestContextTracerAuthRejectionsFollowTheirClass pins how the anchor answers
// the authentication rejections the REST seam can receive: a 401 is a token
// the Tracer no longer accepts and follows the fail posture; a 403 carrying
// 0043 is a producer the Tracer does not authorize, a provisioning defect that
// blocks accounting in every posture; a 403 without that code did not come
// from a Tracer that evaluated the request and follows the fail posture.
func TestContextTracerAuthRejectionsFollowTheirClass(t *testing.T) {
	t.Parallel()

	unauthorized := restSeamError(t, http.StatusUnauthorized)
	forbidden := restSeamErrorWithBody(t, http.StatusForbidden, `{"code":"0043"}`)
	uncodedForbidden := restSeamError(t, http.StatusForbidden)

	for name, settings := range tracerPostures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for label, err := range map[string]error{"401": unauthorized, "403 without code": uncodedForbidden} {
				attempt := ContextTracerAttempt{Dispatched: true, Unavailable: tracerAdmissionUnavailable(err)}
				require.True(t, attempt.Unavailable, label)

				outcome := contextTracerDisposition(settings, attempt, err)
				if name == "enforce+closed" {
					require.Equal(t, reservationReject, outcome.Kind, label)

					var unavailable pkg.ServiceUnavailableError
					require.ErrorAs(t, outcome.Err, &unavailable, label)
					require.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code, label)
				} else {
					require.Equal(t, reservationProceed, outcome.Kind, "%s is an availability failure", label)
					require.NoError(t, outcome.Err, label)
				}
			}

			attempt := ContextTracerAttempt{Unavailable: tracerAdmissionUnavailable(forbidden)}
			require.False(t, attempt.Unavailable)

			outcome := contextTracerDisposition(settings, attempt, forbidden)
			require.Equal(t, reservationReject, outcome.Kind, "an unauthorized producer never authorizes accounting")
			require.Equal(t, pkg.ValidateBusinessError(constant.ErrTracerContractUnavailable, constant.EntityTransaction), outcome.Err)
		})
	}
}

func TestRecordTracerFailureCauseNamesTokenUnavailability(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	for name, scenario := range map[string]struct {
		err  error
		want bool
	}{
		"rejected token": {err: restSeamError(t, http.StatusUnauthorized), want: true},
		"no token":       {err: fmt.Errorf("%w: %w", traceradapter.ErrTracerUnavailable, constant.ErrTracerTokenUnavailable), want: true},
		"outage":         {err: restSeamError(t, http.StatusServiceUnavailable)},
		"forbidden":      {err: restSeamErrorWithBody(t, http.StatusForbidden, `{"code":"0043"}`)},
		"bare forbidden": {err: restSeamError(t, http.StatusForbidden)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, span := provider.Tracer("test").Start(t.Context(), name)
			recordTracerFailureCause(span, scenario.err)
			span.End()

			var found bool

			for _, ended := range recorder.Ended() {
				if ended.Name() != name {
					continue
				}

				for _, attr := range ended.Attributes() {
					if attr.Key == "app.tracer.failure_cause" {
						found = true

						require.Equal(t, attribute.StringValue(tracerFailureCauseTokenUnavailable), attr.Value)
					}
				}
			}

			require.Equal(t, scenario.want, found)
		})
	}
}
