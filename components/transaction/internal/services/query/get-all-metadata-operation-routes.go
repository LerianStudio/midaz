package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataOperationRoutes fetch all Operation Routes from the repository filtered by metadata
func (uc *UseCase) GetAllMetadataOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operation_routes")
	defer span.End()

	logger.Infof("Retrieving operation routes by metadata")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), filter)
	if err != nil || metadata == nil {
		businessErr := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo by metadata", businessErr)

		logger.Warnf("Error getting operation routes on repo by metadata: %v", businessErr)

		return nil, libHTTP.CursorPagination{}, businessErr
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allOperationRoutes, cur, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

			logger.Warnf("Error getting operation routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		logger.Errorf("Error getting operation routes on repo: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "OperationRoute")
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
