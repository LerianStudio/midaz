// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"

	// GetAllMetadataOperationRoutes fetch all Operation Routes from the repository filtered by metadata
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAllMetadataOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operation_routes")
	defer span.End()

	metadata, err := uc.TransactionMetadataRepo.FindList(ctx, constant.EntityOperationRoute, filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, constant.EntityOperationRoute)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operation routes on repo by metadata", err)

		logger.Log(ctx, libLog.LevelWarn, "Error getting operation routes on repo by metadata", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allOperationRoutes, cur, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting operation routes on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, constant.EntityOperationRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operation routes on repo", err)

			logger.Log(ctx, libLog.LevelWarn, "Error getting operation routes on repo", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operation routes on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var filteredOperationRoutes []*mmodel.OperationRoute

	for _, operationRoute := range allOperationRoutes {
		if data, ok := metadataMap[operationRoute.ID.String()]; ok {
			operationRoute.Metadata = data
			filteredOperationRoutes = append(filteredOperationRoutes, operationRoute)
		}
	}

	return filteredOperationRoutes, cur, nil
}
