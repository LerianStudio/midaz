// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/constant"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/log"
)

// tickerFactory creates a channel that receives ticks and a stop function.
// Overridable in tests for deterministic behavior.
var tickerFactory = newRealTicker

// newRealTicker returns a channel that ticks at ConnectionMonitorInterval and a stop func.
func newRealTicker() (<-chan time.Time, func()) {
	t := time.NewTicker(constant.ConnectionMonitorInterval)
	return t.C, t.Stop
}

// RabbitMQMonitor performs periodic background health checks on a RabbitMQ
// connection and calls EnsureChannel to trigger reconnection when the
// connection is dead. This breaks the deadlock where /readyz returns 503
// but nothing triggers reconnection because no publishes happen.
type RabbitMQMonitor struct {
	conn   *libRabbitmq.RabbitMQConnection
	logger log.Logger
	stop   chan struct{}
	done   chan struct{}
}

// NewRabbitMQMonitor creates a new monitor for the given RabbitMQ connection.
func NewRabbitMQMonitor(conn *libRabbitmq.RabbitMQConnection, logger log.Logger) *RabbitMQMonitor {
	return &RabbitMQMonitor{
		conn:   conn,
		logger: logger,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start launches the background monitor goroutine. It checks every
// ConnectionMonitorInterval (10s) whether the RabbitMQ connection is alive.
// If the connection is dead, it calls EnsureChannel to trigger reconnection,
// which updates conn.Connected and makes /readyz recover.
func (m *RabbitMQMonitor) Start() {
	pkg.GoNamed(m.logger, "rabbitmq-monitor", func() { m.monitorLoop() })
}

// Stop signals the monitor to shut down and waits for it to finish.
func (m *RabbitMQMonitor) Stop() {
	close(m.stop)
	<-m.done
}

// monitorLoop is the background goroutine that periodically checks the
// RabbitMQ connection health and attempts reconnection when needed.
func (m *RabbitMQMonitor) monitorLoop() {
	defer close(m.done)

	tickCh, stopTicker := tickerFactory()
	defer stopTicker()

	for {
		select {
		case <-m.stop:
			m.logger.Log(context.Background(), log.LevelInfo, "RabbitMQ connection monitor stopped")

			return
		case <-tickCh:
			m.checkAndReconnect()
		}
	}
}

// isConnectionAlive returns true if the RabbitMQ connection is in a healthy state.
func (m *RabbitMQMonitor) isConnectionAlive() bool {
	if m.conn == nil {
		return false
	}

	if !m.conn.Connected {
		return false
	}

	if m.conn.Connection == nil || m.conn.Connection.IsClosed() {
		return false
	}

	return true
}

// checkAndReconnect verifies the connection and calls EnsureChannel if it is dead.
func (m *RabbitMQMonitor) checkAndReconnect() {
	if m.conn == nil {
		m.logger.Log(context.Background(), log.LevelError, "RabbitMQ connection object is nil, cannot reconnect")
		return
	}

	if m.isConnectionAlive() {
		return
	}

	m.logger.Log(context.Background(), log.LevelWarn, "RabbitMQ connection is dead, attempting reconnection via EnsureChannel...")

	if err := m.conn.EnsureChannel(); err != nil {
		m.logger.Log(context.Background(), log.LevelError, "RabbitMQ reconnection failed",
			log.Err(err), log.Any("retry_in", constant.ConnectionMonitorInterval))

		return
	}

	m.logger.Log(context.Background(), log.LevelInfo, "RabbitMQ connection restored by background monitor")
}
