package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/google/uuid"
)

func (uc *UseCase) GetAllOperationsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations_by_account")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, err := uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, filter.Limit, filter.Page)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get operations on repo", err)

		logger.Errorf("Error getting operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		return nil, err
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(operation.Operation{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			return nil, common.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
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

	return op, nil
}
