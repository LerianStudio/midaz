// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"sync"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	libLog "github.com/LerianStudio/lib-observability/log"
)

// tenantListenerApp wraps a tmevent.TenantEventListener so it satisfies the
// libCommons.App contract, letting us register the listener with the Launcher
// alongside the HTTP server and worker supervisor.
//
// The listener's Start(ctx) returns immediately after subscribing to Redis
// Pub/Sub; the background goroutine it spawns runs until Stop is called.
// This wrapper:
//   - calls Start on Run, then blocks on shutdownCh so the Launcher sees the
//     app as "running" until shutdown.
//   - calls Stop on Shutdown, which both unblocks Run and cancels the
//     background goroutine cleanly.
//
// It mirrors the WorkerSupervisor.Run pattern so shutdown coordination is
// uniform: the Launcher fires the shutdown sequence and every app exits.
type tenantListenerApp struct {
	listener     *tmevent.TenantEventListener
	logger       libLog.Logger
	shutdownCh   chan struct{}
	shutdownOnce sync.Once
}

// newTenantListenerApp returns a Launcher-compatible wrapper around listener.
// Both arguments are required.
func newTenantListenerApp(listener *tmevent.TenantEventListener, logger libLog.Logger) (*tenantListenerApp, error) {
	if listener == nil {
		return nil, fmt.Errorf("tenant listener app: listener is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("tenant listener app: logger is required")
	}

	return &tenantListenerApp{
		listener:   listener,
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}, nil
}

// Run satisfies libCommons.App. It starts the listener and blocks until
// Shutdown is invoked. On shutdown it asks the listener to stop so the
// background goroutine exits.
func (a *tenantListenerApp) Run(_ *libCommons.Launcher) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := a.listener.Start(ctx); err != nil {
		return fmt.Errorf("tenant listener app: start: %w", err)
	}

	<-a.shutdownCh

	if err := a.listener.Stop(); err != nil {
		a.logger.With(
			libLog.String("operation", "tenant_listener_app.shutdown"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "tenant event listener stop returned error")
	}

	return nil
}

// Shutdown signals Run to return. Safe to call multiple times and from
// multiple goroutines concurrently — sync.Once guarantees the channel is
// closed exactly once. The previous select/default idiom was racy: two
// concurrent callers could both observe "not closed" in the default branch
// and both attempt to close, panicking on the second close.
func (a *tenantListenerApp) Shutdown() {
	a.shutdownOnce.Do(func() {
		close(a.shutdownCh)
	})
}
