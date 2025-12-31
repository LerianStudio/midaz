package fuzzy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// assertionPanicRecovery returns true if the panic was an assertion failure (expected),
// false if it was an unexpected panic, and does not recover if no panic occurred.
func assertionPanicRecovery(t *testing.T, panicValue any, context string) bool {
	t.Helper()
	if panicValue == nil {
		return true // no panic
	}

	msg := fmt.Sprintf("%v", panicValue)
	if strings.Contains(msg, "assertion failed") {
		return true // expected assertion panic
	}

	t.Errorf("Unexpected panic (not assertion) in %s: %v", context, panicValue)
	return false
}
