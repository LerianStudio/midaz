package abstractions

import (
	"context"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/mock"
)

// MockAbstraction is a mock implementation of the Abstraction struct for testing
type MockAbstraction struct {
	mock.Mock
}

// createTransactionWithDSL is a mock implementation of the createTransactionWithDSL function
func (m *MockAbstraction) createTransactionWithDSL(ctx context.Context, organizationID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
	args := m.Called(ctx, organizationID, ledgerID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Transaction), args.Error(1)
}
