// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
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

// FlushTimeout returns FlushTimeoutMs as a time.Duration.
func (c BalanceSyncConfig) FlushTimeout() time.Duration {
	return time.Duration(c.FlushTimeoutMs) * time.Millisecond
}

// PollInterval returns PollIntervalMs as a time.Duration.
func (c BalanceSyncConfig) PollInterval() time.Duration {
	return time.Duration(c.PollIntervalMs) * time.Millisecond
}

// BalanceSyncWorker continuously processes balance keys using a dual-trigger collector.
// Keys become eligible immediately after balance mutation (Lua ZADD with dueAt=now).
// The worker accumulates keys and flushes based on batch size OR timeout, whichever comes first.
type BalanceSyncWorker struct {
	redisConn   *libRedis.Client
	logger      libLog.Logger
	idleWait    time.Duration
	batchSize   int64
	maxWorkers  int
	syncConfig  BalanceSyncConfig
	useCase     *command.UseCase
	mtEnabled   bool
	tenantCache *tenantcache.TenantCache
	pgManager   *tmpostgres.Manager
	serviceName string
}

// tenantCollector tracks a running BalanceSyncCollector goroutine for a specific tenant.
// Each tenant gets its own independent collector with dual-trigger batching.
type tenantCollector struct {
	tenantID string
	cancel   context.CancelFunc
	done     chan struct{} // closed when the collector goroutine exits
}

// tenantReconcileInterval is how often the multi-tenant worker checks for
// added/removed tenants in the TenantCache.
const tenantReconcileInterval = 10 * time.Second

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

// NewBalanceSyncWorkerMT creates a BalanceSyncWorker with MT (multi-tenant) fields populated.
// When mtEnabled is true, both tenantCache and pgManager must be non-nil for the worker
// to be considered ready (isMTReady). The worker reads tenant IDs from the shared
// TenantCache (populated by the TenantEventListener) and uses pgManager to resolve per-tenant
// PostgreSQL connections.
// serviceName is the service identifier for logging purposes.
func NewBalanceSyncWorkerMT(
	conn *libRedis.Client,
	logger libLog.Logger,
	useCase *command.UseCase,
	maxWorkers int,
	syncCfg BalanceSyncConfig,
	mtEnabled bool,
	cache *tenantcache.TenantCache,
	pgManager *tmpostgres.Manager,
	serviceName string,
) *BalanceSyncWorker {
	w := NewBalanceSyncWorker(conn, logger, useCase, maxWorkers, syncCfg)
	w.mtEnabled = mtEnabled
	w.tenantCache = cache
	w.pgManager = pgManager
	w.serviceName = serviceName

	return w
}

// isMTReady returns true when the worker is configured for MT (multi-tenant)
// dispatching. mtEnabled, pgManager, and tenantCache must all be set;
// if any is missing the worker falls back to default (single-tenant) behavior.
func (w *BalanceSyncWorker) isMTReady() bool {
	return w.mtEnabled && w.pgManager != nil && w.tenantCache != nil
}

// Run dispatches to multi-tenant or single-tenant execution based on configuration.
// The Launcher parameter is intentionally unused: lib-commons Launcher (v4) does not
// expose a cancellable context or coordinate shutdown between apps. Each execution
// mode creates its own signal.NotifyContext(context.Background(), ...) to handle
// SIGTERM/SIGINT independently — this is the standard pattern across all Midaz workers.
func (w *BalanceSyncWorker) Run(_ *libCommons.Launcher) error {
	if w.isMTReady() {
		return w.runWorkerMT()
	}

	return w.runWorker()
}

