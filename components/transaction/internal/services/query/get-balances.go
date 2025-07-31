package query

import (
	"context"
	"encoding/json"
	"sort"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetBalances methods responsible to get balances from a database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, validate *libTransaction.Responses, transactionStatus string, parser libTransaction.Transaction) ([]*mmodel.Balance, error) {
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
		newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, validate, balances, transactionStatus, parser)
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
		internalKey := libCommons.TransactionInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if !libCommons.IsNilOrEmpty(&value) {
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
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, validate *libTransaction.Responses, balances []*mmodel.Balance, transactionStatus string, parser libTransaction.Transaction) ([]*mmodel.Balance, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		internalKey := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), balance.Alias)

		for k, v := range validate.From {
			if libTransaction.SplitAlias(k) == balance.Alias {
				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
				})
			}
		}

		for k, v := range validate.To {
			if libTransaction.SplitAlias(k) == balance.Alias {
				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
				})
			}
		}
	}

	sort.Slice(balanceOperations, func(i, j int) bool {
		return balanceOperations[i].InternalKey < balanceOperations[j].InternalKey
	})

	err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOperations, validate)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate accounting rules", err)

		logger.Error("Failed to validate accounting rules", err)

		return nil, err
	}

	newBalances, err := uc.RedisRepo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations, parser)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to lock balance", err)
		logger.Error("Failed to lock balance", err)

		return nil, err
	}

	return newBalances, nil
}
