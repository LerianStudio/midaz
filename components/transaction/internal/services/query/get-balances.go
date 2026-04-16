// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// ErrNilValidatePayload is returned when the transaction validate payload is nil.
var ErrNilValidatePayload = errors.New("invalid transaction payload: validate is nil")

// ErrNilBalance is returned when a balance in the transaction payload is nil.
var ErrNilBalance = errors.New("invalid transaction payload: nil balance")

// GetBalances methods responsible to get balances from a database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	if validate == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate payload", ErrNilValidatePayload)
		logger.Error("Failed to validate payload", ErrNilValidatePayload)

		return nil, ErrNilValidatePayload
	}

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, aliases, err := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to resolve balance shard while checking redis cache", err)

		logger.Error("Failed to resolve balance shard while checking redis cache", err)

		return nil, err
	}

	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	if len(aliases) > 0 {
		recoveredBalances, remainingAliases, recoveryErr := uc.recoverLaggedBalancesForAliases(ctx, organizationID, ledgerID, aliases)
		if recoveryErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to replay lagged balance operations", recoveryErr)

			logger.Error("Failed to replay lagged balance operations", recoveryErr.Error())

			return nil, recoveryErr
		}

		if len(recoveredBalances) > 0 {
			balances = append(balances, recoveredBalances...)
		}

		aliases = remainingAliases
	}

	if len(aliases) > 0 {
		if err := uc.ensureConsumerLagFenceForAliases(ctx, organizationID, ledgerID, aliases); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Consumer lag fence blocked stale balance recovery", err)

			logger.Error("Consumer lag fence blocked stale balance recovery", err)

			return nil, err
		}

		if err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, aliases); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to ensure external pre-split balances", err)

			logger.Error("Failed to ensure external pre-split balances", err)

			return nil, err
		}

		balancesByAliases, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account by alias on balance database", err)

			logger.Error("Failed to get account by alias on balance database", err)

			return nil, err
		}

		balances = append(balances, balancesByAliases...)
	}

	newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, transactionInput, validate, balances, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances and update on redis", err)

		logger.Error("Failed to get balances and update on redis", err)

		return nil, err
	}

	return newBalances, nil
}

