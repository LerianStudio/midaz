package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetBalanceByID gets data in the repository.
func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_by_id")
	defer span.End()

	logger.Infof("Trying to get balance")

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		return nil, err
	}

	return balance, nil
}
