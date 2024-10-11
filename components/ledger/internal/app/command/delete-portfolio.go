package command

import (
	"context"
	"errors"
	c "github.com/LerianStudio/midaz/common/constant"
	"reflect"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
)

// DeletePortfolioByID deletes a portfolio from the repository by ids.
func (uc *UseCase) DeletePortfolioByID(ctx context.Context, organizationID, ledgerID, id string) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove portfolio for id: %s", id)

	if err := uc.PortfolioRepo.Delete(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id)); err != nil {
		logger.Errorf("Error deleting portfolio on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return c.ValidateBusinessError(c.PortfolioIDNotFoundBusinessError, reflect.TypeOf(p.Portfolio{}).Name())
		}

		return err
	}

	return nil
}
