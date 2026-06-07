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

	// GetAccountByIDWithDeleted get an Account from the repository by given id (including soft-deleted ones).
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAccountByIDWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id_with_deleted")
	defer span.End()

	account, err := uc.AccountRepo.FindWithDeleted(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting account on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "No account found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account on repo by id", err)

		return nil, err
	}

	if account != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, constant.EntityAccount, id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb account", err)

			logger.Log(ctx, libLog.LevelWarn, "No metadata found")

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
