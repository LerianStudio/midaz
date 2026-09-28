// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextTracerDecisionsPreserveLedgerPosture(t *testing.T) {
	for _, mode := range []string{"advisory", "enforce"} {
		for _, posture := range []string{"open", "closed"} {
			for _, decision := range []tracercontract.Decision{tracercontract.DecisionAllow, tracercontract.DecisionDeny, tracercontract.DecisionReview} {
				t.Run(mode+"/"+posture+"/"+string(decision), func(t *testing.T) {
					settings := mmodel.TracerSettings{Mode: mode, FailPosture: posture}
					attempt := ContextTracerAttempt{Dispatched: true, Result: &tracercontract.ReserveResult{Decision: decision}}
					got := contextTracerDisposition(settings, attempt, nil)
					if mode == "advisory" || decision == tracercontract.DecisionAllow {
						require.Equal(t, reservationProceed, got.Kind)
						require.NoError(t, got.Err)
					} else {
						require.Equal(t, reservationReject, got.Kind)
						var rejected pkg.UnprocessableOperationError
						require.ErrorAs(t, got.Err, &rejected)
						if decision == tracercontract.DecisionReview {
							require.Equal(t, "0535", rejected.Code)
						} else {
							require.Equal(t, "0177", rejected.Code)
						}
					}
					attempt.Result = nil
					got = contextTracerDisposition(settings, attempt, fmt.Errorf("response lost: %w", traceradapter.ErrTracerUnavailable))
					require.Equal(t, mode == "advisory" || posture == "open", got.Kind == reservationProceed)
				})
			}
		}
	}
}

func TestContextTracerRejectsDeterministicFailuresInEveryPosture(t *testing.T) {
	for _, mode := range []string{mmodel.TracerModeAdvisory, mmodel.TracerModeEnforce} {
		for _, posture := range []string{mmodel.TracerFailPostureOpen, mmodel.TracerFailPostureClosed} {
			for _, cause := range []error{constant.ErrInvalidRequestBody, constant.ErrPayloadTooLarge, constant.ErrTracerFactsUnavailable, constant.ErrContextPolicyUnavailable, constant.ErrContextLimitsUnavailable, constant.ErrExpressionCostExceeded, constant.ErrExpressionEvaluation, constant.ErrTracerContractUnavailable, errors.New("unclassified failure")} {
				t.Run(mode+"/"+posture+"/"+cause.Error(), func(t *testing.T) {
					settings := mmodel.TracerSettings{Mode: mode, FailPosture: posture}
					for _, attempt := range []ContextTracerAttempt{{}, {Dispatched: true}} {
						outcome := contextTracerDisposition(settings, attempt, fmt.Errorf("admission: %w", cause))
						require.Equal(t, reservationReject, outcome.Kind)
						require.Error(t, outcome.Err)
						require.Equal(t, "context_invalid", tracerAdmissionMetric(attempt, outcome, cause))
					}
				})
			}
		}
	}
}

func TestContextTracerDeadlineStillFollowsPosture(t *testing.T) {
	for _, posture := range []string{mmodel.TracerFailPostureOpen, mmodel.TracerFailPostureClosed} {
		outcome := contextTracerDisposition(mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: posture}, ContextTracerAttempt{Dispatched: true}, context.DeadlineExceeded)
		require.Equal(t, posture == mmodel.TracerFailPostureOpen, outcome.Kind == reservationProceed)
	}
}

func newCompletionTestCoordinator(t *testing.T) (*ContextTracerCoordinator, *MockContextTracerReserver) {
	t.Helper()

	ctrl := gomock.NewController(t)
	client := NewMockContextTracerReserver(ctrl)
	coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
	require.NoError(t, err)

	return coordinator, client
}

func TestCompleteContextReservationTerminalFailuresAreNotRetried(t *testing.T) {
	transactionID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	settings := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, TimeoutMs: 250}

	for name, respond := range map[string]func(*MockContextTracerReserver){
		"operation conflict": func(client *MockContextTracerReserver) {
			client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(nil, fmt.Errorf("complete: %w", constant.ErrReserveOperationConflict)).Times(1)
		},
		"echo mismatch": func(client *MockContextTracerReserver) {
			client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(completionResult(transactionID, "RELEASED"), nil).Times(1)
		},
		"invalid request": func(client *MockContextTracerReserver) {
			client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(nil, constant.ErrInvalidRequestBody).Times(1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			withFastSharedRetrier(t)
			coordinator, client := newCompletionTestCoordinator(t)
			respond(client)
			reader, factory := newReaderFactory(t)
			uc := &UseCase{ContextTracer: coordinator, MetricsFactory: factory}
			logger := &capturingLogger{}

			uc.completeContextReservation(t.Context(), noop.Span{}, logger, settings, reservationHandle{TransactionID: transactionID}, reservationActionConfirm)
			sharedReservationRetrier.wait()

			errorsLogged := logger.atLevelOrMoreSevere(libLog.LevelError)
			require.Len(t, errorsLogged, 1, "a terminal completion is reported once and not retried")
			require.Contains(t, errorsLogged[0].Msg, "the spend will not be counted against the limit")
			require.Contains(t, errorsLogged[0].Fields, transactionID.String())
			require.Len(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), 1, "no retry is scheduled")
			require.Equal(t, map[string]int64{"confirm/failed": 1}, collectTracerCounters(t, reader))
		})
	}
}

