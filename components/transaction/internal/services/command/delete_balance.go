package command

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// DeleteBalance delete balance in the repository.
func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	logger.Infof("Trying to delete balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		return err
	}

	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteBalance")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)

		logger.Warnf("Error deleting balance: %v", err)

		return err
	}

	defer func() {
		if err != nil {
			allowTransfer := utils.BoolPtr(true)

			err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, balanceID, allowTransfer)
			if err != nil {
				logger.Errorf("Error update balance: %v", err)
			}
		}
	}()

	allowTransfer := utils.BoolPtr(false)

	err = uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, balanceID, allowTransfer)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)

		logger.Errorf("Error update balance: %v", err)

		return err
	}

	balanceCompleteKey := balance.Alias + "#" + balance.Key

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceCompleteKey)

	balanceRedis := mmodel.BalanceRedis{}

	cacheValue, _ := uc.RedisRepo.Get(ctx, cacheKey)
	if !utils.IsNilOrEmpty(&cacheValue) {
		if err := json.Unmarshal([]byte(cacheValue), &balanceRedis); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

			logger.Warnf("Error to Deserialization json: %v", err)
		}
	}

	// TODO: To be defined
	// if balanceRedis.Version != balance.Version {
	// }

	err = uc.RedisRepo.Del(ctx, cacheKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)
		logger.Errorf("Error delete balance: %v", err)

		return err
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance on repo", err)

		logger.Errorf("Error delete balance: %v", err)

		return err
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
