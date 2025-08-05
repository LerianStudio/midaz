package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination())
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
			source := make([]string, 0)

			destination := make([]string, 0)

			for _, op := range trans[i].Operations {
				switch op.Type {
				case constant.DEBIT:
					source = append(source, op.AccountAlias)
				case constant.CREDIT:
					destination = append(destination, op.AccountAlias)
				}
			}

			trans[i].Source = source
			trans[i].Destination = destination

			if data, ok := metadataMap[trans[i].ID]; ok {
				trans[i].Metadata = data
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
		switch op.Type {
		case constant.DEBIT:
			source = append(source, op.AccountAlias)
		case constant.CREDIT:
			destination = append(destination, op.AccountAlias)
		}
	}

	tran.Source = source
	tran.Destination = destination
	tran.Operations = operations

	return tran, nil
}
