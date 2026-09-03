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
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// CreateTransactionV1Input is everything the /v1 create contract carries. There is
// no idempotency hash override: /v1 keys the slot off the canonical serialized
// transaction.
type CreateTransactionV1Input struct {
	OrganizationID    uuid.UUID
	LedgerID          uuid.UUID
	Transaction       mtransaction.Transaction
	TransactionStatus string
	IdempotencyKey    string
	IdempotencyTTL    time.Duration
}

// CreateTransactionV1 posts a transaction under the /v1 contract, frozen at what
// /v1 shipped with: no fee engine, no tracer reservation, and no per-call skip
// controls. A client integrated against it must not acquire fee legs, a tenant
// fee-DB resolution failure or a reservation rejection from a version upgrade it
// never asked for, so this pipeline references none of those seams at all.
//
// It returns the created transaction and whether the idempotency slot answered with
// a replay, so the transport sets X-Idempotency-Replayed itself.
func (uc *UseCase) CreateTransactionV1(ctx context.Context, in CreateTransactionV1Input) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction.orchestrate")
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

	replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, "")
	if err != nil {
		return nil, false, err
	}

	if replay != nil {
		return replay, true, nil
	}

	// First validate: rejects malformed source/distribute before the legs are
	// normalized. Its Responses value is superseded by the re-validation below, so
	// only the error is consumed here.
	//nolint:staticcheck,wastedassign,ineffassign // first validate's value is deliberately superseded by the re-validation; only its error gates malformed input.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	// Ledger settings (Redis cache-aside) drive the accounting-route propagation
	// below and the route validation BuildOperations applies.
	run.ledgerSettings, err = uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	// The /v1 contract runs neither control, so both skip flags are false and
	// neither control is route-eligible. The two *_route_eligible attributes are
	// deliberately NOT folded into the matching *_skipped flags: those are
	// persisted on the transaction row as the audit trail of a skip the CLIENT
	// asked for, so marking them true on a /v1 create would record a claim never
	// made. The two reasons a control did not run stay distinguishable.
	span.SetAttributes(
		attribute.Bool("app.transaction.fees_skipped", false),
		attribute.Bool("app.transaction.tracer_skipped", false),
		attribute.Bool("app.transaction.fees_route_eligible", false),
		attribute.Bool("app.transaction.tracer_route_eligible", false),
	)

	normalizeSendLegs(run)

	// Re-run validation on the normalized input. This is a single = reassignment of
	// the existing validate variable (a *mtransaction.Responses pointer), so the
	// normalized state by construction reaches every downstream reader of validate
	// through WriteTransaction. It MUST NOT be a := rebind.
	validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate fee-inclusive send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate fee-inclusive send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.rollbackCreateClaim(ctx, run)

		return nil, false, err
	}

	run.validate = validate

	// Build the concat-form fromTo from the normalized send: the legs carry the
	// "<index>#alias#balanceKey" form that BuildBalanceOperations keys the validate
	// maps by and that the Lua-returned balances carry. The aliases are already
	// concat'd in place above; this read is idempotent.
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
