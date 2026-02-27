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

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// BalanceSyncWorker continuously processes keys scheduled for pre-expiry actions.
// Ensures that the balance is synced before the key expires.
type BalanceSyncWorker struct {
	redisConn          *libRedis.RedisConnection
	logger             libLog.Logger
	idleWait           time.Duration
	batchSize          int64
	maxWorkers         int
	useCase            *command.UseCase
	multiTenantEnabled bool
	tenantClient       *tmclient.Client
	pgManager          *tmpostgres.Manager
}

func NewBalanceSyncWorker(conn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase, maxWorkers int) *BalanceSyncWorker {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	return &BalanceSyncWorker{
		redisConn:  conn,
		logger:     logger,
		idleWait:   600 * time.Second,
		batchSize:  int64(maxWorkers),
		maxWorkers: maxWorkers,
		useCase:    useCase,
	}
}

// NewBalanceSyncWorkerMultiTenant creates a BalanceSyncWorker with multi-tenant fields populated.
// When multiTenantEnabled is true and pgManager is non-nil, the worker will iterate
// over active tenants and resolve per-tenant PostgreSQL connections.
func NewBalanceSyncWorkerMultiTenant(
	conn *libRedis.RedisConnection,
	logger libLog.Logger,
	useCase *command.UseCase,
	maxWorkers int,
	multiTenantEnabled bool,
	tenantClient *tmclient.Client,
	pgManager *tmpostgres.Manager,
) *BalanceSyncWorker {
	w := NewBalanceSyncWorker(conn, logger, useCase, maxWorkers)
	w.multiTenantEnabled = multiTenantEnabled
	w.tenantClient = tenantClient
	w.pgManager = pgManager

	return w
}

// isMultiTenantReady returns true when the worker is configured for multi-tenant
// dispatching. multiTenantEnabled, pgManager, and tenantClient must all be set;
// if any is missing the worker falls back to single-tenant behavior.
func (w *BalanceSyncWorker) isMultiTenantReady() bool {
	return w.multiTenantEnabled && w.pgManager != nil && w.tenantClient != nil
}

// Run dispatches to multi-tenant or single-tenant execution based on configuration.
func (w *BalanceSyncWorker) Run(_ *libCommons.Launcher) error {
	if w.isMultiTenantReady() {
		return w.runMultiTenant()
	}

	return w.runSingleTenant()
}

// runSingleTenant runs the balance sync loop using the default (shared) database connection.
// This is the original Run() behavior preserved for backward compatibility.
func (w *BalanceSyncWorker) runSingleTenant() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Info("BalanceSyncWorker started (single-tenant mode)")

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: failed to get redis client: %v", err)

		return err
	}

	for {
		if w.shouldShutdown(ctx) {
			w.logger.Info("BalanceSyncWorker: shutting down...")

			return nil
		}

		if w.processBalancesToExpire(ctx, rds) {
			continue
		}

		if w.waitForNextOrBackoff(ctx, rds) {
			w.logger.Info("BalanceSyncWorker: shutting down...")

			return nil
		}
	}
}

// runMultiTenant runs the balance sync loop iterating over all active tenants.
// For each tenant, it resolves a per-tenant PostgreSQL connection and injects it
// into the context before processing. If a tenant's connection fails, it logs and
// skips that tenant without affecting others.
func (w *BalanceSyncWorker) runMultiTenant() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Info("BalanceSyncWorker started (multi-tenant mode)")

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: failed to get redis client: %v", err)

		return err
	}

	for {
		if w.shouldShutdown(ctx) {
			w.logger.Info("BalanceSyncWorker: shutting down...")

			return nil
		}

		tenants, ok := w.discoverActiveTenants(ctx, rds)
		if !ok {
			continue
		}

		if tenants == nil {
			return nil
		}

		processed := false

		for _, tenant := range tenants {
			if w.shouldShutdown(ctx) {
				w.logger.Info("BalanceSyncWorker: shutting down...")

				return nil
			}

			if w.processTenantBalances(ctx, tenant, rds) {
				processed = true
			}
		}

		if !processed {
			if w.waitForNextOrBackoff(ctx, rds) {
				w.logger.Info("BalanceSyncWorker: shutting down...")

				return nil
			}
		}
	}
}

// discoverActiveTenants retrieves the list of active tenants for the transaction service.
// Returns (tenants, true) on success, (nil, false) if an error or empty result requires
// backing off and retrying, or (nil, true) if shutdown was requested during backoff.
func (w *BalanceSyncWorker) discoverActiveTenants(ctx context.Context, rds redis.UniversalClient) ([]*tmclient.TenantSummary, bool) {
	tenants, err := w.tenantClient.GetActiveTenantsByService(ctx, "transaction")
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: failed to get active tenants: %v", err)

		if w.waitForNextOrBackoff(ctx, rds) {
			w.logger.Info("BalanceSyncWorker: shutting down...")

			return nil, true
		}

		return nil, false
	}

	if len(tenants) == 0 {
		w.logger.Info("BalanceSyncWorker: no active tenants found, backing off")

		if w.waitForNextOrBackoff(ctx, rds) {
			w.logger.Info("BalanceSyncWorker: shutting down...")

			return nil, true
		}

		return nil, false
	}

	return tenants, true
}

