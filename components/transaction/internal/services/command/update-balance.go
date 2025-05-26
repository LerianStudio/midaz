package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// SelectForUpdateBalances func that is responsible to select for update balances.
func (uc *UseCase) SelectForUpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate libTransaction.Responses, balances []*mmodel.Balance) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances")
	defer spanUpdateBalances.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)
		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())

		return err
	}

	fromTo := make(map[string]libTransaction.Amount)
	for k, v := range validate.From {
		fromTo[k] = libTransaction.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.DEBIT,
		}
	}

	for k, v := range validate.To {
		fromTo[k] = libTransaction.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.CREDIT,
		}
	}

	err = uc.BalanceRepo.SelectForUpdate(ctxProcessBalances, organizationID, ledgerID, validate.Aliases, fromTo)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return err
	}

	return nil
}

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate libTransaction.Responses, balances []*mmodel.Balance) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)
		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())

		return err
	}

	fromTo := make(map[string]libTransaction.Amount)
	for k, v := range validate.From {
		fromTo[k] = libTransaction.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.DEBIT,
		}
	}

	for k, v := range validate.To {
		fromTo[k] = libTransaction.Amount{
			Asset:     v.Asset,
			Value:     v.Value,
			Scale:     v.Scale,
			Operation: constant.CREDIT,
		}
	}

	newBalances := make([]*mmodel.Balance, 0)

	for _, balance := range balances {
		calculateBalances, err := libTransaction.OperateBalances(fromTo[balance.Alias],
			libTransaction.Balance{
				Scale:     balance.Scale,
				Available: balance.Available,
				OnHold:    balance.OnHold,
			},
			fromTo[balance.Alias].Operation)

		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return err
		}

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
		libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return err
	}

	return nil
}

// Update balance in the repository.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update balance on repo", err)
		logger.Errorf("Error update balance: %v", err)

		return err
	}

	return nil
}
