package helpers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunChaosTests_SkipsBeforeAuthWhenChaosDisabled(t *testing.T) {
	t.Setenv("CHAOS", "")
	t.Setenv("TEST_AUTH_URL", "https://auth.example.com/token")
	t.Setenv("TEST_AUTH_USERNAME", "")
	t.Setenv("TEST_AUTH_PASSWORD", "")

	require.Equal(t, 0, runChaosTests(nil))
}
