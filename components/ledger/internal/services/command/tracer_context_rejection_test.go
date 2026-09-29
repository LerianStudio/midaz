// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// rejectingReservationServer answers every RPC with one status error.
type rejectingReservationServer struct {
	reservationv1.UnimplementedReservationServiceServer

	err error
}

func (s rejectingReservationServer) Reserve(context.Context, *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	return nil, s.err
}

func (s rejectingReservationServer) ConfirmByTransaction(context.Context, *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	return nil, s.err
}

func (s rejectingReservationServer) ReleaseByTransaction(context.Context, *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
	return nil, s.err
}

// grpcSeamClient returns the real gRPC seam client over an in-memory server
// that answers every call with statusErr.
func grpcSeamClient(t *testing.T, statusErr error) *traceradapter.ContextGRPCClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	reservationv1.RegisterReservationServiceServer(server, rejectingReservationServer{err: statusErr})

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	client, err := traceradapter.NewContextGRPCClient("passthrough:///bufnet", traceradapter.ContextClientConfig{Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100},
		traceradapter.WithGRPCDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		))
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Close() })

	return client
}

// grpcSeamError returns the error the real gRPC client produces when the
// Tracer answers a completion with statusErr.
func grpcSeamError(t *testing.T, statusErr error) error {
	t.Helper()

	_, err := grpcSeamClient(t, statusErr).ConfirmByTransaction(t.Context(), uuid.MustParse("77777777-7777-4777-8777-777777777777"))
	require.Error(t, err)

	return fmt.Errorf("reserve tracer context: %w", err)
}

type seamErrorCase struct {
	transport string
	build     func(t *testing.T) error
}

// preEvaluationRejections are the refusals before evaluation each transport
// can receive. Each carries a canonical code the seam recognizes; the case
// names the transport, the status and that code.
func preEvaluationRejections() map[string]seamErrorCase {
	return map[string]seamErrorCase{
		"rest 403 0043": {"rest", func(t *testing.T) error { return restSeamErrorWithBody(t, http.StatusForbidden, `{"code":"0043"}`) }},
		"rest 403 0043 problem document": {"rest", func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusForbidden, `{"type":"about:blank","title":"Insufficient Privileges","status":403,"code":"0043"}`)
		}},
		"rest 400 0487": {"rest", func(t *testing.T) error { return restSeamErrorWithBody(t, http.StatusBadRequest, `{"code":"0487"}`) }},
		"rest 503 0537": {"rest", func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusServiceUnavailable, `{"code":"0537"}`)
		}},
		"grpc PermissionDenied 0043": {"grpc", func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.PermissionDenied, constant.ErrInsufficientPrivileges.Error()))
		}},
		"grpc InvalidArgument 0487": {"grpc", func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.InvalidArgument, constant.ErrReservationTenantRequired.Error()))
		}},
		"grpc FailedPrecondition 0537": {"grpc", func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.FailedPrecondition, constant.ErrContextPolicyUnavailable.Error()))
		}},
	}
}

// uncodedRefusals are refusals without a canonical code the seam recognizes:
// what a mesh denial, an ingress default backend or a Tracer pod without the
// route answers. Neither transport can tell them from an outage.
func uncodedRefusals() map[string]seamErrorCase {
	return map[string]seamErrorCase{
		"rest 403 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusForbidden) }},
		"rest 403 mesh denial":  {"rest", func(t *testing.T) error { return restSeamErrorWithBody(t, http.StatusForbidden, "RBAC: access denied") }},
		"rest 400 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusBadRequest) }},
		"rest 404 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusNotFound) }},
		"rest 404 unknown code": {"rest", func(t *testing.T) error { return restSeamErrorWithBody(t, http.StatusNotFound, `{"code":"0000"}`) }},
		"rest 405 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusMethodNotAllowed) }},
		"rest 409 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusConflict) }},
		"rest 415 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusUnsupportedMediaType) }},
		"rest 422 without code": {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusUnprocessableEntity) }},
		"rest 307 redirect":     {"rest", func(t *testing.T) error { return restSeamError(t, http.StatusTemporaryRedirect) }},
		"grpc PermissionDenied": {"grpc", func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.PermissionDenied, "RBAC: access denied"))
		}},
		"grpc InvalidArgument": {"grpc", func(t *testing.T) error { return grpcSeamError(t, status.Error(codes.InvalidArgument, "bad")) }},
		"grpc NotFound":        {"grpc", func(t *testing.T) error { return grpcSeamError(t, status.Error(codes.NotFound, "no route")) }},
		"grpc FailedPrecondition": {"grpc", func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.FailedPrecondition, "not ready"))
		}},
		"grpc AlreadyExists":   {"grpc", func(t *testing.T) error { return grpcSeamError(t, status.Error(codes.AlreadyExists, "dup")) }},
		"grpc Unauthenticated": {"grpc", func(t *testing.T) error { return grpcSeamError(t, status.Error(codes.Unauthenticated, "no identity")) }},
	}
}

