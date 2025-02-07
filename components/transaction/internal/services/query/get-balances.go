package query

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

// GetBalances methods responsible to get balances.
func (uc *UseCase) GetBalances(ctx context.Context, logger mlog.Logger, organizationID, ledgerID uuid.UUID, validate *goldModel.Responses) ([]*mmodel.Balance, error) {
	span := trace.SpanFromContext(ctx)

	var ids []uuid.UUID

	var aliases []string

	for _, item := range validate.Aliases {
		if pkg.IsUUID(item) {
			ids = append(ids, uuid.MustParse(item))
		} else {
			aliases = append(aliases, item)
		}
	}

	balances := make([]*mmodel.Balance, 0)

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

	if len(balances) != 0 {
		newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, validate, balances)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get balances and update on redis", err)

			logger.Error("Failed to get balances and update on redis", err.Error())

			return nil, err
		}

		if len(newBalances) != 0 {
			return newBalances, nil
		}
	}

	return balances, nil
}

func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *goldModel.Responses, balances []*mmodel.Balance) ([]*mmodel.Balance, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_account_and_lock")
	defer span.End()

	newBalances := make([]*mmodel.Balance, 0)

	for _, balance := range balances {
		internalKey := pkg.LockInternalKey(organizationID, ledgerID, balance.Alias)

		operation := constant.CREDIT

		amount := goldModel.Amount{}
		if from, exists := validate.From[balance.Alias]; exists {
			amount = goldModel.Amount{
				Asset: from.Asset,
				Value: from.Value,
				Scale: from.Scale,
			}
			operation = constant.DEBIT
		}

		if to, exists := validate.To[balance.Alias]; exists {
			amount = to
		}

		logger.Infof("Getting internal key: %s", internalKey)

		b, err := uc.RedisRepo.LockBalanceRedis(ctx, internalKey, *balance, amount, operation)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to lock balance", err)

			logger.Error("Failed to lock balance", err)

			return nil, err
		}

		newBalances = append(newBalances, b)

	}

	return newBalances, nil
}
