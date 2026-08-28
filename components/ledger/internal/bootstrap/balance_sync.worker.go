// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
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

// tenantPGResolver resolves a tenant's transaction-module PostgreSQL handle.
// Satisfied by *tmpostgres.Manager.
type tenantPGResolver interface {
	GetDB(ctx context.Context, tenantID string) (dbresolver.DB, error)
}

// BalanceSyncWorker continuously processes balance keys using a dual-trigger collector.
// Keys become eligible immediately after balance mutation (Lua ZADD with dueAt=now).
// The worker accumulates keys and flushes based on batch size OR timeout, whichever comes first.
type BalanceSyncWorker struct {
	logger         libLog.Logger
	idleWait       time.Duration
	syncConfig     BalanceSyncConfig
	useCase        *command.UseCase
	metricsFactory *metrics.MetricsFactory
	mtEnabled      bool
	tenantCache    *tenantcache.TenantCache
	pgResolver     tenantPGResolver
	serviceName    string
	// collectorsMu guards collectors and evictions. The reconcile loop, the
	// tenant-eviction callback and the ownership checker all reach them from
	// different goroutines.
	collectorsMu sync.Mutex
	collectors   map[string]*tenantCollector
	// evictions counts StopTenantCollector calls per tenant, including those that
	// found no collector. reconcileCollectors samples it before spawning and
	// re-checks it before registering, so an eviction landing mid-spawn still wins.
	evictions map[string]uint64
	// flushBatchFn is the flush body the collectors call. It defaults to
	// flushBatch; tests replace it to observe the context a flush receives.
	flushBatchFn func(ctx context.Context, keys []redisTransaction.SyncKey) bool
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

// maxBatchSize is the upper bound for batch size, derived from PostgreSQL's 65535
// bind-parameter limit. Each balance uses 5 params + 3 shared = (65535-3)/5 = 13106.
// The 5th parameter is overdraft_used (added once the cache script began
// tracking it). Batches above a few hundred rarely make sense for this workload.
const maxBatchSize = 13000

func NewBalanceSyncWorker(logger libLog.Logger, useCase *command.UseCase, syncCfg BalanceSyncConfig) *BalanceSyncWorker {
	// Apply safe defaults for zero-value config (e.g., in tests)
	if syncCfg.BatchSize <= 0 {
		syncCfg.BatchSize = 50
	}

	if syncCfg.BatchSize > maxBatchSize {
		syncCfg.BatchSize = maxBatchSize
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

	w := &BalanceSyncWorker{
		logger:     logger,
		idleWait:   idleWait,
		syncConfig: syncCfg,
		useCase:    useCase,
		collectors: make(map[string]*tenantCollector),
		evictions:  make(map[string]uint64),
	}
	w.flushBatchFn = w.flushBatch

	return w
}

// NewBalanceSyncWorkerMT creates a BalanceSyncWorker with MT (multi-tenant) fields populated.
// When mtEnabled is true, both tenantCache and pgManager must be non-nil for the worker
// to be considered ready (isMTReady). The worker reads tenant IDs from the shared
// TenantCache (populated by the TenantEventListener) and uses pgManager to resolve per-tenant
// PostgreSQL connections.
// serviceName is the service identifier for logging purposes.
func NewBalanceSyncWorkerMT(
	logger libLog.Logger,
	useCase *command.UseCase,
	syncCfg BalanceSyncConfig,
	mtEnabled bool,
	cache *tenantcache.TenantCache,
	pgManager *tmpostgres.Manager,
	serviceName string,
) *BalanceSyncWorker {
	w := NewBalanceSyncWorker(logger, useCase, syncCfg)
	w.mtEnabled = mtEnabled
	w.tenantCache = cache
	w.serviceName = serviceName

	// Guard against a typed-nil pointer being boxed into a non-nil interface,
	// which would make isMTReady report ready with no resolver behind it.
	if pgManager != nil {
		w.pgResolver = pgManager
	}

	return w
}

// WithMetricsFactory sets the metrics factory used to emit balance-sync worker
// metrics (e.g. tenant skips). A nil factory disables metric emission. Returns
// the receiver for fluent wiring at bootstrap.
func (w *BalanceSyncWorker) WithMetricsFactory(factory *metrics.MetricsFactory) *BalanceSyncWorker {
	w.metricsFactory = factory

	return w
}

// emitTenantSkip increments the tenant-skip counter, labelled by tenant. The
// tenant set is bounded by deployment so the label cardinality is safe (T11).
// Best-effort: a nil factory or emit error never affects the worker (Debug log).
func (w *BalanceSyncWorker) emitTenantSkip(ctx context.Context, tenantID string) {
	if w.metricsFactory == nil {
		return
	}

	counter, err := w.metricsFactory.Counter(utils.BalanceSyncTenantSkip)
	if err != nil {
		w.logger.Log(ctx, libLog.LevelDebug, "Failed to create tenant skip counter", libLog.Err(err))

		return
	}

	if addErr := counter.WithLabels(map[string]string{"tenant_id": tenantID}).AddOne(ctx); addErr != nil {
		w.logger.Log(ctx, libLog.LevelDebug, "Failed to emit tenant skip counter", libLog.Err(addErr))
	}
}

// isMTReady returns true when the worker is configured for MT (multi-tenant)
// dispatching. mtEnabled, pgResolver, and tenantCache must all be set;
// if any is missing the worker falls back to default (single-tenant) behavior.
func (w *BalanceSyncWorker) isMTReady() bool {
	return w.mtEnabled && w.pgResolver != nil && w.tenantCache != nil
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

	w.logger.Log(
		ctx, libLog.LevelInfo, "BalanceSyncWorker started (single-tenant, dual-trigger)",
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

	collector.Run(
		ctx,
		// FlushFunc: batch flush grouped by org/ledger, then persisted to PostgreSQL
		func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
			return w.flushBatchFn(flushCtx, keys)
		},
		// FetchKeysFunc: claims due keys from the ZSET via Lua (ZRANGEBYSCORE + SET NX)
		func(fetchCtx context.Context, limit int64) ([]redisTransaction.SyncKey, error) {
			return w.useCase.TransactionRedisRepo.GetBalanceSyncKeys(fetchCtx, limit)
		},
		// WaitForNextFunc: fixed backoff when idle (ZSET empty)
		func(waitCtx context.Context) bool {
			return waitOrDone(waitCtx, w.idleWait)
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

	w.logger.Log(
		ctx, libLog.LevelInfo, "BalanceSyncWorker started (multi-tenant, dual-trigger)",
		libLog.Int("batch_size", w.syncConfig.BatchSize),
		libLog.Int("flush_timeout_ms", w.syncConfig.FlushTimeoutMs),
		libLog.Int("poll_interval_ms", w.syncConfig.PollIntervalMs),
		libLog.String("reconcile_interval", tenantReconcileInterval.String()),
	)

	defer w.stopAllCollectors(ctx)

	// Reconcile immediately on startup so collectors start without waiting for the
	// first tick (10s). On the first call the map is empty, so phases 1-2 are no-ops
	// and phase 3 launches a collector for every tenant in the cache.
	w.reconcileCollectors(ctx)

	ticker := time.NewTicker(tenantReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: shutting down...")

			return nil
		case <-ticker.C:
			w.reconcileCollectors(ctx)
		}
	}
}

// reconcileCollectors synchronizes the set of running collectors with the active tenants
// in the TenantCache. It has three phases:
//  1. Reap: detect collectors whose goroutines exited unexpectedly (e.g., panic)
//  2. Stop: cancel collectors for tenants no longer in the cache
//  3. Start: launch collectors for newly discovered tenants
func (w *BalanceSyncWorker) reconcileCollectors(ctx context.Context) {
	tenantIDs := w.tenantCache.TenantIDs()

	activeSet := make(map[string]struct{}, len(tenantIDs))
	for _, id := range tenantIDs {
		activeSet[id] = struct{}{}
	}

	var (
		removed []*tenantCollector
		missing []string
	)

	w.collectorsMu.Lock()

	// Phase 1: Reap dead collectors (goroutine exited unexpectedly).
	// Removing them from the map allows Phase 3 to restart them if the tenant is still active.
	for id, tc := range w.collectors {
		select {
		case <-tc.done:
			w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: collector exited unexpectedly, will restart",
				libLog.String("tenant_id", id))

			delete(w.collectors, id)
		default:
		}
	}

	// Phase 2: Cancel collectors for removed tenants (non-blocking cancel, deferred wait).
	for id, tc := range w.collectors {
		if _, ok := activeSet[id]; !ok {
			w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: tenant removed from cache, stopping collector",
				libLog.String("tenant_id", id))

			tc.cancel()

			removed = append(removed, tc)

			delete(w.collectors, id)
		}
	}

	// Sampled under the lock alongside the membership check so the pair is
	// consistent with the map state we based the spawn decision on.
	spawnGen := make(map[string]uint64, len(tenantIDs))

	for _, id := range tenantIDs {
		if _, ok := w.collectors[id]; !ok {
			missing = append(missing, id)
			spawnGen[id] = w.evictions[id]
		}
	}

	w.collectorsMu.Unlock()

	// Phase 3: Start collectors for new tenants. Spawning happens outside the lock
	// because the PG preflight reaches the network, and StopTenantCollector runs on
	// the tenant-event goroutine.
	for _, id := range missing {
		tc := w.startTenantCollector(ctx, id)
		if tc == nil {
			continue
		}

		w.collectorsMu.Lock()

		_, taken := w.collectors[id]
		evicted := w.evictions[id] != spawnGen[id]

		if taken || evicted {
			w.collectorsMu.Unlock()

			// Either another goroutine registered a collector for this tenant while we
			// were spawning, or an eviction landed in that window — registering now
			// would resurrect a collector the eviction is about to close the pool
			// under. Drop ours; its buffer is still empty, so the final flush is a
			// no-op.
			w.logger.Log(ctx, libLog.LevelDebug, "BalanceSyncWorker: discarding collector that lost the registration race",
				libLog.String("tenant_id", id), libLog.Bool("evicted", evicted))

			tc.cancel()
			<-tc.done

			continue
		}

		w.collectors[id] = tc

		w.collectorsMu.Unlock()
	}

	// Phase 4: Wait for removed collectors to finish (all cancellations already sent in Phase 2).
	for _, tc := range removed {
		<-tc.done

		w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: collector stopped",
			libLog.String("tenant_id", tc.tenantID))
	}

	if len(tenantIDs) == 0 {
		w.logger.Log(ctx, libLog.LevelDebug, "BalanceSyncWorker: no tenants in cache, will retry on next reconciliation")
	}
}

