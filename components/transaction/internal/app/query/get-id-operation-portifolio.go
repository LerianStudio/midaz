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

func (uc *UseCase) GetOperationByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID, operationID string) (*o.Operation, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByPortfolio(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuid.MustParse(operationID))
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

	return op, nil
}
