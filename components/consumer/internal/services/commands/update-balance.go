package commands

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate libTransaction.Responses, balances []*mmodel.Balance) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "commands.update_balances_new")
	defer spanUpdateBalances.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&spanUpdateBalances, "payload_update_balances", balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to convert balances from struct to JSON string", err)
		logger.Errorf("Failed to convert balances from struct to JSON string: %v", err.Error())

		return err
	}

	fromTo := make(map[string]libTransaction.Amount)
	for k, v := range validate.From {
		fromTo[k] = v
	}

	for k, v := range validate.To {
		fromTo[k] = v
	}

	newBalances := make([]*mmodel.Balance, 0)

	for _, balance := range balances {
		balance.ConvertToLibBalance()
		calculateBalances, err := libTransaction.OperateBalances(fromTo[balance.Alias], *balance.ConvertToLibBalance())

		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return err
		}

		newBalances = append(newBalances, &mmodel.Balance{
			ID:        balance.ID,
			Alias:     balance.Alias,
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