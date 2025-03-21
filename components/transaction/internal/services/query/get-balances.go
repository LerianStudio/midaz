package query

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// GetBalances methods responsible to get balances from database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *libTransaction.Responses) ([]*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, aliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, validate.Aliases)
	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	if len(aliases) > 0 {
		balancesByAliases, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get account by alias on balance database", err)

			logger.Error("Failed to get account by alias on balance database", err.Error())

			return nil, err
		}

		balances = append(balances, balancesByAliases...)
	}

	if len(balances) > 1 {
		newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, validate, balances)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get balances and update on redis", err)

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
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Infof("Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := libCommons.LockInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if value != "" {
			b := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(value), &b); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

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
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *libTransaction.Responses, balances []*mmodel.Balance) ([]*mmodel.Balance, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	newBalances := make([]*mmodel.Balance, 0)

	for _, balance := range balances {
		internalKey := libCommons.LockInternalKey(organizationID, ledgerID, balance.Alias)

		operation := constant.CREDIT

		amount := libTransaction.Amount{}
		if from, exists := validate.From[balance.Alias]; exists {
			amount = libTransaction.Amount{
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
			libOpentelemetry.HandleSpanError(&span, "Failed to lock balance", err)

			logger.Error("Failed to lock balance", err)

			return nil, err
		}

		b.Alias = balance.Alias

		newBalances = append(newBalances, b)
	}

	return newBalances, nil
}