// ValidateIfBalanceExistsOnRedis func that validate if balance exists on redis before to get on database.
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Infof("Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		var internalKey string

		if uc.ShardRouter != nil {
			accountAlias, balanceKey := shard.SplitAliasAndBalanceKey(alias)

			shardID, shardErr := uc.resolveBalanceShard(ctx, organizationID, ledgerID, accountAlias, balanceKey)
			if shardErr != nil {
				// Fail-open: fall back to the non-sharded (legacy) key so the read
				// path is not blocked by transient shard resolution issues. The
				// balance will be looked up in PostgreSQL if it is not found in
				// Redis under the legacy key.
				logger.Warnf("Shard resolution failed for alias %s, falling back to non-sharded key: %v", alias, shardErr)

				internalKey = utils.BalanceInternalKey(organizationID, ledgerID, alias)
			} else {
				internalKey = utils.BalanceShardKey(shardID, organizationID, ledgerID, alias)
			}
		} else {
			internalKey = utils.BalanceInternalKey(organizationID, ledgerID, alias)
		}

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if !libCommons.IsNilOrEmpty(&value) {
			b := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(value), &b); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

				logger.Warnf("Error to Deserialization json: %v", err)

				continue
			}

			balanceAlias, balanceKey := shard.SplitAliasAndBalanceKey(alias)

			if b.Key != "" {
				balanceKey = b.Key
			}

			newBalances = append(newBalances, &mmodel.Balance{
				ID:             b.ID,
				AccountID:      b.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          balanceAlias,
				Key:            balanceKey,
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

	return newBalances, newAliases, nil
}

// GetAccountAndLock func responsible to integrate core business logic to redis.
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	if validate == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate payload", ErrNilValidatePayload)
		logger.Error("Failed to validate payload", ErrNilValidatePayload)

		return nil, ErrNilValidatePayload
	}

	for i, balance := range balances {
		if balance == nil {
			err := fmt.Errorf("at index %d: %w", i, ErrNilBalance)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate payload", err)
			logger.Error("Failed to validate payload", err)

			return nil, err
		}
	}

	guardAliases := make([]string, 0, len(balances))
	for _, balance := range balances {
		guardAliases = append(guardAliases, balance.Alias)
	}

	if err := uc.waitForMigrationUnlock(ctx, organizationID, ledgerID, guardAliases); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed due to in-progress shard migration", err)

		logger.Error("Failed due to in-progress shard migration", err)

		return nil, err
	}

	// Record in-flight writes against each alias so a concurrent migration's
	// waitForDrainByCounter can observe actual progress instead of sleeping a
	// fixed duration. Paired via defer so cancellation, panics, and early
	// returns all decrement. Use context.Background() on decrement: if the
	// request ctx was cancelled we still must decrement, otherwise the counter
	// stays inflated and blocks future migrations until the 60s TTL expires.
	inflightReleases := uc.trackInFlightWrites(ctx, organizationID, ledgerID, guardAliases)
	defer inflightReleases()

	balanceOperations, mapBalances, err := uc.buildBalanceOperations(ctx, organizationID, ledgerID, balances, validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to build balance operations", err)

		logger.Error("Failed to build balance operations", err)

		return nil, err
	}

	sort.Slice(balanceOperations, func(i, j int) bool {
		return balanceOperations[i].InternalKey < balanceOperations[j].InternalKey
	})

	err = uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOperations, validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate accounting rules", err)

		logger.Error("Failed to validate accounting rules", err)

		return nil, err
	}

	if transactionInput != nil {
		txBalances := make([]*pkgTransaction.Balance, 0, len(balanceOperations))
		for _, bo := range balanceOperations {
			txBalances = append(txBalances, bo.Balance.ToTransactionBalance())
		}

		if err = pkgTransaction.ValidateBalancesRules(
			ctx,
			*transactionInput,
			*validate,
			txBalances,
		); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate balances", err)

			logger.Errorf("Failed to validate balances: %v", err)

			return nil, err
		}
	}

	uc.recordShardLoad(ctx, organizationID, ledgerID, balanceOperations)

	if uc.Authorizer != nil && uc.Authorizer.Enabled() {
		newBalances, err := uc.processAuthorizerAtomicOperation(
			ctx,
			organizationID,
			ledgerID,
			transactionID,
			transactionStatus,
			validate.Pending,
			balanceOperations,
			mapBalances,
		)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to authorize balances", err)

			logger.Error("Failed to authorize balances", err)

			return nil, err
		}

		return newBalances, nil
	}

	newBalances, err := uc.RedisRepo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to lock balance", err)

		logger.Error("Failed to lock balance", err)

		return nil, err
	}

	return newBalances, nil
}

// buildBalanceOperations maps balances to their corresponding operations from the validate
// response, resolving shard-aware Redis keys for each balance.
func (uc *UseCase) buildBalanceOperations(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	balances []*mmodel.Balance,
	validate *pkgTransaction.Responses,
) ([]mmodel.BalanceOperation, map[string]*mmodel.Balance, error) {
	balanceOperations := make([]mmodel.BalanceOperation, 0)
	mapBalances := make(map[string]*mmodel.Balance)

	for _, balance := range balances {
		aliasKey := balance.Alias + "#" + balance.Key

		// Generate shard-aware or legacy Redis key based on router availability
		var internalKey string

		var shardID int

		if uc.ShardRouter != nil {
			resolvedShardID, shardErr := uc.resolveBalanceShard(ctx, organizationID, ledgerID, balance.Alias, balance.Key)
			if shardErr != nil {
				return nil, nil, shardErr
			}

			shardID = resolvedShardID
			internalKey = utils.BalanceShardKey(shardID, organizationID, ledgerID, aliasKey)
		} else {
			internalKey = utils.BalanceInternalKey(organizationID, ledgerID, aliasKey)
		}

		for k, v := range validate.From {
			if pkgTransaction.SplitAliasWithKey(k) == aliasKey {
				mapBalances[k] = balance

				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
					ShardID:     shardID,
				})
			}
		}

		for k, v := range validate.To {
			if pkgTransaction.SplitAliasWithKey(k) == aliasKey {
				mapBalances[k] = balance

				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
					ShardID:     shardID,
				})
			}
		}
	}

	return balanceOperations, mapBalances, nil
}

