package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

type balanceDeletionContext struct {
	balance       *mmodel.Balance
	balanceID     uuid.UUID
	cacheKey      string
	cacheValue    string
	hasCacheValue bool
}

// DeleteAllBalancesByAccountID delete all balances by account id in the repository.
func (uc *UseCase) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID uuid.UUID, accountID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_all_balances_by_account_id")
	defer span.End()

	logger.Infof("Trying to delete all balances by account id")

	balances, err := uc.RedisRepo.ListAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on redis", err)

		logger.Errorf("Error getting balances on redis: %v", err)

		return err
	}

	if len(balances) == 0 {
		return nil
	}

	for _, balance := range balances {
		if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
			err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteAllBalancesByAccountID")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)

			logger.Warnf("Error deleting balances: %v", err)

			return err
		}
	}

	deletions := make([]balanceDeletionContext, 0, len(balances))

	for _, balance := range balances {
		cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, fmt.Sprintf("%s#%s", balance.Alias, balance.Key))

		cacheValue, cacheErr := uc.RedisRepo.Get(ctx, cacheKey)
		if cacheErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to read balance cache before deletion", cacheErr)

			logger.Errorf("Error getting balance from cache: %v", cacheErr)

			return cacheErr
		}

		deletions = append(deletions, balanceDeletionContext{
			balance:       balance,
			balanceID:     balance.IDtoUUID(),
			cacheKey:      cacheKey,
			cacheValue:    cacheValue,
			hasCacheValue: cacheValue != "",
		})
	}

	if err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, deletions, false); err != nil {
		return err
	}

	cachesRemoved := make([]balanceDeletionContext, 0, len(deletions))

	for _, deletion := range deletions {
		if err = uc.RedisRepo.Del(ctx, deletion.cacheKey); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

			logger.Errorf("Error delete balance: %v", err)

			uc.restoreBalanceCaches(ctx, cachesRemoved)

			toggleErr := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, deletions, true)
			if toggleErr != nil {
				logger.Errorf("Error toggling balance transfers: %v", toggleErr)
			}

			return err
		}

		cachesRemoved = append(cachesRemoved, deletion)
	}

	balanceIDs := make([]uuid.UUID, 0, len(deletions))
	for _, deletion := range deletions {
		balanceIDs = append(balanceIDs, deletion.balanceID)
	}

	err = uc.BalanceRepo.DeleteAllByIDs(ctx, organizationID, ledgerID, balanceIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

		logger.Errorf("Error delete balance: %v", err)

		uc.restoreBalanceCaches(ctx, cachesRemoved)

		toggleErr := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, deletions, true)
		if toggleErr != nil {
			logger.Errorf("Error toggling balance transfers: %v", toggleErr)
		}

		return err
	}

	return nil
}

func (uc *UseCase) toggleBalanceTransfers(ctx context.Context, organizationID, ledgerID uuid.UUID, deletions []balanceDeletionContext, allow bool) (err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.toggle_balance_transfers")
	defer span.End()

	logger.Infof("Trying to toggle balance transfers")

	allowTransfer := utils.BoolPtr(allow)
	processed := make([]balanceDeletionContext, 0, len(deletions))

	defer func() {
		if err == nil {
			return
		}

		revertAllow := utils.BoolPtr(!allow)

		for i := len(processed) - 1; i >= 0; i-- {
			if rollbackErr := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, processed[i].balanceID, revertAllow); rollbackErr != nil {
				logger.Errorf("Failed to rollback transfer permissions for balance %s: %v", processed[i].balanceID, rollbackErr)

				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to rollback balance transfer permission", rollbackErr)
			}
		}
	}()

	for _, deletion := range deletions {
		if err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, deletion.balanceID, allowTransfer); err != nil {
			return err
		}

		processed = append(processed, deletion)
	}

	return nil
}

// TODO: Do we need to restore the balance caches?
func (uc *UseCase) restoreBalanceCaches(ctx context.Context, deletions []balanceDeletionContext) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.restore_balance_caches")
	defer span.End()

	logger.Infof("Trying to restore balance caches")

	for i := len(deletions) - 1; i >= 0; i-- {
		deletion := deletions[i]

		if !deletion.hasCacheValue {
			continue
		}

		if err := uc.RedisRepo.Set(ctx, deletion.cacheKey, deletion.cacheValue, 0); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to restore balance cache", err)

			logger.Errorf("Error restoring balance cache: %v", err)
		}
	}
}

func (uc *UseCase) updateBalanceTransferPermissions(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, allowTransfer *bool) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance_transfer_permissions")
	defer span.End()

	logger.Infof("Trying to update balance transfer permissions")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, mmodel.UpdateBalance{
		AllowReceiving: allowTransfer,
		AllowSending:   allowTransfer,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)

		logger.Errorf("Error update balance: %v", err)

		return err
	}

	return nil
}
