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

// GetTransactionRouteByID returns an active transaction route of the organization with its metadata,
// whatever ledger it was created under.
func (uc *UseCase) GetTransactionRouteByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_route_by_id")
	defer span.End()

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, id)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, constant.EntityTransactionRoute)
		}

		recordQueryError(ctx, span, logger, "Failed to get transaction route", err)

		return nil, err
	}

	if transactionRoute != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityTransactionRoute, transactionRoute.ID.String())
		if err != nil {
			recordQueryError(ctx, span, logger, "Failed to get metadata on mongodb transaction route", err)

			return nil, err
		}

		if metadata != nil {
			transactionRoute.Metadata = metadata.Data
		}
	}

	return transactionRoute, nil
}
