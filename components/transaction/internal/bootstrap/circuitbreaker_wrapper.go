package bootstrap

import (
	"errors"
	"sync"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// ErrNilCircuitBreaker is returned when a nil circuit breaker is passed to NewCountCachingCircuitBreaker.
var ErrNilCircuitBreaker = errors.New("circuit breaker cannot be nil")

// ErrEmptyServiceName is returned when an empty service name is passed to NewCountCachingCircuitBreaker.
var ErrEmptyServiceName = errors.New("service name cannot be empty")

// CountCachingCircuitBreaker wraps a CircuitBreaker and caches counts before each Execute call.
// This solves the race condition where gobreaker resets counts during state transitions
// before listeners can retrieve them via GetCounts().
type CountCachingCircuitBreaker struct {
	cb          libCircuitBreaker.CircuitBreaker
	serviceName string
	mu          sync.RWMutex
	lastCounts  libCircuitBreaker.Counts
}

// NewCountCachingCircuitBreaker creates a new count-caching wrapper around the given circuit breaker.
// Returns an error if cb is nil or serviceName is empty.
func NewCountCachingCircuitBreaker(cb libCircuitBreaker.CircuitBreaker, serviceName string) (*CountCachingCircuitBreaker, error) {
	if cb == nil {
		return nil, ErrNilCircuitBreaker
	}

	if serviceName == "" {
		return nil, ErrEmptyServiceName
	}

	return &CountCachingCircuitBreaker{
		cb:          cb,
		serviceName: serviceName,
		lastCounts:  libCircuitBreaker.Counts{},
	}, nil
}

// ServiceName returns the service name associated with this circuit breaker.
func (c *CountCachingCircuitBreaker) ServiceName() string {
	return c.serviceName
}

// Execute delegates to the underlying circuit breaker after caching current counts.
// The counts are captured BEFORE the operation runs, ensuring we have the pre-transition state.
func (c *CountCachingCircuitBreaker) Execute(fn func() (any, error)) (any, error) {
	// Capture counts BEFORE executing - this is the critical fix
	c.cacheCurrentCounts()

	return c.cb.Execute(fn)
}

// State returns the current state of the circuit breaker.
func (c *CountCachingCircuitBreaker) State() libCircuitBreaker.State {
	return c.cb.State()
}

// Counts returns the current counts from the underlying circuit breaker.
func (c *CountCachingCircuitBreaker) Counts() libCircuitBreaker.Counts {
	return c.cb.Counts()
}

// GetLastKnownCounts returns the cached counts from before the last Execute call.
// This method should be used by listeners instead of manager.GetCounts() to avoid
// the race condition where counts are reset during state transitions.
func (c *CountCachingCircuitBreaker) GetLastKnownCounts() libCircuitBreaker.Counts {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastCounts
}

// cacheCurrentCounts captures the current counts from the underlying circuit breaker.
func (c *CountCachingCircuitBreaker) cacheCurrentCounts() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastCounts = c.cb.Counts()
}
