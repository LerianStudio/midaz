package bootstrap

import (
	"testing"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCountCachingCircuitBreaker_ValidParameters(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("test-service", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewCountCachingCircuitBreaker(cb, "test-service")

	require.NoError(t, err)
	require.NotNil(t, wrapper)
	assert.Equal(t, "test-service", wrapper.ServiceName())
}

func TestNewCountCachingCircuitBreaker_NilCircuitBreaker_ReturnsError(t *testing.T) {
	t.Parallel()

	wrapper, err := NewCountCachingCircuitBreaker(nil, "test-service")

	require.Error(t, err)
	assert.Nil(t, wrapper)
	assert.Contains(t, err.Error(), "circuit breaker cannot be nil")
}

func TestNewCountCachingCircuitBreaker_EmptyServiceName_ReturnsError(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("test-service", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewCountCachingCircuitBreaker(cb, "")

	require.Error(t, err)
	assert.Nil(t, wrapper)
	assert.Contains(t, err.Error(), "service name cannot be empty")
}
