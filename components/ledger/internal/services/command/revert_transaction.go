// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// CreateRevertTransaction posts a reversal transaction. The action is forced to
// "revert" so that accounting route lookups use the revert rubrics instead of the
// status-derived action, and the parent transaction id links the reversal back to
// its origin.
//
// Neither contract charges fees on a revert: the reverse transaction already carries
// the reversed fee legs reconstructed by TransactionRevert from the persisted parent
// operations. The tracer is a different matter — limits measure GROSS activity, so a
// /v2 revert reserves capacity of its own.
func (uc *UseCase) CreateRevertTransaction(ctx context.Context, organizationID, ledgerID, parentTransactionID uuid.UUID, transactionInput mtransaction.Transaction, transactionStatus, idempotencyKey string, idempotencyTTL time.Duration, policy RouteVersionPolicy) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction.orchestrate")
	defer span.End()

	run := &createTransactionRun{
		organizationID:      organizationID,
		ledgerID:            ledgerID,
		parentTransactionID: parentTransactionID,
		input:               transactionInput,
		status:              transactionStatus,
		action:              constant.ActionRevert,
		idempotencyKey:      idempotencyKey,
		idempotencyTTL:      idempotencyTTL,
	}

	if policy == RouteV2 {
		return uc.createRevertV2(ctx, span, logger, run)
	}

	return uc.createRevertV1(ctx, span, logger, run)
}

// createRevertV1 posts a reversal under the /v1 contract: no fee engine, no tracer
// reservation, no per-call skip controls.
func (uc *UseCase) createRevertV1(ctx context.Context, span trace.Span, logger libLog.Logger, run *createTransactionRun) (*transaction.Transaction, bool, error) {
	if err := uc.prepareCreateTransaction(ctx, span, logger, run); err != nil {
		return nil, false, err
	}

	replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, "")
	if err != nil {
		return nil, false, err
	}

	if replay != nil {
		return replay, true, nil
	}

	//nolint:staticcheck,wastedassign,ineffassign // first validate's value is deliberately superseded by the re-validation; only its error gates malformed input.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.ledgerSettings, err = uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	span.SetAttributes(
		attribute.Bool("app.transaction.fees_skipped", false),
		attribute.Bool("app.transaction.tracer_skipped", false),
		attribute.Bool("app.transaction.fees_route_eligible", false),
		attribute.Bool("app.transaction.tracer_route_eligible", false),
	)

	normalizeSendLegs(run)

	validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate fee-inclusive send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate fee-inclusive send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.validate = validate

	run.fromTo = append(run.fromTo, mtransaction.MutateConcatAliases(run.input.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(run.input.Send.Distribute.To)

	if run.status != constant.PENDING {
		run.fromTo = append(run.fromTo, to...)
	}

	if run.ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, run.validate, run.status)
	}

	ctx, err = uc.stageBalances(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	run.result, err = uc.ProcessBalanceOperations(ctx, ProcessBalanceOperationsInput{
		OrganizationID:    run.organizationID,
		LedgerID:          run.ledgerID,
		TransactionID:     run.transactionID,
		TransactionInput:  &run.input,
		Validate:          run.validate,
		BalanceOperations: run.balanceOps,
		TransactionStatus: run.status,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to process balance operations", libLog.Err(err))

		uc.rollbackCreateSeed(ctx, logger, run)

		return nil, false, err
	}

	tran, err := uc.finalizeCreatedTransaction(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	return tran, false, nil
}

// createRevertV2 posts a reversal under the /v2 contract: the per-call skip controls
// and the tracer reservation lifecycle apply, the fee engine does not.
func (uc *UseCase) createRevertV2(ctx context.Context, span trace.Span, logger libLog.Logger, run *createTransactionRun) (*transaction.Transaction, bool, error) {
	if err := uc.prepareCreateTransaction(ctx, span, logger, run); err != nil {
		return nil, false, err
	}

	replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, "")
	if err != nil {
		return nil, false, err
	}

	if replay != nil {
		return replay, true, nil
	}

	//nolint:staticcheck,wastedassign,ineffassign // first validate's value is deliberately superseded by the re-validation; only its error gates malformed input.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.ledgerSettings, err = uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	honoredFeeSkip, honoredTracerSkip, skipRejectLabel, err := resolveTransactionSkips(run.input, run.ledgerSettings)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, skipRejectLabel, err)
		logger.Log(ctx, libLog.LevelWarn, skipRejectLabel, libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.honoredFeeSkip = honoredFeeSkip
	run.honoredTracerSkip = honoredTracerSkip

	span.SetAttributes(
		attribute.Bool("app.transaction.fees_skipped", run.honoredFeeSkip),
		attribute.Bool("app.transaction.tracer_skipped", run.honoredTracerSkip),
		attribute.Bool("app.transaction.fees_route_eligible", true),
		attribute.Bool("app.transaction.tracer_route_eligible", true),
	)

	normalizeSendLegs(run)

	validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate fee-inclusive send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate fee-inclusive send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.validate = validate

	run.fromTo = append(run.fromTo, mtransaction.MutateConcatAliases(run.input.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(run.input.Send.Distribute.To)

	if run.status != constant.PENDING {
		run.fromTo = append(run.fromTo, to...)
	}

	if run.ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, run.validate, run.status)
	}

	ctx, err = uc.stageBalances(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	// A revert is itself a chargeable transaction: limits measure GROSS activity, so
	// the reversal reserves capacity of its own. The ORIGINAL transaction's
	// reservation is never released or confirmed here.
	reservation := uc.reserveTransaction(ctx, span, logger, run.ledgerSettings.Tracer, run.transactionID,
		run.input.Send.Value, run.input.Send.Asset, firstSourceAccountID(run.validate.Sources, run.balances),
		run.transactionDate, reservationTTLForStatus(run.status), run.honoredTracerSkip)
	if reservation.Kind == reservationReject {
		uc.rollbackCreateSeed(ctx, logger, run)

		return nil, false, reservation.Err
	}

	run.result, err = uc.ProcessBalanceOperations(ctx, ProcessBalanceOperationsInput{
		OrganizationID:    run.organizationID,
		LedgerID:          run.ledgerID,
		TransactionID:     run.transactionID,
		TransactionInput:  &run.input,
		Validate:          run.validate,
		BalanceOperations: run.balanceOps,
		TransactionStatus: run.status,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to process balance operations", libLog.Err(err))

		uc.rollbackCreateSeed(ctx, logger, run)

		uc.releaseReservations(ctx, span, logger, reservation.Handle)

		return nil, false, err
	}

	if run.status != constant.PENDING {
		uc.confirmReservations(ctx, span, logger, reservation.Handle)
	}

	tran, err := uc.finalizeCreatedTransaction(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	return tran, false, nil
}
