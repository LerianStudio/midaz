package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
)

// GetAllTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter commonHTTP.QueryHeader) ([]*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	trans, err := uc.TransactionRepo.FindAll(ctx, organizationID, ledgerID, filter.Limit, filter.Page)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get transactions on repo", err)

		logger.Errorf("Error getting transactions on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(t.Transaction{}).Name(),
				Message:    "Transaction was not found",
				Code:       "TRANSACTION_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if trans != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(t.Transaction{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb transaction", err)

			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(t.Transaction{}).Name(),
				Message:    "Metadata was not found",
				Code:       "TRANSACTION_NOT_FOUND",
				Err:        err,
			}
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range trans {
			if data, ok := metadataMap[trans[i].ID]; ok {
				trans[i].Metadata = data
			}
		}
	}

	return trans, nil
}
