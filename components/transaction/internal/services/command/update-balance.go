// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate pkgTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	fromTo := make(map[string]pkgTransaction.Amount, len(validate.From)+len(validate.To))
	for k, v := range validate.From {
		fromTo[k] = v
	}

	for k, v := range validate.To {
		fromTo[k] = v
	}

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := pkgTransaction.OperateBalances(fromTo[balance.Alias], *balance.ToTransactionBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return err
		}

		newBalances = append(newBalances, &mmodel.Balance{
			ID:        balance.ID,
			Alias:     balance.Alias,
			Available: calculateBalances.Available,
			OnHold:    calculateBalances.OnHold,
			Version:   balance.Version + 1,
		})

		spanBalance.End()
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return err
	}

	return nil
}

// Update balance in the repository and returns the updated balance.
// Overlays Redis cached values for Available, OnHold, and Version to ensure freshest data.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	balance, err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)
		logger.Errorf("Error update balance: %v", err)

		return nil, err
	}

	// Overlay amounts from Redis cache when available to ensure freshest values
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)

	value, rerr := uc.RedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache value on redis", rerr)
		logger.Warnf("Failed to get balance cache value on redis: %v", rerr)
	}

	if value != "" {
		cached := mmodel.BalanceRedis{}
		if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
			logger.Warnf("Error unmarshalling balance cache value: %v", uerr)
		} else {
			balance.Available = cached.Available
			balance.OnHold = cached.OnHold
			balance.Version = cached.Version
		}
	}

	return balance, nil
}
