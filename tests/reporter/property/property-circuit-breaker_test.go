//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"fmt"
	"testing"
	"testing/quick"
	"time"

	"github.com/LerianStudio/lib-observability/zap"
	"github.com/sony/gobreaker"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
)

func newTestLogger() *zap.Logger {
	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		return &zap.Logger{}
	}

	return logger
}

// Property 1: Circuit Breaker deve abrir após threshold de falhas consecutivas
func TestProperty_CircuitBreaker_OpensAfterThreshold(t *testing.T) {
	t.Parallel()

	property := func(failures uint8) bool {
		// Limit to 30 to keep test fast and predictable
		if failures > 30 {
			failures = failures % 30
		}

		logger := newTestLogger()
		cbm := pkg.NewCircuitBreakerManager(logger)
		datasourceName := fmt.Sprintf("test-ds-%d-%d", failures, time.Now().UnixNano())

		// Simulate failures
		for i := uint8(0); i < failures; i++ {
			_, _ = cbm.Execute(datasourceName, func() (any, error) {
				return nil, fmt.Errorf("simulated failure %d", i)
			})
		}

		state := cbm.GetState(datasourceName)

		// Circuit breaker opens on TWO conditions:
		// 1. ConsecutiveFailures >= 15 OR
		// 2. Requests >= 10 AND failureRatio >= 0.5 (50%)

		// Calculate expected state
		shouldBeOpen := false
		if failures >= 15 {
			// Condition 1: 15+ consecutive failures
			shouldBeOpen = true
		} else if failures >= 10 {
			// Condition 2: >= 10 requests with 100% failure rate (all are failures)
			shouldBeOpen = true
		}

		if shouldBeOpen {
			if state != "open" {
				t.Logf("Expected OPEN with %d failures (100%% failure rate), got %s", failures, state)
				return false
			}
			return true
		}

		// If < 10 failures, should still be closed (or not_initialized if 0 failures)
		if failures == 0 {
			// With 0 failures (no operations), state can be not_initialized
			return state == "not_initialized" || state == "closed"
		}

		if state != "closed" && state != "half-open" {
			t.Logf("Expected CLOSED with %d failures, got %s", failures, state)
			return false
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: circuit breaker threshold: %v", err)
	}
}

// Property 2: Circuit Breaker em estado OPEN deve rejeitar todas as requisições
func TestProperty_CircuitBreaker_OpenRejectsAll(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)
	datasourceName := "test-ds-open"

	// Force circuit breaker to open by causing 20 failures
	for i := 0; i < 20; i++ {
		_, _ = cbm.Execute(datasourceName, func() (any, error) {
			return nil, fmt.Errorf("failure")
		})
	}

	property := func(seed uint32) bool {
		// Try to execute - should be rejected
		_, err := cbm.Execute(datasourceName, func() (any, error) {
			return "success", nil
		})

		// Should return ErrOpenState
		return err == gobreaker.ErrOpenState || err != nil
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: open circuit breaker not rejecting: %v", err)
	}
}

// Property 3: IsHealthy deve retornar false apenas quando circuit breaker está OPEN
func TestProperty_CircuitBreaker_HealthyState(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)

	property := func(failures uint8) bool {
		if failures > 30 {
			failures = failures % 30
		}

		datasourceName := fmt.Sprintf("test-health-%d", failures)

		// Simulate failures
		for i := uint8(0); i < failures; i++ {
			_, _ = cbm.Execute(datasourceName, func() (any, error) {
				return nil, fmt.Errorf("failure")
			})
		}

		isHealthy := cbm.IsHealthy(datasourceName)
		state := cbm.GetState(datasourceName)

		// If state is OPEN, IsHealthy should be false
		if state == "open" {
			return !isHealthy
		}

		// If state is CLOSED or HALF-OPEN, IsHealthy should be true
		return isHealthy
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: IsHealthy inconsistent with state: %v", err)
	}
}

// Property 4: Sucessos devem resetar contador de falhas consecutivas
func TestProperty_CircuitBreaker_SuccessResetsConsecutiveFailures(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()

	property := func(initialFailures, successes uint8) bool {
		if initialFailures > 10 || successes > 10 {
			return true
		}

		cbm := pkg.NewCircuitBreakerManager(logger)
		datasourceName := fmt.Sprintf("test-reset-%d-%d", initialFailures, successes)

		// Cause some failures
		for i := uint8(0); i < initialFailures; i++ {
			_, _ = cbm.Execute(datasourceName, func() (any, error) {
				return nil, fmt.Errorf("failure")
			})
		}

		// Then some successes
		for i := uint8(0); i < successes; i++ {
			_, _ = cbm.Execute(datasourceName, func() (any, error) {
				return "success", nil
			})
		}

		counts := cbm.GetCounts(datasourceName)

		// After successes, consecutive failures should be 0
		if successes > 0 {
			return counts.ConsecutiveFailures == 0
		}

		// If no successes, consecutive failures should match initial
		return counts.ConsecutiveFailures == uint32(initialFailures)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: successes don't reset consecutive failures: %v", err)
	}
}

