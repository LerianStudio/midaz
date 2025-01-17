package command

import (
	"context"

	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateAccounts methods that is responsible to update accounts on ledger by gRpc.
func (uc *UseCase) UpdateAccounts(ctx context.Context, logger mlog.Logger, validate goldModel.Responses, token string, organizationID, ledgerID uuid.UUID, accounts []*account.Account) error {
	span := trace.SpanFromContext(ctx)

	e := make(chan error)
	result := make(chan []*account.Account)

	var accountsToUpdate []*account.Account

	go goldModel.UpdateAccounts(constant.DEBIT, validate.From, accounts, result, e)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&span, "Failed to update debit accounts", err)

		return err
	}

	go goldModel.UpdateAccounts(constant.CREDIT, validate.To, accounts, result, e)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&span, "Failed to update credit accounts", err)

		return err
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload_grpc_update_accounts", accountsToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert accountsToUpdate from struct to JSON string", err)

		return err
	}

	acc, err := uc.AccountGRPCRepo.UpdateAccounts(ctx, token, organizationID, ledgerID, accountsToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update accounts gRPC on Ledger", err)

		logger.Error("Failed to update accounts gRPC on Ledger", err.Error())

		return err
	}

	for _, a := range acc.Accounts {
		logger.Infof(a.UpdatedAt)
	}

	return nil
}