var tracerPostures = map[string]mmodel.TracerSettings{
	"advisory":       {Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
	"enforce+open":   {Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
	"enforce+closed": {Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
}

// requireFollowsPosture asserts that err is an availability failure the fail
// posture decides: only enforce+closed blocks accounting, with 0178.
func requireFollowsPosture(t *testing.T, err error) {
	t.Helper()

	require.ErrorIs(t, err, traceradapter.ErrTracerUnavailable)
	require.NotErrorIs(t, err, traceradapter.ErrTracerRequestRejected)
	require.False(t, contextCompletionTerminal(err), "an availability failure is redelivered")

	attempt := ContextTracerAttempt{Dispatched: true, Unavailable: tracerAdmissionUnavailable(err)}
	require.True(t, attempt.Unavailable)

	for posture, settings := range tracerPostures {
		outcome := contextTracerDisposition(settings, attempt, err)

		if posture == "enforce+closed" {
			require.Equal(t, reservationReject, outcome.Kind)

			var unavailable pkg.ServiceUnavailableError
			require.ErrorAs(t, outcome.Err, &unavailable)
			require.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code)

			continue
		}

		require.Equal(t, reservationProceed, outcome.Kind, posture)
		require.NoError(t, outcome.Err)
	}
}

// TestContextTracerPreEvaluationRejectionsAreTerminal pins, per transport and
// per posture, that a coded refusal before evaluation is never an availability
// failure, blocks accounting, and ends a completion without redelivery.
func TestContextTracerPreEvaluationRejectionsAreTerminal(t *testing.T) {
	t.Parallel()

	for name, scenario := range preEvaluationRejections() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := scenario.build(t)
			require.ErrorIs(t, err, traceradapter.ErrTracerRequestRejected, scenario.transport)
			require.False(t, tracerAdmissionUnavailable(err))
			require.True(t, contextCompletionTerminal(err), "a redelivery cannot change a refusal before evaluation")

			for posture, settings := range tracerPostures {
				outcome := contextTracerDisposition(settings, ContextTracerAttempt{}, err)
				require.Equal(t, reservationReject, outcome.Kind, posture)
			}
		})
	}
}

// TestContextTracerUncodedRefusalsFollowPosture pins, per transport and per
// posture, that a refusal without a recognized code is an availability
// failure: the fail posture decides and a completion is redelivered.
func TestContextTracerUncodedRefusalsFollowPosture(t *testing.T) {
	t.Parallel()

	for name, scenario := range uncodedRefusals() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			requireFollowsPosture(t, scenario.build(t))
		})
	}
}

// TestContextTracerTenantServiceOutageFollowsPosture pins that 0161 from the
// Tracer's tenant resolution is an outage on both transports, not a refusal.
func TestContextTracerTenantServiceOutageFollowsPosture(t *testing.T) {
	t.Parallel()

	for name, build := range map[string]func(t *testing.T) error{
		"rest 503 0161": func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusServiceUnavailable, `{"code":"0161"}`)
		},
		"grpc Unavailable 0161": func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.Unavailable, constant.ErrTenantServiceUnavailable.Error()))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			requireFollowsPosture(t, build(t))
		})
	}
}

// TestCompleteContextReservationPreEvaluationRejectionIsReportedOnce drives the
// completion path with the real seam clients: each coded refusal is logged
// once at Error and takes no retrier slot. It swaps the process-wide retrier,
// so it does not run in parallel.
func TestCompleteContextReservationPreEvaluationRejectionIsReportedOnce(t *testing.T) {
	transactionID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	settings := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, TimeoutMs: 250}

	for name, scenario := range preEvaluationRejections() {
		for _, action := range []string{reservationActionConfirm, reservationActionRelease} {
			t.Run(name+"/"+action, func(t *testing.T) {
				withFastSharedRetrier(t)

				seamErr := scenario.build(t)
				stub := &stubContextTracer{completeErr: seamErr}
				reader, factory := newReaderFactory(t)
				uc := &UseCase{ContextTracer: stub.coordinatorFor(t), MetricsFactory: factory}
				logger := &capturingLogger{}

				uc.completeContextReservation(t.Context(), noop.Span{}, logger, settings, reservationHandle{TransactionID: transactionID}, action)
				sharedReservationRetrier.wait()

				require.Len(t, logger.atLevelOrMoreSevere(libLog.LevelError), 1)
				require.Len(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), 1, "no retry is scheduled")
				require.Len(t, append(stub.confirmedTransactions(), stub.releasedTransactions()...), 1, "delivered once, never redelivered")
				require.Equal(t, map[string]int64{action + "/failed": 1}, collectTracerCounters(t, reader))
			})
		}
	}
}

