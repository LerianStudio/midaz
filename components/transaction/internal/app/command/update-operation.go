package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

// UpdateOperation update an operation from the repository by given id.
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID string, uoi *o.UpdateOperationInput) (*o.Operation, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update operation: %v", uoi)

	operation := &o.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(transactionID), uuid.MustParse(operationID), operation)
	if err != nil {
		logger.Errorf("Error updating operation on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Operation{}).Name(),
				Message:    fmt.Sprintf("Operation with id %s was not found", operationID),
				Code:       "OPERATION_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if len(uoi.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uoi.Metadata); err != nil {
			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(o.Operation{}).Name(), operationID, uoi.Metadata)
		if err != nil {
			return nil, err
		}

		operationUpdated.Metadata = uoi.Metadata
	}

	return operationUpdated, nil
}
