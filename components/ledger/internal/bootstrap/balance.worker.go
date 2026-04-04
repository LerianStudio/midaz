// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	redisTransaction "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// BalanceSyncConfig holds configuration for the balance sync dual-trigger.
type BalanceSyncConfig struct {
	// BatchSize is the number of keys to accumulate before flushing (SIZE trigger).
	BatchSize int
	// FlushTimeoutMs is the max time in milliseconds before flushing an incomplete batch (TIMEOUT trigger).
	FlushTimeoutMs int
	// PollIntervalMs is the ZSET polling interval in milliseconds when buffer has items but no new keys.
	PollIntervalMs int
}

// BalanceSyncWorker continuously processes balance keys using a dual-trigger collector.
// Keys become eligible immediately after balance mutation (Lua ZADD with dueAt=now).
// The worker accumulates keys and flushes based on batch size OR timeout, whichever comes first.
type BalanceSyncWorker struct {
	redisConn          *libRedis.Client
	logger             libLog.Logger
	idleWait           time.Duration
	batchSize          int64
	maxWorkers         int
	syncConfig         BalanceSyncConfig
	useCase            *command.UseCase
	multiTenantEnabled bool
	tenantCache        *tenantcache.TenantCache
	pgManager          *tmpostgres.Manager
	serviceName        string
}

func NewBalanceSyncWorker(conn *libRedis.Client, logger libLog.Logger, useCase *command.UseCase, maxWorkers int, syncCfg BalanceSyncConfig) *BalanceSyncWorker {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	// Apply safe defaults for zero-value config (e.g., in tests)
	if syncCfg.BatchSize <= 0 {
		syncCfg.BatchSize = 50
	}

	if syncCfg.FlushTimeoutMs <= 0 {
		syncCfg.FlushTimeoutMs = 500
	}

	if syncCfg.PollIntervalMs <= 0 {
		syncCfg.PollIntervalMs = 50
	}

	// Idle wait defaults to 2x the flush timeout. With dual-trigger and dueAt=now,
	// a long idle backoff (e.g. 10 min) would delay pickup of new keys. Using a short
	// idle wait ensures the worker re-checks the ZSET frequently when transitioning
	// from idle to busy mode.
	idleWait := time.Duration(syncCfg.FlushTimeoutMs*2) * time.Millisecond
	if idleWait < 1*time.Second {
		idleWait = 1 * time.Second
	}

	return &BalanceSyncWorker{
		redisConn:  conn,
		logger:     logger,
		idleWait:   idleWait,
		batchSize:  int64(syncCfg.BatchSize),
		maxWorkers: maxWorkers,
		syncConfig: syncCfg,
		useCase:    useCase,
	}
}

// NewBalanceSyncWorkerMultiTenant creates a BalanceSyncWorker with multi-tenant fields populated.
// When multiTenantEnabled is true, both tenantCache and pgManager must be non-nil for the worker
// to be considered ready (isMultiTenantReady). The worker reads tenant IDs from the shared
// TenantCache (populated by the TenantEventListener) and uses pgManager to resolve per-tenant
// PostgreSQL connections.
// serviceName is the service identifier for logging purposes.
func NewBalanceSyncWorkerMultiTenant(
	conn *libRedis.Client,
	logger libLog.Logger,
	useCase *command.UseCase,
	maxWorkers int,
	syncCfg BalanceSyncConfig,
	multiTenantEnabled bool,
	cache *tenantcache.TenantCache,
	pgManager *tmpostgres.Manager,
	serviceName string,
) *BalanceSyncWorker {
	w := NewBalanceSyncWorker(conn, logger, useCase, maxWorkers, syncCfg)
	w.multiTenantEnabled = multiTenantEnabled
	w.tenantCache = cache
	w.pgManager = pgManager
	w.serviceName = serviceName

	return w
}

// isMultiTenantReady returns true when the worker is configured for multi-tenant
// dispatching. multiTenantEnabled, pgManager, and tenantCache must all be set;
// if any is missing the worker falls back to single-tenant behavior.
func (w *BalanceSyncWorker) isMultiTenantReady() bool {
	return w.multiTenantEnabled && w.pgManager != nil && w.tenantCache != nil
}