// startTenantCollector checks that the tenant's PostgreSQL is resolvable, creates
// a BalanceSyncCollector with tenant-scoped fetch/flush functions, and launches it
// as a goroutine. Returns nil if the tenant's PG connection cannot be established.
// The collector context carries only the tenant ID; the PG handle is resolved per
// flush (see resolveTenantPG).
func (w *BalanceSyncWorker) startTenantCollector(parentCtx context.Context, tenantID string) *tenantCollector {
	tenantCtx := tmcore.ContextWithTenantID(parentCtx, tenantID)

	// A shutdown racing the reconcile loop must not pay for a PG round-trip, nor
	// report the tenant as skipped for a reason the operator cannot act on.
	if err := parentCtx.Err(); err != nil {
		w.logger.Log(parentCtx, libLog.LevelDebug, "BalanceSyncWorker: context done before collector start",
			libLog.String("tenant_id", tenantID))

		return nil
	}

	// Preflight: a tenant whose PG cannot be resolved gets no collector at all.
	if _, err := w.pgResolver.GetDB(tenantCtx, tenantID); err != nil {
		w.logger.Log(parentCtx, libLog.LevelError, "BalanceSyncWorker: failed to get PG connection for tenant",
			libLog.String("tenant_id", tenantID), libLog.Err(err))

		w.emitTenantSkip(parentCtx, tenantID)

		return nil
	}

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
				w.logger.Log(parentCtx, libLog.LevelError, "BalanceSyncWorker: collector panicked",
					libLog.String("tenant_id", tenantID), libLog.String("panic", fmt.Sprint(r)))
			}
		}()

		w.logger.Log(collectorCtx, libLog.LevelInfo, "BalanceSyncWorker: collector started",
			libLog.String("tenant_id", tenantID))

		collector.Run(
			collectorCtx,
			// FlushFunc: resolve this tenant's PG handle, then batch flush grouped by org/ledger
			func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
				pgCtx, err := w.resolveTenantPG(flushCtx, tenantID)
				if err != nil {
					// Shutdown is not a degraded tenant: counting it would add one bogus
					// skip per tenant to the metric on every graceful stop. The drain
					// flush runs with cancellation stripped, so it never lands here.
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						w.logger.Log(flushCtx, libLog.LevelDebug,
							"BalanceSyncWorker: flush context done before PG resolution",
							libLog.String("tenant_id", tenantID))

						return false
					}

					w.logger.Log(flushCtx, libLog.LevelWarn,
						"BalanceSyncWorker: failed to resolve tenant PG for flush, skipping batch",
						libLog.String("tenant_id", tenantID), libLog.Err(err))

					w.emitTenantSkip(flushCtx, tenantID)

					return false
				}

				return w.flushBatchFn(pgCtx, keys)
			},
			// FetchKeysFunc: tenant context enables Redis key namespacing via tmvalkey.GetKeyContext
			func(fetchCtx context.Context, limit int64) ([]redisTransaction.SyncKey, error) {
				return w.useCase.TransactionRedisRepo.GetBalanceSyncKeys(fetchCtx, limit)
			},
			// WaitForNextFunc: fixed backoff when idle (ZSET empty for this tenant)
			func(waitCtx context.Context) bool {
				return waitOrDone(waitCtx, w.idleWait)
			},
		)

		w.logger.Log(parentCtx, libLog.LevelInfo, "BalanceSyncWorker: collector stopped",
			libLog.String("tenant_id", tenantID))
	}()

	return &tenantCollector{
		tenantID: tenantID,
		cancel:   cancel,
		done:     done,
	}
}

