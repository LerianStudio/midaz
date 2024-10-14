package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// ListAccountsByIDs get Accounts from the repository by given ids.
func (uc *UseCase) ListAccountsByIDs(ctx context.Context, ids []uuid.UUID) ([]*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving account for id: %s", ids)

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, ids)
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrIDsNotFoundForAccounts, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	return accounts, nil
}
