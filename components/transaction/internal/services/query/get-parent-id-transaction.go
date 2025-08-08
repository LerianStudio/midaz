package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetParentByTransactionID gets data in the repository.
func (uc *UseCase) GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_parent_by_transaction_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.parent_id", parentID.String()),
	)

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.FindByParentID(ctx, organizationID, ledgerID, parentID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get parent transaction on repo by id", err)

		logger.Errorf("Error getting parent transaction: %v", err)

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), tran.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