// resolveTenantPG returns ctx carrying a freshly resolved transaction-module PG
// handle for tenantID. Resolution is per call: the tenant manager may close or
// swap a tenant's pool at any time, so a handle captured once would dangle.
// The worker only touches the transaction database, so only the module-specific
// context entry is set — getDB looks that one up first.
func (w *BalanceSyncWorker) resolveTenantPG(ctx context.Context, tenantID string) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := w.pgResolver.GetDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return tmcore.ContextWithPG(ctx, db, constant.ModuleTransaction), nil
}

// collectorDrainDeadline bounds how long StopTenantCollector waits for a cancelled
// collector. The collector's own flushRemaining budget (shutdownFlushTimeout) starts
// only once it observes the cancel, so a deadline equal to that budget could fire
// while a legitimate final flush was still inside it — and the caller closes the
// tenant's pool as soon as we return. The margin puts this deadline strictly after
// the collector's, so we only abandon a collector that blew its own.
const collectorDrainDeadline = shutdownFlushTimeout + 10*time.Second

// StopTenantCollector cancels the collector for tenantID, waits for its final flush
// to drain, and forgets it. Call it BEFORE closing the tenant's PostgreSQL pool so
// that flush lands in a live pool. A tenant with no collector is a no-op. The next
// reconcile restarts the collector when the tenant is still in the TenantCache.
func (w *BalanceSyncWorker) StopTenantCollector(tenantID string) {
	w.collectorsMu.Lock()

	if w.evictions == nil {
		w.evictions = make(map[string]uint64)
	}

	// Counted even when no collector is present: that is precisely the case where a
	// reconcile is mid-spawn and must not register its collector afterwards.
	w.evictions[tenantID]++

	tc, ok := w.collectors[tenantID]
	if ok {
		delete(w.collectors, tenantID)
	}

	w.collectorsMu.Unlock()

	if !ok {
		return
	}

	ctx := context.Background()

	w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: tenant evicted, stopping collector",
		libLog.String("tenant_id", tenantID))

	tc.cancel()

	timer := time.NewTimer(collectorDrainDeadline)
	defer timer.Stop()

	select {
	case <-tc.done:
		w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: collector stopped",
			libLog.String("tenant_id", tenantID))
	case <-timer.C:
		w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: collector did not drain before deadline",
			libLog.String("tenant_id", tenantID),
			libLog.String("deadline", collectorDrainDeadline.String()))
	}
}

