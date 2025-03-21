package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"reflect"
)

// GetAllMetadataTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), filter)
	if err != nil || metadata == nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transactions on repo by metadata", err)

		return nil, pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	trans, err := uc.TransactionRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transactions on repo by query params", err)

		logger.Errorf("Error getting transactions on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, err
	}

	for i := range trans {
		if data, ok := metadataMap[trans[i].ID]; ok {
			trans[i].Metadata = data
		}
	}

	return trans, nil
}
