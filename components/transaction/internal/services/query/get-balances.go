package query

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
)

// GetBalances methods responsible to get balances from database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *goldModel.Responses) ([]*mmodel.Balance, error) {
	tracer := pkg.NewTracerFromContext(ctx)
	logger := pkg.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	var ids []uuid.UUID

	var aliases []string

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, newValidateAliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, validate.Aliases)
	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	for _, item := range newValidateAliases {
		if pkg.IsUUID(item) {
			ids = append(ids, uuid.MustParse(item))
		} else {
			aliases = append(aliases, item)
		}
	}

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

	if len(balances) > 1 {
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

// ValidateIfBalanceExistsOnRedis func that validate if balance exists on redis before to get on database.
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	tracer := pkg.NewTracerFromContext(ctx)
	logger := pkg.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Infof("Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := pkg.LockInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if value != "" {
			b := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(value), &b); err != nil {
				mopentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

				logger.Warnf("Error to Deserialization json: %v", err)

				continue
			}

			newBalances = append(newBalances, &mmodel.Balance{
				ID:             b.ID,
				AccountID:      b.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          alias,
				Available:      b.Available,
				OnHold:         b.OnHold,
				Scale:          b.Scale,
				Version:        b.Version,
				AccountType:    b.AccountType,
				AllowSending:   b.AllowSending == 1,
				AllowReceiving: b.AllowReceiving == 1,
				AssetCode:      b.AssetCode,
			})
		} else {
			newAliases = append(newAliases, alias)
		}
	}

	return newBalances, newAliases
}

// GetAccountAndLock func responsible to integrate core business logic to redis.
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *goldModel.Responses, balances []*mmodel.Balance) ([]*mmodel.Balance, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
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

		b.Alias = balance.Alias

		newBalances = append(newBalances, b)
	}

	return newBalances, nil
}