// HasCollector reports whether a collector is running for tenantID. The tenant
// ownership checker reads it so an eviction still sees midaz as owning a tenant
// whose cache entry has TTL-expired while its collector runs.
func (w *BalanceSyncWorker) HasCollector(tenantID string) bool {
	w.collectorsMu.Lock()
	defer w.collectorsMu.Unlock()

	_, ok := w.collectors[tenantID]

	return ok
}

// stopAllCollectors cancels all running tenant collectors and waits for them to finish.
// Each collector's deferred flushRemaining runs on cancellation, draining any buffered keys.
func (w *BalanceSyncWorker) stopAllCollectors(ctx context.Context) {
	w.collectorsMu.Lock()

	running := make(map[string]*tenantCollector, len(w.collectors))
	for id, tc := range w.collectors {
		running[id] = tc

		delete(w.collectors, id)
	}

	w.collectorsMu.Unlock()

	if len(running) == 0 {
		return
	}

	w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: stopping all tenant collectors",
		libLog.Int("count", len(running)))

	// Cancel all collectors concurrently
	for _, tc := range running {
		tc.cancel()
	}

	// Wait for all to finish (each flushes its remaining buffer)
	for id, tc := range running {
		<-tc.done

		w.logger.Log(ctx, libLog.LevelInfo, "BalanceSyncWorker: collector stopped",
			libLog.String("tenant_id", id))
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

	groups := w.groupKeysByOrgLedger(ctx, keys)
	processed := false

	for _, group := range groups {
		if w.processSyncBatch(ctx, group.orgID, group.ledgerID, group.keys) {
			processed = true
		}
	}

	return processed
}

