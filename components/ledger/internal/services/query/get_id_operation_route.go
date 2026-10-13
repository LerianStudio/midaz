// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// GetOperationRouteByID returns an active operation route of the organization with its metadata,
// whatever ledger it was created under.
func (uc *UseCase) GetOperationRouteByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_route_by_id")
	defer span.End()

	operationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, id)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute)
		}

		recordQueryError(ctx, span, logger, "Failed to get operation route on repo by id", err)

		return nil, err
	}

	if operationRoute != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityOperationRoute, operationRoute.ID.String())
		if err != nil {
			recordQueryError(ctx, span, logger, "Failed to get metadata on mongodb operation route", err)

			return nil, err
		}

		if metadata != nil {
			operationRoute.Metadata = metadata.Data
		}
	}

	return operationRoute, nil
}
