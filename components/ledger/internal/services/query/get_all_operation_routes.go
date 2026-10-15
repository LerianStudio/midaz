// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllOperationRoutes lists the organization's operation routes with their metadata.
// A non-nil ledgerID keeps only the routes created under that ledger.
func (uc *UseCase) GetAllOperationRoutes(ctx context.Context, organizationID uuid.UUID, ledgerID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operation_routes")
	defer span.End()

	operationRoutes, cur, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, constant.EntityOperationRoute)
		}

		recordQueryError(ctx, span, logger, "Failed to get operation routes on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if operationRoutes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.TransactionMetadataRepo.FindList(ctx, constant.EntityOperationRoute, metadataFilter)
		if err != nil {
			recordQueryError(ctx, span, logger, "Failed to get operation route metadata on repo", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range operationRoutes {
			if data, ok := metadataMap[operationRoutes[i].ID.String()]; ok {
				operationRoutes[i].Metadata = data
			}
		}
	}

	return operationRoutes, cur, nil
}
