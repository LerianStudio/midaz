// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

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
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RevertIdempotencyReplayedLogMessage is the Warn message the revert use case records
// when the idempotency slot answers with a cached reverse instead of a new one.
const RevertIdempotencyReplayedLogMessage = "Revert replayed a cached reverse transaction"

// RevertTransactionInput names the transaction to reverse. Revert sends no idempotency
// headers, so the use case keys the slot on the reversal hash and applies the default
// TTL.
type RevertTransactionInput struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
}

// KNOWN DEFECT — REVERT IDEMPOTENCY IS NOT SCOPED BY ORIGIN.
//
// Revert sends no X-Idempotency header, so CreateOrCheckTransactionIdempotency falls back to
// key = HashSHA256(preimage), and with no override the create use case serialises the
// reversal payload. TransactionRevert() copies only the origin's economic content
// (description, asset, amount, legs, route, metadata) and NEVER the origin id, so two
// economically-identical origins in the same ledger derive the SAME key and share ONE slot:
// the second revert loses the SetNX, is handed the FIRST origin's cached reverse, and answers
// 201 while its own origin is never reverted. Silently — no error, no distinguishable status.
//
// The fix is an origin-scoped preimage. It is deliberately NOT applied here: v1 revert is
// released, and changing the preimage changes the Redis key shape, so a revert retried across
// a rolling-deploy boundary would land on a different slot and could double-revert. It re-lands
// together with the idempotency keyspace separation, which re-shapes the key anyway, behind a
// dual-write/dual-read migration — one coordinated deploy window instead of two.
//
// Until then the ONLY control is detection: the replayed flag below, its Warn, and the
// X-Idempotency-Replayed header the transports project. Do not treat that as a fix.
// The integration reproduction re-lands together with the fix in the money-path layer
// (the fail-closed integration gate forbids carrying it here as a permanent skip).
//
// RevertTransactionV1 reverses a transaction under the /v1 contract: the full revert
// eligibility gate, then the /v1 create pipeline with the revert action. The reversal
// links back to its origin through the parent transaction id, and the idempotency TTL
// defaults to ParseIdempotencyTTL("") == 300s (an absent X-TTL resolves to 300, never 0;
// a hardcoded 0 would make the Redis idempotency slot permanent). It returns the
// idempotency `replayed` flag alongside the reverse transaction so the transport sets
// X-Idempotency-Replayed itself.
func (uc *UseCase) RevertTransactionV1(ctx context.Context, in RevertTransactionInput) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	transactionReverted, err := uc.prepareRevertTransaction(ctx, span, in)
	if err != nil {
		return nil, false, err
	}

	run := uc.newRevertRun(in, transactionReverted)

	tranReverted, replayed, err := uc.createRevertV1(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	recordRevertReplay(ctx, span, logger, in.TransactionID, replayed)

	return tranReverted, replayed, nil
}

// RevertTransactionV2 reverses a transaction under the /v2 contract: the same eligibility
// gate, then the /v2 create pipeline with the revert action — per-call skip controls and
// the tracer reservation apply, the fee engine does not. The origin-scoping defect
// documented on RevertTransactionV1 applies here too.
func (uc *UseCase) RevertTransactionV2(ctx context.Context, in RevertTransactionInput) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	transactionReverted, err := uc.prepareRevertTransaction(ctx, span, in)
	if err != nil {
		return nil, false, err
	}

	run := uc.newRevertRun(in, transactionReverted)

	tranReverted, replayed, err := uc.createRevertV2(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	recordRevertReplay(ctx, span, logger, in.TransactionID, replayed)

	return tranReverted, replayed, nil
}

// prepareRevertTransaction runs the revert eligibility gate — no parent, not already a
// revert, APPROVED status, non-empty reversal, every routed operation bidirectional — and
// returns the reversal payload TransactionRevert reconstructs from the persisted parent
// operations.
func (uc *UseCase) prepareRevertTransaction(ctx context.Context, span trace.Span, in RevertTransactionInput) (mtransaction.Transaction, error) {
	parent, err := uc.TransactionReader.GetParentByTransactionID(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to retrieve Parent Transaction on query", err)

		return mtransaction.Transaction{}, err
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		return mtransaction.Transaction{}, err
	}

	tran, err := uc.TransactionReader.GetTransactionWithOperationsByID(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return mtransaction.Transaction{}, err
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		return mtransaction.Transaction{}, err
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction CantRevert Transaction", err)

		return mtransaction.Transaction{}, err
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction can't be reverted", err)

		return mtransaction.Transaction{}, err
	}

	// Validate bidirectional routes: operations with a route_id require
	// the referenced OperationRoute to have OperationType "bidirectional".
	for _, op := range tran.Operations {
		if op.RouteID == nil || *op.RouteID == "" {
			continue
		}

		routeUUID, parseErr := uuid.Parse(*op.RouteID)
		if parseErr != nil {
			parseValidationErr := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "RevertTransaction", "routeId")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid routeId format on operation during revert validation", parseValidationErr)

			return mtransaction.Transaction{}, parseValidationErr
		}

		operationRoute, routeErr := uc.TransactionReader.GetOperationRouteByID(ctx, in.OrganizationID, in.LedgerID, nil, routeUUID)
		if routeErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve operation route for revert validation", routeErr)

			return mtransaction.Transaction{}, routeErr
		}

		if operationRoute != nil && operationRoute.OperationType != "bidirectional" {
			err = pkg.ValidateBusinessError(constant.ErrRouteNotBidirectional, "RevertTransaction")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route is not bidirectional", err)

			return mtransaction.Transaction{}, err
		}
	}

	return transactionReverted, nil
}

// newRevertRun builds the run state a reversal posts under: the action is forced to
// "revert" so accounting route lookups use the revert rubrics instead of the
// status-derived action, the parent transaction id links the reversal to its origin, and
// the idempotency key is empty so the slot is keyed on the reversal hash.
func (uc *UseCase) newRevertRun(in RevertTransactionInput, transactionReverted mtransaction.Transaction) *createTransactionRun {
	return &createTransactionRun{
		organizationID:      in.OrganizationID,
		ledgerID:            in.LedgerID,
		parentTransactionID: in.TransactionID,
		input:               transactionReverted,
		status:              constant.CREATED,
		action:              constant.ActionRevert,
		idempotencyTTL:      pkgHTTP.ParseIdempotencyTTL(""),
	}
}

// recordRevertReplay marks a replayed reversal on the span and logs it.
func recordRevertReplay(ctx context.Context, span trace.Span, logger libLog.Logger, transactionID uuid.UUID, replayed bool) {
	if !replayed {
		return
	}

	// A replay is an outcome this span observed, not an input, so it belongs outside the
	// app.request.* namespace (T4). It is also not an error: the span stays green.
	span.SetAttributes(attribute.Bool("app.response.idempotency_replayed", true))

	// Warn — deliberately louder than the create paths, which treat a replay as routine.
	// A create replay is what the caller asked for: they sent X-Idempotency, so a cached
	// answer is the contract. Revert carries no caller key, so nobody asked for this one;
	// it means the caller's revert did NOT happen and the 201 alone cannot tell them so.
	// While the origin-agnostic key above stands, the cached reverse may not
	// even belong to this origin, so this is the only operator-visible trace of the
	// defect — Debug, typically not collected in production, could not carry it.
	logger.Log(ctx, libLog.LevelWarn, RevertIdempotencyReplayedLogMessage, libLog.String("transaction_id", transactionID.String()))
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
