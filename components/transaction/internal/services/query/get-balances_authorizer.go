// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const (
	authorizerRejectionBalanceNotFound = "BALANCE_NOT_FOUND"
	// authorizerScaleValuesPerOp is the number of scale values extracted per balance operation
	// (amount value, available balance, on-hold balance).
	authorizerScaleValuesPerOp = 3
)

// ErrAuthorizerNotEnabled is returned when the authorizer is accessed but not enabled.
var ErrAuthorizerNotEnabled = errors.New("authorizer is not enabled")

func (uc *UseCase) processAuthorizerAtomicOperation( //nolint:gocyclo,cyclop
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balanceOperations []mmodel.BalanceOperation,
	mapBalances map[string]*mmodel.Balance,
) ([]*mmodel.Balance, error) {
	if uc.Authorizer == nil || !uc.Authorizer.Enabled() {
		return nil, ErrAuthorizerNotEnabled
	}

	scaleValues := make([]decimal.Decimal, 0, len(balanceOperations)*authorizerScaleValuesPerOp)
	for _, op := range balanceOperations {
		if op.Balance == nil {
			continue
		}

		scaleValues = append(scaleValues, op.Amount.Value, op.Balance.Available, op.Balance.OnHold)
	}

	scale := pkgTransaction.MaxScale(scaleValues...)
	if scale < pkgTransaction.DefaultScale {
		scale = pkgTransaction.DefaultScale
	}

	if scale > pkgTransaction.MaxAllowedScale {
		return nil, fmt.Errorf("process authorizer atomic operation: %w", pkg.ValidateBusinessError(constant.ErrPrecisionOverflow, "validateBalance"))
	}

	operations, err := buildAuthorizerOperations(balanceOperations, scale)
	if err != nil {
		return nil, err
	}

	request := &authorizerv1.AuthorizeRequest{
		TransactionId:     transactionID.String(),
		OrganizationId:    organizationID.String(),
		LedgerId:          ledgerID.String(),
		Pending:           pending,
		TransactionStatus: transactionStatus,
		Operations:        operations,
	}

	resp, err := uc.Authorizer.Authorize(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("process authorizer atomic operation: %w", pkg.ValidateBusinessError(constant.ErrGRPCServiceUnavailable, "authorizer"))
	}

	if !resp.GetAuthorized() && resp.GetRejectionCode() == authorizerRejectionBalanceNotFound { //nolint:nestif
		if loadErr := uc.loadAuthorizerBalancesForOperations(ctx, organizationID, ledgerID, balanceOperations); loadErr != nil {
			if isConsumerLagStaleBalanceError(loadErr) {
				return nil, loadErr
			}

			return nil, fmt.Errorf("process authorizer atomic operation: %w", pkg.ValidateBusinessError(constant.ErrGRPCServiceUnavailable, "authorizer"))
		}

		resp, err = uc.Authorizer.Authorize(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("process authorizer atomic operation: %w", pkg.ValidateBusinessError(constant.ErrGRPCServiceUnavailable, "authorizer"))
		}
	}

	if !resp.GetAuthorized() {
		logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)
		logger.Warnf("Authorizer rejected transaction %s: code=%s", transactionID, resp.GetRejectionCode())

		return nil, mapAuthorizerRejection(resp.GetRejectionCode())
	}

	balances := convertAuthorizerSnapshots(resp.GetBalances(), organizationID, ledgerID, mapBalances)

	if uc.RedisRepo != nil {
		uc.cacheAuthorizerBalances(ctx, balanceOperations, balances)
	}

	return balances, nil
}

