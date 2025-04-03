// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
)

// MockTransactionsService implements the TransactionsServiceInterface for testing
type MockTransactionsService struct {
	// ReturnTransaction is the transaction to return from the mock functions
	ReturnTransaction *models.Transaction
	// ReturnTransactionList is the list response to return from list functions
	ReturnTransactionList *models.ListResponse[models.Transaction]
	// ReturnError is the error to return from the mock functions
	ReturnError error
}

// ListTransactions is a mock implementation
func (m *MockTransactionsService) ListTransactions(
	ctx context.Context,
	organizationID, ledgerID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Transaction], error) {
	return m.ReturnTransactionList, m.ReturnError
}

// GetTransaction is a mock implementation
func (m *MockTransactionsService) GetTransaction(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
) (*models.Transaction, error) {
	return m.ReturnTransaction, m.ReturnError
}

// UpdateTransaction is a mock implementation
func (m *MockTransactionsService) UpdateTransaction(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
	input any,
) (*models.Transaction, error) {
	return m.ReturnTransaction, m.ReturnError
}

func TestNewAbstraction(t *testing.T) {
	// Create a mock implementation of the createTransactionWithDSL function
	mockCreateTx := func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
		return &models.Transaction{
			ID:     "tx-mock-123",
			Status: models.Status{Code: models.TransactionStatusCompleted},
		}, nil
	}

	// Create a mock transactions service
	mockTxService := &MockTransactionsService{
		ReturnTransaction: &models.Transaction{
			ID:     "tx-mock-123",
			Status: models.Status{Code: models.TransactionStatusCompleted},
		},
		ReturnTransactionList: &models.ListResponse[models.Transaction]{
			Items: []models.Transaction{
				{
					ID:     "tx-mock-123",
					Status: models.Status{Code: models.TransactionStatusCompleted},
				},
			},
			Pagination: models.Pagination{
				Limit:  10,
				Offset: 0,
				Total:  1,
			},
		},
	}

	// Test creating a new abstraction
	abstraction := NewAbstraction(mockCreateTx, mockTxService)

	// Verify the abstraction was created correctly
	assert.NotNil(t, abstraction)
	assert.NotNil(t, abstraction.createTransactionWithDSL)
	assert.NotNil(t, abstraction.transactionsService)
	assert.NotNil(t, abstraction.Deposits)
	assert.NotNil(t, abstraction.Withdrawals)
	assert.NotNil(t, abstraction.Transfers)
}

// MockTransactionCreator is a test helper that records calls to createTransactionWithDSL
type MockTransactionCreator struct {
	// LastInput captures the last input passed to the createTransactionWithDSL function
	LastInput *models.TransactionDSLInput
	// LastOrgID captures the last organization ID passed to the function
	LastOrgID string
	// LastLedgerID captures the last ledger ID passed to the function
	LastLedgerID string
	// ReturnTransaction is the transaction to return from the mock function
	ReturnTransaction *models.Transaction
	// ReturnError is the error to return from the mock function
	ReturnError error
	// CallCount tracks how many times the function was called
	CallCount int
}

// CreateTransactionWithDSL is a mock implementation that records calls and returns predefined results
func (m *MockTransactionCreator) CreateTransactionWithDSL(
	ctx context.Context,
	orgID, ledgerID string,
	input *models.TransactionDSLInput,
) (*models.Transaction, error) {
	m.LastInput = input
	m.LastOrgID = orgID
	m.LastLedgerID = ledgerID
	m.CallCount++
	return m.ReturnTransaction, m.ReturnError
}

// NewMockTransactionCreator creates a new MockTransactionCreator with default success values
func NewMockTransactionCreator() *MockTransactionCreator {
	return &MockTransactionCreator{
		ReturnTransaction: &models.Transaction{
			ID:     "tx-mock-123",
			Status: models.Status{Code: models.TransactionStatusCompleted},
		},
	}
}
