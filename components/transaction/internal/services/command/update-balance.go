package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
)

func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances")
	defer spanUpdateBalances.End()

	err := mopentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)

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

	err = uc.BalanceRepo.SelectForUpdate(ctxProcessBalances, organizationID, ledgerID, validate.Aliases, fromTo)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)

		logger.Error("Failed to update balances on database", err.Error())

		return err
	}

	return nil
}

// UpdateBalancesNew func that is responsible to update balances.
func (uc *UseCase) UpdateBalancesNew(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	err := mopentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)

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

	err = uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)

		logger.Error("Failed to update balances on database", err.Error())

		return err
	}

	return nil
}

// Update balance in the repository.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update balance on repo", err)

		logger.Errorf("Error update balance: %v", err)

		return err
	}

	return nil
}
