package query

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

// GetPortfolioByID get a Portfolio from the repository by given id.
func (uc *UseCase) GetPortfolioByID(ctx context.Context, organizationID, ledgerID, id string) (*p.Portfolio, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving portfolio for id: %s", id)

	portfolio, err := uc.PortfolioRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting portfolio on repo by id: %v", err)

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

	if portfolio != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(p.Portfolio{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb portfolio: %v", err)
			return nil, err
		}

		if metadata != nil {
			portfolio.Metadata = metadata.Data
		}
	}

	return portfolio, nil
}
