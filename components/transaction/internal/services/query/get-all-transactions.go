package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"reflect"
)

// GetAllTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	trans, cur, err := uc.TransactionRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transactions on repo", err)

		logger.Errorf("Error getting transactions on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, libHTTP.CursorPagination{}, err
	}

	if trans != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb transaction", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range trans {
			if data, ok := metadataMap[trans[i].ID]; ok {
				trans[i].Metadata = data
			}

			trans[i], err = uc.GetOperationsByTransaction(ctx, organizationID, ledgerID, trans[i], filter)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to get operations to transaction by id", err)

				return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(transaction.Transaction{}).Name())
			}
		}
	}

	return trans, cur, nil
}

func (uc *UseCase) GetOperationsByTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, filter http.QueryHeader) (*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions_get_operations")
	defer span.End()

	logger.Infof("Retrieving Operations")

	operations, _, err := uc.GetAllOperations(ctx, organizationID, ledgerID, tran.IDtoUUID(), filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Operations", err)

		logger.Errorf("Failed to retrieve Operations with ID: %s, Error: %s", tran.IDtoUUID(), err.Error())

		return nil, err
	}

	source := make([]string, 0)
	destination := make([]string, 0)

	for _, op := range operations {
		if op.Type == constant.DEBIT {
			source = append(source, op.AccountAlias)
		} else {
			destination = append(destination, op.AccountAlias)
		}
	}

	span.End()

	tran.Source = source
	tran.Destination = destination
	tran.Operations = operations

	return tran, nil
}
