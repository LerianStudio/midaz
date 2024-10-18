package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// GetAllAccount fetch all Account from the repository
func (uc *UseCase) GetAllAccount(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, filter commonHTTP.QueryHeader) ([]*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving accounts")

	accounts, err := uc.AccountRepo.FindAll(ctx, organizationID, ledgerID, portfolioID, filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoAccountsFound, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	if accounts != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(a.Account{}).Name(), filter)
		if err != nil {
			return nil, common.ValidateBusinessError(cn.ErrNoAccountsFound, reflect.TypeOf(a.Account{}).Name())
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
