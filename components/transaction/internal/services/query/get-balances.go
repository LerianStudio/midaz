package query

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// GetBalances methods responsible to get balances.
func (uc *UseCase) GetBalances(ctx context.Context, logger mlog.Logger, organizationID, ledgerID uuid.UUID, input []string) ([]*mmodel.Balance, error) {
	span := trace.SpanFromContext(ctx)

	var ids []uuid.UUID

	var aliases []string

	for _, item := range input {
		if pkg.IsUUID(item) {
			ids = append(ids, uuid.MustParse(item))
		} else {
			aliases = append(aliases, item)
		}
	}

	var balances []*mmodel.Balance

	if len(ids) > 0 {
		balancesByIDs, err := uc.BalanceRepo.ListByAccountIDs(ctx, organizationID, ledgerID, ids)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get balances", err)

			logger.Error("Failed to get balances on database", err.Error())

			return nil, err
		}

		balances = append(balances, balancesByIDs...)
	}

	if len(aliases) > 0 {
		balancesByAliases, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by alias gRPC on Ledger", err)

			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())

			return nil, err
		}

		balances = append(balances, balancesByAliases...)
	}

	return balances, nil
}
