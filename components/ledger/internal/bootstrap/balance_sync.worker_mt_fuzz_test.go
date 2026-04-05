// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
)

// FuzzNewBalanceSyncWorkerMT_MaxWorkers fuzzes the mtEnabled flag and serviceName
// of the NewBalanceSyncWorkerMT constructor.
//
// Properties verified (not specific values):
//  1. Constructor never panics for any bool/string combination.
//  2. Returned worker is never nil.
//  3. mtEnabled matches the input value.
//  4. isMTReady() is consistent with field state.
//  5. serviceName is stored exactly as provided (no implicit trimming).
func FuzzNewBalanceSyncWorkerMT_MaxWorkers(f *testing.F) {
	// Seed 1: typical valid input
	f.Add(true, "transaction")
	// Seed 2: disabled
	f.Add(false, "ledger")
	// Seed 3: enabled with empty service name
	f.Add(true, "")
	// Seed 4: disabled with whitespace service name
	f.Add(false, " ")
	// Seed 5: enabled with padded service name
	f.Add(true, "  ledger  ")

	logger := newTestLogger()
	useCase := &command.UseCase{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	if err != nil {
		f.Fatalf("failed to create tenant client: %v", err)
	}
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	cache := tenantcache.NewTenantCache()

	f.Fuzz(func(t *testing.T, mtEnabled bool, serviceName string) {
		// Property: constructor must never panic (enforced by test execution).
		worker := NewBalanceSyncWorkerMT(logger, useCase, BalanceSyncConfig{}, mtEnabled, cache, pgMgr, serviceName)

		// Property: returned worker is never nil.
		if worker == nil {
			t.Fatal("NewBalanceSyncWorkerMT returned nil")
		}

		// Property: mtEnabled matches input.
		if worker.mtEnabled != mtEnabled {
			t.Fatalf("mtEnabled mismatch: got %v, want %v", worker.mtEnabled, mtEnabled)
		}

		// Property: serviceName is stored exactly as provided.
		if worker.serviceName != serviceName {
			t.Fatalf("serviceName mismatch: got %q, want %q", worker.serviceName, serviceName)
		}

		// Property: isMTReady() equals the conjunction of all three conditions.
		expectedReady := mtEnabled && worker.pgManager != nil && worker.tenantCache != nil
		if worker.isMTReady() != expectedReady {
			t.Fatalf("isMTReady() = %v, want %v (mtEnabled=%v, pgManager=%v, tenantCache=%v)",
				worker.isMTReady(), expectedReady, mtEnabled, worker.pgManager != nil, worker.tenantCache != nil)
		}
	})
}
