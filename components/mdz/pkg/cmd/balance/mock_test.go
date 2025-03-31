package balance

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/mock"
)

// mockBalanceRepo is a mock implementation of the repository.Balance interface
type mockBalanceRepo struct {
	mock.Mock
}

func (m *mockBalanceRepo) GetByID(organizationID, ledgerID, balanceID string) (*mmodel.Balance, error) {
	args := m.Called(organizationID, ledgerID, balanceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Balance), args.Error(1)
}

func (m *mockBalanceRepo) Get(organizationID, ledgerID string, limit int, cursor, sortOrder, startDate, endDate string) (*mmodel.Balances, error) {
	args := m.Called(organizationID, ledgerID, limit, cursor, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Balances), args.Error(1)
}

func (m *mockBalanceRepo) GetByAccount(organizationID, ledgerID, accountID string, limit int, cursor, sortOrder, startDate, endDate string) (*mmodel.Balances, error) {
	args := m.Called(organizationID, ledgerID, accountID, limit, cursor, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Balances), args.Error(1)
}

func (m *mockBalanceRepo) Delete(organizationID, ledgerID, balanceID string) error {
	args := m.Called(organizationID, ledgerID, balanceID)
	return args.Error(0)
}
