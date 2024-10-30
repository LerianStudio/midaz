package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

func (uc *UseCase) GetOperationByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID, operationID uuid.UUID) (*o.Operation, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByPortfolio(ctx, organizationID, ledgerID, portfolioID, operationID)
	if err != nil {
		logger.Errorf("Error getting operation on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Operation{}).Name(),
				Message:    "Operation was not found",
				Code:       "OPERATION_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(o.Operation{}).Name(), operationID.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb operation: %v", err)
			return nil, err
		}

		if metadata != nil {
			op.Metadata = metadata.Data
		}
	}

	return op, nil
}