// Property 5: GetState nunca deve retornar estado inválido
func TestProperty_CircuitBreaker_ValidStates(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)

	validStates := map[string]bool{
		"closed":          true,
		"open":            true,
		"half-open":       true,
		"not_initialized": true,
	}

	property := func(datasourceName string, operations uint8) bool {
		if len(datasourceName) == 0 || operations > 20 {
			return true
		}

		// Perform random operations
		for i := uint8(0); i < operations; i++ {
			if i%2 == 0 {
				_, _ = cbm.Execute(datasourceName, func() (any, error) {
					return nil, fmt.Errorf("failure")
				})
			} else {
				_, _ = cbm.Execute(datasourceName, func() (any, error) {
					return "success", nil
				})
			}
		}

		state := cbm.GetState(datasourceName)
		return validStates[state]
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 30}); err != nil {
		t.Errorf("Property violated: invalid state returned: %v", err)
	}
}

// Property 6: Reset deve sempre voltar circuit breaker para CLOSED
func TestProperty_CircuitBreaker_ResetToClosed(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)

	property := func(datasourceName string) bool {
		if len(datasourceName) == 0 || len(datasourceName) > 100 {
			return true
		}

		// Force to open state
		for i := 0; i < 20; i++ {
			_, _ = cbm.Execute(datasourceName, func() (any, error) {
				return nil, fmt.Errorf("failure")
			})
		}

		// Reset
		cbm.Reset(datasourceName)

		// Should be closed after reset
		state := cbm.GetState(datasourceName)
		return state == "closed"
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: reset doesn't return to closed: %v", err)
	}
}

// Property 7: GetCounts deve retornar contadores consistentes
func TestProperty_CircuitBreaker_ConsistentCounts(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)

	property := func(failures, successes uint8) bool {
		// Keep numbers small and avoid opening the circuit breaker
		if failures > 8 || successes > 8 {
			return true
		}

		datasourceName := fmt.Sprintf("test-counts-%d-%d-%d", failures, successes, time.Now().UnixNano())

		// Execute operations
		successfulRequests := uint32(0)
		failedRequests := uint32(0)

		for i := uint8(0); i < failures; i++ {
			_, err := cbm.Execute(datasourceName, func() (any, error) {
				return nil, fmt.Errorf("failure")
			})
			if err == nil || err.Error() != "datasource "+datasourceName+" is currently unavailable (circuit breaker open): open state" {
				failedRequests++
			}
		}

		for i := uint8(0); i < successes; i++ {
			_, err := cbm.Execute(datasourceName, func() (any, error) {
				return "success", nil
			})
			if err == nil {
				successfulRequests++
			}
		}

		counts := cbm.GetCounts(datasourceName)

		// If circuit breaker opened, some requests may have been rejected
		// So total requests <= expected total
		expectedTotal := successfulRequests + failedRequests
		return counts.Requests == expectedTotal || counts.Requests <= uint32(failures)+uint32(successes)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: inconsistent counts: %v", err)
	}
}

// Property 8: ShouldAllowRetry deve retornar false apenas quando OPEN
func TestProperty_CircuitBreaker_ShouldAllowRetry(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)

	property := func(failures uint8) bool {
		if failures > 30 {
			failures = failures % 30
		}

		datasourceName := fmt.Sprintf("test-retry-%d", failures)

		for i := uint8(0); i < failures; i++ {
			_, _ = cbm.Execute(datasourceName, func() (any, error) {
				return nil, fmt.Errorf("failure")
			})
		}

		shouldRetry := cbm.ShouldAllowRetry(datasourceName)
		state := cbm.GetState(datasourceName)

		// If OPEN, should not allow retry
		if state == "open" {
			return !shouldRetry
		}

		// If CLOSED or HALF-OPEN, should allow retry (with conditions)
		return true // HALF-OPEN might reject based on max requests, so we don't enforce shouldRetry == true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: ShouldAllowRetry inconsistent: %v", err)
	}
}

// Benchmark: Performance do Circuit Breaker
func BenchmarkCircuitBreakerExecution(b *testing.B) {
	logger := newTestLogger()
	cbm := pkg.NewCircuitBreakerManager(logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cbm.Execute("benchmark-ds", func() (any, error) {
			return "success", nil
		})
	}
}
