package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateTransactionRoute updates a transaction route by its ID.
// It returns the updated transaction route and an error if the operation fails.
func (uc *UseCase) UpdateTransactionRoute(ctx context.Context, organizationID, ledgerID, id uuid.UUID, input *mmodel.UpdateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_route")
	defer span.End()

	logger.Infof("Trying to update transaction route: %v", input)

	transactionRoute := &mmodel.TransactionRoute{
		Title:       input.Title,
		Description: input.Description,
	}

	transactionRouteUpdated, err := uc.TransactionRouteRepo.Update(ctx, organizationID, ledgerID, id, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction route on repo by id", err)

		logger.Errorf("Error updating transaction route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		logger.Errorf("Error updating metadata on repo by id: %v", err)

		return nil, err
	}

	transactionRouteUpdated.Metadata = metadataUpdated

	return transactionRouteUpdated, nil
}
