package query

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"reflect"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// GetBalanceByID gets data in the repository.
func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_by_id")
	defer span.End()

	logger.Infof("Trying to get balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		return nil, err
	}

	if balance != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Balance{}).Name(), balanceID.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb balance", err)

			logger.Errorf("Error get metadata on mongodb balance: %v", err)

			return nil, err
		}

		if metadata != nil {
			balance.Metadata = metadata.Data
		}
	}

	return balance, nil
}