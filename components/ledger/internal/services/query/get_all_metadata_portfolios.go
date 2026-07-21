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
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataPortfolios fetches all portfolios from the repository.
func (uc *UseCase) GetAllMetadataPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_portfolios")
	defer span.End()

	metadata, err := uc.OnboardingMetadataRepo.FindList(ctx, constant.EntityPortfolio, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get metadata on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting metadata on repo")

		return nil, err
	}

	if len(metadata) == 0 {
		err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, constant.EntityPortfolio)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No metadata found", err)

		logger.Log(ctx, libLog.LevelWarn, "No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	filter.EntityIDs = uuids

	portfolios, err := uc.PortfolioRepo.FindAll(ctx, organizationID, ledgerID, filter)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, constant.EntityPortfolio)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get portfolios on repo", err)

			logger.Log(ctx, libLog.LevelWarn, "No portfolios found")

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Error getting portfolios on repo")

		libOpentelemetry.HandleSpanError(span, "Failed to get portfolios on repo", err)

		return nil, err
	}

	for i := range portfolios {
		if data, ok := metadataMap[portfolios[i].ID]; ok {
			portfolios[i].Metadata = data
		}
	}

	return portfolios, nil
}
