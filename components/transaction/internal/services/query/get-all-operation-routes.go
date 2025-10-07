// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

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
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllOperationRoutes retrieves a paginated list of operation routes with metadata.
//
// Fetches operation routes from PostgreSQL with cursor pagination, then enriches with
// MongoDB metadata. Returns empty array if no routes found.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - filter: Query parameters (cursor pagination, sorting, metadata filters)
//
// Returns:
//   - []*mmodel.OperationRoute: Array of operation routes with metadata
//   - libHTTP.CursorPagination: Pagination cursor info
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_operation_routes"
func (uc *UseCase) GetAllOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operation_routes")
	defer span.End()

	logger.Infof("Retrieving operation routes")

	operationRoutes, cur, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

			logger.Warnf("Error getting operation routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

		logger.Errorf("Error getting operation routes on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if operationRoutes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter)
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation route", err)

			logger.Warnf("Error getting metadata on mongodb operation route: %v", err)

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
