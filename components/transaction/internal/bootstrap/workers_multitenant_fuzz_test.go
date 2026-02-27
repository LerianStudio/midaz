// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

// =============================================================================
// FUZZ TESTS -- Multi-tenant constructors and predicates (T-007)
//
// These fuzz tests verify that the multi-tenant constructors and predicates
// never panic regardless of input values. The fuzzable surface is:
//
//   - maxWorkers (int) in NewBalanceSyncWorkerMultiTenant
//   - multiTenantEnabled (bool) combined with nil/non-nil pointer parameters
//   - isMultiTenantReady() predicates on both BalanceSyncWorker and RedisQueueConsumer
//
// Run with:
//
//	go test -run='^$' -fuzz=FuzzNewBalanceSyncWorkerMultiTenant_MaxWorkers -fuzztime=30s \
//	    ./components/transaction/internal/bootstrap/
//	go test -run='^$' -fuzz=FuzzNewRedisQueueConsumerMultiTenant_MultiTenantEnabled -fuzztime=30s \
//	    ./components/transaction/internal/bootstrap/
//	go test -run='^$' -fuzz=FuzzIsMultiTenantReady_FieldCombinations -fuzztime=30s \
//	    ./components/transaction/internal/bootstrap/
//
// =============================================================================

import (
	"math"
	"testing"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
)

// FuzzNewBalanceSyncWorkerMultiTenant_MaxWorkers fuzzes the maxWorkers parameter
// and multiTenantEnabled flag of the NewBalanceSyncWorkerMultiTenant constructor.
//
// Properties verified (not specific values):
//  1. Constructor never panics for any int/bool combination.
//  2. Returned worker is never nil.
//  3. maxWorkers is always > 0 after construction (default applied for <= 0).
//  4. multiTenantEnabled matches the input value.
//  5. isMultiTenantReady() is consistent with field state.
func FuzzNewBalanceSyncWorkerMultiTenant_MaxWorkers(f *testing.F) {
	// Seed 1: typical valid input
	f.Add(5, true)
	// Seed 2: zero maxWorkers (boundary -- triggers default)
	f.Add(0, false)
	// Seed 3: negative maxWorkers (boundary -- triggers default)
	f.Add(-1, true)
	// Seed 4: large maxWorkers (stress)
	f.Add(math.MaxInt32, false)
	// Seed 5: minimum int (extreme negative boundary)
	f.Add(math.MinInt32, true)
	// Seed 6: one worker (minimum valid)
	f.Add(1, false)
	// Seed 7: large negative (another extreme)
	f.Add(-1000000, false)

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	f.Fuzz(func(t *testing.T, maxWorkers int, multiTenantEnabled bool) {
		// Property: constructor must never panic (enforced by test execution).
		worker := NewBalanceSyncWorkerMultiTenant(conn, logger, useCase, maxWorkers, multiTenantEnabled, tenantClient, pgMgr)

		// Property: returned worker is never nil.
		if worker == nil {
			t.Fatal("NewBalanceSyncWorkerMultiTenant returned nil")
		}

		// Property: maxWorkers is always positive after construction.
		if worker.maxWorkers <= 0 {
			t.Fatalf("maxWorkers should be > 0 after construction, got %d (input was %d)", worker.maxWorkers, maxWorkers)
		}

		// Property: multiTenantEnabled matches input.
		if worker.multiTenantEnabled != multiTenantEnabled {
			t.Fatalf("multiTenantEnabled mismatch: got %v, want %v", worker.multiTenantEnabled, multiTenantEnabled)
		}

		// Property: isMultiTenantReady() is true only when enabled AND pgManager AND tenantClient non-nil.
		ready := worker.isMultiTenantReady()
		if multiTenantEnabled && worker.pgManager != nil && worker.tenantClient != nil && !ready {
			t.Fatal("isMultiTenantReady() should be true when enabled, pgManager, and tenantClient are non-nil")
		}

		if !multiTenantEnabled && ready {
			t.Fatal("isMultiTenantReady() should be false when multiTenantEnabled is false")
		}

		// Property: batchSize equals maxWorkers (post-default).
		if worker.batchSize != int64(worker.maxWorkers) {
			t.Fatalf("batchSize (%d) should equal maxWorkers (%d)", worker.batchSize, worker.maxWorkers)
		}
	})
}

