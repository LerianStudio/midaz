// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountByAlias gets an account from the repository by alias, including soft-deleted ones.
func (uc *UseCase) GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_alias")
	defer span.End()

	account, err := uc.AccountRepo.FindAlias(ctx, organizationID, ledgerID, portfolioID, alias)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting account on repo by alias", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "No accounts found for provided alias")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account on repo by alias", err)

			return nil, err
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, constant.EntityAccount, alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb account", err)

			logger.Log(ctx, libLog.LevelError, "Error get metadata on mongodb account", libLog.Err(err))

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