// Run dispatches to multi-tenant or single-tenant execution based on configuration.
func (w *BalanceSyncWorker) Run(_ *libCommons.Launcher) error {
	if w.isMultiTenantReady() {
		return w.runMultiTenant()
	}

	return w.runSingleTenant()
}

// runSingleTenant runs the balance sync loop using the default (shared) database connection.
// Uses the dual-trigger collector (size OR timeout) for near-real-time balance persistence.
func (w *BalanceSyncWorker) runSingleTenant() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker started (single-tenant, dual-trigger: batch_size=%d, flush_timeout=%dms, poll_interval=%dms)",
		w.syncConfig.BatchSize, w.syncConfig.FlushTimeoutMs, w.syncConfig.PollIntervalMs))

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: failed to get redis client: %v", err))

		return err
	}

	collector := NewBalanceSyncCollector(
		w.syncConfig.BatchSize,
		time.Duration(w.syncConfig.FlushTimeoutMs)*time.Millisecond,
		time.Duration(w.syncConfig.PollIntervalMs)*time.Millisecond,
		w.idleWait,
		w.logger,
	)

	collector.SetFlushCallback(func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
		return w.flushBatch(flushCtx, keys)
	})

	collector.Run(ctx,
		func(fetchCtx context.Context, limit int64) ([]redisTransaction.SyncKey, error) {
			return w.useCase.TransactionRedisRepo.GetBalanceSyncKeys(fetchCtx, limit)
		},
		func(waitCtx context.Context) bool {
			return w.waitForNextOrBackoff(waitCtx, rds)
		},
	)

	w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

	return nil
}

// runMultiTenant runs the balance sync loop iterating over all active tenants.
// Each tenant gets its own dual-trigger collector for independent batch accumulation.
// If a tenant's connection fails, it logs and skips that tenant without affecting others.
func (w *BalanceSyncWorker) runMultiTenant() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker started (multi-tenant, dual-trigger: batch_size=%d, flush_timeout=%dms, poll_interval=%dms)",
		w.syncConfig.BatchSize, w.syncConfig.FlushTimeoutMs, w.syncConfig.PollIntervalMs))

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: failed to get redis client: %v", err))

		return err
	}

	for {
		if w.shouldShutdown(ctx) {
			w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

			return nil
		}

		tenantIDs, ok := w.discoverActiveTenants(ctx, rds)
		if !ok {
			continue
		}

		if tenantIDs == nil {
			return nil
		}

		processed := false

		for _, tenantID := range tenantIDs {
			if w.shouldShutdown(ctx) {
				w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

				return nil
			}

			if w.processTenantBalances(ctx, tenantID, rds) {
				processed = true
			}
		}

		if !processed {
			if w.waitForNextOrBackoff(ctx, rds) {
				w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

				return nil
			}
		}
	}
}

// discoverActiveTenants reads tenant IDs from the shared TenantCache.
// Returns (tenantIDs, true) on success, (nil, false) if no tenants are cached
// (backs off and retries), or (nil, true) if shutdown was requested during backoff.
func (w *BalanceSyncWorker) discoverActiveTenants(ctx context.Context, rds redis.UniversalClient) ([]string, bool) {
	tenantIDs := w.tenantCache.TenantIDs()

	if len(tenantIDs) == 0 {
		w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: no tenants in cache, backing off")

		if w.waitForNextOrBackoff(ctx, rds) {
			w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

			return nil, true
		}

		return nil, false
	}

	return tenantIDs, true
}

