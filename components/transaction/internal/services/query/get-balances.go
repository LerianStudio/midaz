package query

import (
	"context"
	"encoding/json"
	"sort"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// lockOperation represents a balance operation with associated metadata for transaction processing
type lockOperation struct {
	balance     *mmodel.Balance
	alias       string
	amount      libTransaction.Amount
	internalKey string
	isFrom      bool
}

// GetBalances methods responsible to get balances from a database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *libTransaction.Responses, transactionStatus string) ([]*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_status", transactionStatus),
	)

	if err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", validate); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, aliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, validate.Aliases)
	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	if len(aliases) > 0 {
		balancesByAliases, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account by alias on balance database", err)

			logger.Error("Failed to get account by alias on balance database", err.Error())

			return nil, err
		}

		balances = append(balances, balancesByAliases...)
	}

	if len(balances) > 1 {
		newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, validate, balances, transactionStatus)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances and update on redis", err)

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
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *libTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) ([]*mmodel.Balance, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	newBalances := make([]*mmodel.Balance, 0)

	operations := make([]lockOperation, 0)

	for _, balance := range balances {
		internalKey := libCommons.TransactionInternalKey(organizationID, ledgerID, balance.Alias)

		for k, v := range validate.From {
			if libTransaction.SplitAlias(k) == balance.Alias {
				operations = append(operations, lockOperation{
					balance:     balance,
					alias:       k,
					amount:      v,
					internalKey: internalKey,
					isFrom:      true,
				})
			}
		}

		for k, v := range validate.To {
			if libTransaction.SplitAlias(k) == balance.Alias {
				operations = append(operations, lockOperation{
					balance:     balance,
					alias:       k,
					amount:      v,
					internalKey: internalKey,
					isFrom:      false,
				})
			}
		}
	}

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].internalKey < operations[j].internalKey
	})

	err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate accounting rules", err)

		logger.Error("Failed to validate accounting rules", err)

		return nil, err
	}

	// Prepare batch operation parameters for atomic execution
	if len(operations) == 0 {
		return newBalances, nil
	}

	// Collect all keys, amounts, and balances for batch operation
	keys := make([]string, 0, len(operations))
	amounts := make([]libTransaction.Amount, 0, len(operations))
	balanceList := make([]mmodel.Balance, 0, len(operations))
	aliasMap := make(map[int]string) // Map index to alias for result processing

	for i, op := range operations {
		keys = append(keys, op.internalKey)
		amounts = append(amounts, op.amount)
		balanceList = append(balanceList, *op.balance)
		aliasMap[i] = op.alias
	}

	// Execute atomic batch operation - all succeed or all fail
	results, err := uc.RedisRepo.AddSumBalancesAtomicRedis(ctx, keys, transactionStatus, validate.Pending, amounts, balanceList)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to execute atomic balance operations", err)
		logger.Error("Failed to execute atomic balance operations", err)
		return nil, err
	}

	// Process results and restore aliases
	for i, result := range results {
		if result != nil {
			result.Alias = aliasMap[i]
			newBalances = append(newBalances, result)
		}
	}

	return newBalances, nil
}
