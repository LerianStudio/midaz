package command

import (
	"context"

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

	// Create a batch balance operation telemetry entity
	op := uc.Telemetry.NewBalanceOperation("balances_update_batch", "batch-update")

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("balance_count", len(balances)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	err := mopentelemetry.SetSpanAttributesFromStruct(&op.span, "payload_update_balances", balances)
	if err != nil {
		// Record error but continue
		op.RecordError(ctx, "balances_struct_conversion_error", err)
		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())
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

	err = uc.BalanceRepo.SelectForUpdate(ctx, organizationID, ledgerID, validate.Aliases, fromTo)
	if err != nil {
		// Record error
		op.RecordError(ctx, "balances_update_error", err)
		op.End(ctx, "failed")

		logger.Error("Failed to update balances on database", err.Error())

		return err
	}

	// Record business metrics if needed
	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}

// UpdateBalancesNew func that is responsible to update balances.
func (uc *UseCase) UpdateBalancesNew(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a batch balance operation telemetry entity
	op := uc.Telemetry.NewBalanceOperation("balances_update_new", "batch-update")

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("balance_count", len(balances)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	err := mopentelemetry.SetSpanAttributesFromStruct(&op.span, "payload_update_balances", balances)
	if err != nil {
		// Record error but continue
		op.RecordError(ctx, "balances_struct_conversion_error", err)
		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())
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

	err = uc.BalanceRepo.BalancesUpdate(ctx, organizationID, ledgerID, newBalances)
	if err != nil {
		// Record error
		op.RecordError(ctx, "balances_update_error", err)
		op.End(ctx, "failed")

		logger.Error("Failed to update balances on database", err.Error())

		return err
	}

	// Add new balance count to telemetry
	op.WithAttributes(
		attribute.Int("new_balance_count", len(newBalances)),
	)

	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}

// Update balance in the repository.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a balance operation telemetry entity
	op := uc.Telemetry.NewBalanceOperation("update", balanceID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to update balance")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		// Record error
		op.RecordError(ctx, "balance_update_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Error update balance: %v", err)

		return err
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}
