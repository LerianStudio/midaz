package command

import (
	"context"
	"reflect"
	"time"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// CreateAccount creates a new account persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, cai *a.CreateAccountInput) (*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create account: %v", cai)

	if common.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	if err := common.ValidateAccountType(cai.Type); err != nil {
		return nil, common.ValidateBusinessError(err, reflect.TypeOf(a.Account{}).Name())
	}

	var status a.Status
	if cai.Status.IsEmpty() {
		status = a.Status{
			Code:           "ACTIVE",
			AllowReceiving: true,
			AllowSending:   true,
		}
	} else {
		status = cai.Status
	}

	balanceValue := float64(0)

	balance := a.Balance{
		Available: &balanceValue,
		OnHold:    &balanceValue,
		Scale:     &balanceValue,
	}

	isAsset, _ := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, "", cai.AssetCode)
	if !isAsset {
		return nil, common.ValidateBusinessError(cn.ErrAssetCodeNotFound, reflect.TypeOf(a.Account{}).Name())
	}

	if cai.EntityID == nil {
		portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, portfolioID)
		if err != nil {
			logger.Errorf("Error find portfolio to get Entity ID: %v", err)
			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !common.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			return nil, err
		}

		if acc.AssetCode != cai.AssetCode {
			return nil, common.ValidateBusinessError(cn.ErrMismatchedAssetCode, reflect.TypeOf(a.Account{}).Name())
		}
	}

	if !common.IsNilOrEmpty(cai.Alias) {
		_, err := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, portfolioID, *cai.Alias)
		if err != nil {
			return nil, err
		}
	}

	account := &a.Account{
		ID:              common.GenerateUUIDv7().String(),
		AssetCode:       cai.AssetCode,
		Alias:           cai.Alias,
		Name:            cai.Name,
		Type:            cai.Type,
		ParentAccountID: cai.ParentAccountID,
		ProductID:       cai.ProductID,
		OrganizationID:  organizationID.String(),
		PortfolioID:     portfolioID.String(),
		LedgerID:        ledgerID.String(),
		EntityID:        *cai.EntityID,
		Balance:         balance,
		Status:          status,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	acc, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		logger.Errorf("Error creating account: %v", err)
		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(a.Account{}).Name(), acc.ID, cai.Metadata)
	if err != nil {
		logger.Errorf("Error creating account metadata: %v", err)
		return nil, err
	}

	acc.Metadata = metadata

	return acc, nil
}