// FuzzNewRedisQueueConsumerMultiTenant_MultiTenantEnabled fuzzes the
// multiTenantEnabled flag of the NewRedisQueueConsumerMultiTenant constructor.
//
// Properties verified:
//  1. Constructor never panics.
//  2. Returned consumer is never nil.
//  3. multiTenantEnabled matches input.
//  4. isMultiTenantReady() is consistent with field state.
func FuzzNewRedisQueueConsumerMultiTenant_MultiTenantEnabled(f *testing.F) {
	// Seed 1: enabled with pgManager (typical multi-tenant)
	f.Add(true, true)
	// Seed 2: disabled with no pgManager (typical single-tenant)
	f.Add(false, false)
	// Seed 3: enabled without pgManager (fallback case)
	f.Add(true, false)
	// Seed 4: disabled with pgManager (misconfiguration edge case)
	f.Add(false, true)
	// Seed 5: enabled with pgManager (duplicate to ensure stability)
	f.Add(true, true)

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	f.Fuzz(func(t *testing.T, multiTenantEnabled bool, hasPGManager bool) {
		var mgr *tmpostgres.Manager
		if hasPGManager {
			mgr = pgMgr
		}

		// Property: constructor must never panic.
		consumer := NewRedisQueueConsumerMultiTenant(logger, handler, multiTenantEnabled, tenantClient, mgr)

		// Property: returned consumer is never nil.
		if consumer == nil {
			t.Fatal("NewRedisQueueConsumerMultiTenant returned nil")
		}

		// Property: multiTenantEnabled matches input.
		if consumer.multiTenantEnabled != multiTenantEnabled {
			t.Fatalf("multiTenantEnabled mismatch: got %v, want %v", consumer.multiTenantEnabled, multiTenantEnabled)
		}

		// Property: isMultiTenantReady() follows the predicate logic.
		ready := consumer.isMultiTenantReady()
		expectReady := multiTenantEnabled && mgr != nil && consumer.tenantClient != nil

		if ready != expectReady {
			t.Fatalf("isMultiTenantReady() = %v, want %v (enabled=%v, hasPGManager=%v, hasTenantClient=%v)",
				ready, expectReady, multiTenantEnabled, hasPGManager, consumer.tenantClient != nil)
		}
	})
}

// FuzzIsMultiTenantReady_FieldCombinations fuzzes both predicates by
// constructing structs with fuzz-driven field states and verifying the
// predicate never panics and always returns a consistent boolean.
//
// Properties verified:
//  1. isMultiTenantReady() never panics on any struct state.
//  2. Result is true IFF multiTenantEnabled == true AND pgManager != nil AND tenantClient != nil.
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
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	handler := in.TransactionHandler{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	f.Fuzz(func(t *testing.T, workerEnabled bool, workerHasPGMgr bool, consumerEnabled bool, consumerHasPGMgr bool) {
		// --- BalanceSyncWorker predicate ---
		worker := NewBalanceSyncWorker(conn, logger, useCase, 5)
		worker.multiTenantEnabled = workerEnabled

		if workerHasPGMgr {
			worker.pgManager = pgMgr
		} else {
			worker.pgManager = nil
		}

		// Property: never panics.
		workerReady := worker.isMultiTenantReady()

		// Property: result matches predicate logic.
		// tenantClient is not set in this fuzz test, so isMultiTenantReady() is always false.
		expectWorkerReady := workerEnabled && workerHasPGMgr && worker.tenantClient != nil
		if workerReady != expectWorkerReady {
			t.Fatalf("BalanceSyncWorker.isMultiTenantReady() = %v, want %v (enabled=%v, hasPGMgr=%v, hasTenantClient=%v)",
				workerReady, expectWorkerReady, workerEnabled, workerHasPGMgr, worker.tenantClient != nil)
		}

		// Property: idempotent.
		if worker.isMultiTenantReady() != workerReady {
			t.Fatal("BalanceSyncWorker.isMultiTenantReady() is not idempotent")
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
		// tenantClient is not set in this fuzz test, so isMultiTenantReady() is always false.
		expectConsumerReady := consumerEnabled && consumerHasPGMgr && consumer.tenantClient != nil
		if consumerReady != expectConsumerReady {
			t.Fatalf("RedisQueueConsumer.isMultiTenantReady() = %v, want %v (enabled=%v, hasPGMgr=%v, hasTenantClient=%v)",
				consumerReady, expectConsumerReady, consumerEnabled, consumerHasPGMgr, consumer.tenantClient != nil)
		}

		// Property: idempotent.
		if consumer.isMultiTenantReady() != consumerReady {
			t.Fatal("RedisQueueConsumer.isMultiTenantReady() is not idempotent")
		}

		// --- Zero-value struct edge case ---
		zeroWorker := &BalanceSyncWorker{}
		if zeroWorker.isMultiTenantReady() {
			t.Fatal("zero-value BalanceSyncWorker.isMultiTenantReady() should be false")
		}

		zeroConsumer := &RedisQueueConsumer{}
		if zeroConsumer.isMultiTenantReady() {
			t.Fatal("zero-value RedisQueueConsumer.isMultiTenantReady() should be false")
		}
	})
}
