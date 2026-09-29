// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// reservePreparedTransaction reserves limit capacity for FEE-INCLUSIVE
// amounts before accounting runs; it belongs to the /v2 contract only. A nil
// ContextTracer (TRACER_BASE_URL unset) proceeds without a tracer call;
// otherwise mode, honored skip and fail posture decide the outcome. It never
// mutates Send.Value or any balance state.
func (uc *UseCase) reservePreparedTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, input ContextTracerInput, transaction mtransaction.Transaction, validated *mtransaction.Responses, balances []*mmodel.Balance) reservationOutcome {
	if uc.ContextTracer == nil {
		return reservationOutcome{Kind: reservationProceed}
	}

	started := time.Now()
	attempt, err := uc.ContextTracer.AdmitPrepared(ctx, input, transaction, validated, balances)

	outcome := contextTracerDisposition(input.Settings, attempt, err)
	if !attempt.Skipped {
		emitTracerMetric(ctx, uc.MetricsFactory, "admission", tracerAdmissionMetric(attempt, outcome, err), time.Since(started))
	}

	outcome.Handle = reservationHandle{ContextAttempt: &attempt, TransactionID: input.Key.TransactionID, Amount: input.Amount, Asset: input.AssetCode}

	if err != nil {
		recordTracerCoordinationError(span, err)
		recordTracerFailureCause(span, err)
		span.SetAttributes(attribute.Bool("app.response.tracer.reservation_skipped", outcome.Kind == reservationProceed))
	}

	if attempt.Result != nil {
		span.SetAttributes(attribute.String("app.response.tracer.decision", string(attempt.Result.Decision)))
	}

	if outcome.Kind == reservationReject && attempt.Dispatched {
		uc.releaseReservations(ctx, span, logger, outcome.Handle)
	}

	return outcome
}

func contextTracerDisposition(settings mmodel.TracerSettings, attempt ContextTracerAttempt, admissionErr error) reservationOutcome {
	if attempt.Skipped {
		return reservationOutcome{Kind: reservationProceed}
	}

	if admissionErr != nil {
		if !tracerAdmissionUnavailable(admissionErr) {
			return reservationOutcome{Kind: reservationReject, Err: contextTracerRejection(admissionErr)}
		}

		if settings.Mode == mmodel.TracerModeAdvisory || settings.FailPosture == mmodel.TracerFailPostureOpen {
			return reservationOutcome{Kind: reservationProceed}
		}

		return reservationOutcome{Kind: reservationReject, Err: pkg.ValidateBusinessError(constant.ErrTransactionReservationUnavailable, constant.EntityTransaction)}
	}

	if attempt.Result == nil {
		return reservationOutcome{Kind: reservationReject, Err: pkg.ValidateBusinessError(constant.ErrTracerContractUnavailable, constant.EntityTransaction)}
	}

	if settings.Mode == mmodel.TracerModeAdvisory || attempt.Result.Decision == tracercontract.DecisionAllow {
		return reservationOutcome{Kind: reservationProceed}
	}

	if attempt.Result.Decision == tracercontract.DecisionReview {
		return reservationOutcome{Kind: reservationReject, Err: pkg.ValidateBusinessError(constant.ErrTransactionReviewRequired, constant.EntityTransaction)}
	}

	return reservationOutcome{Kind: reservationReject, Err: pkg.ValidateBusinessError(constant.ErrTransactionReservationDenied, constant.EntityTransaction)}
}

