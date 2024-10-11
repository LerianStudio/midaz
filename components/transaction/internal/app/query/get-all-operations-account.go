package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

func (uc *UseCase) GetAllOperationsByAccount(ctx context.Context, organizationID, ledgerID, accountID string, filter commonHTTP.QueryHeader) ([]*o.Operation, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving operations by account")

	op, err := uc.OperationRepo.FindAllByAccount(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(accountID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting operations on repo: %v", err)

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
