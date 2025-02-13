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

	result := make(chan []*mmodel.Balance)

	var balancesToUpdate []*mmodel.Balance

	go goldModel.UpdateBalances(constant.DEBIT, validate.From, balances, result)
	rDebit := <-result
	balancesToUpdate = append(balancesToUpdate, rDebit...)

	go goldModel.UpdateBalances(constant.CREDIT, validate.To, balances, result)
	rCredit := <-result
	balancesToUpdate = append(balancesToUpdate, rCredit...)

	err = uc.BalanceRepo.SelectForUpdate(ctxProcessBalances, organizationID, ledgerID, balancesToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)

		logger.Error("Failed to update balances on database", err.Error())

		return err
	}

	spanUpdateBalances.End()

	return nil
}
