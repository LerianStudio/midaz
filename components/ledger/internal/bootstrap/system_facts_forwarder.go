// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"github.com/LerianStudio/lib-system-facts/outbox"
)

const (
	// systemFactsForwarderComponent and systemFactsForwarderGoroutine name the
	// forwarder goroutine in the panic-recovery metrics and logs.
	systemFactsForwarderComponent = "ledger.system-facts-forwarder"
	systemFactsForwarderGoroutine = "ledger-system-facts-forwarder"
	// systemFactsForwarderShutdownTimeout bounds the forwarder's Shutdown with
	// the same 30s budget lib-commons gives the HTTP server's graceful drain,
	// so the forwarder never outlives the server it shares a pod with.
	systemFactsForwarderShutdownTimeout = 30 * time.Second
)

// systemFactsForwarder is the forwarder lifecycle the Launcher app drives;
// *outbox.Forwarder in production.
type systemFactsForwarder interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

var _ systemFactsForwarder = (*outbox.Forwarder)(nil)

// systemFactsForwarderRunnable adapts the System Facts forwarder to the
// libCommons.App interface: it runs the forwarder on its own goroutine, blocks
// until SIGINT/SIGTERM and then shuts the forwarder down under a bounded
// budget. signalContext is the wait seam; nil means signal.NotifyContext.
type systemFactsForwarderRunnable struct {
	forwarder     systemFactsForwarder
	logger        libLog.Logger
	signalContext func() (context.Context, context.CancelFunc)
}

// newSystemFactsForwarderRunnable builds the Launcher app for forwarder. A nil
// forwarder yields a runnable whose Run returns at once.
func newSystemFactsForwarderRunnable(forwarder *outbox.Forwarder, logger libLog.Logger) *systemFactsForwarderRunnable {
	if forwarder == nil {
		return &systemFactsForwarderRunnable{logger: logger}
	}

	return &systemFactsForwarderRunnable{forwarder: forwarder, logger: logger}
}

// Run starts the forwarder, blocks until SIGINT/SIGTERM and then shuts it
// down. The forwarder runs under its own context rather than the signal one so
// that Shutdown, not a cancelled context, ends the in-flight cycle: the records
// it holds are persisted FAILED and retried after the retry window, instead of
// staying PROCESSING until the kit's processing timeout. A forwarder error is
// logged at Error and a Shutdown error at Warn; neither is propagated, because
// at shutdown the Launcher cannot meaningfully react.
func (r *systemFactsForwarderRunnable) Run(_ *libCommons.Launcher) error {
	if r == nil || r.forwarder == nil {
		return nil
	}

	logger := r.logger
	if logger == nil {
		logger = libLog.NewNop()
	}

	signalCtx, stop := r.notifyContext()
	defer stop()

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	runDone := make(chan struct{})

	libRuntime.SafeGoWithContextAndComponent(runCtx, logger, systemFactsForwarderComponent, systemFactsForwarderGoroutine, libRuntime.KeepRunning,
		func(ctx context.Context) {
			defer close(runDone)

			if err := r.forwarder.Run(ctx); err != nil {
				logger.Log(ctx, libLog.LevelError, "System Facts forwarder stopped with an error", libLog.Err(err))
			}
		})

	<-signalCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), systemFactsForwarderShutdownTimeout)
	defer cancel()

	if err := r.forwarder.Shutdown(shutdownCtx); err != nil {
		logger.Log(shutdownCtx, libLog.LevelWarn, "System Facts forwarder Shutdown returned an error", libLog.Err(err))
	}

	select {
	case <-runDone:
	case <-shutdownCtx.Done():
	}

	return nil
}

// notifyContext is the context that ends on SIGINT/SIGTERM, or the test seam.
func (r *systemFactsForwarderRunnable) notifyContext() (context.Context, context.CancelFunc) {
	if r.signalContext != nil {
		return r.signalContext()
	}

	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
