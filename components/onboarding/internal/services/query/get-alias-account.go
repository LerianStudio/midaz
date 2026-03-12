// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

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

	// GetAccountByAlias get an Account from the repository by given alias (including soft-deleted ones).
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_alias")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving account for alias: %s", alias))

	account, err := uc.AccountRepo.FindAlias(ctx, organizationID, ledgerID, portfolioID, alias)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting account on repo by alias: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("No accounts found for alias: %s", alias))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account on repo by alias", err)

			return nil, err
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb account", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error get metadata on mongodb account: %v", err))

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
