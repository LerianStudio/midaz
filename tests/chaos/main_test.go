package chaos

import (
	"os"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestMain(m *testing.M) {
	_ = h.AuthenticateFromEnv()
	os.Exit(m.Run())
}