// processTenantBalances resolves the per-tenant PostgreSQL connection, augments the context
// with the tenant ID and module connection, then processes expired balances for that tenant.
// Returns true if any balances were processed.
func (w *BalanceSyncWorker) processTenantBalances(ctx context.Context, tenant *tmclient.TenantSummary, rds redis.UniversalClient) bool {
	tenantCtx := tmcore.ContextWithTenantID(ctx, tenant.ID)

	conn, err := w.pgManager.GetConnection(tenantCtx, tenant.ID)
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: failed to get PG connection for tenant %s: %v", tenant.ID, err)

		return false
	}

	db, err := conn.GetDB()
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: failed to get DB for tenant %s: %v", tenant.ID, err)

		return false
	}

	tenantCtx = tmcore.ContextWithModulePGConnection(tenantCtx, "transaction", db)

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

func (w *BalanceSyncWorker) processBalancesToExpire(ctx context.Context, rds redis.UniversalClient) bool {
	members, err := w.useCase.RedisRepo.GetBalanceSyncKeys(ctx, w.batchSize)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			w.logger.Warnf("BalanceSyncWorker: get balance sync keys error: %v", err)
		}

		return false
	}

	if len(members) == 0 {
		return false
	}

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
			w.logger.Info("BalanceSyncWorker: shutting down...")

			return true
		}

		member := m

		sem <- struct{}{}

		wg.Add(1)

		go func(member string) {
			defer func() { <-sem }()

			defer wg.Done()

			defer func() {
				if r := recover(); r != nil {
					w.logger.Errorf("BalanceSyncWorker: panic recovered while processing %s: %v", member, r)
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

// waitForNextOrBackoff waits based on the next schedule entry or backs off if none.
// Returns true if it shut down while waiting.
func (w *BalanceSyncWorker) waitForNextOrBackoff(ctx context.Context, rds redis.UniversalClient) bool {
	next, err := rds.ZRangeWithScores(ctx, utils.BalanceSyncScheduleKey, 0, 0).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		w.logger.Warnf("BalanceSyncWorker: zrangewithscores error: %v", err)

		return waitOrDone(ctx, w.idleWait, w.logger)
	}

	if len(next) == 0 {
		w.logger.Info("BalanceSyncWorker: nothing scheduled; back off.")

		return waitOrDone(ctx, w.idleWait, w.logger)
	}

	w.logger.Infof("BalanceSyncWorker: next: %+v", next[0])

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
		w.logger.Warnf("BalanceSyncWorker: TTL error for %s: %v", member, err)

		return
	}

	// Handle missing key regardless of TTL sentinel representation (-2 or -2s)
	if ttl == -2 || ttl == -2*time.Second {
		w.logger.Warnf("BalanceSyncWorker: already-gone key: %s, removing from schedule", member)

		if remErr := w.useCase.RedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Warnf("BalanceSyncWorker: failed to remove expired balance sync key %s: %v", member, remErr)
		}

		return
	}

	val, err := rds.Get(ctx, member).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			w.logger.Warnf("BalanceSyncWorker: missing key on GET: %s, removing from schedule", member)

			if remErr := w.useCase.RedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
				w.logger.Warnf("BalanceSyncWorker: failed to remove missing balance sync key %s: %v", member, remErr)
			}
		} else {
			w.logger.Warnf("BalanceSyncWorker: GET error for %s: %v", member, err)
		}

		return
	}

	organizationID, ledgerID, err := w.extractIDsFromMember(member)
	if err != nil {
		w.logger.Warnf("BalanceSyncWorker: extractIDsFromMember error for %s: %v", member, err)

		if remErr := w.useCase.RedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Warnf("BalanceSyncWorker: failed to remove unparsable balance sync key %s: %v", member, remErr)
		}

		return
	}

	var balance mmodel.BalanceRedis
	if err := json.Unmarshal([]byte(val), &balance); err != nil {
		w.logger.Warnf("BalanceSyncWorker: Unmarshal error for %s: %v", member, err)

		if remErr := w.useCase.RedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Warnf("BalanceSyncWorker: failed to remove unmarshalable balance sync key %s: %v", member, remErr)
		}

		return
	}

	synced, err := w.useCase.SyncBalance(ctx, organizationID, ledgerID, balance)
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: SyncBalance error for member %s with content %+v: %v", member, balance, err)

		return
	}

	if synced {
		metricFactory.Counter(utils.BalanceSynced).WithLabels(map[string]string{
			"organization_id": organizationID.String(),
			"ledger_id":       ledgerID.String(),
		}).AddOne(ctx)

		w.logger.Infof("BalanceSyncWorker: Synced key %s", member)
	}

	if remErr := w.useCase.RedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
		w.logger.Warnf("BalanceSyncWorker: failed to remove balance sync key %s: %v", member, remErr)
	}
}

// waitOrDone waits for d or returns true if ctx is done first.
func waitOrDone(ctx context.Context, d time.Duration, logger libLog.Logger) bool {
	if d <= 0 {
		return false
	}

	logger.Infof("BalanceSyncWorker: waiting for %s", d.String())

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

// waitUntilDue waits until the given dueAtUnix time.
// Returns true if the context was cancelled while waiting.
func (w *BalanceSyncWorker) waitUntilDue(ctx context.Context, dueAtUnix int64, logger libLog.Logger) bool {
	nowUnix := time.Now().Unix()
	if dueAtUnix <= nowUnix {
		return false
	}

	waitFor := time.Duration(dueAtUnix-nowUnix) * time.Second
	if waitFor <= 0 {
		return false
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
