package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitServiceOrError_ValidatesErrorHandling tests that InitServiceOrError
// properly wraps errors from InitServers instead of panicking.
func TestInitServiceOrError_ValidatesErrorHandling(t *testing.T) {
	// Set minimal required env vars to pass config loading
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "test")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_PASSWORD", "test")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("MONGO_PORT", "27017")
	t.Setenv("REDIS_HOST", "localhost:9999") // Invalid to trigger error

	var (
		service TransactionService
		err     error
	)

	// Must not panic - should return error gracefully
	require.NotPanics(t, func() {
		service, err = InitServiceOrError()
	}, "InitServiceOrError must not panic - it should return errors")

	assert.Nil(t, service)
	assert.Error(t, err)
}

// TestInitService_Deprecated_Panics tests that the deprecated InitService function
// still panics on error (backward compatibility).
func TestInitService_Deprecated_Panics(t *testing.T) {
	// Set minimal config to trigger Redis error
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "test")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_PASSWORD", "test")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("MONGO_PORT", "27017")
	t.Setenv("REDIS_HOST", "localhost:9999")

	// Use recover to catch the expected panic
	defer func() {
		r := recover()
		assert.NotNil(t, r, "InitService should panic on error")
		t.Logf("InitService panicked as expected: %v", r)
	}()

	// This should panic
	_ = InitService()

	// If we get here, the test failed
	t.Error("InitService did not panic as expected")
}

// TestInitServiceOrError_NoPanic verifies InitServiceOrError doesn't panic and returns an error.
func TestInitServiceOrError_NoPanic(t *testing.T) {
	// Set minimal config with invalid Redis
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "test")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_PASSWORD", "test")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("MONGO_PORT", "27017")
	t.Setenv("REDIS_HOST", "localhost:9999")

	var (
		service TransactionService
		err     error
	)

	// Must not panic - should return error gracefully
	require.NotPanics(t, func() {
		service, err = InitServiceOrError()
	}, "InitServiceOrError must not panic - it should return errors")

	assert.Nil(t, service)
	assert.Error(t, err)
}