func tracerAdmissionUnavailable(err error) bool {
	return errors.Is(err, traceradapter.ErrTracerUnavailable) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// Failure posture applies only to positively identified availability failures.
// Invalid facts, policy defects and unknown errors never authorize accounting.
func contextTracerRejection(err error) error {
	if errors.Is(err, constant.ErrInvalidRequestBody) {
		return pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Invalid Tracer Context", Message: "The transaction cannot be represented within the supported validation contract."}
	}

	if errors.Is(err, constant.ErrPayloadTooLarge) {
		return pkg.PayloadTooLargeError{Code: constant.ErrPayloadTooLarge.Error(), Title: "Payload Too Large", Message: "The validation context exceeds the configured size limit."}
	}

	for _, cause := range []error{constant.ErrTracerFactsUnavailable, constant.ErrContextPolicyUnavailable, constant.ErrContextLimitsUnavailable, constant.ErrExpressionCostExceeded, constant.ErrExpressionEvaluation} {
		if errors.Is(err, cause) {
			return pkg.ValidateBusinessError(cause, constant.EntityTransaction)
		}
	}

	return pkg.ValidateBusinessError(constant.ErrTracerContractUnavailable, constant.EntityTransaction)
}

// completeContextReservation delivers a known accounting outcome inline and
// hands a transient failure to the shared retrier. It never fails the request:
// accounting has already run, and an undelivered outcome expires by Tracer TTL.
// An admission that failed for availability skips the inline call, so an
// unreachable Tracer does not cost the request a second timeout.
func (uc *UseCase) completeContextReservation(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, identity reservationHandle, action string) {
	transition := identity.transitionByTransaction(action)
	timeout := time.Duration(settings.TimeoutMs) * time.Millisecond
	retry := contextTracerRetryTransport{coordinator: uc.ContextTracer, logger: logger, timeout: timeout, transition: transition}

	if attempt := identity.ContextAttempt; attempt != nil && attempt.Unavailable {
		emitTracerMetric(ctx, uc.MetricsFactory, action, "failed", 0)
		sharedReservationRetrier.schedule(ctx, retry, logger, transition, traceradapter.ErrTracerUnavailable)

		return
	}

	started := time.Now()
	err := uc.ContextTracer.Complete(context.WithoutCancel(ctx), transition.TransactionID, action, timeout)

	result := "delivered"
	if err != nil && !contextReleaseSettled(transition, err) {
		result = "failed"
	}

	emitTracerMetric(ctx, uc.MetricsFactory, action, result, time.Since(started))

	if err == nil || concludeContextCompletion(ctx, span, logger, transition, err) {
		return
	}

	logReservationByTransactionFailure(ctx, span, logger, transition, err)
	sharedReservationRetrier.schedule(ctx, retry, logger, transition, err)
}

// warnContextReservationOutcomeUnknown names a dispatched admission left
// unsettled because accounting may or may not have run.
func warnContextReservationOutcomeUnknown(ctx context.Context, logger libLog.Logger, handle reservationHandle) {
	if handle.ContextAttempt == nil || !handle.ContextAttempt.Dispatched {
		return
	}

	logger.Log(ctx, libLog.LevelWarn, "Transaction outcome unknown, reservation left to tracer TTL",
		libLog.String("transaction_id", handle.TransactionID.String()))
}

// tracerFailureCauseTokenUnavailable marks a failure the ledger caused by
// holding no token the Tracer accepts. It shares the unavailable response code
// with an outage, so the span attribute is what tells them apart.
//
// #nosec G101 -- span attribute value, not a credential value.
const tracerFailureCauseTokenUnavailable = "token_unavailable"

// recordTracerFailureCause names on span a tracer failure cause the response
// code does not distinguish.
func recordTracerFailureCause(span trace.Span, err error) {
	if errors.Is(err, constant.ErrTracerTokenUnavailable) {
		span.SetAttributes(attribute.String("app.tracer.failure_cause", tracerFailureCauseTokenUnavailable))
	}
}

// tracerBusinessCauses are the canonical causes of a coordination failure the
// request itself provoked: a coded refusal before evaluation (0043, 0487,
// 0527), an unusable contract (0534), and the caller-bound expression cost,
// payload size and operation conflict (0342, 0143, 0530). Every other cause is
// technical.
var tracerBusinessCauses = []error{
	constant.ErrInsufficientPrivileges,
	constant.ErrReservationTenantRequired,
	constant.ErrContextPolicyUnavailable,
	constant.ErrTracerContractUnavailable,
	constant.ErrExpressionCostExceeded,
	constant.ErrPayloadTooLarge,
	constant.ErrReserveOperationConflict,
}

// recordTracerCoordinationError records err on span: a business cause keeps
// the span green, anything else flips it red. An availability failure is
// technical whatever cause it wraps.
func recordTracerCoordinationError(span trace.Span, err error) {
	if err == nil {
		return
	}

	if tracerBusinessCoordinationError(err) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Tracer coordination rejected", err)
		return
	}

	libOtel.HandleSpanError(span, "Tracer coordination incomplete", err)
}

func tracerBusinessCoordinationError(err error) bool {
	if tracerAdmissionUnavailable(err) {
		return false
	}

	for _, cause := range tracerBusinessCauses {
		if errors.Is(err, cause) {
			return true
		}
	}

	return false
}