// processTenantBalances resolves the per-tenant PostgreSQL connection, augments the context
// with the tenant ID and module connection, then processes expired balances for that tenant.
// Returns true if any balances were processed.
func (w *BalanceSyncWorker) processTenantBalances(ctx context.Context, tenantID string, rds redis.UniversalClient) bool {
	tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)

	conn, err := w.pgManager.GetConnection(tenantCtx, tenantID)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: failed to get PG connection for tenant %s: %v", tenantID, err))

		return false
	}

	db, err := conn.GetDB()
	if err != nil {
		w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: failed to get DB for tenant %s: %v", tenantID, err))

		return false
	}

	tenantCtx = tmcore.ContextWithPG(tenantCtx, db)
	tenantCtx = tmcore.ContextWithPG(tenantCtx, db, constant.ModuleTransaction)

	return w.processBalancesToExpire(tenantCtx, rds)
}

func (w *BalanceSyncWorker) shouldShutdown(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// orgLedgerGroup holds keys grouped by organization and ledger for batch processing.
type orgLedgerGroup struct {
	orgID    uuid.UUID
	ledgerID uuid.UUID
	keys     []redisTransaction.SyncKey
}

// flushBatch groups keys by (orgID, ledgerID) and processes each group via SyncBalancesBatch.
// This is the flush callback used by the BalanceSyncCollector.
func (w *BalanceSyncWorker) flushBatch(ctx context.Context, keys []redisTransaction.SyncKey) bool {
	if len(keys) == 0 {
		return false
	}

	w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: flushBatch called with %d keys", len(keys)))

	groups := w.groupKeysByOrgLedger(keys)
	processed := false

	for _, group := range groups {
		w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: syncing group org=%s ledger=%s with %d keys", group.orgID, group.ledgerID, len(group.keys)))

		if w.processBalancesToExpireBatch(ctx, group.orgID, group.ledgerID, group.keys) {
			processed = true
		}
	}

	return processed
}

// groupKeysByOrgLedger groups Redis balance keys by their (organizationID, ledgerID) pair.
// Keys that cannot be parsed are logged and skipped.
func (w *BalanceSyncWorker) groupKeysByOrgLedger(keys []redisTransaction.SyncKey) []orgLedgerGroup {
	type groupKey struct {
		orgID    uuid.UUID
		ledgerID uuid.UUID
	}

	grouped := make(map[groupKey][]redisTransaction.SyncKey, 1) // typically 1 group in single-tenant

	for _, key := range keys {
		orgID, ledgerID, err := w.extractIDsFromMember(key.Key)
		if err != nil {
			w.logger.Log(context.Background(), libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to extract IDs from key %s: %v — removing from schedule", key.Key, err))

			// Clean up the claimed entry to prevent it from becoming a poison record.
			// Uses the batch variant with a single element so the conditional ZREM
			// and lock cleanup run through the same Lua script path.
			if _, remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(context.Background(), []redisTransaction.SyncKey{key}); remErr != nil {
				w.logger.Log(context.Background(), libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to remove unparseable key %s: %v", key.Key, remErr))
			}

			continue
		}

		gk := groupKey{orgID: orgID, ledgerID: ledgerID}
		grouped[gk] = append(grouped[gk], key)
	}

	result := make([]orgLedgerGroup, 0, len(grouped))
	for gk, groupKeys := range grouped {
		result = append(result, orgLedgerGroup{
			orgID:    gk.orgID,
			ledgerID: gk.ledgerID,
			keys:     groupKeys,
		})
	}

	return result
}

func (w *BalanceSyncWorker) processBalancesToExpire(ctx context.Context, rds redis.UniversalClient) bool {
	members, err := w.useCase.TransactionRedisRepo.GetBalanceSyncKeys(ctx, w.batchSize)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: get balance sync keys error: %v", err))
		}

		return false
	}

	if len(members) == 0 {
		return false
	}

	// Check for shutdown before starting batch processing
	if w.shouldShutdown(ctx) {
		w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")
		return true
	}

	// This is guaranteed by the worker's scheduling mechanism which fetches keys
	// from a single ZSET scoped per tenant context. In multi-tenant mode,
	// processTenantBalances is called per-tenant, ensuring batch homogeneity.
	orgID, ledgerID, extractErr := w.extractIDsFromMember(members[0].Key)
	if extractErr == nil {
		return w.processBalancesToExpireBatch(ctx, orgID, ledgerID, members)
	}

	w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to extract IDs for batch, falling back to individual processing: %v", extractErr))

	// Fallback: individual processing (only when batch mode fails)
	workers := w.maxWorkers
	if int64(workers) > w.batchSize {
		workers = int(w.batchSize)
	}

	if workers <= 0 {
		workers = 1
	}

	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup

	for _, m := range members {
		if w.shouldShutdown(ctx) {
			w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

			return true
		}

		member := m.Key

		sem <- struct{}{}

		wg.Add(1)

		go func(member string) {
			defer func() { <-sem }()

			defer wg.Done()

			defer func() {
				if r := recover(); r != nil {
					w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: panic recovered while processing %s: %v", member, r))
				}
			}()

			if w.shouldShutdown(ctx) {
				return
			}

			w.processBalanceToExpire(ctx, rds, member)
		}(member)
	}

	wg.Wait()

	return true
}

// processBalancesToExpireBatch processes all due keys using batch aggregation.
// This is more efficient than individual processing as it:
//  1. Fetches all balance values in single MGET
//  2. Aggregates by composite key, keeping only highest version
//  3. Persists in single database transaction
//  4. Removes all processed keys in batch
//
// Returns true if any balances were processed.
func (w *BalanceSyncWorker) processBalancesToExpireBatch(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []redisTransaction.SyncKey) bool {
	if len(keys) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "balance.worker.process_batch")
	defer span.End()

	result, err := w.useCase.SyncBalancesBatch(ctx, organizationID, ledgerID, keys)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: batch sync failed: %v", err))

		// Emit failure metric for monitoring
		counter, counterErr := metricFactory.Counter(utils.BalanceSyncBatchFailures)
		if counterErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to create counter %v: %v", utils.BalanceSyncBatchFailures, counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
			}).AddOne(ctx); metricErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to increment counter %v: %v", utils.BalanceSyncBatchFailures, metricErr))
			}
		}

		return false
	}

	if result.BalancesSynced > 0 {
		counter, counterErr := metricFactory.Counter(utils.BalanceSynced)
		if counterErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to create counter %v: %v", utils.BalanceSynced, counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
				"mode":            "batch",
			}).Add(ctx, result.BalancesSynced); metricErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to add counter %v: %v", utils.BalanceSynced, metricErr))
			}
		}
	}

	// Log aggregation ratio for monitoring deduplication effectiveness
	if result.KeysProcessed > 0 {
		aggregationRatio := float64(result.BalancesAggregated) / float64(result.KeysProcessed)
		w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: batch processed=%d, aggregated=%d (ratio=%.2f), synced=%d",
			result.KeysProcessed, result.BalancesAggregated, aggregationRatio, result.BalancesSynced))
	}

	return result.BalancesSynced > 0 || result.BalancesAggregated > 0
}

