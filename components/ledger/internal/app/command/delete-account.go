package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// DeleteAccountByID delete an account from the repository by ids.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID, portfolioID, id string) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove account for id: %s", id)

	if err := uc.AccountRepo.Delete(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuid.MustParse(id)); err != nil {
		logger.Errorf("Error deleting account on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Code:       "0052",
				Title:      "Account ID Not Found",
				Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
				Err:        err,
			}
		}

		return err
	}

	return nil
}
