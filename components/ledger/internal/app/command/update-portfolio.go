package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
)

// UpdatePortfolioByID update a portfolio from the repository by given id.
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id string, upi *p.UpdatePortfolioInput) (*p.Portfolio, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update portfolio: %v", upi)

	portfolio := &p.Portfolio{
		Name:   upi.Name,
		Status: upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id), portfolio)
	if err != nil {
		logger.Errorf("Error updating portfolio on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
				Code:       "0035",
				Title:      "Portfolio ID Not Found",
				Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
				Err:        err,
			}
		}

		return nil, err
	}

	if len(upi.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, upi.Metadata); err != nil {
			return nil, err
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(p.Portfolio{}).Name(), id, upi.Metadata); err != nil {
			return nil, err
		}

		portfolioUpdated.Metadata = upi.Metadata
	}

	return portfolioUpdated, nil
}
