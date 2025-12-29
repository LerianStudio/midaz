//go:build integration
// +build integration

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitServers_ErrorHandling tests that InitServers properly handles errors
// from child module initialization.
func TestInitServers_ErrorHandling(t *testing.T) {
	// Minimal config to trigger errors in child modules.
	// This test is a guard rail for the contract: initialization must return errors, not panic.
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "test")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_PASSWORD", "test")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("MONGO_PORT", "27017")
	t.Setenv("REDIS_HOST", "localhost:9999") // invalid on purpose

	var (
		service *Service
		err     error
	)

	// Fail fast: if this panics, the rest of the assertions are meaningless and may cascade.
	require.NotPanics(t, func() {
		service, err = InitServers()
	})

	require.Nil(t, service)
	require.Error(t, err)
	assert.NotEmpty(t, err.Error())
}
