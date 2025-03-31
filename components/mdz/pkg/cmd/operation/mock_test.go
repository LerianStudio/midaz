package operation

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/mock"
)

// mockOperationRepo is a mock implementation of the repository.Operation interface
type mockOperationRepo struct {
	mock.Mock
}

func (m *mockOperationRepo) GetByID(organizationID, ledgerID, operationID string) (*mmodel.Operation, error) {
	args := m.Called(organizationID, ledgerID, operationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Operation), args.Error(1)
}

func (m *mockOperationRepo) Get(organizationID, ledgerID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.Operations, error) {
	args := m.Called(organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Operations), args.Error(1)
}

func (m *mockOperationRepo) GetByAccount(organizationID, ledgerID, accountID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.Operations, error) {
	args := m.Called(organizationID, ledgerID, accountID, limit, page, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.Operations), args.Error(1)
}
