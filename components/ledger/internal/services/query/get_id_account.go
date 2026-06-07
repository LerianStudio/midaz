// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// GetAccountByID get an Account from the repository by given id.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (_ *mmodel.Account, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "get_account", start, err)
	}()

	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting account on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, constant.EntityAccount, id.String())
		if err != nil {
			logger.Log(ctx, libLog.LevelError, "Error get metadata on mongodb account", libLog.Err(err))

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
