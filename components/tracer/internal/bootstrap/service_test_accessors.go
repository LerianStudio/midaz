// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package bootstrap

// HasSupervisorForTest reports whether the Service has a multi-tenant
// WorkerSupervisor wired. Single-tenant services return false.
//
// Test-only accessor guarded by the integration build tag so the production
// binary cannot depend on it.
func (app *Service) HasSupervisorForTest() bool {
	return app.supervisor != nil
}

// HasPGManagerForTest reports whether the Service has the per-tenant
// PostgreSQL pool manager wired. Single-tenant services return false.
func (app *Service) HasPGManagerForTest() bool {
	return app.pgManager != nil
}

// HasEventListenerForTest reports whether the Service has a tenant event
// listener wired. Single-tenant services return false.
func (app *Service) HasEventListenerForTest() bool {
	return app.eventListener != nil
}

// HasSingletonSyncWorkerForTest reports whether the singleton rule sync
// worker is wired (single-tenant mode only).
func (app *Service) HasSingletonSyncWorkerForTest() bool {
	return app.syncWorker != nil
}

// HasSingletonCleanupWorkerForTest reports whether the singleton usage
// cleanup worker is wired (single-tenant mode only).
func (app *Service) HasSingletonCleanupWorkerForTest() bool {
	return app.cleanupWorker != nil
}
