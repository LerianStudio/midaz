package constant

// Operation types used in the ledger when processing transactions.
const (
	// DEBIT represents a debit operation.
	DEBIT = "DEBIT"
	// CREDIT represents a credit operation.
	CREDIT = "CREDIT"
	// ONHOLD represents a hold (reservation) operation.
	ONHOLD = "ON_HOLD"
	// RELEASE represents a release of a previously held amount.
	RELEASE = "RELEASE"
)
