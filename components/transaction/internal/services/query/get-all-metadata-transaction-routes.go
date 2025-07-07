package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataTransactionRoutes fetch all Transaction Routes from the repository filtered by metadata
func (uc *UseCase) GetAllMetadataTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transaction_routes")
	defer span.End()

	logger.Infof("Retrieving transaction routes by metadata")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), filter)
	if err != nil || metadata == nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction routes on repo by metadata", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allTransactionRoutes, cur, err := uc.TransactionRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction routes on repo", err)

		logger.Errorf("Error getting transaction routes on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		return nil, libHTTP.CursorPagination{}, err
	}

	var filteredTransactionRoutes []*mmodel.TransactionRoute

	for _, transactionRoute := range allTransactionRoutes {
		if data, ok := metadataMap[transactionRoute.ID.String()]; ok {
			transactionRoute.Metadata = data
			filteredTransactionRoutes = append(filteredTransactionRoutes, transactionRoute)
		}
	}

	return filteredTransactionRoutes, cur, nil
}
