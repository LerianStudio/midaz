package constant

// CREATED represents a transaction status indicating it has been created.
const (
	CREATED             = "CREATED"
	APPROVED            = "APPROVED"
	PENDING             = "PENDING"
	CANCELED            = "CANCELED"
	NOTED               = "NOTED"
	UniqueViolationCode = "23505"
)

// BalanceStatus represents the state of async balance updates for a transaction.
// Used for saga-like consistency tracking.
// NOTE: These values intentionally overlap with other status strings (e.g., transaction lifecycle PENDING).
// Always reference them via the BalanceStatus* constants to avoid confusion.
type BalanceStatus string

const (
	// BalanceStatusPending indicates balance update is queued but not yet confirmed.
	BalanceStatusPending BalanceStatus = "PENDING"
	// BalanceStatusConfirmed indicates balance update completed successfully.
	BalanceStatusConfirmed BalanceStatus = "CONFIRMED"
	// BalanceStatusFailed indicates balance update failed after max retries (in DLQ).
	BalanceStatusFailed BalanceStatus = "FAILED"
)

// String returns the string representation of BalanceStatus.
func (b BalanceStatus) String() string {
	return string(b)
}

// Ptr returns a pointer to the BalanceStatus value.
// Useful for assigning to *BalanceStatus fields.
func (b BalanceStatus) Ptr() *BalanceStatus {
	return &b
}
