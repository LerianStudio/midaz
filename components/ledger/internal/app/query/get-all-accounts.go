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

// GetAllAccount fetch all Account from the repository
func (uc *UseCase) GetAllAccount(ctx context.Context, organizationID, ledgerID, portfolioID string, filter common.QueryHeader) ([]*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving accounts")

	accounts, err := uc.AccountRepo.FindAll(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Message:    "Account was not found",
				Code:       "ACCOUNT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if accounts != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(a.Account{}).Name(), filter)
		if err != nil {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Message:    "Metadata was not found",
				Code:       "ACCOUNT_NOT_FOUND",
				Err:        err,
			}
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range accounts {
			if data, ok := metadataMap[accounts[i].ID]; ok {
				accounts[i].Metadata = data
			}
		}
	}

	return accounts, nil
}
