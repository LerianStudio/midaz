package command

import (
	"context"
	"errors"
	"reflect"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// UpdateAccount update an account from the repository by given id.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID, uai *a.UpdateAccountInput) (*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update account: %v", uai)

	if common.IsNilOrEmpty(uai.Alias) {
		uai.Alias = nil
	} else {
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *uai.Alias)
		if err != nil {
			return nil, err
		}
	}

	account := &a.Account{
		Name:      uai.Name,
		Status:    uai.Status,
		Alias:     uai.Alias,
		ProductID: uai.ProductID,
		Metadata:  uai.Metadata,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(a.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	return accountUpdated, nil
}
