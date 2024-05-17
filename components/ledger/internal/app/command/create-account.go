package command

import (
	"context"
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

	var status a.Status
	if cai.Status.IsEmpty() {
		status = a.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cai.Status
	}

	balanceValue := float64(0)

	var balance a.Balance
	if cai.Balance.IsEmpty() {
		balance = a.Balance{
			Available: &balanceValue,
			OnHold:    &balanceValue,
			Scale:     &balanceValue,
		}
	} else {
		balance = cai.Balance
	}

	var entityID string

	if cai.EntityID == nil {
		portfolio, err := uc.PortfolioRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(portfolioID))
		if err != nil {
			logger.Errorf("Error find portfolio to get Entity ID: %v", err)
			return nil, err
		}

		entityID = portfolio.EntityID
	} else {
		entityID = *cai.EntityID
	}

	account := &a.Account{
		ID:              uuid.New().String(),
		InstrumentCode:  cai.InstrumentCode,
		Alias:           cai.Alias,
		Name:            cai.Name,
		Type:            cai.Type,
		ParentAccountID: cai.ParentAccountID,
		ProductID:       cai.ProductID,
		OrganizationID:  organizationID,
		PortfolioID:     portfolioID,
		LedgerID:        ledgerID,
		EntityID:        entityID,
		Balance:         balance,
		Status:          status,
		AllowSending:    true,
		AllowReceiving:  true,
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
			return nil, err
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
