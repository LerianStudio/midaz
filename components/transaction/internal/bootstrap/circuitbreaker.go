// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
)

var (
	// ErrNilLogger indicates that the logger parameter is nil.
	ErrNilLogger = errors.New("logger cannot be nil")
	// ErrInvalidFailureRatio indicates that FailureRatio is outside valid bounds (0.0-1.0).
	ErrInvalidFailureRatio = errors.New("failure_ratio must be between 0.0 and 1.0")
	// ErrInvalidConsecutiveFailures indicates that ConsecutiveFailures must be greater than 0.
	ErrInvalidConsecutiveFailures = errors.New("consecutive_failures must be greater than 0")
	// ErrInvalidMaxRequests indicates that MaxRequests must be greater than 0.
	ErrInvalidMaxRequests = errors.New("max_requests must be greater than 0")
	// ErrInvalidTimeout indicates that Timeout must be positive.
	ErrInvalidTimeout = errors.New("timeout must be positive")
	// ErrInvalidInterval indicates that Interval must be positive.
	ErrInvalidInterval = errors.New("interval must be positive")
	// ErrInvalidMinRequests indicates that MinRequests must be greater than 0 when FailureRatio is used.
	ErrInvalidMinRequests = errors.New("min_requests must be greater than 0 when failure_ratio is configured")
)

const defaultHealthCheckInterval = 30 * time.Second

// CircuitBreakerHealthChecker defines the health signal required for active recovery probes.
type CircuitBreakerHealthChecker interface {
	CheckHealth() bool
}

// CircuitBreakerManager manages circuit breaker infrastructure for broker producer.
type CircuitBreakerManager struct {
	Manager libCircuitBreaker.Manager
	logger  libLog.Logger

	healthChecker CircuitBreakerHealthChecker
	probeInterval time.Duration

	probeMu      sync.Mutex
	probeRunning bool
	probeCancel  context.CancelFunc
	probeWG      sync.WaitGroup
}

// NewCircuitBreakerManager creates a new circuit breaker manager.
func NewCircuitBreakerManager(
	logger libLog.Logger,
	cbConfig redpanda.CircuitBreakerConfig,
	stateListener libCircuitBreaker.StateChangeListener,
) (*CircuitBreakerManager, error) {
	if logger == nil {
		return nil, ErrNilLogger
	}

	if cbConfig.FailureRatio < 0 || cbConfig.FailureRatio > 1.0 {
		return nil, ErrInvalidFailureRatio
	}

	if cbConfig.ConsecutiveFailures == 0 {
		return nil, ErrInvalidConsecutiveFailures
	}

	if cbConfig.MaxRequests == 0 {
		return nil, ErrInvalidMaxRequests
	}

	if cbConfig.Timeout <= 0 {
		return nil, ErrInvalidTimeout
	}

	if cbConfig.Interval <= 0 {
		return nil, ErrInvalidInterval
	}

	if cbConfig.FailureRatio > 0 && cbConfig.MinRequests == 0 {
		return nil, ErrInvalidMinRequests
	}

	cbManager := libCircuitBreaker.NewManager(logger)
	cbManager.GetOrCreate(redpanda.CircuitBreakerServiceName, redpanda.ProducerCircuitBreakerConfig(cbConfig))

	if stateListener != nil {
		cbManager.RegisterStateChangeListener(stateListener)
	}

	probeInterval := cbConfig.HealthCheckInterval
	if probeInterval <= 0 {
		probeInterval = defaultHealthCheckInterval
	}

	return &CircuitBreakerManager{
		Manager:       cbManager,
		logger:        logger,
		probeInterval: probeInterval,
	}, nil
}

// SetHealthChecker injects the active probe health checker used for hybrid recovery.
func (cbm *CircuitBreakerManager) SetHealthChecker(checker CircuitBreakerHealthChecker) {
	if cbm == nil {
		return
	}

	cbm.probeMu.Lock()
	defer cbm.probeMu.Unlock()

	cbm.healthChecker = checker
}

// Start begins manager lifecycle.
func (cbm *CircuitBreakerManager) Start() {
	if cbm == nil {
		return
	}

	cbm.probeMu.Lock()
	if cbm.probeRunning {
		cbm.probeMu.Unlock()
		return
	}

	healthChecker := cbm.healthChecker

	interval := cbm.probeInterval
	if interval <= 0 {
		interval = defaultHealthCheckInterval
	}

	if healthChecker == nil {
		cbm.probeMu.Unlock()
		cbm.logger.Info("Starting circuit breaker manager in passive recovery mode (active health probe disabled)")

		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	cbm.probeCancel = cancel
	cbm.probeRunning = true
	cbm.probeWG.Add(1)
	cbm.probeMu.Unlock()

	cbm.logger.Infof("Starting circuit breaker manager with hybrid recovery (probe interval=%s)", interval)

	go cbm.runHealthProbeLoop(ctx, interval)
}

// Stop gracefully stops manager lifecycle.
func (cbm *CircuitBreakerManager) Stop() {
	if cbm == nil {
		return
	}

	cbm.probeMu.Lock()
	cancel := cbm.probeCancel
	wasRunning := cbm.probeRunning
	cbm.probeCancel = nil
	cbm.probeRunning = false
	cbm.probeMu.Unlock()

	if cancel != nil {
		cancel()
	}

	if wasRunning {
		cbm.probeWG.Wait()
	}

	cbm.logger.Info("Stopping circuit breaker manager")
}

func (cbm *CircuitBreakerManager) runHealthProbeLoop(ctx context.Context, interval time.Duration) {
	defer cbm.probeWG.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cbm.Manager.GetState(redpanda.CircuitBreakerServiceName) != libCircuitBreaker.StateOpen {
				continue
			}

			if cbm.healthChecker == nil || !cbm.healthChecker.CheckHealth() {
				continue
			}

			cbm.logger.Infof("Circuit breaker active probe recovered service=%s; resetting breaker", redpanda.CircuitBreakerServiceName)
			cbm.Manager.Reset(redpanda.CircuitBreakerServiceName)
		}
	}
}

// CircuitBreakerRunnable wraps CircuitBreakerManager to implement the Runnable interface.
type CircuitBreakerRunnable struct {
	manager *CircuitBreakerManager
}

// NewCircuitBreakerRunnable creates a new runnable wrapper for the circuit breaker manager.
func NewCircuitBreakerRunnable(manager *CircuitBreakerManager) *CircuitBreakerRunnable {
	return &CircuitBreakerRunnable{manager: manager}
}

// Run implements the Runnable interface for integration with libCommons.Launcher.
func (r *CircuitBreakerRunnable) Run(_ *libCommons.Launcher) error {
	if r.manager == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r.manager.Start()
	<-ctx.Done()
	r.manager.Stop()

	return nil
}
