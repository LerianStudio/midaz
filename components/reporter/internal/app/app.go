// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package app is the unified composition root for the reporter component. It
// reconciles the two former deploy units — the REST API (formerly
// reporter-manager) and the RabbitMQ consumer + PDF worker (formerly
// reporter-worker) — behind a single binary whose active surfaces are
// selected at runtime by RUN_MODE.
//
// RUN_MODE values:
//   - "api"    → REST API surface only (production api Deployment).
//   - "worker" → RabbitMQ consumer + health server surface only (production
//     worker Deployment).
//   - "all"    → both surfaces in one process (DEV ONLY; never used in
//     production, where the two Deployments split api/worker from the same
//     image).
//
// Default (unset/blank) is "all" for local developer convenience, mirroring
// the locked Phase 5 decision.
//
// Mechanism: RUN_MODE is the INPUT that decides which surfaces are
// constructed (non-nil); the libCommons launcher is the MECHANISM. Each
// surface contributes its own libCommons.App runnable(s); every selected
// runnable is registered in ONE launcher, which owns the blocking run and the
// SIGTERM-driven graceful shutdown. After the launcher terminates, each
// surface's ordered Shutdown() runs so no teardown step is lost.
package app

import (
	"context"
	"fmt"
	"strings"

	managerBootstrap "github.com/LerianStudio/midaz/v4/components/reporter/internal/manager/bootstrap"
	workerBootstrap "github.com/LerianStudio/midaz/v4/components/reporter/internal/worker/bootstrap"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-observability/log"
)

// RunMode selects which reporter surfaces a single binary instance runs.
type RunMode string

const (
	// RunModeAPI runs only the REST API surface (production api Deployment).
	RunModeAPI RunMode = "api"
	// RunModeWorker runs only the RabbitMQ consumer + health server surface
	// (production worker Deployment).
	RunModeWorker RunMode = "worker"
	// RunModeAll runs both surfaces in one process. DEV ONLY — production
	// always splits api and worker into separate Deployments from the same
	// image.
	RunModeAll RunMode = "all"
)

// ParseRunMode normalizes the RUN_MODE env value into a RunMode. Blank/unset
// defaults to RunModeAll (local developer convenience). An unrecognized value
// is rejected so a typo (e.g. "wokrer") fails fast at bootstrap rather than
// silently starting the wrong — or no — surface.
func ParseRunMode(raw string) (RunMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(RunModeAll):
		return RunModeAll, nil
	case string(RunModeAPI):
		return RunModeAPI, nil
	case string(RunModeWorker):
		return RunModeWorker, nil
	default:
		return "", fmt.Errorf("invalid RUN_MODE %q: must be one of api|worker|all", raw)
	}
}

// runsAPI reports whether the mode activates the REST API surface.
func (m RunMode) runsAPI() bool { return m == RunModeAPI || m == RunModeAll }

// runsWorker reports whether the mode activates the consumer/worker surface.
func (m RunMode) runsWorker() bool { return m == RunModeWorker || m == RunModeAll }

// Service is the unified reporter service. It holds whichever surfaces the
// RunMode selected; an unselected surface stays nil and contributes nothing
// to the launcher or the shutdown sequence.
type Service struct {
	mode    RunMode
	manager *managerBootstrap.Service
	worker  *workerBootstrap.Service
	logger  libLog.Logger
}

// InitService builds the reporter service for the given RunMode. Shared
// infrastructure is initialized inside each surface's own bootstrap (the
// manager's InitServers and the worker's InitWorker), so both multi-tenant
// paths — the worker's MT RabbitMQ consumer and the manager's in-process MT
// schema discovery — stay independently and correctly tenant-scoped. The
// orchestrator never shares a single tenant manager across the two surfaces.
//
// When RunMode is api or worker only the matching surface is constructed,
// avoiding any connection the inactive surface would otherwise open.
func InitService(mode RunMode, logger libLog.Logger) (*Service, error) {
	svc := &Service{mode: mode, logger: logger}

	if mode.runsAPI() {
		logger.Log(context.Background(), libLog.LevelInfo, "Reporter: initializing API surface", libLog.String("run_mode", string(mode)))

		manager, err := managerBootstrap.InitServers()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize reporter API surface: %w", err)
		}

		svc.manager = manager
	}

	if mode.runsWorker() {
		logger.Log(context.Background(), libLog.LevelInfo, "Reporter: initializing worker surface", libLog.String("run_mode", string(mode)))

		worker, err := workerBootstrap.InitWorker()
		if err != nil {
			// Under RUN_MODE=all the manager surface was already constructed (and
			// opened its connections) above. Tear it down before bailing so a
			// worker-init failure does not leak the manager's pools.
			if svc.manager != nil {
				svc.manager.Shutdown()
			}

			return nil, fmt.Errorf("failed to initialize reporter worker surface: %w", err)
		}

		svc.worker = worker
	}

	return svc, nil
}

// Run starts every selected surface under ONE libCommons launcher and blocks
// until SIGTERM/SIGINT. Each surface's runnable owns its own signal-driven
// drain inside Run(); the launcher waits for all of them to unblock. After
// the launcher terminates, each surface's ordered Shutdown() runs so every
// teardown step (HTTP drain + resource cleanup for the API surface; reconciler
// cancel, health checker, health server, PDF pool, event listener,
// multi-tenant resources, RabbitMQ, MongoDB, telemetry flush for the worker
// surface) is preserved.
func (s *Service) Run() {
	launcherOpts := []libCommons.LauncherOption{
		libCommons.WithLogger(s.logger),
	}

	// API surface: register the HTTP server and start the drain listener so
	// /readyz reports 503 the moment SIGTERM arrives.
	var stopManagerDrain func()

	if s.manager != nil {
		stopManagerDrain = s.manager.StartDrainListener()
		launcherOpts = append(launcherOpts, libCommons.RunApp("Reporter HTTP API", s.manager.HTTPApp()))
	}

	// Worker surface: start the health server before the consumer so probes
	// are available immediately, then register the consumer runnable.
	if s.worker != nil {
		s.worker.StartHealthServer()
		launcherOpts = append(launcherOpts, libCommons.RunApp("Reporter RabbitMQ Consumer", s.worker.ConsumerApp()))
	}

	libCommons.NewLauncher(launcherOpts...).Run()

	// Launcher has terminated (all surfaces unblocked on SIGTERM). Run each
	// surface's ordered shutdown. The manager drain listener is stopped first
	// so it does not outlive the process.
	if stopManagerDrain != nil {
		stopManagerDrain()
	}

	if s.worker != nil {
		s.worker.Shutdown()
	}

	if s.manager != nil {
		s.manager.Shutdown()
	}
}
