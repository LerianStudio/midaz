// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type stateTransitionSpy struct {
	mu          sync.Mutex
	transitions []libCircuitBreaker.State
}

type toggledHealthChecker struct {
	healthy atomic.Bool
}

func (c *toggledHealthChecker) CheckHealth() bool {
	return c.healthy.Load()
}

func (s *stateTransitionSpy) OnStateChange(_ string, _ libCircuitBreaker.State, to libCircuitBreaker.State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitions = append(s.transitions, to)
}

func (s *stateTransitionSpy) hasState(target libCircuitBreaker.State) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, state := range s.transitions {
		if state == target {
			return true
		}
	}

	return false
}

func testCircuitBreakerConfig() redpanda.CircuitBreakerConfig {
	return redpanda.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		OperationTimeout:    5 * time.Second,
	}
}

func TestNewCircuitBreakerManager_CreatesManagerSuccessfully(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	cbm, err := NewCircuitBreakerManager(logger, testCircuitBreakerConfig(), nil)

	require.NoError(t, err)
	assert.NotNil(t, cbm)
	assert.NotNil(t, cbm.Manager)
	assert.Equal(t, libCircuitBreaker.StateClosed, cbm.Manager.GetState(redpanda.CircuitBreakerServiceName))
}

func TestNewCircuitBreakerManager_ReturnsErrorOnNilLogger(t *testing.T) {
	cbm, err := NewCircuitBreakerManager(nil, testCircuitBreakerConfig(), nil)

	assert.Nil(t, cbm)
	assert.ErrorIs(t, err, ErrNilLogger)
}

func TestNewCircuitBreakerManager_ValidatesConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)

	t.Run("invalid failure ratio", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.FailureRatio = 2
		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		assert.Nil(t, cbm)
		assert.ErrorIs(t, err, ErrInvalidFailureRatio)
	})

	t.Run("invalid consecutive failures", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.ConsecutiveFailures = 0
		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		assert.Nil(t, cbm)
		assert.ErrorIs(t, err, ErrInvalidConsecutiveFailures)
	})

	t.Run("invalid max requests", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.MaxRequests = 0
		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		assert.Nil(t, cbm)
		assert.ErrorIs(t, err, ErrInvalidMaxRequests)
	})

	t.Run("invalid timeout", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.Timeout = 0
		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		assert.Nil(t, cbm)
		assert.ErrorIs(t, err, ErrInvalidTimeout)
	})

	t.Run("invalid interval", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.Interval = 0
		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		assert.Nil(t, cbm)
		assert.ErrorIs(t, err, ErrInvalidInterval)
	})

	t.Run("invalid min requests when failure ratio configured", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.MinRequests = 0
		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		assert.Nil(t, cbm)
		assert.ErrorIs(t, err, ErrInvalidMinRequests)
	})
}

func TestNewCircuitBreakerManager_AllowsBoundaryValues(t *testing.T) {
	logger := libZap.InitializeLogger()

	t.Run("failure ratio zero does not require min requests", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.FailureRatio = 0
		cfg.MinRequests = 0

		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		require.NoError(t, err)
		require.NotNil(t, cbm)
	})

	t.Run("failure ratio one is valid", func(t *testing.T) {
		cfg := testCircuitBreakerConfig()
		cfg.FailureRatio = 1
		cfg.MinRequests = 1

		cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
		require.NoError(t, err)
		require.NotNil(t, cbm)
	})
}

func TestCircuitBreakerRunnable_RunWithNilManager(t *testing.T) {
	runnable := NewCircuitBreakerRunnable(nil)
	assert.NoError(t, runnable.Run(nil))
}

func TestNewCircuitBreakerManager_RegistersStateListenerAndTrips(t *testing.T) {
	logger := libZap.InitializeLogger()
	listener := &stateTransitionSpy{}

	cfg := testCircuitBreakerConfig()
	cfg.ConsecutiveFailures = 1
	cfg.FailureRatio = 0
	cfg.MaxRequests = 1
	cfg.MinRequests = 1
	cfg.Timeout = 50 * time.Millisecond
	cfg.Interval = time.Second

	cbm, err := NewCircuitBreakerManager(logger, cfg, listener)
	require.NoError(t, err)

	_, _ = cbm.Manager.Execute(redpanda.CircuitBreakerServiceName, func() (any, error) {
		return nil, errors.New("broker down")
	})

	require.Eventually(t, func() bool {
		return cbm.Manager.GetState(redpanda.CircuitBreakerServiceName) == libCircuitBreaker.StateOpen
	}, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		return listener.hasState(libCircuitBreaker.StateOpen)
	}, time.Second, 10*time.Millisecond)
}

func TestCircuitBreakerManager_HybridRecovery_ActiveProbeResetsOpenCircuit(t *testing.T) {
	logger := libZap.InitializeLogger()

	cfg := testCircuitBreakerConfig()
	cfg.ConsecutiveFailures = 1
	cfg.FailureRatio = 0
	cfg.MaxRequests = 1
	cfg.MinRequests = 1
	cfg.Timeout = 5 * time.Second
	cfg.HealthCheckInterval = 20 * time.Millisecond

	cbm, err := NewCircuitBreakerManager(logger, cfg, nil)
	require.NoError(t, err)

	checker := &toggledHealthChecker{}
	checker.healthy.Store(false)
	cbm.SetHealthChecker(checker)

	cbm.Start()
	t.Cleanup(cbm.Stop)

	_, execErr := cbm.Manager.Execute(redpanda.CircuitBreakerServiceName, func() (any, error) {
		return nil, errors.New("broker down")
	})
	require.Error(t, execErr)

	require.Eventually(t, func() bool {
		return cbm.Manager.GetState(redpanda.CircuitBreakerServiceName) == libCircuitBreaker.StateOpen
	}, time.Second, 10*time.Millisecond)

	checker.healthy.Store(true)

	require.Eventually(t, func() bool {
		return cbm.Manager.GetState(redpanda.CircuitBreakerServiceName) == libCircuitBreaker.StateClosed
	}, time.Second, 10*time.Millisecond)
}
