package command

import (
	"context"
	"errors"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DeleteAllBalancesByAccountID delete all balances by account id in the repository.
func (uc *UseCase) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID uuid.UUID, accountID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_all_balances_by_account_id")
	defer span.End()

	logger.Infof("Trying to delete all balances by account id: %s", accountID.String())

	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances by account id on repo", err)

		logger.Errorf("Error getting balances by account id on repo: %v", err)

		return err
	}

	if len(balances) == 0 {
		return nil
	}

	for _, balance := range balances {
		cacheBalance, err := uc.RedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, fmt.Sprintf("%s#%s", balance.Alias, balance.Key))
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance by key on redis", err)

				logger.Errorf("Error getting balance by key on redis: %v", err)

				return err
			}
		}

		if cacheBalance != nil {
			err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "ListBalanceByAccountIDAndKey")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because there is transactions happening.", err)

			logger.Warnf("Balance cannot be deleted because there is transactions happening: %v", err)

			return err
		}

		if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
			err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteAllBalancesByAccountID")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)

			logger.Warnf("Error deleting balances: %v", err)

			return err
		}
	}

	if err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, false); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to toggle balance transfers for account on repo", err)

		logger.Errorf("Error toggling balance transfers for account on repo: %v", err)

		return err
	}

	balanceIDs := make([]uuid.UUID, 0, len(balances))
	for _, balance := range balances {
		balanceIDs = append(balanceIDs, balance.IDtoUUID())
	}

	err = uc.BalanceRepo.DeleteAllByIDs(ctx, organizationID, ledgerID, balanceIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

		logger.Errorf("Error delete balance: %v", err)

		toggleErr := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, true)
		if toggleErr != nil {
			logger.Errorf("Error toggling balance transfers for account %s: %v", accountID.String(), toggleErr)
		}

		return err
	}

	return nil
}

func (uc *UseCase) toggleBalanceTransfers(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, allow bool) (err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.toggle_balance_transfers")
	defer span.End()

	logger.Infof("Trying to toggle balance transfers")

	allowTransfer := utils.BoolPtr(allow)

	defer func() {
		if err == nil {
			return
		}

		if rollbackErr := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, utils.BoolPtr(!allow)); rollbackErr != nil {
			logger.Errorf("Failed to rollback transfer permissions for account %s: %v", accountID.String(), rollbackErr)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to rollback balance transfer permission", rollbackErr)
		}
	}()

	if err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allowTransfer); err != nil {
		return err
	}

	return nil
}

func (uc *UseCase) updateBalanceTransferPermissions(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, allowTransfer *bool) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance_transfer_permissions_for_account")
	defer span.End()

	logger.Infof("Trying to update balance transfer permissions for account %s", accountID.String())

	err := uc.BalanceRepo.UpdateAllByAccountID(ctx, organizationID, ledgerID, accountID, mmodel.UpdateBalance{
		AllowReceiving: allowTransfer,
		AllowSending:   allowTransfer,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance transfer permissions for account on repo", err)

		logger.Errorf("Error update balance transfer permissions for account: %v", err)

		return err
	}

	return nil
}
