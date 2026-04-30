// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataPortfolios fetches all portfolios from the repository.
func (uc *UseCase) GetAllMetadataPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_portfolios")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Retrieving portfolios")

	metadata, err := uc.OnboardingMetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get metadata on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting metadata on repo")

		return nil, err
	}

	if len(metadata) == 0 {
		err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

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
			err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

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
