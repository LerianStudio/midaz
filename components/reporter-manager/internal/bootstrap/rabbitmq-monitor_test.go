// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) *zap.Logger {
	t.Helper()

	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	require.NoError(t, err)

	return logger
}

// fakeTicker returns a controllable ticker for tests.
func fakeTicker() (tickCh chan time.Time, cleanup func(), factory func() (<-chan time.Time, func())) {
	tickCh = make(chan time.Time, 10)
	stopped := make(chan struct{})
	once := sync.Once{}

	factory = func() (<-chan time.Time, func()) {
		return tickCh, func() {
			once.Do(func() { close(stopped) })
		}
	}

	cleanup = func() {
		select {
		case <-stopped:
		default:
			once.Do(func() { close(stopped) })
		}
	}

	return tickCh, cleanup, factory
}

func TestNewRabbitMQMonitor(t *testing.T) {
	t.Parallel()

	logger := newTestLogger(t)

	conn := &libRabbitmq.RabbitMQConnection{
		Logger: logger,
	}

	monitor := NewRabbitMQMonitor(conn, logger)
	require.NotNil(t, monitor)
	assert.Equal(t, conn, monitor.conn)
	assert.NotNil(t, monitor.stop)
	assert.NotNil(t, monitor.done)
}

func TestRabbitMQMonitor_IsConnectionAlive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		conn   *libRabbitmq.RabbitMQConnection
		expect bool
	}{
		{
			name:   "nil connection struct",
			conn:   nil,
			expect: false,
		},
		{
			name: "not connected flag",
			conn: &libRabbitmq.RabbitMQConnection{
				Connected: false,
			},
			expect: false,
		},
		{
			name: "connected but nil AMQP connection",
			conn: &libRabbitmq.RabbitMQConnection{
				Connected:  true,
				Connection: nil,
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := newTestLogger(t)
			monitor := &RabbitMQMonitor{
				conn:   tt.conn,
				logger: logger,
			}

			assert.Equal(t, tt.expect, monitor.isConnectionAlive())
		})
	}
}

func TestRabbitMQMonitor_CheckAndReconnect_DeadConnection(t *testing.T) {
	t.Parallel()

	logger := newTestLogger(t)

	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: "amqp://invalid:invalid@localhost:0",
		Connected:              false,
		Logger:                 logger,
	}

	monitor := &RabbitMQMonitor{
		conn:   conn,
		logger: logger,
	}

	// Should attempt EnsureChannel and fail gracefully (not panic)
	monitor.checkAndReconnect()
}

func TestRabbitMQMonitor_CheckAndReconnect_NilAMQPConnection(t *testing.T) {
	t.Parallel()

	logger := newTestLogger(t)

	conn := &libRabbitmq.RabbitMQConnection{
		Connected:  true,
		Connection: nil,
		Logger:     logger,
	}

	monitor := &RabbitMQMonitor{
		conn:   conn,
		logger: logger,
	}

	// Connection is nil despite Connected=true, should attempt reconnect
	monitor.checkAndReconnect()
}

func TestRabbitMQMonitor_ConnectionMonitorIntervalConstant(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 10*time.Second, constant.ConnectionMonitorInterval)
}

