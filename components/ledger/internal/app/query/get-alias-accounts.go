package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
)

// ListAccountsByAlias get Accounts from the repository by given alias.
func (uc *UseCase) ListAccountsByAlias(ctx context.Context, aliases []string) ([]*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving account for alias: %s", aliases)

	accounts, err := uc.AccountRepo.ListAccountsByAlias(ctx, aliases)
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Code:       "0063",
				Title:      "Failed To Retrieve Accounts By Aliases",
				Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
				Err:        err,
			}
		}

		return nil, err
	}

	return accounts, nil
}
