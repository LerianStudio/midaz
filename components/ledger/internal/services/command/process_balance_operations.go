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
)

// ProcessBalanceOperations validates balance rules and executes the atomic Lua
// script that mutates balances in Redis.
//
// The caller is responsible for building balance operations (via
// BuildBalanceOperations) and validating accounting rules (via
// query.ValidateAccountingRules) before calling this method.
//
// Returns the before/after balance snapshots for operation building and
// PostgreSQL persistence.
func (uc *UseCase) ProcessBalanceOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balanceOperations []mmodel.BalanceOperation, transactionStatus string) (*mmodel.BalanceAtomicResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.process_balance_operations")
	defer span.End()

	// Validate balance rules (eligibility, asset codes, sending/receiving permissions).
	if transactionInput != nil {
		seen := make(map[string]bool)
		txBalances := make([]*pkgTransaction.Balance, 0, len(balanceOperations))

		for _, bo := range balanceOperations {
			if seen[bo.Alias] {
				continue
			}

			seen[bo.Alias] = true

			txBalances = append(txBalances, bo.Balance.ToTransactionBalance())
		}

		if err := pkgTransaction.ValidateBalancesRules(
			ctx,
			*transactionInput,
			*validate,
			txBalances,
		); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate balances", err)
			logger.Log(ctx, libLog.LevelError, "Failed to validate balances", libLog.Err(err))

			return nil, err
		}
	}

	// Execute the atomic Lua script that mutates balances in Redis.
	result, err := uc.TransactionRedisRepo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to execute atomic balance operation", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute atomic balance operation", libLog.Err(err))

		return nil, err
	}

	return result, nil
}
