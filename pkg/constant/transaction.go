package constant

// Transaction status constants represent the lifecycle states of a transaction.
const (
	// CREATED indicates a transaction has been created but not yet processed.
	CREATED = "CREATED"
	// APPROVED indicates a transaction has been validated and approved for execution.
	APPROVED = "APPROVED"
	// PENDING indicates a transaction is awaiting further action or validation.
	PENDING = "PENDING"
	// CANCELED indicates a transaction has been canceled and will not be executed.
	CANCELED = "CANCELED"
	// NOTED indicates a transaction has been recorded for informational purposes.
	NOTED = "NOTED"
	// UniqueViolationCode is the PostgreSQL error code for unique constraint violations.
	UniqueViolationCode = "23505"
)
