package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func (uc *UseCase) UpdateBalances(ctx context.Context, logger mlog.Logger, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, balances []*mmodel.Balance) error {
	span := trace.SpanFromContext(ctx)

	result := make(chan []*mmodel.Balance)

	var balancesToUpdate []*mmodel.Balance

	go goldModel.UpdateBalances(constant.DEBIT, validate.From, balances, result)
	rDebit := <-result
	balancesToUpdate = append(balancesToUpdate, rDebit...)

	go goldModel.UpdateBalances(constant.CREDIT, validate.To, balances, result)
	rCredit := <-result
	balancesToUpdate = append(balancesToUpdate, rCredit...)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload_grpc_update_balances", balancesToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert balancesToUpdate from struct to JSON string", err)

		return err
	}

	err = uc.BalanceRepo.SelectForUpdate(ctx, organizationID, ledgerID, balancesToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update balances on database", err)

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