// TestRabbitMQMonitor_Lifecycle groups all tests that modify the package-level
// tickerFactory variable to prevent data races. Subtests run sequentially.
// NOTE: Cannot use t.Parallel() because subtests modify the package-level tickerFactory variable.
func TestRabbitMQMonitor_Lifecycle(t *testing.T) {
	t.Run("Success - StartAndStop", func(t *testing.T) {
		logger := newTestLogger(t)

		conn := &libRabbitmq.RabbitMQConnection{
			Connected: false,
			Logger:    logger,
		}

		tickCh, cleanup, factory := fakeTicker()
		defer cleanup()

		originalFactory := tickerFactory
		tickerFactory = factory

		defer func() { tickerFactory = originalFactory }()

		monitor := NewRabbitMQMonitor(conn, logger)
		monitor.Start()

		// Send a tick to verify the monitor processes it
		tickCh <- time.Now()
		time.Sleep(50 * time.Millisecond)

		// Stop should not hang
		done := make(chan struct{})

		go func() {
			monitor.Stop()
			close(done)
		}()

		select {
		case <-done:
			// OK - stopped successfully
		case <-time.After(2 * time.Second):
			t.Fatal("monitor.Stop() timed out")
		}
	})

	t.Run("Success - TickTriggersCheck", func(t *testing.T) {
		logger := newTestLogger(t)

		conn := &libRabbitmq.RabbitMQConnection{
			ConnectionStringSource: "amqp://invalid:invalid@localhost:0",
			Connected:              false,
			Logger:                 logger,
		}

		tickCh, cleanup, factory := fakeTicker()
		defer cleanup()

		originalFactory := tickerFactory
		tickerFactory = factory

		defer func() { tickerFactory = originalFactory }()

		monitor := NewRabbitMQMonitor(conn, logger)
		monitor.Start()

		// Fire multiple ticks
		for i := 0; i < 3; i++ {
			tickCh <- time.Now()
			time.Sleep(20 * time.Millisecond)
		}

		done := make(chan struct{})

		go func() {
			monitor.Stop()
			close(done)
		}()

		select {
		case <-done:
			// OK
		case <-time.After(2 * time.Second):
			t.Fatal("monitor.Stop() timed out after multiple ticks")
		}
	})

	t.Run("Success - StopBeforeTick", func(t *testing.T) {
		logger := newTestLogger(t)

		conn := &libRabbitmq.RabbitMQConnection{
			Connected: false,
			Logger:    logger,
		}

		_, cleanup, factory := fakeTicker()
		defer cleanup()

		originalFactory := tickerFactory
		tickerFactory = factory

		defer func() { tickerFactory = originalFactory }()

		monitor := NewRabbitMQMonitor(conn, logger)
		monitor.Start()

		// Stop immediately without any ticks
		done := make(chan struct{})

		go func() {
			monitor.Stop()
			close(done)
		}()

		select {
		case <-done:
			// OK
		case <-time.After(2 * time.Second):
			t.Fatal("monitor.Stop() timed out when stopped before any ticks")
		}
	})

	t.Run("Success - StartUsesPanicRecovery", func(t *testing.T) {
		logger := newTestLogger(t)

		conn := &libRabbitmq.RabbitMQConnection{
			Connected: false,
			Logger:    logger,
		}

		// Install a tickerFactory that panics, simulating an unexpected failure
		// inside monitorLoop. With pkg.GoNamed(), the panic is recovered and the
		// process survives. With a bare "go" statement, this would crash the
		// test process.
		originalFactory := tickerFactory
		tickerFactory = func() (<-chan time.Time, func()) {
			panic("simulated panic inside monitor goroutine")
		}

		defer func() { tickerFactory = originalFactory }()

		monitor := NewRabbitMQMonitor(conn, logger)
		monitor.Start()

		// Wait for the goroutine to execute and (hopefully) recover the panic.
		// The done channel will be closed by monitorLoop's defer even on panic.
		select {
		case <-monitor.done:
			// Goroutine finished -- panic was recovered by pkg.GoNamed wrapper
		case <-time.After(2 * time.Second):
			t.Fatal("monitor goroutine did not finish -- panic recovery may be missing")
		}

		// If we reach here the test process survived, proving panic recovery works.
	})

	t.Run("Success - StopIsIdempotentWithDone", func(t *testing.T) {
		logger := newTestLogger(t)

		conn := &libRabbitmq.RabbitMQConnection{
			Connected: false,
			Logger:    logger,
		}

		_, cleanup, factory := fakeTicker()
		defer cleanup()

		originalFactory := tickerFactory
		tickerFactory = factory

		defer func() { tickerFactory = originalFactory }()

		monitor := NewRabbitMQMonitor(conn, logger)
		monitor.Start()

		// Stop and verify done channel is closed
		monitor.Stop()

		// done channel should be closed, reading should not block
		select {
		case <-monitor.done:
			// OK - done channel is closed
		default:
			t.Fatal("done channel should be closed after Stop()")
		}
	})
}
