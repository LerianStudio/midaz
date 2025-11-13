package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// DeleteBalance delete balance in the repository.
func (uc *UseCase) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID uuid.UUID, accountID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	logger.Infof("Trying to delete balance")

	balances, err := uc.RedisRepo.ListAllByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on redis", err)

		logger.Errorf("Error getting balances on redis: %v", err)

		return err
	}

	if len(balances) > 0 {
		for _, balance := range balances {
			if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
				err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteAllBalancesByAccountID")

				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)

				logger.Warnf("Error deleting balances: %v", err)

				return err
			}

			defer func() {
				if err != nil {
					allowTransfer := utils.BoolPtr(true)

					updateTransferErr := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, balance.IDtoUUID(), allowTransfer)
					if updateTransferErr != nil {
						logger.Errorf("Error re-enabling balance transfers during rollback: %v", updateTransferErr)
					}
				}
			}()

			allowTransfer := utils.BoolPtr(false)

			err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, balance.IDtoUUID(), allowTransfer)
			if err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)

				logger.Errorf("Error update balance: %v", err)

				return err
			}

			balanceCompleteKey := balance.Alias + "#" + balance.Key

			cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceCompleteKey)

			err = uc.RedisRepo.Del(ctx, cacheKey)
			if err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

				logger.Errorf("Error delete balance: %v", err)

				return err
			}

			err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balance.IDtoUUID())
			if err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

				logger.Errorf("Error delete balance: %v", err)

				return err
			}
		}
	}

	return nil
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
