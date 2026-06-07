// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"

	// GetAccountTypeByID get an Account Type from the repository by given id.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_type_by_id")
	defer span.End()

	accountType, err := uc.AccountTypeRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting account type on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account type on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "No account type found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account type on repo by id", err)

		return nil, err
	}

	if accountType != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, constant.EntityAccountType, id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb account type", err)

			logger.Log(ctx, libLog.LevelWarn, "No metadata found")

			return nil, err
		}

		if metadata != nil {
			accountType.Metadata = metadata.Data
		}
	}

	return accountType, nil
}
