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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// CreateTransactionV2Input is everything the /v2 create contract carries.
type CreateTransactionV2Input struct {
	OrganizationID    uuid.UUID
	LedgerID          uuid.UUID
	Transaction       mtransaction.Transaction
	TransactionStatus string
	IdempotencyKey    string
	IdempotencyTTL    time.Duration

	// IdempotencyHashSource keys the idempotency slot off the raw body as
	// submitted, with the action discriminator folded in. Empty falls back to the
	// canonical serialized transaction.
	IdempotencyHashSource string
}

// CreateTransactionV2 posts a transaction under the /v2 contract: the fee engine,
// the tracer reservation lifecycle and the per-call skip controls all apply.
//
// The seam order is the contract. The single ledger-settings read carries the skip
// opt-ins, so the skips resolve off it with no extra I/O; an honored fee skip then
// bypasses the fee engine before its package lookup, and an honored tracer skip
// bypasses the reserve anchor before any request is built. The fee seam mutates the
// send, so the validate is re-run over the fee-inclusive legs and the reserve
// observes fee-inclusive amounts.
//
// It returns the created transaction and whether the idempotency slot answered with
// a replay, so the transport sets X-Idempotency-Replayed itself.
func (uc *UseCase) CreateTransactionV2(ctx context.Context, in CreateTransactionV2Input) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.create_transaction_v2")
	defer span.End()

	run := &createTransactionRun{
		organizationID: in.OrganizationID,
		ledgerID:       in.LedgerID,
		input:          in.Transaction,
		status:         in.TransactionStatus,
		idempotencyKey: in.IdempotencyKey,
		idempotencyTTL: in.IdempotencyTTL,
	}

	if err := uc.prepareCreateTransaction(ctx, span, logger, run); err != nil {
		return nil, false, err
	}

	replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, in.IdempotencyHashSource)
	if err != nil {
		return nil, false, err
	}

	if replay != nil {
		return replay, true, nil
	}

	// First validate: rejects malformed source/distribute on the RAW input
	// before fees are computed. Its Responses value is intentionally superseded
	// by the post-fee re-validation below (the fee engine mutates the send), so
	// only the error is consumed here. The binding is kept (not `_, err`) to
	// preserve the single-`:=`/single-`=` seam shape the structural gate
	// (create_transaction_seam_structure_test.go) enforces.
	//nolint:staticcheck,wastedassign,ineffassign // first validate's value is deliberately superseded by the post-fee re-validation; only its error gates malformed input before fees run.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	// Ledger settings (Redis cache-aside) are read once here, above the fee seam.
	// They carry the per-call skip opt-ins (Overrides), so resolving the skips
	// off this single read keeps the gate free of extra I/O and lets an honored
	// fee skip bypass the engine entirely — before any package lookup.
	run.ledgerSettings, err = uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	// Resolve the two per-call skips (fees, tracer) once, off the settings just
	// read — no extra I/O. An honored fee skip short-circuits applyFees below
	// before the fee package lookup; an honored tracer skip short-circuits the
	// reserve anchor. A skip requested without the per-ledger opt-in is a 422:
	// release the idempotency key and reject.
	honoredFeeSkip, honoredTracerSkip, skipRejectLabel, err := resolveTransactionSkips(run.input, run.ledgerSettings)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, skipRejectLabel, err)
		logger.Log(ctx, libLog.LevelWarn, skipRejectLabel, libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.honoredFeeSkip = honoredFeeSkip
	run.honoredTracerSkip = honoredTracerSkip

	// Record the resolved skips as system observations (not request inputs): they
	// reflect what the two-key gate actually honored, and they are persisted to the
	// transaction row below for the durable audit trail.
	// The two *_route_eligible attributes are deliberately NOT folded into the matching
	// *_skipped flags: those are persisted on the transaction row as the audit trail of a
	// skip the CLIENT asked for, so marking them true on every create would record a
	// claim never made. The two reasons a control did not run stay distinguishable.
	span.SetAttributes(
		attribute.Bool("app.transaction.fees_skipped", run.honoredFeeSkip),
		attribute.Bool("app.transaction.tracer_skipped", run.honoredTracerSkip),
		attribute.Bool("app.transaction.fees_route_eligible", true),
		attribute.Bool("app.transaction.tracer_route_eligible", true),
	)

	// Fee seam: drive the in-process fee engine over the validated transaction,
	// mutating run.input.Send.* (fee legs + moved Send.Value on deductible fees).
	// The settings read + skip resolution above precede this seam; the seam still
	// runs before the single validate reassignment below, which is upstream of
	// PropagateRouteValidation — that mutator decorates the post-fee validate, and
	// every downstream consumer reads the same pointer. applyFees resolves the
	// tenant's fee DB internally, only once it has decided fees actually apply, so
	// the MT tenant resolution rides inside the same gate as the fee computation.
	if err = uc.applyFees(ctx, &run.input, run.organizationID, run.ledgerID, run.status == constant.NOTED, run.honoredFeeSkip); err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to apply fees", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to apply fees", libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	normalizeSendLegs(run)

	// Re-run validation on the fee-mutated input. This is a single = reassignment
	// of the existing validate variable (a *mtransaction.Responses pointer), so
	// the fee-inclusive state by construction reaches every downstream reader of
	// validate through WriteTransaction. It MUST NOT be a := rebind.
	validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate fee-inclusive send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate fee-inclusive send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.validate = validate

	// Build the concat-form fromTo from the FEE-INCLUSIVE, normalized send. This
	// runs after applyFees + the second validate so the slice carries the fee
	// legs in the same "<index>#alias#balanceKey" form that buildBalanceOperations
	// keys the validate maps by and that the Lua-returned balances carry — without
	// it the `balances × fromTo` match loop in BuildOperations never emits the fee
	// Operation rows. The aliases are already concat'd in place above; this read
	// is idempotent.
	run.fromTo = append(run.fromTo, mtransaction.MutateConcatAliases(run.input.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(run.input.Send.Distribute.To)

	if run.status != constant.PENDING {
		run.fromTo = append(run.fromTo, to...)
	}

	if run.ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, run.validate, run.status)
	}

	run.action = mtransaction.StatusToAction(run.status)

	ctx, err = uc.stageBalances(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	// Reserve anchor (F3-T13): hold usage-limit capacity against the FEE-INCLUSIVE
	// transaction immediately before the balance commit. This observes the validated
	// fee-inclusive send amount; it never mutates Send.Value or balance state. A
	// DENIED decision (enforce) or a fail-closed unavailable tracer rejects here,
	// before ProcessBalanceOperations moves any balance, releasing the idempotency key
	// and the Redis-queue seed exactly as the ProcessBalanceOperations failure path
	// does below. The returned handle is confirmed on success / released on abort.
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

		// The balance commit failed (no funds moved), so return the held
		// reservation capacity. Best-effort: a transport failure here is
		// reconciled by the TTL reaper.
		uc.releaseReservations(ctx, span, logger, reservation.Handle)

		return nil, false, err
	}

	// Confirm anchor (F3-T14, success phase): the balance commit succeeded, so
	// the held capacity is consumed. PENDING transactions defer the confirm to
	// /commit (and release to /cancel) — see F3-T15 — so the reservation stays
	// open here for them. Downstream BuildOperations/WriteTransaction failures
	// do NOT release: the balance has already moved and the backup queue
	// reconstructs the transaction, so the consumed capacity stands.
	if run.status != constant.PENDING {
		uc.confirmReservations(ctx, span, logger, reservation.Handle)
	}

	tran, err := uc.finalizeCreatedTransaction(ctx, span, logger, run)
	if err != nil {
		return nil, false, err
	}

	return tran, false, nil
}
