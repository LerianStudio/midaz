// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// GetPortfolioByID get a Portfolio from the repository by given id.
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) GetPortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_portfolio_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving portfolio for id: %s", id))

	portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting portfolio on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get portfolio on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "No portfolio found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get portfolio on repo by id", err)

		return nil, err
	}

	if portfolio != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb portfolio", err)

			logger.Log(ctx, libLog.LevelWarn, "No metadata found")

			return nil, err
		}

		if metadata != nil {
			portfolio.Metadata = metadata.Data
		}
	}

	return portfolio, nil
}
