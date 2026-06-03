// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libCommons "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"

	feeError "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
)

// FeeApplier drives the in-process fee engine over a transaction's validated
// send/distribute structure. It is the narrow port the transaction handler
// depends on so the fee use case can be injected at bootstrap and faked in
// tests. The signature mirrors fees services.UseCase.CalculateFee: the engine
// mutates cf.Transaction.Send.* in place (legs are appended to Source.From /
// Distribute.To, and Send.Value moves on deductible fees) and returns a
// business error when a package rule rejects the transaction.
type FeeApplier interface {
	CalculateFee(ctx context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error
}

// applyFees drives the fee engine on the validated transaction and folds the
// resulting fee legs back into transactionInput. It mirrors the shape of
// enrichOverdraftOperations: a single seam that loads packages, runs the
// engine, and mutates the transaction so every downstream consumer of the
// re-run validate sees the fee-inclusive state.
//
// The fee engine owns its own package lookup + send/distribute mutation; this
// method only adapts the ledger transaction into the engine's FeeCalculate
// envelope and copies the mutated Send back out. The caller MUST re-run
// ValidateSendSourceAndDistribute after this returns nil so the fee legs reach
// the persistence path (BuildOperations / ProcessBalanceOperations /
// WriteTransaction) through a single reassigned validate pointer.
//
// On isRevert=true this is a no-op: the reverse transaction already carries the
// reversed fee legs reconstructed by TransactionRevert from the persisted
// parent operations, so re-charging here would double the fees.
//
// On isAnnotation=true (NOTED transactions) this is also a no-op: an annotation
// is one-sided and records no real balance movement, so charging it a fee would
// emit fee legs that have no funding side and break its invariants.
//
// A nil applier (fee engine disabled / not wired) is also a no-op so the
// transaction path stays unchanged when no fee use case is injected.
func (handler *TransactionHandler) applyFees(
	ctx context.Context,
	transactionInput *mtransaction.Transaction,
	organizationID, ledgerID uuid.UUID,
	isRevert, isAnnotation bool,
) error {
	if isRevert || isAnnotation || handler.FeeApplier == nil {
		return nil
	}

	logger := libCommons.NewLoggerFromContext(ctx)

	cf := &model.FeeCalculate{
		LedgerID:    ledgerID,
		Transaction: *transactionInput,
	}

	if err := handler.FeeApplier.CalculateFee(ctx, cf, organizationID); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to apply fees to transaction", libLog.Err(err))

		return translateFeeError(err)
	}

	// The engine mutated cf.Transaction.Send in place (fee legs + moved value
	// for deductible fees). Fold the mutated send back into the caller's
	// transaction so the second validate runs over the fee-inclusive shape.
	*transactionInput = cf.Transaction

	return nil
}

// translateFeeError maps a feeshared error type onto the equivalent ledger
// pkg error type so ledger's http.WithError surfaces the correct 4xx status.
// The fee engine returns feeshared.*Error values, which are a distinct Go type
// family from pkg.*Error; without translation they fall through to
// http.WithError's default branch and become a 500 instead of the intended
// business 422/404/400. Code/Title/Message are preserved verbatim.
func translateFeeError(err error) error {
	switch e := err.(type) {
	case feeError.ValidationError:
		return pkg.ValidationError{EntityType: e.EntityType, Title: e.Title, Message: e.Message, Code: e.Code, Err: e.Err}
	case feeError.UnprocessableOperationError:
		return pkg.UnprocessableOperationError{EntityType: e.EntityType, Title: e.Title, Message: e.Message, Code: e.Code, Err: e.Err}
	case feeError.EntityNotFoundError:
		return pkg.EntityNotFoundError{EntityType: e.EntityType, Title: e.Title, Message: e.Message, Code: e.Code, Err: e.Err}
	case feeError.EntityConflictError:
		return pkg.EntityConflictError{EntityType: e.EntityType, Title: e.Title, Message: e.Message, Code: e.Code, Err: e.Err}
	default:
		// Unknown/technical fee failure: wrap as an internal error so the HTTP
		// layer returns 500 with no fee-internal detail leaked to the client.
		return pkg.ValidateInternalError(err, "")
	}
}
