// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// DeleteAllBalancesByAccountID delete all balances by account id in the repository.
func (uc *UseCase) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, requestID string) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_all_balances_by_account_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "delete_all_balances", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", requestID),
	)

	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balances by account id on repo", err)

		logger.Log(ctx, libLog.LevelError, "Error getting balances by account id on repo", libLog.Err(err))

		return err
	}

	if len(balances) == 0 {
		return nil
	}

	// Plant delete markers so the honored-lock pre-pass rejects concurrent mutations for the
	// whole delete. Release them only when the delete fails, so a rejected guard, permission
	// flip, or soft-delete leaves the account usable; a successful delete lets the delete marker
	// expire by its own TTL.
	release := uc.plantBalanceDeleteMarkers(ctx, organizationID, ledgerID, balances)

	defer func() {
		if err != nil {
			release()
		}
	}()

	for _, balance := range balances {
		cacheBalance, cacheErr := uc.TransactionRedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, fmt.Sprintf("%s#%s", balance.Alias, balance.Key))
		if cacheErr != nil && !errors.Is(cacheErr, redis.Nil) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance by key on redis", cacheErr)

			logger.Log(ctx, libLog.LevelError, "Error getting balance by key on redis", libLog.Err(cacheErr))

			return cacheErr
		}

		if cacheBalance != nil {
			if !cacheBalance.Available.IsZero() || !cacheBalance.OnHold.IsZero() {
				err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "ListBalanceByAccountIDAndKey")

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in it.", err)

				logger.Log(ctx, libLog.LevelWarn, "Balance cannot be deleted because it still has funds in it", libLog.Err(err))

				return err
			}
		}

		if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
			err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteAllBalancesByAccountID")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in it.", err)

			logger.Log(ctx, libLog.LevelWarn, "Error deleting balances", libLog.Err(err))

			return err
		}
	}

	if err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, false); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to toggle balance transfers for account on repo", err)

		logger.Log(ctx, libLog.LevelError, "Error toggling balance transfers for account on repo", libLog.Err(err))

		return err
	}

	balanceIDs := make([]uuid.UUID, 0, len(balances))
	for _, balance := range balances {
		balanceIDs = append(balanceIDs, balance.IDtoUUID())
	}

	err = uc.BalanceRepo.DeleteAllByIDs(ctx, organizationID, ledgerID, balanceIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance on repo", err)

		logger.Log(ctx, libLog.LevelError, "Error delete balance", libLog.Err(err))

		toggleErr := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, true)
		if toggleErr != nil {
			logger.Log(ctx, libLog.LevelError, "Error toggling balance transfers for account",
				libLog.String("account_id", accountID.String()), libLog.Err(toggleErr))
		}

		return err
	}

	// Drop the stale cache entries now that the rows are soft-deleted. Non-fatal: a failed
	// eviction is logged and never fails the already-committed delete.
	uc.evictBalanceCaches(ctx, organizationID, ledgerID, balances)

	return nil
}

func (uc *UseCase) toggleBalanceTransfers(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, allow bool) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.toggle_balance_transfers")
	defer span.End()

	allowTransfer := utils.BoolPtr(allow)

	defer func() {
		if err == nil {
			return
		}

		if rollbackErr := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, utils.BoolPtr(!allow)); rollbackErr != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to rollback transfer permissions for account",
				libLog.String("account_id", accountID.String()), libLog.Err(rollbackErr))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to rollback balance transfer permission", rollbackErr)
		}
	}()

	if err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allowTransfer); err != nil {
		return err
	}

	return nil
}

func (uc *UseCase) updateBalanceTransferPermissions(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, allowTransfer *bool) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance_transfer_permissions_for_account")
	defer span.End()

	err := uc.BalanceRepo.UpdateAllByAccountID(ctx, organizationID, ledgerID, accountID, mmodel.UpdateBalance{
		AllowReceiving: allowTransfer,
		AllowSending:   allowTransfer,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update balance transfer permissions for account on repo", err)

		logger.Log(ctx, libLog.LevelError, "Error update balance transfer permissions for account", libLog.Err(err))

		return err
	}

	return nil
}
