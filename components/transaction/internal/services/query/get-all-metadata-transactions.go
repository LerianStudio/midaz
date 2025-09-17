package query

import (
	"context"
	"errors"
	"reflect"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transactions on repo by metadata", err)

		logger.Warnf("Error getting transactions on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(metadata) == 0 {
		logger.Infof("No metadata found")

		return nil, libHTTP.CursorPagination{}, nil
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, uuids, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting transactions on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transactions on repo", err)

			logger.Warnf("Error getting transactions on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transactions on repo", err)

		return nil, libHTTP.CursorPagination{}, err
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

	return trans, cur, nil
}
