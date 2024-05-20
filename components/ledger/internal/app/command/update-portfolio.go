package command

import (
	"context"
	"errors"
	"fmt"
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

	if upi.Name == "" && upi.Status.IsEmpty() && upi.Metadata == nil {
		return nil, common.UnprocessableOperationError{
			Message: "at least one of the allowed fields must be sent with a valid value [name, status.code, status.description, metadata]",
			Code:    "0006",
			Err:     nil,
		}
	}

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
				Message:    fmt.Sprintf("Portfolio with ledger id %s and portfolio id %s was not found", ledgerID, id),
				Code:       "PORTFOLIO_NOT_FOUND",
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
