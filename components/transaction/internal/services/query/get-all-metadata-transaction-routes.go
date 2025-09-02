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

// GetAllMetadataTransactionRoutes fetch all Transaction Routes from the repository filtered by metadata
func (uc *UseCase) GetAllMetadataTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transaction_routes")
	defer span.End()

	logger.Infof("Retrieving transaction routes by metadata")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo by metadata", err)

		logger.Warnf("Error getting transaction routes on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allTransactionRoutes, cur, err := uc.TransactionRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting transaction routes on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

			logger.Warnf("Error getting transaction routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

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