func (uc *UseCase) loadAuthorizerBalancesForOperations(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	balanceOperations []mmodel.BalanceOperation,
) error {
	shardIDs := make([]int32, 0)
	if uc.ShardRouter != nil {
		uniqueShardIDs := make(map[int32]struct{}, len(balanceOperations))

		for _, op := range balanceOperations {
			if op.Balance == nil {
				continue
			}

			balanceKey := op.Balance.Key
			if balanceKey == "" {
				balanceKey = constant.DefaultBalanceKey
			}

			resolved := uc.ShardRouter.ResolveBalance(op.Balance.Alias, balanceKey)
			if resolved > math.MaxInt32 {
				resolved = 0
			}

			shardID := int32(resolved)
			uniqueShardIDs[shardID] = struct{}{}
		}

		for shardID := range uniqueShardIDs {
			shardIDs = append(shardIDs, shardID)
		}

		sort.Slice(shardIDs, func(i, j int) bool {
			return shardIDs[i] < shardIDs[j]
		})
	}

	if err := uc.ensureConsumerLagFenceForPartitions(ctx, shardIDs); err != nil {
		return err
	}

	_, err := uc.Authorizer.LoadBalances(ctx, &authorizerv1.LoadBalancesRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		ShardIds:       shardIDs,
	})

	return err
}

func isConsumerLagStaleBalanceError(err error) bool {
	var serviceUnavailableErr pkg.ServiceUnavailableError

	if !errors.As(err, &serviceUnavailableErr) {
		return false
	}

	return serviceUnavailableErr.Code == constant.ErrConsumerLagStaleBalance.Error()
}

// cacheAuthorizerBalances writes the authorizer-returned balance snapshots back
// to Redis so that subsequent reads hit the cache with fresh values. Each
// balance is matched to its original operation internal key and serialised as
// BalanceRedis JSON.
func (uc *UseCase) cacheAuthorizerBalances(
	ctx context.Context,
	balanceOperations []mmodel.BalanceOperation,
	balances []*mmodel.Balance,
) {
	keyByOperationAlias := make(map[string]string, len(balanceOperations))
	for _, op := range balanceOperations {
		if op.InternalKey != "" {
			keyByOperationAlias[op.Alias] = op.InternalKey
		}
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // only logger is needed in this helper
	cacheTTL := uc.balanceCacheTTL()

	for _, balance := range balances {
		if balance == nil {
			continue
		}

		internalKey, ok := keyByOperationAlias[balance.Alias]
		if !ok {
			continue
		}

		payload, marshalErr := json.Marshal(balanceToRedis(balance))
		if marshalErr != nil {
			logger.Warnf("Failed to marshal authorizer balance cache for %s: %v", balance.Alias, marshalErr)
			continue
		}

		if cacheErr := uc.RedisRepo.Set(ctx, internalKey, string(payload), cacheTTL); cacheErr != nil {
			logger.Warnf("Failed to cache authorizer balance on redis for %s: %v", balance.Alias, cacheErr)
		}
	}
}

// balanceToRedis converts a Balance into its BalanceRedis representation that
// is stored in the Redis cache.
func balanceToRedis(b *mmodel.Balance) mmodel.BalanceRedis {
	if b == nil {
		return mmodel.BalanceRedis{}
	}

	return mmodel.ToBalanceRedis(b, pkgTransaction.SplitAliasWithKey(b.Alias))
}

func mapAuthorizerRejection(rejectionCode string) error {
	switch rejectionCode {
	case "INSUFFICIENT_FUNDS", "AMOUNT_EXCEEDS_HOLD":
		return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance") //nolint:wrapcheck
	case "BALANCE_NOT_FOUND", "ACCOUNT_INELIGIBLE":
		return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, "validateBalance") //nolint:wrapcheck
	case "REQUEST_TOO_LARGE":
		return pkg.ValidateBusinessError(constant.ErrRequestTooLarge, "validateBalance") //nolint:wrapcheck
	case "INTERNAL_ERROR":
		return pkg.ValidateBusinessError(constant.ErrInternalServer, "authorizer") //nolint:wrapcheck
	default:
		return pkg.ValidateBusinessError(constant.ErrGRPCServiceUnavailable, "authorizer") //nolint:wrapcheck
	}
}
