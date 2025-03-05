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

	// Extract UUIDs and aliases for balance updating
	var uuids []uuid.UUID
	var aliases []string
	
	for _, item := range validate.Aliases {
		if pkg.IsUUID(item) {
			uuids = append(uuids, uuid.MustParse(item))
		} else {
			aliases = append(aliases, item)
		}
	}
	
	// Update the balances
	err = uc.BalanceRepo.SelectForUpdate(ctxProcessBalances, organizationID, ledgerID, aliases, uuids, fromTo)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)

		logger.Error("Failed to update balances on database", err.Error())

		return err
	}

	spanUpdateBalances.End()

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
