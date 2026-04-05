// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"testing/quick"

	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/stretchr/testify/require"
)

// TestProperty_RedisQueueConsumer_PredicateEqualsEnabledAndPGManagerNonNil verifies
// the same predicate correctness invariant on RedisQueueConsumer.
func TestProperty_RedisQueueConsumer_PredicateEqualsEnabledAndPGManagerNonNil(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tc, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))

	cache := tenantcache.NewTenantCache()

	property := func(enabled bool, hasPGManager bool, hasTenantCache bool) bool {
		consumer := NewRedisQueueConsumer(logger, handler)
		consumer.multiTenantEnabled = enabled

		if hasPGManager {
			consumer.pgManager = pgMgr
		} else {
			consumer.pgManager = nil
		}

		if hasTenantCache {
			consumer.tenantCache = cache
		} else {
			consumer.tenantCache = nil
		}

		got := consumer.isMultiTenantReady()
		expected := enabled && hasPGManager && hasTenantCache

		// Property: predicate result MUST equal (enabled && pgManager != nil && tenantCache != nil).
		if got != expected {
			return false
		}

		// Property: idempotent -- calling again yields the same result.
		return consumer.isMultiTenantReady() == got
	}

	err = quick.Check(property, &quick.Config{MaxCount: 200})
	require.NoError(t, err,
		"Property violated: RedisQueueConsumer.isMultiTenantReady() != (multiTenantEnabled && pgManager != nil && tenantCache != nil)")
}

// TestProperty_NewRedisQueueConsumerMultiTenant_PreservesBaseFields verifies that
// the multi-tenant constructor preserves all base constructor fields AND correctly
// sets the multi-tenant fields.
func TestProperty_NewRedisQueueConsumerMultiTenant_PreservesBaseFields(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	consumerCache := tenantcache.NewTenantCache()

	property := func(enabled bool, hasPGManager bool) bool {
		var mgr *tmpostgres.Manager
		if hasPGManager {
			mgr = pgMgr
		}

		consumer := NewRedisQueueConsumerMultiTenant(logger, handler, enabled, consumerCache, mgr, "transaction")

		// Property: constructor never returns nil.
		if consumer == nil {
			return false
		}

		// Property: base fields preserved (referential equality for Logger).
		if consumer.Logger != logger {
			return false
		}

		// Property: multi-tenant fields set correctly.
		if consumer.multiTenantEnabled != enabled {
			return false
		}

		if consumer.tenantCache != consumerCache {
			return false
		}

		if hasPGManager && consumer.pgManager != pgMgr {
			return false
		}

		if !hasPGManager && consumer.pgManager != nil {
			return false
		}

		// Property: isMultiTenantReady() is consistent with field state.
		expectedReady := enabled && hasPGManager && consumer.tenantCache != nil

		return consumer.isMultiTenantReady() == expectedReady
	}

	err = quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Property violated: NewRedisQueueConsumerMultiTenant did not preserve base fields or set multi-tenant fields correctly")
}

// TestProperty_MultiTenantConstructors_NeverPanic verifies the nil-safety
// invariant: no constructor or predicate ever panics for any combination of
// nil/non-nil pointers.
func TestProperty_MultiTenantConstructors_NeverPanic(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	useCase := &command.UseCase{}
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	neverPanicCache := tenantcache.NewTenantCache()

	property := func(
		workerEnabled bool,
		workerHasUseCase bool, workerHasTenantCache bool, workerHasPGManager bool,
		consumerEnabled bool,
		consumerHasTenantCache bool, consumerHasPGManager bool,
	) bool {
		// --- BalanceSyncWorker constructor with varying nil pointers ---
		var wUseCase *command.UseCase
		if workerHasUseCase {
			wUseCase = useCase
		}

		var wTenantCache *tenantcache.TenantCache
		if workerHasTenantCache {
			wTenantCache = neverPanicCache
		}

		var wPGManager *tmpostgres.Manager
		if workerHasPGManager {
			wPGManager = pgMgr
		}

		// Property: NewBalanceSyncWorkerMT must never panic.
		worker := NewBalanceSyncWorkerMT(logger, wUseCase, BalanceSyncConfig{}, workerEnabled, wTenantCache, wPGManager, "transaction")
		if worker == nil {
			return false
		}

		// Property: isMTReady() must never panic.
		workerReady := worker.isMTReady()
		expectedWorkerReady := workerEnabled && wPGManager != nil && wTenantCache != nil

		if workerReady != expectedWorkerReady {
			return false
		}

		// --- RedisQueueConsumer constructor with varying nil pointers ---
		var cTenantCache *tenantcache.TenantCache
		if consumerHasTenantCache {
			cTenantCache = neverPanicCache
		}

		var cPGManager *tmpostgres.Manager
		if consumerHasPGManager {
			cPGManager = pgMgr
		}

		// Property: NewRedisQueueConsumerMultiTenant must never panic.
		consumer := NewRedisQueueConsumerMultiTenant(logger, handler, consumerEnabled, cTenantCache, cPGManager, "transaction")
		if consumer == nil {
			return false
		}

		// Property: isMultiTenantReady() must never panic.
		consumerReady := consumer.isMultiTenantReady()
		expectedConsumerReady := consumerEnabled && cPGManager != nil && cTenantCache != nil

		if consumerReady != expectedConsumerReady {
			return false
		}

		// --- Zero-value struct edge case ---
		zeroWorker := &BalanceSyncWorker{}
		if zeroWorker.isMTReady() {
			return false
		}

		zeroConsumer := &RedisQueueConsumer{}
		if zeroConsumer.isMultiTenantReady() {
			return false
		}

		return true
	}

	err = quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Property violated: a constructor or predicate panicked or returned an inconsistent result for a nil/non-nil combination")
}