func TestCompleteContextReservationSettledReleaseIsNotALoss(t *testing.T) {
	withFastSharedRetrier(t)

	transactionID := uuid.MustParse("13131313-1313-4131-8131-131313131313")
	coordinator, client := newCompletionTestCoordinator(t)
	client.EXPECT().ReleaseByTransaction(gomock.Any(), transactionID).Return(nil, fmt.Errorf("complete: %w", constant.ErrReserveOperationConflict)).Times(1)

	reader, factory := newReaderFactory(t)
	uc := &UseCase{ContextTracer: coordinator, MetricsFactory: factory}
	logger := &capturingLogger{}

	uc.completeContextReservation(t.Context(), noop.Span{}, logger, mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, TimeoutMs: 250}, reservationHandle{TransactionID: transactionID}, reservationActionRelease)
	sharedReservationRetrier.wait()

	require.Empty(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), "an already-settled release is neither an error nor retried")
	require.Len(t, logger.atLevelOrMoreSevere(libLog.LevelInfo), 1)
	require.Equal(t, map[string]int64{"release/delivered": 1}, collectTracerCounters(t, reader))
}

func TestCompleteContextReservationUnansweredAdmissionGoesStraightToRetrier(t *testing.T) {
	withFastSharedRetrier(t)

	transactionID := uuid.MustParse("88888888-8888-4888-8888-888888888888")
	coordinator, client := newCompletionTestCoordinator(t)
	reader, factory := newReaderFactory(t)
	uc := &UseCase{ContextTracer: coordinator, MetricsFactory: factory}
	handle := reservationHandle{TransactionID: transactionID, ContextAttempt: &ContextTracerAttempt{Dispatched: true, Unavailable: true}}

	requestReturned := make(chan struct{})

	client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).DoAndReturn(func(ctx context.Context, id uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
		select {
		case <-requestReturned:
		case <-ctx.Done():
			t.Error("an unanswered admission is never completed on the request path")
			return nil, ctx.Err()
		}
		deadline, bounded := ctx.Deadline()
		assert.True(t, bounded)
		assert.LessOrEqual(t, time.Until(deadline), 250*time.Millisecond)
		return completionResult(id, "CONFIRMED"), nil
	}).Times(1)

	uc.completeContextReservation(t.Context(), noop.Span{}, &capturingLogger{}, mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, TimeoutMs: 250}, handle, reservationActionConfirm)
	close(requestReturned)

	sharedReservationRetrier.wait()
	require.Equal(t, map[string]int64{"confirm/failed": 1}, collectTracerCounters(t, reader), "a completion handed straight to the retrier counts as failed inline")
}

func TestContextRetryTransportStopsOnTerminalFailure(t *testing.T) {
	transactionID := uuid.MustParse("12121212-1212-4121-8121-121212121212")

	for name, scenario := range map[string]struct {
		action     string
		respond    func(*MockContextTracerReserver)
		errorLines int
		infoLines  int
	}{
		"confirm conflict is a loss": {
			action: reservationActionConfirm,
			respond: func(client *MockContextTracerReserver) {
				client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(nil, constant.ErrReserveOperationConflict).Times(1)
			},
			errorLines: 1,
		},
		"release conflict is settled": {
			action: reservationActionRelease,
			respond: func(client *MockContextTracerReserver) {
				client.EXPECT().ReleaseByTransaction(gomock.Any(), transactionID).Return(nil, constant.ErrReserveOperationConflict).Times(1)
			},
			infoLines: 1,
		},
		"echo mismatch is a loss": {
			action: reservationActionRelease,
			respond: func(client *MockContextTracerReserver) {
				client.EXPECT().ReleaseByTransaction(gomock.Any(), transactionID).Return(completionResult(transactionID, "CONFIRMED"), nil).Times(1)
			},
			errorLines: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			coordinator, client := newCompletionTestCoordinator(t)
			scenario.respond(client)
			logger := &capturingLogger{}
			transition := reservationTransition{Action: scenario.action, TransactionID: transactionID}
			transport := contextTracerRetryTransport{coordinator: coordinator, logger: logger, transition: transition}

			retrier := newReservationRetrier(fastRetryPolicy())
			retrier.run(t.Context(), transport, logger, transition, traceradapter.ErrTracerUnavailable)

			lines := logger.snapshot()
			require.Len(t, logger.atLevelOrMoreSevere(libLog.LevelError), scenario.errorLines, "the transport's report is the only Error; the sequence is not exhausted")
			require.Len(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), scenario.errorLines, "no \"delivered on retry\" Warn follows a terminal answer")
			require.Len(t, lines, scenario.errorLines+scenario.infoLines)
		})
	}
}

func TestContextRetryTransportRetriesTransientFailure(t *testing.T) {
	transactionID := uuid.MustParse("14141414-1414-4141-8141-141414141414")
	coordinator, client := newCompletionTestCoordinator(t)
	gomock.InOrder(
		client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(nil, traceradapter.ErrTracerUnavailable),
		client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(completionResult(transactionID, "CONFIRMED"), nil),
	)

	logger := &capturingLogger{}
	transition := reservationTransition{Action: reservationActionConfirm, TransactionID: transactionID}
	transport := contextTracerRetryTransport{coordinator: coordinator, logger: logger, transition: transition}

	newReservationRetrier(fastRetryPolicy()).run(t.Context(), transport, logger, transition, traceradapter.ErrTracerUnavailable)

	require.Empty(t, logger.atLevelOrMoreSevere(libLog.LevelError))
	warnings := logger.atLevelOrMoreSevere(libLog.LevelWarn)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Msg, "delivered on retry")
}

func TestContextRetryTransportRejectsReservationIDOperations(t *testing.T) {
	transport := contextTracerRetryTransport{}

	_, err := transport.Reserve(t.Context(), traceradapter.ReserveRequest{})
	require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
	require.ErrorIs(t, transport.Confirm(t.Context(), uuid.Nil), constant.ErrTracerContractUnavailable)
	require.ErrorIs(t, transport.Release(t.Context(), uuid.Nil), constant.ErrTracerContractUnavailable)
}
