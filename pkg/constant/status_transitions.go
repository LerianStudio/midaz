package constant

import "github.com/LerianStudio/midaz/v3/pkg/assert"

// ValidTransactionStatuses is the set of valid transaction status codes.
var ValidTransactionStatuses = map[string]bool{
	CREATED:  true,
	PENDING:  true,
	APPROVED: true,
	CANCELED: true,
	NOTED:    true,
}

// ValidStatusTransitions defines allowed status transitions.
// Terminal states (APPROVED, CANCELED, NOTED) have no valid transitions.
var ValidStatusTransitions = map[string][]string{
	CREATED:  {PENDING, APPROVED, NOTED},
	PENDING:  {APPROVED, CANCELED},
	APPROVED: {}, // Terminal
	CANCELED: {}, // Terminal
	NOTED:    {}, // Terminal
}

// TerminalStatuses are statuses that cannot transition to other statuses.
var TerminalStatuses = map[string]bool{
	APPROVED: true,
	CANCELED: true,
	NOTED:    true,
}

// AssertValidStatusCode panics if status code is unknown.
// Use for validating status codes from internal sources (programming errors).
func AssertValidStatusCode(code string) {
	assert.That(ValidTransactionStatuses[code],
		"unknown transaction status code",
		"code", code)
}

// AssertValidStatusTransition panics if transition is not allowed.
// Use for validating state machine transitions (programming errors).
func AssertValidStatusTransition(from, to string) {
	// First validate both codes
	AssertValidStatusCode(from)
	AssertValidStatusCode(to)

	allowed := ValidStatusTransitions[from]
	for _, s := range allowed {
		if s == to {
			return
		}
	}

	assert.Never("invalid status transition",
		"from", from,
		"to", to,
		"allowed", allowed)
}

// IsTerminalStatus returns true if the status is a terminal state
// (cannot transition to any other status).
func IsTerminalStatus(status string) bool {
	return TerminalStatuses[status]
}

// GetAllowedTransitions returns the list of statuses that can be transitioned
// to from the given status. Returns nil for terminal states.
func GetAllowedTransitions(status string) []string {
	return ValidStatusTransitions[status]
}
