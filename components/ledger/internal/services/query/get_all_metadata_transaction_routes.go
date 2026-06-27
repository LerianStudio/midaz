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

	// GetAllMetadataTransactionRoutes fetch all Transaction Routes from the repository filtered by metadata
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAllMetadataTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transaction_routes")
	defer span.End()

	metadata, err := uc.TransactionMetadataRepo.FindList(ctx, constant.EntityTransactionRoute, filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, constant.EntityTransactionRoute)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transaction routes on repo by metadata", err)

		logger.Log(ctx, libLog.LevelWarn, "Error getting transaction routes on repo by metadata", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allTransactionRoutes, cur, err := uc.TransactionRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting transaction routes on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, constant.EntityTransactionRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transaction routes on repo", err)

			logger.Log(ctx, libLog.LevelWarn, "Error getting transaction routes on repo", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transaction routes on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var filteredTransactionRoutes []*mmodel.TransactionRoute

	for _, transactionRoute := range allTransactionRoutes {
		if data, ok := metadataMap[transactionRoute.ID.String()]; ok {
			transactionRoute.Metadata = data
			filteredTransactionRoutes = append(filteredTransactionRoutes, transactionRoute)
		}
	}

	if len(filteredTransactionRoutes) > 0 {
		if err := uc.enrichTransactionRoutesWithOperationRoutes(ctx, filteredTransactionRoutes); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to enrich transaction routes with operation routes", err)

			logger.Log(ctx, libLog.LevelError, "Failed to enrich transaction routes with operation routes", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return filteredTransactionRoutes, cur, nil
}
