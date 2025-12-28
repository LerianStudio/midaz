package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllOperationsByAccount retrieves all operations for a specific account with filtering and pagination.
func (uc *UseCase) GetAllOperationsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations_by_account")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, cur, err := waitForOperations(ctx, func(ctx context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error) {
		return uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination())
	})
	if err != nil {
		return uc.handleOperationsFetchResult(logger, &span, op, cur, err)
	}

	if len(op) == 0 {
		return ensureNonNilOperations(op), cur, nil
	}

	if err := uc.enrichOperationsWithMetadata(ctx, &span, op); err != nil {
		logger.Warnf("Error getting metadata on mongodb operation: %v", err)
		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
	}

	return op, cur, nil
}
