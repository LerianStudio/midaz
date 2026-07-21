// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
)

// DeleteAccountTypeByID deletes an account type by its ID.
// It returns an error if the operation fails or if the account type is not found.
//
// Streaming note: account_type.* events are intentionally NOT emitted —
// see CreateAccountType for the rationale.
func (uc *UseCase) DeleteAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_type_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "delete_account_type", start, err)
	}()

	if err := uc.AccountTypeRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

			logger.Log(ctx, libLog.LevelWarn, "Account type ID not found", libLog.Err(err), libLog.String("account_type_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete account type on repo", err)

			return err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to delete account type", libLog.Err(err), libLog.String("account_type_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete account type on repo", err)

		return err
	}

	return nil
}
