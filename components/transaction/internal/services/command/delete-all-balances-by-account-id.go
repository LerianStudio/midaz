package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DeleteAllBalancesByAccountID delete all balances by account id in the repository.
func (uc *UseCase) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, requestID string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_all_balances_by_account_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", requestID),
	)

	logger.Infof("Trying to delete all balances by account id: %s", accountID.String())

	balances, err := uc.getBalancesByAccountID(ctx, &span, logger, organizationID, ledgerID, accountID)
	if err != nil {
		return err
	}

	if len(balances) == 0 {
		return nil
	}

	if err := uc.validateBalancesForDeletion(ctx, &span, logger, organizationID, ledgerID, balances); err != nil {
		return err
	}

	return uc.performBalanceDeletion(ctx, &span, logger, organizationID, ledgerID, accountID, balances)
}

// getBalancesByAccountID retrieves all balances for an account
func (uc *UseCase) getBalancesByAccountID(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error) {
	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balances by account id on repo", err)
		logger.Errorf("Error getting balances by account id on repo: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return balances, nil
}

// validateBalancesForDeletion validates that all balances can be deleted
func (uc *UseCase) validateBalancesForDeletion(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	for _, balance := range balances {
		if err := uc.validateSingleBalance(ctx, span, logger, organizationID, ledgerID, balance); err != nil {
			return err
		}
	}

	return nil
}

// validateSingleBalance validates a single balance for deletion
func (uc *UseCase) validateSingleBalance(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) error {
	if err := uc.checkBalanceInCache(ctx, span, logger, organizationID, ledgerID, balance); err != nil {
		return err
	}

	return uc.checkBalanceHasFunds(span, logger, balance)
}

// checkBalanceInCache checks if balance is in cache (indicating active transactions)
func (uc *UseCase) checkBalanceInCache(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) error {
	cacheBalance, err := uc.RedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, fmt.Sprintf("%s#%s", balance.Alias, balance.Key))
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance by key on redis", err)
		logger.Errorf("Error getting balance by key on redis: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	if cacheBalance != nil {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "ListBalanceByAccountIDAndKey")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because there is transactions happening.", err)
		logger.Warnf("Balance cannot be deleted because there is transactions happening: %v", err)

		return err
	}

	return nil
}

// checkBalanceHasFunds checks if balance has any funds
func (uc *UseCase) checkBalanceHasFunds(span *trace.Span, logger libLog.Logger, balance *mmodel.Balance) error {
	if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
		err := pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteAllBalancesByAccountID")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Warnf("Error deleting balances: %v", err)

		return err
	}

	return nil
}

// performBalanceDeletion performs the actual balance deletion with rollback support
func (uc *UseCase) performBalanceDeletion(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID, balances []*mmodel.Balance) error {
	if err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, false); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to toggle balance transfers for account on repo", err)
		logger.Errorf("Error toggling balance transfers for account on repo: %v", err)

		return err
	}

	balanceIDs := uc.extractBalanceIDs(balances)

	err := uc.BalanceRepo.DeleteAllByIDs(ctx, organizationID, ledgerID, balanceIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance on repo", err)
		logger.Errorf("Error delete balance: %v", err)

		uc.rollbackBalanceTransfers(ctx, logger, organizationID, ledgerID, accountID)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return nil
}

// extractBalanceIDs extracts balance IDs from balance list
func (uc *UseCase) extractBalanceIDs(balances []*mmodel.Balance) []uuid.UUID {
	balanceIDs := make([]uuid.UUID, 0, len(balances))
	for _, balance := range balances {
		balanceIDs = append(balanceIDs, balance.IDtoUUID())
	}

	return balanceIDs
}

// rollbackBalanceTransfers rolls back balance transfer toggles
func (uc *UseCase) rollbackBalanceTransfers(ctx context.Context, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID) {
	if toggleErr := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, true); toggleErr != nil {
		logger.Errorf("Error toggling balance transfers for account %s: %v", accountID.String(), toggleErr)
	}
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

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return nil
}
