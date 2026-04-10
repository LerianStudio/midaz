// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// ProcessBalanceOperationsInput groups all parameters required by
// ProcessBalanceOperations. Using a struct instead of positional args
// improves readability, makes future extensions backward-compatible,
// and clearly separates "own" fields from pass-through values.
type ProcessBalanceOperationsInput struct {
	OrganizationID    uuid.UUID
	LedgerID          uuid.UUID
	TransactionID     uuid.UUID
	TransactionInput  *pkgTransaction.Transaction // nil skips balance-rule validation (state transitions)
	Validate          *pkgTransaction.Responses
	BalanceOperations []mmodel.BalanceOperation
	TransactionStatus string
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
	)

	// Validate balance rules (eligibility, asset codes, sending/receiving permissions).
	// Skipped for state transitions where rules were enforced on the original transaction.
	if !skipBalanceValidation {
		txBalances := deduplicateBalances(input.BalanceOperations)

		if err := pkgTransaction.ValidateBalancesRules(
			ctx,
			*input.TransactionInput,
			*input.Validate,
			txBalances,
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
func deduplicateBalances(operations []mmodel.BalanceOperation) []*pkgTransaction.Balance {
	seen := make(map[string]bool, len(operations))
	balances := make([]*pkgTransaction.Balance, 0, len(operations))

	for _, bo := range operations {
		if seen[bo.Alias] {
			continue
		}

		seen[bo.Alias] = true

		balances = append(balances, bo.Balance.ToTransactionBalance())
	}

	return balances
}
