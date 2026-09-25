// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func (uc *UseCase) reservePreparedTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, input ContextTracerInput, transaction mtransaction.Transaction, validated *mtransaction.Responses, balances []*mmodel.Balance) reservationOutcome {
	if uc.ContextTracer == nil {
		return uc.reserveTransaction(ctx, span, logger, input.Settings, input.Key.TransactionID, input.Amount, input.AssetCode, firstSourceAccountID(validated.Sources, balances), input.Timestamp, reservationTTLPolicy(input.LongLived), input.HonoredSkip)
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
		span.SetAttributes(attribute.Bool("app.response.tracer.reservation_skipped", outcome.Kind == reservationProceed))
	}

	if attempt.Result != nil {
		span.SetAttributes(attribute.String("app.response.tracer.decision", string(attempt.Result.Decision)))
	}

	if outcome.Kind == reservationReject {
		uc.concludeContextReservation(ctx, span, outcome.Handle, false)
	}

	return outcome
}

func contextTracerDisposition(settings mmodel.TracerSettings, attempt ContextTracerAttempt, admissionErr error) reservationOutcome {
	if attempt.Skipped {
		return reservationOutcome{Kind: reservationProceed}
	}

	if attempt.IntentAttempted && !attempt.Frozen {
		return reservationOutcome{Kind: reservationReject, Err: pkg.ValidateBusinessError(constant.ErrTracerContractUnavailable, constant.EntityTransaction)}
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

func (uc *UseCase) beginContextReservation(ctx context.Context, handle reservationHandle) error {
	if handle.ContextAttempt == nil {
		return nil
	}

	if uc.ContextTracer == nil {
		return pkg.ValidateBusinessError(constant.ErrTracerContractUnavailable, constant.EntityTransaction)
	}

	return uc.ContextTracer.BeginExecution(ctx, *handle.ContextAttempt)
}

func (uc *UseCase) concludeContextReservation(ctx context.Context, span trace.Span, handle reservationHandle, confirmed bool) {
	if handle.ContextAttempt == nil {
		return
	}

	if uc.ContextTracer == nil {
		recordTracerCoordinationError(span, constant.ErrTracerContractUnavailable)
		return
	}

	outcome := tracerreservation.Released
	if confirmed {
		outcome = tracerreservation.Confirmed
	}

	if err := uc.ContextTracer.Conclude(ctx, *handle.ContextAttempt, outcome); err != nil {
		recordTracerCoordinationError(span, err)
	}
}