// groupKeysByOrgLedger groups Redis balance keys by their (organizationID, ledgerID) pair.
// Grouping is necessary because SyncBalancesBatch operates within a single org+ledger scope:
// the PostgreSQL query filters by (organization_id, ledger_id), and the aggregation engine
// deduplicates by composite key within that scope. A mixed batch would either require
// multiple DB queries or risk incorrect deduplication across ledgers.
// In practice, single-tenant and MT modes almost always produce a single group (one
// collector per tenant), but the grouping protects against edge cases in key namespacing.
// Keys that cannot be parsed are logged, removed from the ZSET (poison record cleanup),
// and skipped.
func (w *BalanceSyncWorker) groupKeysByOrgLedger(ctx context.Context, keys []redisTransaction.SyncKey) []orgLedgerGroup {
	type groupKey struct {
		orgID    uuid.UUID
		ledgerID uuid.UUID
	}

	grouped := make(map[groupKey][]redisTransaction.SyncKey, 1) // typically 1 group in single-tenant

	for _, key := range keys {
		orgID, ledgerID, err := w.extractIDsFromMember(key.Key)
		if err != nil {
			w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: failed to extract IDs from key, removing from schedule",
				libLog.String("key", key.Key), libLog.Err(err))

			// Clean up the claimed entry to prevent it from becoming a poison record.
			// Uses the batch variant with a single element so the conditional ZREM
			// and lock cleanup run through the same Lua script path.
			if _, remErr := w.useCase.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, []redisTransaction.SyncKey{key}); remErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: failed to remove unparseable key",
					libLog.String("key", key.Key), libLog.Err(remErr))
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

// processSyncBatch delegates to SyncBalancesBatch and emits metrics.
// The use case handles the full pipeline: MGET → aggregate → persist → conditional ZREM.
// Returns true if any balances were synced or aggregated.
const syncBatchTimeout = 5 * time.Minute

func (w *BalanceSyncWorker) processSyncBatch(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []redisTransaction.SyncKey) bool {
	if len(keys) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, syncBatchTimeout)
	defer cancel()

	_, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "balance.worker.process_batch")
	defer span.End()

	// Empty in single-tenant. Prometheus treats an empty label as absent, so the
	// existing single-tenant series keep their identity.
	tenantID := tmcore.GetTenantIDContext(ctx)

	result, err := w.useCase.SyncBalancesBatch(ctx, organizationID, ledgerID, keys)
	if err != nil {
		// The worker's span carries no scope IDs, so a stuck tenant is not
		// locatable from the log line without them.
		w.logger.Log(ctx, libLog.LevelError, "BalanceSyncWorker: batch sync failed",
			libLog.String("tenant_id", tenantID),
			libLog.String("organization_id", organizationID.String()),
			libLog.String("ledger_id", ledgerID.String()),
			libLog.Err(err))

		// Emit failure metric for monitoring
		counter, counterErr := metricFactory.Counter(utils.BalanceSyncBatchFailures)
		if counterErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: failed to create failure counter", libLog.Err(counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
				"tenant_id":       tenantID,
			}).AddOne(ctx); metricErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: failed to emit failure counter", libLog.Err(metricErr))
			}
		}

		return false
	}

	if result.BalancesSynced > 0 {
		counter, counterErr := metricFactory.Counter(utils.BalanceSynced)
		if counterErr != nil {
			w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: failed to create synced counter", libLog.Err(counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
				"mode":            "batch",
			}).Add(ctx, result.BalancesSynced); metricErr != nil {
				w.logger.Log(ctx, libLog.LevelWarn, "BalanceSyncWorker: failed to emit synced counter", libLog.Err(metricErr))
			}
		}
	}

	if result.KeysProcessed > 0 {
		w.logger.Log(
			ctx, libLog.LevelInfo, "BalanceSyncWorker: batch sync completed",
			libLog.Int("processed", result.KeysProcessed),
			libLog.Int("aggregated", result.BalancesAggregated),
			libLog.Int("synced", int(result.BalancesSynced)),
			libLog.Int("removed", int(result.KeysRemoved)),
		)
	}

	return result.BalancesSynced > 0 || result.BalancesAggregated > 0 || result.KeysRemoved > 0
}

