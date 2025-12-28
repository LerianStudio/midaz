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
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllTransactionRoutes fetch all Transaction Routes from the repository
func (uc *UseCase) GetAllTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transaction_routes")
	defer span.End()

	logger.Infof("Retrieving transaction routes")

	transactionRoutes, cur, err := uc.TransactionRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

			logger.Warnf("Transaction routes not found: %v", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

		logger.Errorf("Error getting transaction routes on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	if transactionRoutes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), metadataFilter)
		if err != nil {
			businessErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb transaction route", businessErr)

			logger.Warnf("Error getting metadata on mongodb transaction route: %v", businessErr)

			return nil, libHTTP.CursorPagination{}, businessErr
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range transactionRoutes {
			if data, ok := metadataMap[transactionRoutes[i].ID.String()]; ok {
				transactionRoutes[i].Metadata = data
			}
		}
	}

	return transactionRoutes, cur, nil
}
