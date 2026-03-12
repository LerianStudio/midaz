// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// UpdateAccountType updates an account type by its ID.
	// It returns the updated account type and an error if the operation fails.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) UpdateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account_type")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update account type: %v", input))

	accountType := &mmodel.AccountType{
		Name:        input.Name,
		Description: input.Description,
	}

	accountTypeUpdated, err := uc.AccountTypeRepo.Update(ctx, organizationID, ledgerID, id, accountType)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating account type on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Account type ID not found: %s", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), id.String(), input.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating metadata: %v", err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata", err)

		return nil, err
	}

	accountTypeUpdated.Metadata = metadataUpdated

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully updated account type with ID: %s", accountTypeUpdated.ID))

	return accountTypeUpdated, nil
}
