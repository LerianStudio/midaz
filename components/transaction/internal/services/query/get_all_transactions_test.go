package query

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/testing/builders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTransactionRepository is a mock implementation
type MockTransactionRepository struct {
	mock.Mock
}

func (m *MockTransactionRepository) FindAll(ctx context.Context, params map[string]interface{}) ([]*mmodel.Transaction, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*mmodel.Transaction), args.Error(1)
}

func (m *MockTransactionRepository) FindByID(ctx context.Context, id string) (*mmodel.Transaction, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*mmodel.Transaction), args.Error(1)
}

// Table-driven tests for GetAllTransactions
func TestGetAllTransactions(t *testing.T) {
	tests := []struct {
		name          string
		params        map[string]interface{}
		mockSetup     func(*MockTransactionRepository)
		expectedCount int
		expectedError bool
		checkResult   func(*testing.T, []*mmodel.Transaction)
	}{
		{
			name:   "successful fetch with no filters",
			params: map[string]interface{}{},
			mockSetup: func(m *MockTransactionRepository) {
				transactions := builders.NewTransactionBuilder().BuildMany(5)
				m.On("FindAll", mock.Anything, mock.Anything).Return(transactions, nil)
			},
			expectedCount: 5,
			expectedError: false,
		},
		{
			name: "fetch with status filter",
			params: map[string]interface{}{
				"status": "completed",
			},
			mockSetup: func(m *MockTransactionRepository) {
				builder := builders.NewTransactionBuilder().
					WithStatus(mmodel.StatusCompleted)
				transactions := builder.BuildMany(3)
				m.On("FindAll", mock.Anything, mock.Anything).Return(transactions, nil)
			},
			expectedCount: 3,
			expectedError: false,
			checkResult: func(t *testing.T, txs []*mmodel.Transaction) {
				for _, tx := range txs {
					assert.Equal(t, mmodel.StatusCompleted, tx.Status)
				}
			},
		},
		{
			name: "fetch with date range filter",
			params: map[string]interface{}{
				"from_date": time.Now().AddDate(0, 0, -7),
				"to_date":   time.Now(),
			},
			mockSetup: func(m *MockTransactionRepository) {
				m.On("FindAll", mock.Anything, mock.Anything).Return([]*mmodel.Transaction{}, nil)
			},
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:   "repository error",
			params: map[string]interface{}{},
			mockSetup: func(m *MockTransactionRepository) {
				m.On("FindAll", mock.Anything, mock.Anything).Return([]*mmodel.Transaction(nil), assert.AnError)
			},
			expectedCount: 0,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockRepo := new(MockTransactionRepository)
			tt.mockSetup(mockRepo)

			svc := &TransactionQueryService{
				repo: mockRepo,
			}

			// Act
			result, err := svc.GetAllTransactions(context.Background(), tt.params)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)

				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// Benchmark tests
func BenchmarkGetAllTransactions(b *testing.B) {
	mockRepo := new(MockTransactionRepository)
	transactions := builders.NewTransactionBuilder().BuildMany(100)
	mockRepo.On("FindAll", mock.Anything, mock.Anything).Return(transactions, nil)

	svc := &TransactionQueryService{repo: mockRepo}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.GetAllTransactions(ctx, map[string]interface{}{})
	}
}

// Property-based test example
func TestTransactionAmountInvariants(t *testing.T) {
	// Property: Total amount of operations should equal transaction amount
	builder := builders.NewTransactionBuilder().WithAmount(10000)

	// Add operations that should sum to transaction amount
	builder.WithOperation(mmodel.Operation{Amount: 6000})
	builder.WithOperation(mmodel.Operation{Amount: 4000})

	tx := builder.Build()

	var totalOps int64
	for _, op := range tx.Operations {
		totalOps += op.Amount
	}

	assert.Equal(t, tx.Amount, totalOps, "Transaction amount should equal sum of operations")
}
