// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/google/uuid"
)

type ShardRebalanceStatus struct {
	Paused bool                         `json:"paused"`
	Loads  []internalsharding.ShardLoad `json:"loads"`
}

func (uc *UseCase) SetShardRebalancePaused(ctx context.Context, paused bool) error {
	if uc == nil || uc.ShardManager == nil {
		return fmt.Errorf("shard manager not configured")
	}

	return uc.ShardManager.SetRebalancerPaused(ctx, paused)
}

func (uc *UseCase) GetShardRebalanceStatus(ctx context.Context) (*ShardRebalanceStatus, error) {
	if uc == nil || uc.ShardManager == nil || uc.ShardRouter == nil {
		return nil, fmt.Errorf("shard manager not configured")
	}

	paused, err := uc.ShardManager.IsRebalancerPaused(ctx)
	if err != nil {
		return nil, err
	}

	loads, err := uc.ShardManager.GetShardLoads(ctx, uc.ShardRouter.ShardCount(), 0)
	if err != nil {
		return nil, err
	}

	if loads == nil {
		loads = make([]internalsharding.ShardLoad, 0)
	}

	return &ShardRebalanceStatus{Paused: paused, Loads: loads}, nil
}

func (uc *UseCase) MigrateAccountShard(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	alias string,
	targetShard int,
) (*internalsharding.MigrationResult, error) {
	if uc == nil || uc.ShardManager == nil || uc.ShardRouter == nil {
		return nil, fmt.Errorf("shard manager not configured")
	}

	if uc.BalanceRepo == nil {
		return nil, fmt.Errorf("balance repository not configured")
	}

	if alias == "" {
		return nil, fmt.Errorf("alias is required")
	}

	if strings.ContainsAny(alias, "*?[]") {
		return nil, pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, "alias")
	}

	if targetShard < 0 || targetShard >= uc.ShardRouter.ShardCount() {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "targetShard")
	}

	if shard.IsExternal(alias) {
		return nil, pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, "alias")
	}

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		return nil, err
	}

	if len(balances) == 0 {
		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	balanceKeys := make([]string, 0, len(balances))
	for _, balance := range balances {
		if balance.Key == "" {
			continue
		}

		balanceKeys = append(balanceKeys, balance.Key)
	}

	return uc.ShardManager.MigrateAccount(ctx, organizationID, ledgerID, alias, targetShard, balanceKeys)
}
