// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ProcessBalanceOperationsInput groups all parameters required by
// ProcessBalanceOperations. Using a struct instead of positional args
// improves readability, makes future extensions backward-compatible,
// and clearly separates "own" fields from pass-through values.
type ProcessBalanceOperationsInput struct {
	OrganizationID    uuid.UUID
	LedgerID          uuid.UUID
	TransactionID     uuid.UUID
	TransactionInput  *mtransaction.Transaction // nil skips balance-rule validation (state transitions)
	Validate          *mtransaction.Responses
	BalanceOperations []mmodel.BalanceOperation
	TransactionStatus string

	// AccountBlockExceptionGrant is the single-use account-block exception the
	// request presented, as read from the cache, or nil when it presented none.
	//
	// It authorizes nothing on its own. This step binds it to the ONE logical
	// debit of the batch it covers — the primary debit leg on the granted alias
	// plus the overdraft companions the system derived from it — and that binding
	// relaxes the account block and the sending/receiving deny on exactly those
	// balances. It is validated against the transaction and consumed inside the
	// atomic balance script: never here, and never by the Go validation.
	AccountBlockExceptionGrant *mtransaction.AccountBlockExceptionGrant
}

// ProcessBalanceOperations validates balance rules and executes the atomic Lua
// script that mutates balances in Redis.
//
// When input.TransactionInput is non-nil (new transactions), balance rules
// (eligibility, asset codes, sending/receiving permissions) are validated
// before the atomic operation. State transitions (approve, cancel, revert)
// pass nil to skip re-validation because these rules were already enforced
// when the original transaction was created.
//
// The caller is responsible for building balance operations (via
// buildBalanceOperations) and validating accounting rules (via
// query.ValidateAccountingRules) before calling this method.
//
// Returns the before/after balance snapshots for operation building and
// PostgreSQL persistence.
func (uc *UseCase) ProcessBalanceOperations(ctx context.Context, input ProcessBalanceOperationsInput) (*mmodel.BalanceAtomicResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.process_balance_operations")
	defer span.End()

	skipBalanceValidation := input.TransactionInput == nil

	span.SetAttributes(
		attribute.String("app.organization_id", input.OrganizationID.String()),
		attribute.String("app.ledger_id", input.LedgerID.String()),
		attribute.String("app.transaction_id", input.TransactionID.String()),
		attribute.String("app.transaction_status", input.TransactionStatus),
		attribute.Int("app.balance_operations_count", len(input.BalanceOperations)),
		attribute.Bool("app.skip_balance_validation", skipBalanceValidation),
		attribute.Bool("app.request.account_block_exception_presented", input.AccountBlockExceptionGrant != nil),
	)

	// Bind a presented account-block exception to the ONE logical debit of this
	// batch it authorizes: the primary debit leg on the granted alias plus the
	// overdraft companion legs the system derived from it. Resolving it HERE, once,
	// is what keeps the Go pre-validation and the atomic script agreeing on exactly
	// which balances the grant covers.
	//
	// A bind that cannot be made rejects before the script runs, so the identifier
	// is not consumed and stays available for the transaction it was minted for.
	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(
		input.AccountBlockExceptionGrant,
		accountBlockExceptionLegs(input.BalanceOperations),
	)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account block exception does not bind to a debit of this transaction", err)
		logger.Log(ctx, libLog.LevelWarn, "Account block exception does not bind to a debit of this transaction", libLog.Err(err))

		return nil, err
	}

	span.SetAttributes(attribute.Int("app.account_block_exception_bypassed_balances", len(binding.InternalKeys())))

	// Validate balance rules (eligibility, asset codes, sending/receiving permissions).
	// Skipped for state transitions where rules were enforced on the original transaction.
	if !skipBalanceValidation {
		txBalances, err := deduplicateBalances(input.BalanceOperations)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Corrupted balance OverdraftLimit", err)
			logger.Log(ctx, libLog.LevelError, "Corrupted balance OverdraftLimit during deduplication", libLog.Err(err))

			return nil, err
		}

		if err := mtransaction.ValidateBalancesRules(
			ctx,
			*input.TransactionInput,
			*input.Validate,
			txBalances,
			binding,
		); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance rule validation failed", err)

			// Business validation failure (caller error) — warn, not error.
			logger.Log(ctx, libLog.LevelWarn, "Balance rule validation failed", libLog.Err(err))

			return nil, err
		}
	}

	// Execute the atomic Lua script that mutates balances in Redis.
	result, err := uc.TransactionRedisRepo.ProcessBalanceAtomicOperation(
		ctx,
		input.OrganizationID,
		input.LedgerID,
		input.TransactionID,
		input.TransactionStatus,
		input.Validate.Pending,
		input.BalanceOperations,
		binding,
	)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to execute atomic balance operation", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute atomic balance operation", libLog.Err(err))

		return nil, err
	}

	return result, nil
}

// deduplicateBalances extracts unique balances from operations by alias.
//
// When double-entry splitting is active, a single source account produces two
// balance operations (e.g. DEBIT + ONHOLD for PENDING) sharing the same alias.
// ValidateBalancesRules expects one balance per alias — the count check
// len(balances) == len(validate.From) + len(validate.To) relies on this
// deduplication. This invariant holds because validate.From/To maps use the
// composite alias as key (one entry per account), not per split operation.
//
// Returns an error when any balance has a corrupted OverdraftLimit string,
// so the caller can fail the transaction rather than silently treat the
// limit as zero — see Balance.ToTransactionBalance.
func deduplicateBalances(operations []mmodel.BalanceOperation) ([]*mtransaction.Balance, error) {
	seen := make(map[string]bool, len(operations))
	balances := make([]*mtransaction.Balance, 0, len(operations))

	for _, bo := range operations {
		if seen[bo.Alias] {
			continue
		}

		seen[bo.Alias] = true

		txBal, err := bo.Balance.ToTransactionBalance()
		if err != nil {
			return nil, fmt.Errorf("corrupted balance %s: %w", bo.Balance.ID, err)
		}

		balances = append(balances, txBal)
	}

	return balances, nil
}

// accountBlockExceptionLegs projects the batch onto what the binding resolution
// reads. It lives here rather than in mtransaction because that package is
// imported BY the model and so cannot import it back.
//
// Operations without a balance are skipped: they carry no alias or balance key to
// bind against, and counting one would bind a grant to a leg with no balance to
// bypass the block on.
func accountBlockExceptionLegs(operations []mmodel.BalanceOperation) []mtransaction.AccountBlockExceptionLeg {
	legs := make([]mtransaction.AccountBlockExceptionLeg, 0, len(operations))

	for _, op := range operations {
		if op.Balance == nil {
			continue
		}

		legs = append(legs, mtransaction.AccountBlockExceptionLeg{
			Alias:       op.Balance.Alias,
			BalanceKey:  op.Balance.Key,
			EntryKey:    op.Alias,
			Direction:   op.Amount.Direction,
			Amount:      op.Amount.Value.String(),
			InternalKey: op.InternalKey,
		})
	}

	return legs
}
