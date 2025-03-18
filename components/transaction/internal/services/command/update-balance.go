package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances")
	defer spanUpdateBalances.End()

	// Record operation metrics
	uc.RecordBalanceMetric(ctx, "balances_update_batch_attempt", "batch-update",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("balance_count", len(balances)))

	err := mopentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)

		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())

		// Record error
		uc.RecordEntityError(ctx, "balance", "balances_struct_conversion_error", "batch-update",
			attribute.String("error_detail", err.Error()))
	}

	fromTo := make(map[string]goldModel.Amount)
	for k, v := range validate.From {
		fromTo[k] = goldModel.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.DEBIT,
		}
	}

	for k, v := range validate.To {
		fromTo[k] = goldModel.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.CREDIT,
		}
	}

	err = uc.BalanceRepo.SelectForUpdate(ctxProcessBalances, organizationID, ledgerID, validate.Aliases, fromTo)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)

		logger.Error("Failed to update balances on database", err.Error())

		// Record error
		uc.RecordEntityError(ctx, "balance", "balances_update_error", "batch-update",
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.RecordTransactionDuration(ctx, startTime, "balances_update_batch", "error", "batch-update",
			attribute.String("error", "database_update_failed"))

		return err
	}

	// Record transaction duration with success status
	uc.RecordTransactionDuration(ctx, startTime, "balances_update_batch", "success", "batch-update",
		attribute.Int("balance_count", len(balances)))

	// Record business metric for successful balance update
	uc.RecordBalanceMetric(ctx, "balances_update_batch_success", "batch-update",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("balance_count", len(balances)))

	return nil
}

// UpdateBalancesNew func that is responsible to update balances.
func (uc *UseCase) UpdateBalancesNew(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	// Record operation metrics
	uc.RecordBalanceMetric(ctx, "balances_update_new_attempt", "batch-update",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("balance_count", len(balances)))

	err := mopentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)

		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())

		// Record error
		uc.RecordEntityError(ctx, "balance", "balances_struct_conversion_error", "batch-update",
			attribute.String("error_detail", err.Error()))
	}

	fromTo := make(map[string]goldModel.Amount)
	for k, v := range validate.From {
		fromTo[k] = goldModel.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.DEBIT,
		}
	}

	for k, v := range validate.To {
		fromTo[k] = goldModel.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.CREDIT,
		}
	}

	newBalances := make([]*mmodel.Balance, 0)

	for _, balance := range balances {
		calculateBalances := goldModel.OperateBalances(fromTo[balance.Alias],
			goldModel.Balance{
				Scale:     balance.Scale,
				Available: balance.Available,
				OnHold:    balance.OnHold,
			},
			fromTo[balance.Alias].Operation)

		newBalances = append(newBalances, &mmodel.Balance{
			ID:        balance.ID,
			Alias:     balance.Alias,
			Scale:     calculateBalances.Scale,
			Available: calculateBalances.Available,
			OnHold:    calculateBalances.OnHold,
			Version:   balance.Version + 1,
		})
	}

	err = uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)

		logger.Error("Failed to update balances on database", err.Error())

		// Record error
		uc.RecordEntityError(ctx, "balance", "balances_update_error", "batch-update",
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.RecordTransactionDuration(ctx, startTime, "balances_update_new", "error", "batch-update",
			attribute.String("error", "database_update_failed"))

		return err
	}

	// Record transaction duration with success status
	uc.RecordTransactionDuration(ctx, startTime, "balances_update_new", "success", "batch-update",
		attribute.Int("balance_count", len(newBalances)))

	// Record business metric for successful balance update
	uc.RecordBalanceMetric(ctx, "balances_update_new_success", "batch-update",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("balance_count", len(newBalances)))

	return nil
}

// Update balance in the repository.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	// Record operation metrics
	uc.RecordBalanceMetric(ctx, "balance_update_attempt", balanceID.String(),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to update balance")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update balance on repo", err)

		logger.Errorf("Error update balance: %v", err)

		// Record error
		uc.RecordEntityError(ctx, "balance", "balance_update_error", balanceID.String(),
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.RecordTransactionDuration(ctx, startTime, "balance_update", "error", balanceID.String(),
			attribute.String("error", "database_update_failed"))

		return err
	}

	// Record transaction duration with success status
	uc.RecordTransactionDuration(ctx, startTime, "balance_update", "success", balanceID.String())

	// Record business metric for successful balance update
	uc.RecordBalanceMetric(ctx, "balance_update_success", balanceID.String(),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	return nil
}
