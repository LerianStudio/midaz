package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/google/uuid"
)

// UpdatePortfolioByID update a portfolio from the repository by given id.
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_portfolio_by_id")
	defer span.End()

	logger.Infof("Trying to update portfolio: %v", upi)

	portfolio := &mmodel.Portfolio{
		Name:   upi.Name,
		Status: upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, organizationID, ledgerID, id, portfolio)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update portfolio on repo by id", err)

		logger.Errorf("Error updating portfolio on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	portfolioUpdated.Metadata = metadataUpdated

	return portfolioUpdated, nil
}