// runWorker runs the default balance sync loop using the shared database connection.
// Uses the dual-trigger collector (size OR timeout) for near-real-time balance persistence.
func (w *BalanceSyncWorker) runWorker() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker started (single-tenant, dual-trigger)",
		libLog.Int("batch_size", w.syncConfig.BatchSize),
		libLog.Int("flush_timeout_ms", w.syncConfig.FlushTimeoutMs),
		libLog.Int("poll_interval_ms", w.syncConfig.PollIntervalMs),
	)

	collector := NewBalanceSyncCollector(
		w.syncConfig.BatchSize,
		w.syncConfig.FlushTimeout(),
		w.syncConfig.PollInterval(),
		w.logger,
	)

	collector.Run(ctx,
		// FlushFunc: batch flush grouped by org/ledger, then persisted to PostgreSQL
		func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
			return w.flushBatch(flushCtx, keys)
		},
		// FetchKeysFunc: claims due keys from the ZSET via Lua (ZRANGEBYSCORE + SET NX)
		func(fetchCtx context.Context, limit int64) ([]redisTransaction.SyncKey, error) {
			return w.useCase.TransactionRedisRepo.GetBalanceSyncKeys(fetchCtx, limit)
		},
		// WaitForNextFunc: fixed backoff when idle (ZSET empty)
		func(waitCtx context.Context) bool {
			return waitOrDone(waitCtx, w.idleWait, w.logger)
		},
	)

	w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

	return nil
}

// runWorkerMT runs one BalanceSyncCollector per active tenant, each with its own
// dual-trigger (size OR timeout) batch accumulation. A reconciliation loop periodically
// checks the TenantCache for added/removed tenants and starts/stops collectors accordingly.
//
// Unlike runWorker which runs a single collector inline, the MT path launches each
// collector as a goroutine. When the parent context is cancelled (SIGTERM/SIGINT),
// all tenant collectors are stopped and their remaining buffers are flushed.
func (w *BalanceSyncWorker) runWorkerMT() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker started (multi-tenant, dual-trigger: batch_size=%d, flush_timeout=%dms, poll_interval=%dms, reconcile_interval=%s)",
		w.syncConfig.BatchSize, w.syncConfig.FlushTimeoutMs, w.syncConfig.PollIntervalMs, tenantReconcileInterval))

	collectors := make(map[string]*tenantCollector)
	defer w.stopAllCollectors(ctx, collectors)

	// Reconcile immediately on startup, then on a ticker.
	w.reconcileCollectors(ctx, collectors)

	ticker := time.NewTicker(tenantReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

			return nil
		case <-ticker.C:
			w.reconcileCollectors(ctx, collectors)
		}
	}
}

// reconcileCollectors synchronizes the set of running collectors with the active tenants
// in the TenantCache. It has three phases:
//  1. Reap: detect collectors whose goroutines exited unexpectedly (e.g., panic)
//  2. Stop: cancel collectors for tenants no longer in the cache
//  3. Start: launch collectors for newly discovered tenants
func (w *BalanceSyncWorker) reconcileCollectors(ctx context.Context, collectors map[string]*tenantCollector) {
	tenantIDs := w.tenantCache.TenantIDs()

	activeSet := make(map[string]struct{}, len(tenantIDs))
	for _, id := range tenantIDs {
		activeSet[id] = struct{}{}
	}

	// Phase 1: Reap dead collectors (goroutine exited unexpectedly).
	// Removing them from the map allows Phase 3 to restart them if the tenant is still active.
	for id, tc := range collectors {
		select {
		case <-tc.done:
			w.logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("BalanceSyncWorker: collector for tenant %s exited unexpectedly, will restart", id))

			delete(collectors, id)
		default:
		}
	}

	// Phase 2: Cancel collectors for removed tenants (non-blocking cancel, deferred wait).
	var removed []*tenantCollector

	for id, tc := range collectors {
		if _, ok := activeSet[id]; !ok {
			w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: tenant %s removed from cache, stopping collector", id))

			tc.cancel()

			removed = append(removed, tc)

			delete(collectors, id)
		}
	}

	// Phase 3: Start collectors for new tenants.
	for _, id := range tenantIDs {
		if _, ok := collectors[id]; ok {
			continue // already running
		}

		tc := w.startTenantCollector(ctx, id)
		if tc != nil {
			collectors[id] = tc
		}
	}

	// Phase 4: Wait for removed collectors to finish (all cancellations already sent in Phase 2).
	for _, tc := range removed {
		<-tc.done

		w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: collector for tenant %s stopped", tc.tenantID))
	}

	if len(tenantIDs) == 0 {
		w.logger.Log(ctx, libLog.LevelDebug, "BalanceSyncWorker: no tenants in cache, will retry on next reconciliation")
	}
}

