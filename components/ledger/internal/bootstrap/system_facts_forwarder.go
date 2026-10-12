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
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"github.com/LerianStudio/lib-system-facts/outbox"
)

const (
	// systemFactsForwarderComponent and systemFactsForwarderGoroutine name the
	// forwarder goroutine in the panic-recovery metrics and logs.
	systemFactsForwarderComponent = "ledger.system-facts-forwarder"
	systemFactsForwarderGoroutine = "ledger-system-facts-forwarder"
	// systemFactsForwarderShutdownTimeout is the HTTP server's graceful-drain
	// budget, so the forwarder never outlives the server it shares a pod with.
	systemFactsForwarderShutdownTimeout = 30 * time.Second
	// A forwarder that stopped before shutdown restarts after
	// systemFactsForwarderRestartBackoff, doubling per consecutive stop up to
	// systemFactsForwarderRestartBackoffMax; a run longer than
	// systemFactsForwarderStableRun resets the backoff.
	systemFactsForwarderRestartBackoff    = time.Second
	systemFactsForwarderRestartBackoffMax = 30 * time.Second
	systemFactsForwarderStableRun         = time.Minute
)

// systemFactsForwarderRestartsTotal counts forwarder restarts. Any rate of it
// needs an operator: a forwarder that keeps stopping is not draining.
var systemFactsForwarderRestartsTotal = metrics.Metric{
	Name:        "system_facts_forwarder_restarts_total",
	Unit:        "1",
	Description: "Restarts of the System Facts forwarder after its Run returned before shutdown.",
}

// systemFactsForwarder is the forwarder lifecycle the Launcher app drives;
// *outbox.Forwarder in production.
type systemFactsForwarder interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

var _ systemFactsForwarder = (*outbox.Forwarder)(nil)

// systemFactsForwarderRunnable adapts the System Facts forwarder to the
// libCommons.App interface: it supervises Run on its own goroutine until
// SIGINT/SIGTERM, then shuts the forwarder down under a bounded budget.
// signalContext, now and wait are the test seams; nil means the real signal
// context, clock and timer.
type systemFactsForwarderRunnable struct {
	forwarder      systemFactsForwarder
	logger         libLog.Logger
	metricsFactory *metrics.MetricsFactory
	signalContext  func() (context.Context, context.CancelFunc)
	now            func() time.Time
	wait           func(ctx context.Context, d time.Duration) error
}

// newSystemFactsForwarderRunnable builds the Launcher app for forwarder.
func newSystemFactsForwarderRunnable(forwarder *outbox.Forwarder, logger libLog.Logger, factory *metrics.MetricsFactory) *systemFactsForwarderRunnable {
	if logger == nil {
		logger = libLog.NewNop()
	}

	return &systemFactsForwarderRunnable{
		forwarder:      forwarder,
		logger:         logger,
		metricsFactory: factory,
		now:            time.Now,
		wait:           waitFor,
	}
}

// Run supervises the forwarder until SIGINT/SIGTERM and then shuts it down.
// The forwarder runs under its own context rather than the signal one so that
// Shutdown, not a cancelled context, ends the in-flight cycle: the records it
// holds are persisted FAILED and retried after the retry window. A Shutdown
// error is logged at Warn; nothing is propagated, because at shutdown the
// Launcher cannot meaningfully react.
func (r *systemFactsForwarderRunnable) Run(_ *libCommons.Launcher) error {
	signalCtx, stop := r.notifyContext()
	defer stop()

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	runDone := make(chan struct{})

	libRuntime.SafeGoWithContextAndComponent(runCtx, r.logger, systemFactsForwarderComponent, systemFactsForwarderGoroutine, libRuntime.KeepRunning,
		func(ctx context.Context) {
			defer close(runDone)

			r.supervise(ctx, signalCtx)
		})

	<-signalCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), systemFactsForwarderShutdownTimeout)
	defer cancel()

	if err := r.forwarder.Shutdown(shutdownCtx); err != nil {
		r.logger.Log(shutdownCtx, libLog.LevelWarn, "System Facts forwarder Shutdown returned an error", libLog.Err(err))
	}

	select {
	case <-runDone:
	case <-shutdownCtx.Done():
	}

	return nil
}

// supervise runs the forwarder until signalCtx ends, restarting it with
// backoff whenever Run returns before shutdown. Each restart is logged at
// Error and counted.
func (r *systemFactsForwarderRunnable) supervise(ctx, signalCtx context.Context) {
	backoff := systemFactsForwarderRestartBackoff

	for {
		started := r.clock()
		err := r.forwarder.Run(ctx)

		if signalCtx.Err() != nil || ctx.Err() != nil {
			return
		}

		if r.clock().Sub(started) > systemFactsForwarderStableRun {
			backoff = systemFactsForwarderRestartBackoff
		}

		r.logUnexpectedStop(ctx, err, backoff)
		incrementSystemFactsCounter(ctx, r.metricsFactory, r.logger, systemFactsForwarderRestartsTotal, nil)

		if r.pause(ctx, signalCtx, backoff) != nil {
			return
		}

		backoff = min(backoff*2, systemFactsForwarderRestartBackoffMax)
	}
}

func (r *systemFactsForwarderRunnable) logUnexpectedStop(ctx context.Context, err error, backoff time.Duration) {
	if err != nil {
		r.logger.Log(ctx, libLog.LevelError, "System Facts forwarder stopped with an error; restarting",
			libLog.String("restart_in", backoff.String()), libLog.Err(err))

		return
	}

	r.logger.Log(ctx, libLog.LevelError, "System Facts forwarder stopped unexpectedly; restarting",
		libLog.String("restart_in", backoff.String()))
}

// pause waits d out, ending early when the run or the signal context ends.
func (r *systemFactsForwarderRunnable) pause(ctx, signalCtx context.Context, d time.Duration) error {
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stop := context.AfterFunc(signalCtx, cancel)
	defer stop()

	return r.sleep(waitCtx, d)
}

func (r *systemFactsForwarderRunnable) clock() time.Time {
	if r.now == nil {
		return time.Now()
	}

	return r.now()
}

func (r *systemFactsForwarderRunnable) sleep(ctx context.Context, d time.Duration) error {
	if r.wait == nil {
		return waitFor(ctx, d)
	}

	return r.wait(ctx, d)
}

// notifyContext is the context that ends on SIGINT/SIGTERM, or the test seam.
func (r *systemFactsForwarderRunnable) notifyContext() (context.Context, context.CancelFunc) {
	if r.signalContext != nil {
		return r.signalContext()
	}

	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// waitFor blocks for d, or until ctx ends, returning its error.
func waitFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
