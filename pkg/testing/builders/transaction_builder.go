package builders

import (
	"time"
	"github.com/google/uuid"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// TransactionBuilder provides a fluent interface for creating test transactions
type TransactionBuilder struct {
	transaction *mmodel.Transaction
}

// NewTransactionBuilder creates a new transaction builder with defaults
func NewTransactionBuilder() *TransactionBuilder {
	id := uuid.New().String()
	return &TransactionBuilder{
		transaction: &mmodel.Transaction{
			ID:          id,
			Description: "Test Transaction",
			Amount:      1000,
			Currency:    "USD",
			Status:      mmodel.StatusPending,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Operations:  []mmodel.Operation{},
			Metadata:    map[string]interface{}{},
		},
	}
}

// WithID sets the transaction ID
func (b *TransactionBuilder) WithID(id string) *TransactionBuilder {
	b.transaction.ID = id
	return b
}

// WithAmount sets the transaction amount
func (b *TransactionBuilder) WithAmount(amount int64) *TransactionBuilder {
	b.transaction.Amount = amount
	return b
}

// WithCurrency sets the transaction currency
func (b *TransactionBuilder) WithCurrency(currency string) *TransactionBuilder {
	b.transaction.Currency = currency
	return b
}

// WithStatus sets the transaction status
func (b *TransactionBuilder) WithStatus(status mmodel.Status) *TransactionBuilder {
	b.transaction.Status = status
	return b
}

// WithOperation adds an operation to the transaction
func (b *TransactionBuilder) WithOperation(op mmodel.Operation) *TransactionBuilder {
	b.transaction.Operations = append(b.transaction.Operations, op)
	return b
}

// WithMetadata sets metadata key-value pairs
func (b *TransactionBuilder) WithMetadata(key string, value interface{}) *TransactionBuilder {
	b.transaction.Metadata[key] = value
	return b
}

// Build returns the constructed transaction
func (b *TransactionBuilder) Build() *mmodel.Transaction {
	return b.transaction
}

// BuildMany creates multiple transactions with variations
func (b *TransactionBuilder) BuildMany(count int) []*mmodel.Transaction {
	transactions := make([]*mmodel.Transaction, count)
	for i := 0; i < count; i++ {
		tx := *b.transaction
		tx.ID = uuid.New().String()
		tx.Amount = b.transaction.Amount + int64(i*100)
		transactions[i] = &tx
	}
	return transactions
}