// waitForNextOrBackoff waits based on the next schedule entry or backs off if none.
// Returns true if it shut down while waiting.
func (w *BalanceSyncWorker) waitForNextOrBackoff(ctx context.Context, rds redis.UniversalClient) bool {
	next, err := rds.ZRangeWithScores(ctx, utils.BalanceSyncScheduleKey, 0, 0).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: zrangewithscores error: %v", err))

		return waitOrDone(ctx, w.idleWait, w.logger)
	}

	if len(next) == 0 {
		w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: nothing scheduled; back off.")

		return waitOrDone(ctx, w.idleWait, w.logger)
	}

	w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: next: %+v", next[0]))

	return w.waitUntilDue(ctx, int64(next[0].Score), w.logger)
}

// processBalanceToExpire handles a single scheduled member lifecycle.
// WHY: Reduce cognitive complexity of Run by isolating the per-member logic.
func (w *BalanceSyncWorker) processBalanceToExpire(ctx context.Context, rds redis.UniversalClient, member string) {
	// Timeout shorter than lock TTL (600s) to ensure operations don't exceed the lock duration
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "balance.worker.process_balance_to_expire")
	defer span.End()

	if member == "" {
		return
	}

	ttl, err := rds.TTL(ctx, member).Result()
	if err != nil {
		w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: TTL error for %s: %v", member, err))

		return
	}

	// Handle missing key regardless of TTL sentinel representation (-2 or -2s)
	if ttl == -2 || ttl == -2*time.Second {
		w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: already-gone key: %s, removing from schedule", member))

		if remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to remove expired balance sync key %s: %v", member, remErr))
		}

		return
	}

	val, err := rds.Get(ctx, member).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: missing key on GET: %s, removing from schedule", member))

			if remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to remove missing balance sync key %s: %v", member, remErr))
			}
		} else {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: GET error for %s: %v", member, err))
		}

		return
	}

	organizationID, ledgerID, err := w.extractIDsFromMember(member)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: extractIDsFromMember error for %s: %v", member, err))

		if remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to remove unparsable balance sync key %s: %v", member, remErr))
		}

		return
	}

	var balance mmodel.BalanceRedis
	if err := json.Unmarshal([]byte(val), &balance); err != nil {
		w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: Unmarshal error for %s: %v", member, err))

		if remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to remove unmarshalable balance sync key %s: %v", member, remErr))
		}

		return
	}

	synced, err := w.useCase.SyncBalance(ctx, organizationID, ledgerID, balance)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: SyncBalance error for member %s, balanceID=%s: %v", member, balance.ID, err))

		return
	}

	if synced {
		counter, counterErr := metricFactory.Counter(utils.BalanceSynced)
		if counterErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to create counter %v: %v", utils.BalanceSynced, counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
				"mode":            "individual",
			}).AddOne(ctx); metricErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to increment counter %v: %v", utils.BalanceSynced, metricErr))
			}
		}

		w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: Synced key %s", member))
	}

	if remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
		w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: failed to remove balance sync key %s: %v", member, remErr))
	}
}

