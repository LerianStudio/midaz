// Package fuzzy provides fuzz testing for the Midaz API.
// This file contains the main entry point for running the fuzz tests.
package fuzzy

import (
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestMain sets up and runs the tests in the fuzzy package with authentication helpers.
func TestMain(m *testing.M) {
	h.RunTestsWithAuth(m)
}
