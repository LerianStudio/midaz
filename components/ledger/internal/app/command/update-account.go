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

// UpdateAccount update an account from the repository by given id.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID, portfolioID, id string, uai *a.UpdateAccountInput) (*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update account: %v", uai)

	if common.IsNilOrEmpty(uai.Alias) {
		uai.Alias = nil
	}

	account := &a.Account{
		Name:      uai.Name,
		Status:    uai.Status,
		Alias:     uai.Alias,
		ProductID: uai.ProductID,
		Metadata:  uai.Metadata,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuid.MustParse(id), account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

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

	if len(uai.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uai.Metadata); err != nil {
			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(a.Account{}).Name(), id, uai.Metadata)
		if err != nil {
			return nil, err
		}

		accountUpdated.Metadata = uai.Metadata
	}

	return accountUpdated, nil
}
