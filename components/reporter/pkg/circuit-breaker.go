// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"sync"

	"github.com/LerianStudio/reporter/pkg/constant"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/sony/gobreaker"
)

//go:generate mockgen --destination=circuit-breaker.mock.go --package=pkg --copyright_file=../COPYRIGHT . CircuitBreakerExecutor

// Compile-time interface satisfaction check.
var _ CircuitBreakerExecutor = (*CircuitBreakerManager)(nil)

// CircuitBreakerExecutor defines the interface for executing operations through a circuit breaker.
type CircuitBreakerExecutor interface {
	// Execute runs a function through the circuit breaker for the given datasource.
	Execute(datasourceName string, fn func() (any, error)) (any, error)
	// IsHealthy returns true if the circuit breaker for the datasource is in a healthy state.
	IsHealthy(datasourceName string) bool
	// GetState returns the current state of a circuit breaker as a string.
	GetState(datasourceName string) string
}

// CircuitBreakerManager manages circuit breakers for datasources
type CircuitBreakerManager struct {
	breakers map[string]*gobreaker.CircuitBreaker
	mu       sync.RWMutex
	logger   log.Logger
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(logger log.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*gobreaker.CircuitBreaker),
		logger:   logger,
	}
}

// newSettings returns the default gobreaker.Settings for a given datasource name.
func (cbm *CircuitBreakerManager) newSettings(datasourceName string) gobreaker.Settings {
	return gobreaker.Settings{
		Name:        fmt.Sprintf("datasource-%s", datasourceName),
		MaxRequests: constant.CircuitBreakerMaxRequests,
		Interval:    constant.CircuitBreakerInterval,
		Timeout:     constant.CircuitBreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.ConsecutiveFailures >= constant.CircuitBreakerThreshold {
				return true
			}

			if counts.Requests >= constant.CircuitBreakerMinRequests {
				failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
				return failureRatio >= constant.CircuitBreakerFailureRatio
			}

			return false
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbm.logger.Log(context.Background(), log.LevelWarn, "Circuit Breaker state changed", log.String("name", name), log.String("from", from.String()), log.String("to", to.String()))

			switch to {
			case gobreaker.StateOpen:
				cbm.logger.Log(context.Background(), log.LevelError, "Circuit Breaker OPENED - datasource is unhealthy, requests will fast-fail", log.String("name", name))
			case gobreaker.StateHalfOpen:
				cbm.logger.Log(context.Background(), log.LevelInfo, "Circuit Breaker HALF-OPEN - testing datasource recovery", log.String("name", name))
			case gobreaker.StateClosed:
				cbm.logger.Log(context.Background(), log.LevelInfo, "Circuit Breaker CLOSED - datasource is healthy", log.String("name", name))
			}
		},
	}
}

// GetOrCreate returns existing circuit breaker or creates a new one
func (cbm *CircuitBreakerManager) GetOrCreate(datasourceName string) *gobreaker.CircuitBreaker {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[datasourceName]
	cbm.mu.RUnlock()

	if exists {
		return breaker
	}

	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	// Double-check after acquiring write lock
	if breaker, exists = cbm.breakers[datasourceName]; exists {
		return breaker
	}

	breaker = gobreaker.NewCircuitBreaker(cbm.newSettings(datasourceName))
	cbm.breakers[datasourceName] = breaker
	cbm.logger.Log(context.Background(), log.LevelInfo, "Created circuit breaker for datasource", log.String("datasource", datasourceName))

	return breaker
}

// Execute runs a function through the circuit breaker
func (cbm *CircuitBreakerManager) Execute(datasourceName string, fn func() (any, error)) (any, error) {
	breaker := cbm.GetOrCreate(datasourceName)

	result, err := breaker.Execute(fn)
	if err != nil {
		if err == gobreaker.ErrOpenState {
			cbm.logger.Log(context.Background(), log.LevelWarn, "Circuit breaker is OPEN - request rejected immediately", log.String("datasource", datasourceName))
			return nil, fmt.Errorf("datasource %s is currently unavailable (circuit breaker open): %w", datasourceName, err)
		}

		if err == gobreaker.ErrTooManyRequests {
			cbm.logger.Log(context.Background(), log.LevelWarn, "Circuit breaker is HALF-OPEN - too many test requests", log.String("datasource", datasourceName))
			return nil, fmt.Errorf("datasource %s is recovering (too many requests): %w", datasourceName, err)
		}
	}

	return result, err
}

// GetState returns the current state of a circuit breaker
func (cbm *CircuitBreakerManager) GetState(datasourceName string) string {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[datasourceName]
	cbm.mu.RUnlock()

	if !exists {
		return "not_initialized"
	}

	state := breaker.State()
	switch state {
	case gobreaker.StateClosed:
		return constant.CircuitBreakerStateClosed
	case gobreaker.StateOpen:
		return constant.CircuitBreakerStateOpen
	case gobreaker.StateHalfOpen:
		return constant.CircuitBreakerStateHalfOpen
	default:
		return "unknown"
	}
}

// GetCounts returns the current counts for a circuit breaker
func (cbm *CircuitBreakerManager) GetCounts(datasourceName string) gobreaker.Counts {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[datasourceName]
	cbm.mu.RUnlock()

	if !exists {
		return gobreaker.Counts{}
	}

	return breaker.Counts()
}

// Reset resets a circuit breaker to closed state by creating a new instance
func (cbm *CircuitBreakerManager) Reset(datasourceName string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	if _, exists := cbm.breakers[datasourceName]; exists {
		cbm.logger.Log(context.Background(), log.LevelInfo, "Manually resetting circuit breaker for datasource", log.String("datasource", datasourceName))

		cbm.breakers[datasourceName] = gobreaker.NewCircuitBreaker(cbm.newSettings(datasourceName))
		cbm.logger.Log(context.Background(), log.LevelInfo, "Circuit breaker reset completed for datasource", log.String("datasource", datasourceName))
	}
}

// IsHealthy returns true if the circuit breaker is in a healthy state (closed or half-open)
// Returns false if the circuit breaker is open (datasource is unhealthy)
func (cbm *CircuitBreakerManager) IsHealthy(datasourceName string) bool {
	state := cbm.GetState(datasourceName)
	return state != constant.CircuitBreakerStateOpen
}

// ShouldAllowRetry determines if a retry should be attempted based on circuit breaker state
func (cbm *CircuitBreakerManager) ShouldAllowRetry(datasourceName string) bool {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[datasourceName]
	cbm.mu.RUnlock()

	if !exists {
		return true
	}

	state := breaker.State()
	counts := breaker.Counts()

	if state == gobreaker.StateOpen {
		cbm.logger.Log(context.Background(), log.LevelWarn, "Circuit breaker is OPEN - blocking retry attempt", log.String("datasource", datasourceName))
		return false
	}

	if state == gobreaker.StateHalfOpen && counts.Requests >= constant.CircuitBreakerMaxRequests {
		cbm.logger.Log(context.Background(), log.LevelWarn, "Circuit breaker is HALF-OPEN and at max capacity - blocking retry attempt", log.String("datasource", datasourceName))
		return false
	}

	return true
}
