package onboarding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitServiceOrError_RedisError tests that InitServiceOrError returns an error
// when Redis connection fails.
func TestInitServiceOrError_RedisError(t *testing.T) {
	// Set minimal config to pass config loading
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999") // Invalid port to force connection error

	var (
		service interface{}
		err     error
	)

	require.NotPanics(t, func() {
		service, err = InitServiceOrError()
	})

	assert.Nil(t, service)
	assert.Error(t, err)
}

// TestInitService_Deprecated_Panics tests that the deprecated InitService function
// still panics on error (backward compatibility).
func TestInitService_Deprecated_Panics(t *testing.T) {
	// Set minimal config but with invalid Redis to trigger panic
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999") // Invalid port to force error

	// Use recover to catch the expected panic
	defer func() {
		r := recover()
		assert.NotNil(t, r, "InitService should panic on Redis error")
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
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	var (
		service interface{}
		err     error
	)

	// Must not panic - should return error gracefully
	require.NotPanics(t, func() {
		service, err = InitServiceOrError()
	}, "InitServiceOrError must not panic - it should return errors")

	assert.Nil(t, service)
	assert.Error(t, err)
}

// TestInitServiceWithOptionsOrError_NilOptions tests that nil options work correctly.
func TestInitServiceWithOptionsOrError_NilOptions(t *testing.T) {
	// Set minimal config with invalid Redis
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	var (
		service interface{}
		err     error
	)

	// Must not panic with nil options - should return error gracefully
	require.NotPanics(t, func() {
		service, err = InitServiceWithOptionsOrError(nil)
	}, "InitServiceWithOptionsOrError must not panic with nil options")

	assert.Nil(t, service)
	assert.Error(t, err)
}
