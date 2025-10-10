// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving operation routes filtered by metadata.
package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataOperationRoutes retrieves a list of operation routes filtered by metadata.
//
// This use case performs a metadata-first query, retrieving a list of entity IDs
// from MongoDB that match the metadata filter, and then fetching the corresponding
// operation routes from PostgreSQL.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - filter: Query parameters, including metadata filters.
//
// Returns:
//   - []*mmodel.OperationRoute: A slice of operation routes with their metadata.
//   - libHTTP.CursorPagination: Pagination information for the result set.
//   - error: An error if the retrieval fails.
func (uc *UseCase) GetAllMetadataOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operation_routes")
	defer span.End()

	logger.Infof("Retrieving operation routes by metadata")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo by metadata", err)

		logger.Warnf("Error getting operation routes on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allOperationRoutes, cur, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting operation routes on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

			logger.Warnf("Error getting operation routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

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
