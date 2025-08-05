package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetTransactionRouteByID retrieves a transaction route by its ID.
// It returns the transaction route if found, otherwise it returns an error.
func (uc *UseCase) GetTransactionRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_route_by_id")
	defer span.End()

	logger.Infof("Retrieving transaction route for id: %s", id)

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction route", err)

		logger.Errorf("Error getting transaction route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		return nil, err
	}

	if transactionRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb transaction route", err)

			logger.Errorf("Error get metadata on mongodb transaction route: %v", err)

			return nil, err
		}

		if metadata != nil {
			transactionRoute.Metadata = metadata.Data
		}
	}

	return transactionRoute, nil
}
