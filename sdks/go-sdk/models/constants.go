package models

// Transaction status constants define the possible states of a transaction in the Midaz system.
// These constants are used throughout the SDK to represent transaction statuses in a consistent way.
//
// Transaction Lifecycle:
// 1. A transaction is created with status "pending" if it requires explicit commitment
// 2. When committed, the transaction transitions to "completed"
// 3. If issues occur, the transaction may transition to "failed"
// 4. A pending transaction can be cancelled, transitioning to "cancelled"
//
// Usage Examples:
//
//	// Check if a transaction is pending and needs to be committed
//	if transaction.Status == models.TransactionStatusPending {
//	    // Commit the transaction
//	    committedTx, err := client.Transactions.CommitTransaction(
//	        context.Background(),
//	        "org-123",
//	        "ledger-456",
//	        transaction.ID,
//	    )
//	}
//
//	// Handle different transaction statuses
//	switch transaction.Status {
//	case models.TransactionStatusCompleted:
//	    fmt.Println("Transaction completed successfully")
//	case models.TransactionStatusPending:
//	    fmt.Println("Transaction is pending commitment")
//	case models.TransactionStatusFailed:
//	    fmt.Println("Transaction failed: ", transaction.FailureReason)
//	case models.TransactionStatusCancelled:
//	    fmt.Println("Transaction was cancelled")
//	}
const (
	// TransactionStatusPending represents a transaction that is not yet completed
	// Pending transactions have been created but require explicit commitment
	// before their operations are applied to account balances. This status
	// is useful for implementing approval workflows or two-phase commits.
	TransactionStatusPending = "pending"

	// TransactionStatusCompleted represents a successfully completed transaction
	// Completed transactions have been fully processed and their operations
	// have been applied to the relevant account balances. This is the final
	// state for successful transactions.
	TransactionStatusCompleted = "completed"

	// TransactionStatusFailed represents a transaction that failed to process
	// Failed transactions encountered an error during processing and were
	// not applied to account balances. The transaction's FailureReason field
	// provides details about why the transaction failed.
	TransactionStatusFailed = "failed"

	// TransactionStatusCancelled represents a transaction that was cancelled
	// Cancelled transactions were explicitly cancelled before being committed.
	// Only pending transactions can be cancelled; completed transactions cannot
	// be reversed through cancellation.
	TransactionStatusCancelled = "cancelled"
)

// Account status constants define the possible states of an account in the Midaz system.
// These constants are used throughout the SDK to represent account statuses in a consistent way.
const (
	// StatusActive represents an active resource that can be used normally
	// Active accounts can participate in transactions as both source and destination.
	StatusActive = "ACTIVE"

	// StatusInactive represents a temporarily inactive resource
	// Inactive accounts cannot participate in new transactions but can be reactivated.
	StatusInactive = "INACTIVE"

	// StatusPending represents a resource awaiting activation or approval
	// Pending accounts are in the process of being set up or approved.
	StatusPending = "PENDING"

	// StatusClosed represents a permanently closed resource
	// Closed accounts cannot participate in new transactions and cannot be reopened.
	StatusClosed = "CLOSED"
)
