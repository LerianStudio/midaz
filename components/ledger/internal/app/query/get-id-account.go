package query

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

// GetAccountByID get an Account from the repository by given id.
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID, portfolioID, id string) (*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving account for id: %s", id)

	account, err := uc.AccountRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Code:       "0052",
				Title:      "Account ID Not Found",
				Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
				Err:        err,
			}
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(a.Account{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb account: %v", err)
			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