// TestCompleteContextReservationUncodedRefusalIsRedelivered drives the
// completion path with the real seam clients: a refusal without a recognized
// code is handed to the retrier. It swaps the process-wide retrier, so it does
// not run in parallel.
func TestCompleteContextReservationUncodedRefusalIsRedelivered(t *testing.T) {
	transactionID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	settings := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, TimeoutMs: 250}

	for name, scenario := range uncodedRefusals() {
		for _, action := range []string{reservationActionConfirm, reservationActionRelease} {
			t.Run(name+"/"+action, func(t *testing.T) {
				withFastSharedRetrier(t)

				stub := &stubContextTracer{completeErr: scenario.build(t)}
				uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}

				uc.completeContextReservation(t.Context(), noop.Span{}, &libLog.NopLogger{}, settings, reservationHandle{TransactionID: transactionID}, action)
				sharedReservationRetrier.wait()

				require.Greater(t, len(append(stub.confirmedTransactions(), stub.releasedTransactions()...)), 1, "an availability failure is redelivered")
			})
		}
	}
}

// TestReservePreparedTransactionRefusedBeforeEvaluationIsNotReleased pins, per
// posture, that a Reserve the Tracer refused before evaluating it is not
// followed by an inline release, while a dispatched Reserve rejected for
// another cause is. It swaps the process-wide retrier, so it does not run in
// parallel.
func TestReservePreparedTransactionRefusedBeforeEvaluationIsNotReleased(t *testing.T) {
	type reserveCase struct {
		build       func(t *testing.T) error
		wantRelease bool
	}

	scenarios := map[string]reserveCase{
		"rest 422 0534 contract failure": {build: func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusUnprocessableEntity, `{"code":"0534"}`)
		}, wantRelease: true},
		"grpc FailedPrecondition 0534 contract failure": {build: func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.FailedPrecondition, constant.ErrTracerContractUnavailable.Error()))
		}, wantRelease: true},
	}

	for name, rejection := range preEvaluationRejections() {
		scenarios[name] = reserveCase{build: rejection.build}
	}

	for name, scenario := range scenarios {
		for posture, base := range tracerPostures {
			t.Run(name+"/"+posture, func(t *testing.T) {
				withFastSharedRetrier(t)

				settings := enforceSettings(base.FailPosture)
				settings.Mode = base.Mode

				ctx, span, logger := anchorDeps()
				stub := &stubContextTracer{reserveErr: scenario.build(t)}
				uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
				transaction, validated, balances := anchorPrepared()
				input := anchorInput(settings, false)

				out := uc.reservePreparedTransaction(ctx, span, logger, input, transaction, validated, balances)
				sharedReservationRetrier.wait()

				require.Equal(t, reservationReject, out.Kind, "a deterministic failure never authorizes accounting, in any posture")
				require.Len(t, stub.reserves(), 1)
				require.Equal(t, scenario.wantRelease, out.Handle.ContextAttempt.Dispatched)

				if scenario.wantRelease {
					require.Equal(t, []uuid.UUID{input.Key.TransactionID}, stub.releasedTransactions())
				} else {
					require.Empty(t, stub.releasedTransactions(), "the Tracer holds nothing after a refusal before evaluation")
				}
			})
		}
	}
}

// TestRecordTracerCoordinationErrorClassifiesByCause pins that the span of a
// coordination failure stays green for a business cause, however deeply the
// seam wraps it, and turns red for anything else.
func TestRecordTracerCoordinationErrorClassifiesByCause(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	business := map[string]func(t *testing.T) error{
		"rest 422 0534": func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusUnprocessableEntity, `{"code":"0534"}`)
		},
		"rest 413 0143": func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusRequestEntityTooLarge, `{"code":"0143"}`)
		},
		"rest 409 0540": func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusConflict, `{"code":"0540"}`)
		},
		"grpc InvalidArgument 0342": func(t *testing.T) error {
			return grpcSeamError(t, status.Error(codes.InvalidArgument, constant.ErrExpressionCostExceeded.Error()))
		},
	}
	for name, rejection := range preEvaluationRejections() {
		business[name] = rejection.build
	}

	technical := map[string]func(t *testing.T) error{
		"rest 503": func(t *testing.T) error { return restSeamError(t, http.StatusServiceUnavailable) },
		"rest 401": func(t *testing.T) error { return restSeamError(t, http.StatusUnauthorized) },
		"rest 429 0537": func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusTooManyRequests, `{"code":"0537"}`)
		},
		"rest 400 0094": func(t *testing.T) error { return restSeamErrorWithBody(t, http.StatusBadRequest, `{"code":"0094"}`) },
		"rest 422 0531": func(t *testing.T) error {
			return restSeamErrorWithBody(t, http.StatusUnprocessableEntity, `{"code":"0531"}`)
		},
		"facts unavailable": func(*testing.T) error { return constant.ErrTracerFactsUnavailable },
	}
	for name, refusal := range uncodedRefusals() {
		technical[name] = refusal.build
	}

	for want, cases := range map[otelcodes.Code]map[string]func(t *testing.T) error{otelcodes.Unset: business, otelcodes.Error: technical} {
		for name, build := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				err := build(t)
				spanName := t.Name()

				_, span := provider.Tracer("test").Start(t.Context(), spanName)
				recordTracerCoordinationError(span, err)
				span.End()

				var found bool

				for _, ended := range recorder.Ended() {
					if ended.Name() == spanName {
						found = true

						require.Equal(t, want, ended.Status().Code)
					}
				}

				require.True(t, found)
			})
		}
	}
}
