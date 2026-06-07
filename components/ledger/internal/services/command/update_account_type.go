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
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
)

// UpdateAccountType updates an account type by its ID.
// It returns the updated account type and an error if the operation fails.
//
// Streaming note: account_type.* events are intentionally NOT emitted —
// see CreateAccountType for the rationale.
func (uc *UseCase) UpdateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateAccountTypeInput) (_ *mmodel.AccountType, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account_type")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_account_type", start, err)
	}()

	accountType := &mmodel.AccountType{
		Name:        input.Name,
		Description: input.Description,
	}

	accountTypeUpdated, err := uc.AccountTypeRepo.Update(ctx, organizationID, ledgerID, id, accountType)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

			logger.Log(ctx, libLog.LevelWarn, "Account type ID not found", libLog.Err(err), libLog.String("account_type_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update account type on repo by id", libLog.Err(err), libLog.String("account_type_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityAccountType, id.String(), input.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update account type metadata", libLog.Err(err), libLog.String("account_type_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata", err)

		return nil, err
	}

	accountTypeUpdated.Metadata = metadataUpdated

	return accountTypeUpdated, nil
}
