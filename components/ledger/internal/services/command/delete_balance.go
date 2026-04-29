// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// DeleteBalance delete balance in the repository.
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to delete balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting balance: %v", err))

		return err
	}

	// Scope protection: internal-scope balances are managed exclusively by
	// the system (e.g. overdraft reserves) and cannot be deleted via the
	// public API. This guard runs BEFORE the funds-check so the caller
	// receives the specific 0169 code instead of a generic validation.
	if balance != nil && balance.Settings != nil && balance.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		err = pkg.ValidateBusinessError(constant.ErrDeletionOfInternalBalance, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Internal-scope balance cannot be deleted", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected deletion of internal-scope balance", libLog.Err(err))

		return err
	}

	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error deleting balance: %v", err))

		return err
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance on repo", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error delete balance: %v", err))

		return err
	}

	return nil
}
