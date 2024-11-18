package query

import (
	"context"
	"errors"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
)

// GetAllMetadataTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter commonHTTP.QueryHeader) ([]*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(t.Transaction{}).Name(), filter)
	if err != nil || metadata == nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get transactions on repo by metadata", err)

		return nil, common.ValidateBusinessError(cn.ErrNoTransactionsFound, reflect.TypeOf(t.Transaction{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	trans, err := uc.TransactionRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get transactions on repo by query params", err)

		logger.Errorf("Error getting transactions on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoTransactionsFound, reflect.TypeOf(t.Transaction{}).Name())
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
