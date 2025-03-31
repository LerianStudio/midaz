package transaction

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/mock"
)

// mockTransactionRepo is a mock implementation of the repository.Transaction interface
type mockTransactionRepo struct {
	mock.Mock
}

func (m *mockTransactionRepo) Create(organizationID, ledgerID string, inp mmodel.CreateTransactionInput) (*mmodel.Transaction, error) {
	args := m.Called(organizationID, ledgerID, inp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Transaction), args.Error(1)
}

func (m *mockTransactionRepo) CreateDSL(organizationID, ledgerID string, dslContent string) (*mmodel.Transaction, error) {
	args := m.Called(organizationID, ledgerID, dslContent)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Transaction), args.Error(1)
}

func (m *mockTransactionRepo) Get(organizationID, ledgerID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.Transactions, error) {
	args := m.Called(organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Transactions), args.Error(1)
}

func (m *mockTransactionRepo) GetByID(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error) {
	args := m.Called(organizationID, ledgerID, transactionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Transaction), args.Error(1)
}

func (m *mockTransactionRepo) Revert(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error) {
	args := m.Called(organizationID, ledgerID, transactionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Transaction), args.Error(1)
}