func (uc *UseCase) ensureExternalPreSplitBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) error {
	if uc.ShardRouter == nil || len(aliases) == 0 {
		return nil
	}

	allowedExternalShardIDs, err := uc.collectAllowedExternalShardIDs(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		return err
	}

	keysByAlias := uc.collectExternalKeysByAlias(aliases, allowedExternalShardIDs)
	if len(keysByAlias) == 0 {
		return nil
	}

	defaultAliases := make([]string, 0, len(keysByAlias))
	for alias := range keysByAlias {
		defaultAliases = append(defaultAliases, alias+"#"+constant.DefaultBalanceKey)
	}

	templateByAlias, err := uc.fetchTemplatesByAlias(ctx, organizationID, ledgerID, defaultAliases)
	if err != nil {
		return err
	}

	return uc.materializePreSplitBalances(ctx, keysByAlias, templateByAlias)
}

// collectAllowedExternalShardIDs resolves shard IDs for all non-external aliases,
// returning the set of shard IDs that external pre-split balances are allowed to target.
func (uc *UseCase) collectAllowedExternalShardIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) (map[int]struct{}, error) {
	allowed := make(map[int]struct{})

	for _, aliasWithKey := range aliases {
		alias, balanceKey := shard.SplitAliasAndBalanceKey(aliasWithKey)
		if shard.IsExternal(alias) {
			continue
		}

		shardID, err := uc.resolveBalanceShard(ctx, organizationID, ledgerID, alias, balanceKey)
		if err != nil {
			return nil, err
		}

		allowed[shardID] = struct{}{}
	}

	return allowed, nil
}

// collectExternalKeysByAlias filters aliases to only external pre-split entries whose
// shard IDs are valid and present in the allowed set, grouping balance keys by alias.
func (uc *UseCase) collectExternalKeysByAlias(aliases []string, allowedShardIDs map[int]struct{}) map[string]map[string]struct{} {
	keysByAlias := make(map[string]map[string]struct{})

	for _, aliasWithKey := range aliases {
		alias, balanceKey := shard.SplitAliasAndBalanceKey(aliasWithKey)
		if !shard.IsExternal(alias) || !shard.IsExternalBalanceKey(balanceKey) {
			continue
		}

		shardID, parsed := shard.ParseExternalBalanceShardID(balanceKey)
		if !parsed || shardID < 0 || shardID >= uc.ShardRouter.ShardCount() {
			continue
		}

		if len(allowedShardIDs) > 0 {
			if _, ok := allowedShardIDs[shardID]; !ok {
				continue
			}
		}

		if keysByAlias[alias] == nil {
			keysByAlias[alias] = make(map[string]struct{})
		}

		keysByAlias[alias][balanceKey] = struct{}{}
	}

	return keysByAlias
}

// fetchTemplatesByAlias retrieves default balances for the given aliases and indexes
// them by alias for quick lookup during materialization.
func (uc *UseCase) fetchTemplatesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, defaultAliases []string) (map[string]*mmodel.Balance, error) {
	templates, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, defaultAliases)
	if err != nil {
		return nil, err
	}

	templateByAlias := make(map[string]*mmodel.Balance, len(templates))
	for _, balance := range templates {
		if balance == nil || balance.Alias == "" {
			continue
		}

		templateByAlias[balance.Alias] = balance
	}

	return templateByAlias, nil
}

// materializePreSplitBalances creates new balance records for each external pre-split
// key, using the corresponding default balance as a template.
func (uc *UseCase) materializePreSplitBalances(ctx context.Context, keysByAlias map[string]map[string]struct{}, templateByAlias map[string]*mmodel.Balance) error {
	now := time.Now().UTC()

	for alias, keys := range keysByAlias {
		template, ok := templateByAlias[alias]
		if !ok {
			return fmt.Errorf("materialize pre-split balances: %w", pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Balance"))
		}

		for balanceKey := range keys {
			newBalance := &mmodel.Balance{
				ID:             libCommons.GenerateUUIDv7().String(),
				OrganizationID: template.OrganizationID,
				LedgerID:       template.LedgerID,
				AccountID:      template.AccountID,
				Alias:          template.Alias,
				Key:            balanceKey,
				AssetCode:      template.AssetCode,
				Available:      decimal.Zero,
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    template.AccountType,
				AllowSending:   template.AllowSending,
				AllowReceiving: template.AllowReceiving,
				CreatedAt:      now,
				UpdatedAt:      now,
			}

			if err := uc.BalanceRepo.CreateIfNotExists(ctx, newBalance); err != nil {
				return fmt.Errorf("failed to materialize external pre-split balance %s#%s: %w", alias, balanceKey, err)
			}
		}
	}

	return nil
}
