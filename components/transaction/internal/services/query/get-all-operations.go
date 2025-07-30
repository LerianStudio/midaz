package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

func (uc *UseCase) GetAllOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, cur, err := uc.OperationRepo.FindAll(ctx, organizationID, ledgerID, transactionID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get all operations on repo", err)

		logger.Errorf("Error getting all operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		return nil, libHTTP.CursorPagination{}, err
	}

	if op != nil {
		metadataFilter := http.QueryHeader{
			Limit:     filter.Limit,
			Page:      filter.Page,
			Cursor:    filter.Cursor,
			SortOrder: filter.SortOrder,
			StartDate: filter.StartDate,
			EndDate:   filter.EndDate,
			Metadata:  &bson.M{},
		}

		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(operation.Operation{}).Name(), metadataFilter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range op {
			if data, ok := metadataMap[op[i].ID]; ok {
				op[i].Metadata = data
			}
		}
	}

	return op, cur, nil
}
