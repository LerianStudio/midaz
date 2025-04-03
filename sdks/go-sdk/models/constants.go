package models

// Transaction status constants
const (
	// TransactionStatusPending represents a transaction that is not yet completed
	TransactionStatusPending = "pending"

	// TransactionStatusCompleted represents a successfully completed transaction
	TransactionStatusCompleted = "completed"

	// TransactionStatusFailed represents a transaction that failed to process
	TransactionStatusFailed = "failed"

	// TransactionStatusCancelled represents a transaction that was cancelled
	TransactionStatusCancelled = "cancelled"
)