// startTenantCollector resolves the tenant's PostgreSQL connection, creates a
// BalanceSyncCollector with tenant-scoped fetch/flush functions, and launches it
// as a goroutine. Returns nil if the tenant's PG connection cannot be established.
func (w *BalanceSyncWorker) startTenantCollector(parentCtx context.Context, tenantID string) *tenantCollector {
	tenantCtx := tmcore.ContextWithTenantID(parentCtx, tenantID)

	conn, err := w.pgManager.GetConnection(tenantCtx, tenantID)
	if err != nil {
		w.logger.Log(parentCtx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: failed to get PG connection for tenant %s: %v", tenantID, err))

		return nil
	}

	db, err := conn.GetDB()
	if err != nil {
		w.logger.Log(parentCtx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: failed to get DB for tenant %s: %v", tenantID, err))

		return nil
	}

	tenantCtx = tmcore.ContextWithPG(tenantCtx, db)
	tenantCtx = tmcore.ContextWithPG(tenantCtx, db, constant.ModuleTransaction)

	collectorCtx, cancel := context.WithCancel(tenantCtx)

	collector := NewBalanceSyncCollector(
		w.syncConfig.BatchSize,
		w.syncConfig.FlushTimeout(),
		w.syncConfig.PollInterval(),
		w.logger,
	)

	done := make(chan struct{})

	go func() {
		defer close(done)

		defer func() {
			if r := recover(); r != nil {
				w.logger.Log(parentCtx, libLog.LevelError, fmt.Sprintf("BalanceSyncWorker: collector for tenant %s panicked: %v", tenantID, r))
			}
		}()

		w.logger.Log(collectorCtx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: collector started for tenant %s", tenantID))

		collector.Run(collectorCtx,
			// FlushFunc: batch flush grouped by org/ledger
			func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
				return w.flushBatch(flushCtx, keys)
			},
			// FetchKeysFunc: tenant context enables Redis key namespacing via tmvalkey.GetKeyContext
			func(fetchCtx context.Context, limit int64) ([]redisTransaction.SyncKey, error) {
				return w.useCase.TransactionRedisRepo.GetBalanceSyncKeys(fetchCtx, limit)
			},
			// WaitForNextFunc: fixed backoff when idle (ZSET empty for this tenant)
			func(waitCtx context.Context) bool {
				return waitOrDone(waitCtx, w.idleWait, w.logger)
			},
		)

		w.logger.Log(parentCtx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: collector stopped for tenant %s", tenantID))
	}()

	return &tenantCollector{
		tenantID: tenantID,
		cancel:   cancel,
		done:     done,
	}
}

// stopAllCollectors cancels all running tenant collectors and waits for them to finish.
// Each collector's deferred flushRemaining runs on cancellation, draining any buffered keys.
func (w *BalanceSyncWorker) stopAllCollectors(ctx context.Context, collectors map[string]*tenantCollector) {
	if len(collectors) == 0 {
		return
	}

	w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: stopping %d tenant collector(s)...", len(collectors)))

	// Cancel all collectors concurrently
	for _, tc := range collectors {
		tc.cancel()
	}

	// Wait for all to finish (each flushes its remaining buffer)
	for id, tc := range collectors {
		<-tc.done

		w.logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("BalanceSyncWorker: collector stopped for tenant %s", id))

		delete(collectors, id)
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

// waitOrDone waits for d or returns true if ctx is done first.
func waitOrDone(ctx context.Context, d time.Duration, logger libLog.Logger) bool {
	if d <= 0 {
		return false
	}

	logger.Log(ctx, libLog.LevelDebug, "BalanceSyncWorker: idle wait",
		libLog.String("duration", d.String()),
	)

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
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
