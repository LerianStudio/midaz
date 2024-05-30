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

// GetAllMetadataAccounts fetch all Accounts from the repository
func (uc *UseCase) GetAllMetadataAccounts(ctx context.Context, organizationID, ledgerID, portfolioID string, filter common.QueryHeader) ([]*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving accounts")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(a.Account{}).Name(), filter.Metadata)
	if err != nil || metadata == nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(a.Account{}).Name(),
			Message:    "Accounts by metadata was not found",
			Code:       "ACCOUNT_NOT_FOUND",
			Err:        err,
		}
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	accounts, err := uc.AccountRepo.ListByIDs(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuids)
	if err != nil {
		logger.Errorf("Error getting accounts on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Message:    "Accounts by metadata was not found",
				Code:       "ACCOUNT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	for i := range accounts {
		if data, ok := metadataMap[accounts[i].ID]; ok {
			accounts[i].Metadata = data
		}
	}

	return accounts, nil
}
