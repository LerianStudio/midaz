package query

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// GetAccountsLedger methods responsible to get accounts on ledger by gRpc.
func (uc *UseCase) GetAccountsLedger(ctx context.Context, logger mlog.Logger, token string, organizationID, ledgerID uuid.UUID, input []string) ([]*account.Account, error) {
	span := trace.SpanFromContext(ctx)

	var ids []string

	var aliases []string

	for _, item := range input {
		if pkg.IsUUID(item) {
			ids = append(ids, item)
		} else {
			aliases = append(aliases, item)
		}
	}

	var accounts []*account.Account

	if len(ids) > 0 {
		gRPCAccounts, err := uc.AccountGRPCRepo.GetAccountsByIds(ctx, token, organizationID, ledgerID, ids)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by ids gRPC on Ledger", err)

			logger.Error("Failed to get account gRPC by ids on Ledger", err.Error())

			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	if len(aliases) > 0 {
		gRPCAccounts, err := uc.AccountGRPCRepo.GetAccountsByAlias(ctx, token, organizationID, ledgerID, aliases)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by alias gRPC on Ledger", err)

			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())

			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	return accounts, nil
}
