package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// UpdateAccountByID update an account from the repository by given id.
func (uc *UseCase) UpdateAccountByID(ctx context.Context, organizationID, ledgerID, portfolioID, id string, uai *a.UpdateAccountInput) (*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update account: %v", uai)

	account := &a.Account{
		Name:           uai.Name,
		Status:         uai.Status,
		Alias:          uai.Alias,
		AllowSending:   uai.AllowSending,
		AllowReceiving: uai.AllowReceiving,
		ProductID:      uai.ProductID,
		Metadata:       uai.Metadata,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuid.MustParse(id), account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Message:    fmt.Sprintf("Account with id %s was not found", id),
				Code:       "ACCOUNT_NOT_FOUND",
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
