package integration

import (
	"os"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestMain allows setup before any integration tests run.
func TestMain(m *testing.M) {
	_ = h.AuthenticateFromEnv()
	os.Exit(m.Run())
}
