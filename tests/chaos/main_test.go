package chaos

import (
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestMain runs all tests in the package with authentication helpers.
func TestMain(m *testing.M) {
	h.RunTestsWithAuth(m)
}
