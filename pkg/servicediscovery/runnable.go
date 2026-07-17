// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
)

// Runnable adapts service-discovery register/deregister to the libCommons.App
// interface. It registers asynchronously at start, blocks until SIGINT/SIGTERM,
// then deregisters on shutdown. A deregister failure is logged at Warn but not
// propagated: TTL expiry is the backstop and the Launcher cannot meaningfully
// react at shutdown.
type Runnable struct {
	manager *libsd.Manager
	svc     libsd.Service
	logger  libLog.Logger
	metrics MetricsRecorder

	// notifyContext builds the signal-scoped context that gates the runnable's
	// lifetime. It defaults to signal.NotifyContext in production; tests inject a
	// factory returning a context they cancel to simulate SIGTERM deterministically.
	notifyContext func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
}

// NewRunnable builds a Runnable that registers svc against manager on start and
// deregisters it on SIGINT/SIGTERM. A nil recorder is safe: it is replaced by a
// no-op recorder so the lifecycle never dereferences nil.
func NewRunnable(manager *libsd.Manager, svc libsd.Service, logger libLog.Logger, recorder MetricsRecorder) *Runnable {
	return &Runnable{manager: manager, svc: svc, logger: logger, metrics: orNop(recorder)}
}

// Run registers the service asynchronously against the signal-scoped context,
// blocks until SIGINT/SIGTERM, then deregisters under a fresh short-lived
// context (the signal context is already cancelled by then).
func (r *Runnable) Run(_ *libCommons.Launcher) error {
	if r == nil || r.manager == nil {
		return nil
	}

	notify := r.notifyContext
	if notify == nil {
		notify = signal.NotifyContext
	}

	sigCtx, stop := notify(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	metrics := orNop(r.metrics)

	// RegisterAsync is non-blocking and retries in the background until sigCtx
	// is cancelled. It needs the app-lifetime (signal) context, not a
	// request-scoped one.
	r.manager.RegisterAsync(sigCtx, r.svc)
	metrics.RegisterInitiated(sigCtx)

	<-sigCtx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), DeregisterTimeout)
	defer cancel()

	result := ResultOK

	if err := r.manager.Deregister(ctx, r.svc.ID); err != nil {
		result = ResultError

		if r.logger != nil {
			r.logger.Log(
				context.Background(), libLog.LevelWarn,
				"service discovery deregister returned error",
				libLog.String("service_id", r.svc.ID),
				libLog.Err(err),
			)
		}
	}

	metrics.DeregisterResult(ctx, result)

	// Close after deregister (the library requires Deregister-before-Close and
	// Close does not deregister). Close stops the background watcher goroutine
	// lazy-spawned by Resolve. A close error is logged at Warn but not propagated:
	// the Launcher cannot meaningfully react at shutdown, mirroring the
	// deregister-failure handling above.
	if err := r.manager.Close(); err != nil && r.logger != nil {
		r.logger.Log(
			context.Background(), libLog.LevelWarn,
			"service discovery manager close returned error",
			libLog.Err(err),
		)
	}

	return nil
}