// waitOrDone waits for d or returns true if ctx is done first.
func waitOrDone(ctx context.Context, d time.Duration, logger libLog.Logger) bool {
	if d <= 0 {
		return false
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: waiting for %s", d.String()))

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

// waitUntilDue waits until the given dueAt score time.
// The score uses microsecond precision (seconds*1e6 + microseconds) to match
// the ZADD scores set by the Lua balance_atomic_operation script.
// Returns true if the context was cancelled while waiting.
func (w *BalanceSyncWorker) waitUntilDue(ctx context.Context, dueAtScore int64, logger libLog.Logger) bool {
	nowMicro := time.Now().UnixMicro()
	if dueAtScore <= nowMicro {
		// Due time already passed but item not processed (ZSET/sync-queue desync).
		// Apply minimal backoff to prevent busy loop when:
		// - ZSET has entry with expired score
		// - Sync queue is empty (lock already claimed or data expired)
		// Without this delay, the worker would spin indefinitely.
		return waitOrDone(ctx, 500*time.Millisecond, logger)
	}

	waitFor := time.Duration(dueAtScore-nowMicro) * time.Microsecond
	if waitFor <= 0 {
		return waitOrDone(ctx, 500*time.Millisecond, logger)
	}

	return waitOrDone(ctx, waitFor, logger)
}

// extractIDsFromMember parses a Redis member key that follows the pattern
// balance:{transactions}:<organizationID>:<ledgerID>:@account#key
func (w *BalanceSyncWorker) extractIDsFromMember(member string) (organizationID uuid.UUID, ledgerID uuid.UUID, err error) {
	var (
		first     uuid.UUID
		haveFirst bool
	)

	start := 0

	for i := 0; i <= len(member); i++ {
		if i == len(member) || member[i] == ':' {
			if i > start {
				seg := member[start:i]
				if len(seg) == 36 {
					if u, e := uuid.Parse(seg); e == nil {
						if !haveFirst {
							first = u
							haveFirst = true
						} else {
							return first, u, nil
						}
					}
				}
			}

			start = i + 1
		}
	}

	return uuid.UUID{}, uuid.UUID{}, fmt.Errorf("balance sync key missing two UUIDs (orgID, ledgerID): %q", member)
}
