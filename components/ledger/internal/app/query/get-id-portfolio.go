package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	"github.com/google/uuid"
)

// GetPortfolioByID get a Portfolio from the repository by given id.
func (uc *UseCase) GetPortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_portfolio_by_id")
	defer span.End()

	logger.Infof("Retrieving portfolio for id: %s", id)

	portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get portfolio on repo by id", err)

		logger.Errorf("Error getting portfolio on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	if portfolio != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb portfolio", err)

			logger.Errorf("Error get metadata on mongodb portfolio: %v", err)

			return nil, err
		}

		if metadata != nil {
			portfolio.Metadata = metadata.Data
		}
	}

	return portfolio, nil
}
