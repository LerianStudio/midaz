// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
)

// FuzzNewRedisQueueConsumerMultiTenant_MultiTenantEnabled fuzzes the
// multiTenantEnabled flag and serviceName of the NewRedisQueueConsumerMultiTenant constructor.
//
// Properties verified:
//  1. Constructor never panics.
//  2. Returned consumer is never nil.
//  3. multiTenantEnabled matches input.
//  4. isMultiTenantReady() is consistent with field state.
//  5. serviceName is stored exactly as provided.
func FuzzNewRedisQueueConsumerMultiTenant_MultiTenantEnabled(f *testing.F) {
	// Seed 1: enabled with pgManager (typical multi-tenant)
	f.Add(true, true, "transaction")
	// Seed 2: disabled with no pgManager (typical single-tenant)
	f.Add(false, false, "ledger")
	// Seed 3: enabled without pgManager (fallback case)
	f.Add(true, false, "")
	// Seed 4: disabled with pgManager (misconfiguration edge case)
	f.Add(false, true, " ")
	// Seed 5: enabled with pgManager, different service name
	f.Add(true, true, "  ledger  ")

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	if err != nil {
		f.Fatalf("failed to create tenant client: %v", err)
	}
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	consumerCache := tenantcache.NewTenantCache()

	f.Fuzz(func(t *testing.T, multiTenantEnabled bool, hasPGManager bool, serviceName string) {
		var mgr *tmpostgres.Manager
		if hasPGManager {
			mgr = pgMgr
		}

		// Property: constructor must never panic.
		consumer := NewRedisQueueConsumerMultiTenant(logger, handler, multiTenantEnabled, consumerCache, mgr, serviceName)

		// Property: returned consumer is never nil.
		if consumer == nil {
			t.Fatal("NewRedisQueueConsumerMultiTenant returned nil")
		}

		// Property: multiTenantEnabled matches input.
		if consumer.multiTenantEnabled != multiTenantEnabled {
			t.Fatalf("multiTenantEnabled mismatch: got %v, want %v", consumer.multiTenantEnabled, multiTenantEnabled)
		}

		// Property: serviceName is stored exactly as provided.
		if consumer.serviceName != serviceName {
			t.Fatalf("serviceName mismatch: got %q, want %q", consumer.serviceName, serviceName)
		}

		// Property: isMultiTenantReady() follows the predicate logic.
		ready := consumer.isMultiTenantReady()
		expectReady := multiTenantEnabled && mgr != nil && consumer.tenantCache != nil

		if ready != expectReady {
			t.Fatalf("isMultiTenantReady() = %v, want %v (enabled=%v, hasPGManager=%v, hasTenantCache=%v)",
				ready, expectReady, multiTenantEnabled, hasPGManager, consumer.tenantCache != nil)
		}
	})
}

// FuzzIsMultiTenantReady_FieldCombinations fuzzes both predicates by
// constructing structs with fuzz-driven field states and verifying the
// predicate never panics and always returns a consistent boolean.
//
// Properties verified:
//  1. isMultiTenantReady() never panics on any struct state.
//  2. Result is true IFF multiTenantEnabled == true AND pgManager != nil AND tenantCache != nil.
//  3. Zero-value structs always return false.
//  4. Predicate is idempotent (calling twice returns same result).
func FuzzIsMultiTenantReady_FieldCombinations(f *testing.F) {
	// Seed 1: both enabled, both have pgManager
	f.Add(true, true, true, true)
	// Seed 2: all disabled/nil
	f.Add(false, false, false, false)
	// Seed 3: worker enabled with pgManager, consumer disabled
	f.Add(true, true, false, false)
	// Seed 4: worker disabled, consumer enabled with pgManager
	f.Add(false, false, true, true)
	// Seed 5: both enabled but no pgManager
	f.Add(true, false, true, false)
	// Seed 6: mixed -- worker has pgManager but disabled, consumer enabled with pgManager
	f.Add(false, true, true, true)

	logger := newTestLogger()
	useCase := &command.UseCase{}
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	if err != nil {
		f.Fatalf("failed to create tenant client: %v", err)
	}
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	f.Fuzz(func(t *testing.T, workerEnabled bool, workerHasPGMgr bool, consumerEnabled bool, consumerHasPGMgr bool) {
		// --- BalanceSyncWorker predicate ---
		worker := NewBalanceSyncWorker(logger, useCase, BalanceSyncConfig{})
		worker.mtEnabled = workerEnabled

		if workerHasPGMgr {
			worker.pgManager = pgMgr
		} else {
			worker.pgManager = nil
		}

		// Property: never panics.
		workerReady := worker.isMTReady()

		// Property: result matches predicate logic.
		// tenantCache is not set in this fuzz test, so isMTReady() is always false.
		expectWorkerReady := workerEnabled && workerHasPGMgr && worker.tenantCache != nil
		if workerReady != expectWorkerReady {
			t.Fatalf("BalanceSyncWorker.isMTReady() = %v, want %v (enabled=%v, hasPGMgr=%v, hasTenantCache=%v)",
				workerReady, expectWorkerReady, workerEnabled, workerHasPGMgr, worker.tenantCache != nil)
		}

		// Property: idempotent.
		if worker.isMTReady() != workerReady {
			t.Fatal("BalanceSyncWorker.isMTReady() is not idempotent")
		}

		// --- RedisQueueConsumer predicate ---
		consumer := NewRedisQueueConsumer(logger, handler)
		consumer.multiTenantEnabled = consumerEnabled

		if consumerHasPGMgr {
			consumer.pgManager = pgMgr
		} else {
			consumer.pgManager = nil
		}

		// Property: never panics.
		consumerReady := consumer.isMultiTenantReady()

		// Property: result matches predicate logic.
		// tenantCache is not set in this fuzz test, so isMultiTenantReady() is always false.
		expectConsumerReady := consumerEnabled && consumerHasPGMgr && consumer.tenantCache != nil
		if consumerReady != expectConsumerReady {
			t.Fatalf("RedisQueueConsumer.isMultiTenantReady() = %v, want %v (enabled=%v, hasPGMgr=%v, hasTenantCache=%v)",
				consumerReady, expectConsumerReady, consumerEnabled, consumerHasPGMgr, consumer.tenantCache != nil)
		}

		// Property: idempotent.
		if consumer.isMultiTenantReady() != consumerReady {
			t.Fatal("RedisQueueConsumer.isMultiTenantReady() is not idempotent")
		}

		// --- Zero-value struct edge case ---
		zeroWorker := &BalanceSyncWorker{}
		if zeroWorker.isMTReady() {
			t.Fatal("zero-value BalanceSyncWorker.isMTReady() should be false")
		}

		zeroConsumer := &RedisQueueConsumer{}
		if zeroConsumer.isMultiTenantReady() {
			t.Fatal("zero-value RedisQueueConsumer.isMultiTenantReady() should be false")
		}
	})
}