// waitOrDone sleeps for duration d and returns false, or returns true immediately
// if the context is cancelled during the wait (shutdown requested). Used both as
// the WaitForNextFunc callback (idle backoff between polls) and as error backoff
// in the collector's fetch loop. A duration <= 0 returns false without sleeping.
func waitOrDone(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return false
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

// extractIDsFromMember parses the organizationID and ledgerID from a Redis balance key.
// Key pattern: balance:{transactions}:<orgID>:<ledgerID>:<alias>#<balanceKey>
//
// Instead of strings.Split (which allocates a []string), the parser scans byte-by-byte
// looking for colon-separated segments of exactly 36 characters (UUID string length).
// The first valid UUID found is orgID, the second is ledgerID. This approach is
// allocation-free and position-independent — it works even if the key format gains
// additional prefixes or suffixes.
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

// LegacyBalanceSyncDrainer drains entries from the legacy ZSET (balance-sync, pre-v3.6.2).
// It reuses the same flush pipeline as the main worker but reads from the legacy key
// with a longer idle wait. Once the legacy ZSET is fully drained, the drainer idles.
type LegacyBalanceSyncDrainer struct {
	logger   libLog.Logger
	idleWait time.Duration
	useCase  *command.UseCase
	syncCfg  BalanceSyncConfig
}

// NewLegacyBalanceSyncDrainer creates a drainer for the pre-v3.6.2 balance-sync ZSET.
func NewLegacyBalanceSyncDrainer(logger libLog.Logger, useCase *command.UseCase, syncCfg BalanceSyncConfig) *LegacyBalanceSyncDrainer {
	if syncCfg.BatchSize <= 0 {
		syncCfg.BatchSize = 50
	}

	if syncCfg.FlushTimeoutMs <= 0 {
		syncCfg.FlushTimeoutMs = 2000
	}

	if syncCfg.PollIntervalMs <= 0 {
		syncCfg.PollIntervalMs = 1000
	}

	return &LegacyBalanceSyncDrainer{
		logger:   logger,
		idleWait: 30 * time.Second, // long backoff — legacy ZSET drains to zero
		useCase:  useCase,
		syncCfg:  syncCfg,
	}
}

// Run implements the Launcher interface for the legacy drainer.
func (d *LegacyBalanceSyncDrainer) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d.logger.Log(ctx, libLog.LevelInfo, "LegacyBalanceSyncDrainer started — draining balance-sync (pre-v3.6.2)")

	// Reuse the same collector/flush infrastructure as the main worker,
	// but fetch from the legacy ZSET key via GetBalanceSyncKeysLegacy.
	worker := NewBalanceSyncWorker(d.logger, d.useCase, d.syncCfg)
	worker.idleWait = d.idleWait

	collector := NewBalanceSyncCollector(
		d.syncCfg.BatchSize,
		d.syncCfg.FlushTimeout(),
		d.syncCfg.PollInterval(),
		d.logger,
	)

	collector.Run(
		ctx,
		func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
			return worker.flushBatch(flushCtx, keys)
		},
		func(fetchCtx context.Context, limit int64) ([]redisTransaction.SyncKey, error) {
			return d.useCase.TransactionRedisRepo.GetBalanceSyncKeysLegacy(fetchCtx, limit)
		},
		func(waitCtx context.Context) bool {
			return waitOrDone(waitCtx, d.idleWait)
		},
	)

	d.logger.Log(ctx, libLog.LevelInfo, "LegacyBalanceSyncDrainer: shutting down...")

	return nil
}
