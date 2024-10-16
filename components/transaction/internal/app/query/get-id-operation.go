package query

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/common/mlog"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

// GetOperationByID gets data in the repository.
func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID string) (*o.Operation, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to get operation")

	operation, err := uc.OperationRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(transactionID), uuid.MustParse(operationID))
	if err != nil {
		logger.Errorf("Error getting operation: %v", err)
		return nil, err
	}

	if operation != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(o.Operation{}).Name(), operationID)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb operation: %v", err)
			return nil, err
		}

		if metadata != nil {
			operation.Metadata = metadata.Data
		}
	}

	return operation, nil
}
