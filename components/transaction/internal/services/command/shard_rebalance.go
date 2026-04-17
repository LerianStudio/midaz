// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

var (
	errShardManagerNotConfigured      = errors.New("shard manager not configured")
	errBalanceRepositoryNotConfigured = errors.New("balance repository not configured")
	errAliasRequired                  = errors.New("alias is required")
)

// ShardRebalanceStatus holds the current state of the shard rebalancer.
type ShardRebalanceStatus struct {
	Paused bool                         `json:"paused"`
	Loads  []internalsharding.ShardLoad `json:"loads"`
}

// SetShardRebalancePaused pauses or resumes the shard rebalancer.
func (uc *UseCase) SetShardRebalancePaused(ctx context.Context, paused bool) error {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed; logger/tenant/trackingID are intentionally ignored.

	ctx, span := tracer.Start(ctx, "command.set_shard_rebalance_paused")
	defer span.End()

	if uc == nil || uc.ShardManager == nil {
		return errShardManagerNotConfigured
	}

	if err := uc.ShardManager.SetRebalancerPaused(ctx, paused); err != nil {
		return fmt.Errorf("failed to set rebalancer paused: %w", err)
	}

	return nil
}

// GetShardRebalanceStatus returns the current shard rebalance status.
func (uc *UseCase) GetShardRebalanceStatus(ctx context.Context) (*ShardRebalanceStatus, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed; logger/tenant/trackingID are intentionally ignored.

	ctx, span := tracer.Start(ctx, "command.get_shard_rebalance_status")
	defer span.End()

	if uc == nil || uc.ShardManager == nil || uc.ShardRouter == nil {
		return nil, errShardManagerNotConfigured
	}

	paused, err := uc.ShardManager.IsRebalancerPaused(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check rebalancer paused: %w", err)
	}

	loads, err := uc.ShardManager.GetShardLoads(ctx, uc.ShardRouter.ShardCount(), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard loads: %w", err)
	}

	if loads == nil {
		loads = make([]internalsharding.ShardLoad, 0)
	}

	return &ShardRebalanceStatus{Paused: paused, Loads: loads}, nil
}

// MigrateAccountShard migrates an account's balances to a different shard.
func (uc *UseCase) MigrateAccountShard(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	alias string,
	targetShard int,
) (*internalsharding.MigrationResult, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed; logger/tenant/trackingID are intentionally ignored.

	ctx, span := tracer.Start(ctx, "command.migrate_account_shard")
	defer span.End()

	if uc == nil || uc.ShardManager == nil || uc.ShardRouter == nil {
		return nil, errShardManagerNotConfigured
	}

	if uc.BalanceRepo == nil {
		return nil, errBalanceRepositoryNotConfigured
	}

	if alias == "" {
		return nil, errAliasRequired
	}

	if strings.ContainsAny(alias, "*?[]") {
		return nil, pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, "alias") //nolint:wrapcheck
	}

	if targetShard < 0 || targetShard >= uc.ShardRouter.ShardCount() {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "targetShard") //nolint:wrapcheck
	}

	if shard.IsExternal(alias) {
		return nil, pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, "alias") //nolint:wrapcheck
	}

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		return nil, err
	}

	if len(balances) == 0 {
		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name()) //nolint:wrapcheck
	}

	balanceKeys := make([]string, 0, len(balances))
	for _, balance := range balances {
		if balance.Key == "" {
			continue
		}

		balanceKeys = append(balanceKeys, balance.Key)
	}

	result, err := uc.ShardManager.MigrateAccount(ctx, organizationID, ledgerID, alias, targetShard, balanceKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate account shard: %w", err)
	}

	return result, nil
}
