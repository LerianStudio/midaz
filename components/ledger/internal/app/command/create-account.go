package command

import (
	"context"
	c "github.com/LerianStudio/midaz/common/constant"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// CreateAccount creates a new account persists data in the repository.
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID, portfolioID string, cai *a.CreateAccountInput) (*a.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create account: %v", cai)

	if common.IsNilOrEmpty(&cai.Name) {
		cai.Name = cai.AssetCode + " " + cai.Type + " account"
	}

	if common.IsNilOrEmpty(cai.Alias) {
		cai.Alias = nil
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

	if cai.EntityID == nil {
		portfolio, err := uc.PortfolioRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID))
		if err != nil {
			logger.Errorf("Error find portfolio to get Entity ID: %v", err)
			return nil, err
		}

		cai.EntityID = &portfolio.EntityID
	}

	if !common.IsNilOrEmpty(cai.ParentAccountID) {
		acc, err := uc.AccountRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			return nil, err
		}

		if acc.AssetCode != cai.AssetCode {
			return nil, c.ValidateBusinessError(c.MismatchedAssetCodeBusinessError, reflect.TypeOf(a.Account{}).Name())
		}
	}

	if !common.IsNilOrEmpty(cai.Alias) {
		_, err := uc.AccountRepo.FindByAlias(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID), *cai.Alias)
		if err != nil {
			return nil, err
		}
	}

	account := &a.Account{
		ID:              uuid.New().String(),
		AssetCode:       cai.AssetCode,
		Alias:           cai.Alias,
		Name:            cai.Name,
		Type:            cai.Type,
		ParentAccountID: cai.ParentAccountID,
		ProductID:       cai.ProductID,
		OrganizationID:  organizationID,
		PortfolioID:     portfolioID,
		LedgerID:        ledgerID,
		EntityID:        *cai.EntityID,
		Balance:         balance,
		Status:          status,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	port, err := uc.AccountRepo.Create(ctx, account)
	if err != nil {
		logger.Errorf("Error creating account: %v", err)
		return nil, err
	}

	if cai.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cai.Metadata); err != nil {
			return nil, c.ValidateBusinessError(err, reflect.TypeOf(a.Account{}).Name())
		}

		meta := m.Metadata{
			EntityID:   port.ID,
			EntityName: reflect.TypeOf(a.Account{}).Name(),
			Data:       cai.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(a.Account{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating account metadata: %v", err)
			return nil, err
		}

		port.Metadata = cai.Metadata
	}

	return port, nil
}
