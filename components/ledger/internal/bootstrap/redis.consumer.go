// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/LerianStudio/lib-observability/v2/metrics"
	libRuntime "github.com/LerianStudio/lib-observability/v2/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	postgreTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionquarantine"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	CronTimeToRun     = 30 * time.Minute
	MessageTimeOfLife = 30
	MaxWorkers        = 100
	CycleLockTTL      = 1800 // 30 minutes in seconds — matches CronTimeToRun

	// QuarantineThreshold is the number of consecutive consumer cycles a poison
	// record may fail before it is moved to the durable Postgres quarantine
	// table. Below this threshold the record is left in Redis to be retried on
	// the next cycle (a poison record is the only durable copy of an authorized
	// transaction, so it is never deleted or skipped silently forever).
	QuarantineThreshold = 3

	// redisBackupConsumerComponent scopes panic-observability signals (the
	// panic_recovered_total counter, structured logs, span events) emitted by
	// the per-record replay goroutines to this subsystem in dashboards.
	redisBackupConsumerComponent = "ledger.redis-backup-consumer"
)

type RedisQueueConsumer struct {
	Logger             libLog.Logger
	TransactionHandler in.TransactionHandler
	quarantineRepo     transactionquarantine.Repository
	metricsFactory     *metrics.MetricsFactory
	multiTenantEnabled bool
	tenantCache        *tenantcache.TenantCache
	pgManager          *tmpostgres.Manager
}

func NewRedisQueueConsumer(logger libLog.Logger, handler in.TransactionHandler) *RedisQueueConsumer {
	return &RedisQueueConsumer{
		Logger:             logger,
		TransactionHandler: handler,
	}
}

// WithQuarantineRepository sets the durable quarantine repository used to
// persist poison records before they are removed from Redis. It returns the
// receiver for fluent wiring at bootstrap.
func (r *RedisQueueConsumer) WithQuarantineRepository(repo transactionquarantine.Repository) *RedisQueueConsumer {
	r.quarantineRepo = repo

	return r
}

// WithMetricsFactory sets the metrics factory used to emit backup-queue
// observability metrics each cycle. A nil factory disables metric emission.
func (r *RedisQueueConsumer) WithMetricsFactory(factory *metrics.MetricsFactory) *RedisQueueConsumer {
	r.metricsFactory = factory

	return r
}

// NewRedisQueueConsumerMultiTenant creates a RedisQueueConsumer with multi-tenant fields populated.
// When multiTenantEnabled is true, both tenantCache and pgManager must be non-nil for the consumer
// to be considered ready (isMultiTenantReady). The consumer reads tenant IDs from the shared
// TenantCache (populated by the TenantEventListener) and uses pgManager to resolve per-tenant
// PostgreSQL connections.
func NewRedisQueueConsumerMultiTenant(
	logger libLog.Logger,
	handler in.TransactionHandler,
	multiTenantEnabled bool,
	cache *tenantcache.TenantCache,
	pgManager *tmpostgres.Manager,
) *RedisQueueConsumer {
	c := NewRedisQueueConsumer(logger, handler)
	c.multiTenantEnabled = multiTenantEnabled
	c.tenantCache = cache
	c.pgManager = pgManager

	return c
}

// isMultiTenantReady returns true when the consumer is configured for multi-tenant
// dispatching. multiTenantEnabled, pgManager, and tenantCache must all be set;
// if any is missing the consumer falls back to single-tenant behavior.
func (r *RedisQueueConsumer) isMultiTenantReady() bool {
	return r.multiTenantEnabled && r.pgManager != nil && r.tenantCache != nil
}

// Run dispatches to multi-tenant or single-tenant execution based on configuration.
func (r *RedisQueueConsumer) Run(_ *libCommons.Launcher) error {
	if r.isMultiTenantReady() {
		return r.runMultiTenant()
	}

	return r.runSingleTenant()
}

// runSingleTenant runs the Redis queue consumer using the default (shared) database connection.
// This is the original Run() behavior preserved for backward compatibility.
func (r *RedisQueueConsumer) runSingleTenant() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(CronTimeToRun)
	defer ticker.Stop()

	r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer started (single-tenant mode)")

	for {
		select {
		case <-ctx.Done():
			r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer: shutting down...")
			return nil

		case <-ticker.C:
			r.executeCycle(ctx)
		}
	}
}

// executeCycle acquires the cycle-level distributed lock and, if this pod is the
// leader, processes all backup queue messages. Extracted from the ticker case to
// scope the defer-based lock release correctly (defer inside a for-select does not
// run at the end of each iteration).
func (r *RedisQueueConsumer) executeCycle(ctx context.Context) {
	acquired, release := r.acquireCycleLock(ctx)
	if !acquired {
		return
	}

	defer release()

	r.readMessagesAndProcess(ctx)
}

// runMultiTenant runs the Redis queue consumer iterating over all active tenants.
// On each tick, it discovers active tenants and processes messages for each tenant
// with an enriched context containing the per-tenant PostgreSQL connection.
// If a tenant's connection fails, it logs and skips that tenant without affecting others.
func (r *RedisQueueConsumer) runMultiTenant() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(CronTimeToRun)
	defer ticker.Stop()

	r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer started (multi-tenant mode)")

	for {
		select {
		case <-ctx.Done():
			r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer: shutting down...")
			return nil

		case <-ticker.C:
			r.executeMultiTenantCycle(ctx)
		}
	}
}

// executeMultiTenantCycle acquires the cycle-level distributed lock and, if this
// pod is the leader, processes backup queue messages for every active tenant.
// Extracted from the ticker case to scope the defer-based lock release correctly.
func (r *RedisQueueConsumer) executeMultiTenantCycle(ctx context.Context) {
	acquired, release := r.acquireCycleLock(ctx)
	if !acquired {
		return
	}

	defer release()

	tenantIDs := r.tenantCache.TenantIDs()
	if len(tenantIDs) == 0 {
		r.Logger.Log(ctx, libLog.LevelDebug, "RedisQueueConsumer: no tenants in cache, skipping cycle")

		return
	}

	for _, tenantID := range tenantIDs {
		if ctx.Err() != nil {
			r.Logger.Log(ctx, libLog.LevelDebug, "RedisQueueConsumer: context cancelled, stopping tenant iteration")

			return
		}

		tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)

		conn, err := r.pgManager.GetConnection(tenantCtx, tenantID)
		if err != nil {
			r.Logger.Log(ctx, libLog.LevelError, "RedisQueueConsumer: failed to get PG connection for tenant", libLog.String("tenant_id", tenantID), libLog.Err(err))

			continue
		}

		db, err := conn.GetDB()
		if err != nil {
			r.Logger.Log(ctx, libLog.LevelError, "RedisQueueConsumer: failed to get DB for tenant", libLog.String("tenant_id", tenantID), libLog.Err(err))

			continue
		}

		tenantCtx = tmcore.ContextWithPG(tenantCtx, db)
		tenantCtx = tmcore.ContextWithPG(tenantCtx, db, constant.ModuleTransaction)

		r.readMessagesAndProcess(tenantCtx)
	}
}

// acquireCycleLock attempts to acquire the cycle-level distributed lock via SetNX.
// Returns (true, releaseFunc) if the lock was acquired, or (false, nil) if another
// pod already holds it or an error occurred.
// The release function deletes the lock key; callers should defer it.
func (r *RedisQueueConsumer) acquireCycleLock(ctx context.Context) (bool, func()) {
	cycleLockKey := utils.RedisConsumerCycleLockKey()
	podID := podIdentifier()

	success, err := r.TransactionHandler.Command.TransactionRedisRepo.SetNX(ctx, cycleLockKey, podID, CycleLockTTL)
	if err != nil {
		r.Logger.Log(ctx, libLog.LevelWarn, "Failed to acquire backup consumer cycle lock", libLog.Err(err))

		return false, nil
	}

	if !success {
		r.Logger.Log(ctx, libLog.LevelDebug, "Another pod holds the backup consumer lock, skipping cycle")

		return false, nil
	}

	r.Logger.Log(ctx, libLog.LevelDebug, "Cycle lock acquired", libLog.String("pod_id", podID))

	release := func() {
		if delErr := r.TransactionHandler.Command.TransactionRedisRepo.Del(ctx, cycleLockKey); delErr != nil {
			r.Logger.Log(ctx, libLog.LevelWarn, "Failed to release backup consumer cycle lock", libLog.Err(delErr))
		}
	}

	return true, release
}

// podIdentifier returns a stable identifier for the current pod, used as the
// value stored in the cycle lock for debugging and observability.
func podIdentifier() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}

	return hostname
}

func (r *RedisQueueConsumer) readMessagesAndProcess(ctx context.Context) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.consumer.read_messages_from_queue")
	defer span.End()

	r.Logger.Log(ctx, libLog.LevelDebug, "Init cron to read messages from redis...")

	messages, err := r.TransactionHandler.Command.TransactionRedisRepo.ReadAllMessagesFromQueue(ctx)
	if err != nil {
		r.Logger.Log(ctx, libLog.LevelError, "Failed to read messages from redis", libLog.Err(err))
		return
	}

	r.Logger.Log(ctx, libLog.LevelDebug, "Read messages from queue", libLog.Int("message_count", len(messages)))

	// Emit the queue-depth gauge once per cycle (best-effort, nil-factory safe).
	// Computed from the records already in hand so no extra Redis round-trip is
	// needed. Depth is reported even on an empty cycle.
	r.emitDepthGauge(ctx, int64(len(messages)))

	if len(messages) == 0 {
		return
	}

	sem := make(chan struct{}, MaxWorkers)

	var wg sync.WaitGroup

	totalMessagesLessThanOneHour := 0

	// oldestTTL tracks the earliest record TTL across successfully-parsed
	// records, computed in the SAME pass that dispatches processing so each
	// record is unmarshalled exactly once per cycle. Records that fail to parse
	// are routed to quarantine and excluded from the age computation.
	var oldestTTL time.Time

Outer:
	for key, message := range messages {
		if ctx.Err() != nil {
			r.Logger.Log(ctx, libLog.LevelWarn, "Shutdown in progress: skipping remaining messages")
			break Outer
		}

		var transaction mmodel.TransactionRedisQueue
		if err := json.Unmarshal([]byte(message), &transaction); err != nil {
			// Unmarshal failure: the payload did not parse, so the org/ledger/tx
			// IDs must come from the field key. The raw string is the financial
			// copy to quarantine.
			r.Logger.Log(ctx, libLog.LevelWarn, "Error unmarshalling message from Redis", libLog.String("key", key), libLog.Err(err))

			orgID, ledgerID, txID, parsed := parsePoisonKeyIDs(key)
			if !parsed {
				r.Logger.Log(ctx, libLog.LevelError, "Unparseable backup record with unparseable key; cannot quarantine, left in backup queue",
					libLog.String("redis_key", key))

				continue
			}

			r.quarantinePoisonRecord(ctx, span, r.Logger, key, orgID, ledgerID, txID, []byte(message), "unmarshal_failure")

			continue
		}

		if oldestTTL.IsZero() || transaction.TTL.Before(oldestTTL) {
			oldestTTL = transaction.TTL
		}

		if transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix() {
			totalMessagesLessThanOneHour++
			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		// Route any panic through the lib-observability trident (recover +
		// structured log + span event + panic_recovered_total counter). The
		// semaphore release and WaitGroup.Done stay in this func's defer so they
		// run even when processMessage panics. Loop variables are per-iteration
		// (Go 1.22+), so the closure captures them directly.
		libRuntime.SafeGoWithContextAndComponent(ctx, r.Logger, redisBackupConsumerComponent,
			"ledger-redis-backup-consumer", libRuntime.KeepRunning,
			func(ctx context.Context) {
				defer func() {
					<-sem
					wg.Done()
				}()

				r.processMessage(ctx, key, message, transaction)
			})
	}

	wg.Wait()

	// Emit the oldest-age gauge from the TTL tracked during the dispatch pass.
	// Deterministic: derived from the pre-fan-out parse, not from the concurrent
	// workers (which would be racy).
	r.emitOldestAgeGauge(ctx, oldestTTL)

	r.Logger.Log(ctx, libLog.LevelDebug, "Messages under time-of-life threshold", libLog.Int("threshold_minutes", MessageTimeOfLife), libLog.Int("message_count", totalMessagesLessThanOneHour))
	r.Logger.Log(ctx, libLog.LevelDebug, "Finished processing eligible messages", libLog.Int("eligible_count", len(messages)-totalMessagesLessThanOneHour))
}

// processMessage handles a single Redis backup queue message: rebuilds balances
// and operations, and writes the transaction via the async path.
// Duplicate-processing prevention is handled at the cycle level by acquireCycleLock;
// only the leader pod reaches this method.
//
//nolint:gocognit,gocyclo // Will be refactored into smaller helpers; tracked separately.
func (r *RedisQueueConsumer) processMessage(ctx context.Context, key, rawPayload string, m mmodel.TransactionRedisQueue) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	logger := r.Logger.With(libLog.String(libConstants.HeaderID, m.HeaderID))

	ctxWithLogger := libObservability.ContextWithLogger(
		libObservability.ContextWithHeaderID(msgCtx, m.HeaderID),
		logger,
	)

	msgCtxWithSpan, msgSpan := tracer.Start(ctxWithLogger, "redis.consumer.process_message")
	defer msgSpan.End()

	select {
	case <-msgCtxWithSpan.Done():
		logger.Log(msgCtxWithSpan, libLog.LevelWarn, "Transaction message processing cancelled due to shutdown/timeout")

		return
	default:
	}

	if m.Validate == nil {
		logger.Log(ctx, libLog.LevelWarn, "Message has nil Validate field; routing to quarantine flow", libLog.String("key", key))

		r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID, m.TransactionID, []byte(rawPayload), "nil_validate")

		return
	}

	// Both guards below reject structural properties of an immutable payload:
	// no later cycle can make them valid, so retaining the record would
	// reprocess it forever without ever reaching the quarantine threshold.
	effectMode, effectModeErr := mmodel.ResolveTransactionEffectMode(&m)
	if effectModeErr != nil {
		logger.Log(msgCtxWithSpan, libLog.LevelError, "Transaction backup effect mode is invalid; routing to quarantine flow",
			libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(effectModeErr))

		r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID, m.TransactionID, []byte(rawPayload), "invalid_effect_mode")

		return
	}

	if effectMode == mmodel.TransactionEffectAnnotationOnly &&
		(m.AttemptOwner != "" || m.ExpectedOutcome != "" || len(m.Balances) != 0 || len(m.BalancesAfter) != 0) {
		logger.Log(msgCtxWithSpan, libLog.LevelError, "Annotation backup carries financial evidence; routing to quarantine flow",
			libLog.String("transaction_id", m.TransactionID.String()))

		r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID, m.TransactionID, []byte(rawPayload), "annotation_financial_evidence")

		return
	}
	// The public Transaction input deliberately omits this internal field from
	// JSON. Restore it only from the queue's explicit durable discriminator so a
	// BLOCK/UNBLOCK backup rebuild cannot silently relabel operations.
	m.TransactionInput.OperationTypeOverride = m.OperationTypeOverride

	if requiresAtomicOutcomeBackup(m) {
		if len(m.BalancesAfter) == 0 {
			// A terminal lifecycle seed is written before Lua. Only Lua adds the
			// after-state atomically with movement, so a seed alone is never a
			// terminal fact and must not promote PostgreSQL.
			logger.Log(msgCtxWithSpan, libLog.LevelWarn, "Transaction lifecycle backup is waiting for balance outcome",
				libLog.String("transaction_id", m.TransactionID.String()))

			return
		}

		rawOutcome, outcomeErr := r.TransactionHandler.Command.TransactionRedisRepo.Get(msgCtxWithSpan,
			utils.TransactionBalanceOutcomeKey(m.OrganizationID, m.LedgerID, m.TransactionID))
		if outcomeErr != nil || rawOutcome == "" {
			logger.Log(msgCtxWithSpan, libLog.LevelError, "Transaction balance outcome is unavailable; backup retained",
				libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(outcomeErr))

			return
		}

		outcome := mmodel.BalanceExecutionOutcome{}
		if decodeErr := json.Unmarshal([]byte(rawOutcome), &outcome); decodeErr != nil ||
			outcome.Identity != m.TransactionID || outcome.Owner != m.AttemptOwner ||
			outcome.Outcome != m.ExpectedOutcome ||
			!mmodel.RedisBalanceSetEconomicEqual(outcome.Before, m.Balances) ||
			!mmodel.RedisBalanceSetEconomicEqual(outcome.After, m.BalancesAfter) {
			logger.Log(msgCtxWithSpan, libLog.LevelError, "Transaction balance outcome does not match backup; backup retained",
				libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(decodeErr))

			return
		}
	}

	balances := make([]*mmodel.Balance, 0, len(m.Balances))

	for _, balance := range m.Balances {
		rebuilt, balanceErr := balanceFromBackup(balance, m.OrganizationID, m.LedgerID)
		if balanceErr != nil {
			logger.Log(msgCtxWithSpan, libLog.LevelError, "Transaction backup carries malformed balance evidence; routing to quarantine flow",
				libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(balanceErr))

			r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID,
				m.TransactionID, []byte(rawPayload), "malformed_balance_evidence")

			return
		}

		balances = append(balances, rebuilt)
	}

	// Parse AFTER balances from backup queue (nil for legacy entries written by old pods)
	var balancesAfter []*mmodel.Balance
	if len(m.BalancesAfter) > 0 {
		balancesAfter = make([]*mmodel.Balance, 0, len(m.BalancesAfter))

		for _, balance := range m.BalancesAfter {
			rebuilt, balanceErr := balanceFromBackup(balance, m.OrganizationID, m.LedgerID)
			if balanceErr != nil {
				logger.Log(msgCtxWithSpan, libLog.LevelError, "Transaction backup carries malformed balance evidence; routing to quarantine flow",
					libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(balanceErr))

				r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID,
					m.TransactionID, []byte(rawPayload), "malformed_balance_evidence")

				return
			}

			balancesAfter = append(balancesAfter, rebuilt)
		}

		logger.Log(ctx, libLog.LevelDebug, "Using AFTER balances from backup for direct persistence", libLog.Int("balance_count", len(balancesAfter)))
	}

	parentTransactionID, poisonReason, err := r.resolveBackupParentTransactionID(msgCtxWithSpan, m)
	if err != nil {
		logger.Log(msgCtxWithSpan, libLog.LevelError, "Failed to resolve revert origin from durable claim",
			libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(err))

		return
	}

	if poisonReason != "" {
		if poisonReason == "revert_balance_outcome_missing" {
			// This is a valid pre-Lua seed, not corrupt financial data. Keep it
			// in Redis so an HTTP retry can elect the PostgreSQL RECOVERING
			// owner and prove/clean the pre-movement attempt. Quarantining and
			// deleting it would destroy that automatic recovery proof.
			logger.Log(msgCtxWithSpan, libLog.LevelWarn, "Revert backup is waiting for pre-movement recovery",
				libLog.String("transaction_id", m.TransactionID.String()))

			return
		}

		if poisonReason == "phase_zero_pre_movement_seed_drained" {
			// The shared rollout marker admits this cleanup only after every
			// phase-zero pod is unready and its in-flight requests are drained.
			// With no claim and no Lua-authored after-state, this exact seed is a
			// proven abandoned pre-movement attempt. Release the exact H1 first:
			// if this process crashes before deleting the seed, redelivery can
			// repeat the idempotent release; the reverse order would orphan H1.
			if m.ParentTransactionID == nil {
				return
			}

			claim, claimErr := r.TransactionHandler.Command.GetRevertClaim(msgCtxWithSpan,
				m.OrganizationID, m.LedgerID, *m.ParentTransactionID)
			if claimErr != nil || claim != nil {
				logger.Log(msgCtxWithSpan, libLog.LevelWarn, "Drained phase-zero seed gained a durable claim; preserving recovery evidence",
					libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(claimErr))

				return
			}

			legacyKey, keyErr := persistedDrainedLegacyFenceKey(m)
			if keyErr != nil {
				poisonReason = "phase_zero_pre_movement_seed_legacy_fence_unproven"

				logger.Log(msgCtxWithSpan, libLog.LevelError, "Drained phase-zero seed lacks an exact H1 witness",
					libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(keyErr))
			} else {
				released, releaseErr := r.TransactionHandler.Command.TransactionRedisRepo.ReleaseUnownedEmptyKey(
					msgCtxWithSpan, legacyKey)
				if releaseErr != nil || !released {
					logger.Log(msgCtxWithSpan, libLog.LevelError, "Failed to release drained phase-zero H1",
						libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(releaseErr))

					return
				}

				removed, removeErr := r.TransactionHandler.Command.TransactionRedisRepo.RemoveMessageFromQueueIfStatus(
					msgCtxWithSpan, key, m.TransactionStatus, "", "", true)
				if removeErr != nil || !removed {
					logger.Log(msgCtxWithSpan, libLog.LevelError, "Failed to remove drained phase-zero revert seed",
						libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(removeErr))

					return
				}

				r.clearBackupAttempt(msgCtxWithSpan, logger, key)
				logger.Log(msgCtxWithSpan, libLog.LevelInfo, "Removed drained phase-zero pre-movement revert seed",
					libLog.String("transaction_id", m.TransactionID.String()))

				return
			}
		}

		logger.Log(msgCtxWithSpan, libLog.LevelError, "Revert backup has no trustworthy origin; routing to quarantine flow",
			libLog.String("transaction_id", m.TransactionID.String()), libLog.String("reason", poisonReason))
		r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID,
			m.TransactionID, []byte(rawPayload), poisonReason)

		return
	}

	amount := m.TransactionInput.Send.Value

	tran := &postgreTransaction.Transaction{
		ID:                       m.TransactionID.String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           m.OrganizationID.String(),
		LedgerID:                 m.LedgerID.String(),
		Description:              m.TransactionInput.Description,
		Amount:                   &amount,
		AssetCode:                m.TransactionInput.Send.Asset,
		ChartOfAccountsGroupName: m.TransactionInput.ChartOfAccountsGroupName,
		CreatedAt:                m.TransactionDate,
		UpdatedAt:                time.Now(),
		Route:                    m.TransactionInput.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:                  m.TransactionInput.RouteID,
		Metadata:                 m.TransactionInput.Metadata,
		Status: postgreTransaction.Status{
			Code:        m.TransactionStatus,
			Description: &m.TransactionStatus,
		},
		RevertRolloutMode:  m.RevertRolloutMode,
		RevertRolloutToken: m.RevertRolloutToken,
		RedisGeneration:    m.RedisGeneration,
	}

	// Prefer the action captured before movement, then fall back to the terminal
	// status only where the mapping is unambiguous. The same action is used when
	// rebuilding and single-assigning missing operation IDs.
	action := m.Action
	if action == "" {
		switch m.TransactionStatus {
		case constant.PENDING:
			action = constant.ActionHold
		case constant.APPROVED:
			action = constant.ActionCommit
		case constant.CANCELED:
			action = constant.ActionCancel
		case constant.NOTED:
			action = constant.ActionDirect
		}
	}

	var operations []*operation.Operation

	operationsWereMissing := len(m.Operations) == 0

	if !operationsWereMissing {
		operations = make([]*operation.Operation, 0, len(m.Operations))
		for _, r := range m.Operations {
			operations = append(operations, operation.OperationFromRedis(r))
		}

		logger.Log(ctx, libLog.LevelDebug, "Using materialized operations from backup", libLog.Int("operation_count", len(operations)))
	} else {
		var fromTo []mtransaction.FromTo

		fromTo = append(fromTo, mtransaction.MutateConcatAliases(m.TransactionInput.Send.Source.From)...)
		to := mtransaction.MutateConcatAliases(m.TransactionInput.Send.Distribute.To)

		if m.TransactionStatus != constant.PENDING && m.TransactionStatus != constant.CANCELED {
			fromTo = append(fromTo, to...)
		}

		ledgerSettings, err := r.TransactionHandler.Query.GetParsedLedgerSettings(msgCtxWithSpan, m.OrganizationID, m.LedgerID)
		if err != nil {
			logger.Log(msgCtxWithSpan, libLog.LevelError, "Failed to get ledger settings for backup consumer message. Routing to quarantine flow.", libLog.String("transactionId", m.TransactionID.String()), libLog.Err(err))

			r.quarantinePoisonRecord(msgCtxWithSpan, msgSpan, logger, key, m.OrganizationID, m.LedgerID, m.TransactionID, []byte(rawPayload), "ledger_settings_fetch_failure")

			return
		}

		var routeCache *mmodel.TransactionRouteCache

		if ledgerSettings.Accounting.ValidateRoutes {
			// Prefer the new TransactionRouteID (UUID) over the deprecated TransactionRoute string.
			var trID uuid.UUID

			var parseErr error

			if !libCommons.IsNilOrEmpty(m.Validate.TransactionRouteID) {
				trID, parseErr = uuid.Parse(*m.Validate.TransactionRouteID)
				if parseErr != nil {
					logger.Log(ctx, libLog.LevelDebug, "Failed to parse TransactionRouteID", libLog.String("transaction_route_id", *m.Validate.TransactionRouteID), libLog.Err(parseErr))
				}
			} else if m.Validate.TransactionRoute != "" {
				trID, parseErr = uuid.Parse(m.Validate.TransactionRoute)
				if parseErr != nil {
					logger.Log(ctx, libLog.LevelDebug, "Failed to parse TransactionRoute UUID", libLog.String("transaction_route", m.Validate.TransactionRoute), libLog.Err(parseErr))
				}
			}

			if parseErr == nil && trID != uuid.Nil {
				cache, cacheErr := r.TransactionHandler.Query.GetOrCreateTransactionRouteCache(msgCtxWithSpan, m.OrganizationID, m.LedgerID, trID)
				if cacheErr != nil {
					logger.Log(ctx, libLog.LevelDebug, "Failed to get route cache", libLog.String("route_id", trID.String()), libLog.Err(cacheErr))
				} else {
					routeCache = &cache
				}
			}
		}

		var buildErr error

		operations, _, buildErr = r.TransactionHandler.BuildOperations(
			msgCtxWithSpan, balances, balancesAfter, fromTo, m.TransactionInput, *tran, m.Validate, m.TransactionDate, m.TransactionStatus == constant.NOTED, ledgerSettings.Accounting.ValidateRoutes, routeCache, action,
		)
		if buildErr != nil {
			libOpentelemetry.HandleSpanError(msgSpan, "Failed to validate balances", buildErr)

			logger.Log(ctx, libLog.LevelError, "Failed to validate balance", libLog.Err(buildErr))

			return
		}

		if len(balancesAfter) == 0 {
			// Legacy non-revert backups may predate the atomic after-state.
			// Keep that degraded path observable; new reverts are quarantined
			// before this point unless the Lua-authored snapshot is present.
			msgSpan.SetAttributes(attribute.Bool("app.replay.recomputed_balances_after", true))
			r.emitReplayRecomputedBalancesAfterMetric(msgCtxWithSpan, logger)
		}
	}

	var executionAttempt *mmodel.BalanceExecutionAttempt
	if m.AttemptOwner != "" && m.ExpectedOutcome != "" {
		executionAttempt = &mmodel.BalanceExecutionAttempt{
			ExecutionKey:    utils.TransactionBalanceExecutionKey(m.OrganizationID, m.LedgerID, m.TransactionID),
			OutcomeKey:      utils.TransactionBalanceOutcomeKey(m.OrganizationID, m.LedgerID, m.TransactionID),
			Owner:           m.AttemptOwner,
			Outcome:         m.ExpectedOutcome,
			Identity:        m.TransactionID,
			RedisGeneration: m.RedisGeneration,
		}
	}

	terminalReplay := false

	if operationsWereMissing || executionAttempt != nil {
		var enrichErr error

		operations, terminalReplay, enrichErr = r.TransactionHandler.Command.UpdateTransactionBackupOperations(
			msgCtxWithSpan, m.OrganizationID, m.LedgerID, m.TransactionID, operations,
			mmodel.BalancesToRedis(balancesAfter), action, executionAttempt,
			mmodel.TransactionEconomicContext{
				ParentTransactionID: m.ParentTransactionID, TransactionStatus: m.TransactionStatus,
				TransactionAmount: m.TransactionInput.Send.Value.String(), TransactionAssetCode: m.TransactionInput.Send.Asset,
			},
		)
		if enrichErr != nil {
			libOpentelemetry.HandleSpanError(msgSpan, "Failed to bind rebuilt operations to backup", enrichErr)
			logger.Log(ctx, libLog.LevelError, "Failed to bind rebuilt operations to backup",
				libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(enrichErr))

			return
		}
	}

	tran.Source = m.Validate.Sources
	tran.Destination = m.Validate.Destinations

	tran.Operations = operations
	if terminalReplay {
		payload := postgreTransaction.TransactionProcessingPayload{
			Transaction: tran, Input: &m.TransactionInput, Validate: m.Validate,
			BalancesAfter:         balancesAfter,
			EffectModeVersion:     mmodel.TransactionEffectModeVersion,
			EffectMode:            mmodel.TransactionEffectBalanceMutation,
			OperationTypeOverride: m.OperationTypeOverride,
			AttemptOwner:          m.AttemptOwner, ExpectedOutcome: m.ExpectedOutcome,
			RevertRolloutMode: m.RevertRolloutMode, RevertRolloutToken: m.RevertRolloutToken,
			RedisGeneration: m.RedisGeneration,
		}
		if _, replayErr := r.TransactionHandler.Command.FinalizeDurableTransactionPersistence(
			msgCtxWithSpan, m.OrganizationID, m.LedgerID, payload); replayErr != nil {
			libOpentelemetry.HandleSpanError(msgSpan, "Failed to finalize terminal backup replay", replayErr)
			logger.Log(ctx, libLog.LevelError, "Failed to finalize terminal backup replay",
				libLog.String("transaction_id", m.TransactionID.String()), libLog.Err(replayErr))

			return
		}

		r.clearBackupAttempt(msgCtxWithSpan, logger, key)

		return
	}

	utils.SanitizeAccountAliases(&m.TransactionInput)

	if err := r.TransactionHandler.Command.WriteTransactionAsync(
		msgCtxWithSpan, m.OrganizationID, m.LedgerID, &m.TransactionInput, m.Validate, balances, balancesAfter, tran,
		executionAttempt,
	); err != nil {
		libOpentelemetry.HandleSpanError(msgSpan, "Failed sending message to queue", err)

		logger.Log(ctx, libLog.LevelError, "Failed sending message to queue", libLog.String("key", key), libLog.Err(err))

		return
	}

	logger.Log(ctx, libLog.LevelDebug, "Transaction message processed", libLog.String("key", key))

	// Success: a previously-failing record has now replayed. Clear its attempts
	// counter so it does not accrue toward the quarantine threshold. The backup
	// record itself is removed downstream by the async write path after the
	// confirmed Postgres persist (RemoveTransactionFromRedisQueueIfStatus).
	r.clearBackupAttempt(msgCtxWithSpan, logger, key)
}

func persistedDrainedLegacyFenceKey(m mmodel.TransactionRedisQueue) (string, error) {
	if m.RevertLegacyFenceKey == "" || m.ParentTransactionID == nil || m.TransactionInput.IsEmpty() {
		return "", fmt.Errorf("persisted legacy fence identity is incomplete")
	}

	legacyHash, err := utils.LegacyTransactionIdempotencyHash(m.TransactionInput)
	if err != nil {
		return "", fmt.Errorf("derive immutable backup legacy fence identity: %w", err)
	}

	expected := utils.IdempotencyInternalKey(m.OrganizationID, m.LedgerID, legacyHash)
	if m.RevertLegacyFenceKey != expected {
		return "", fmt.Errorf("persisted legacy fence differs from immutable backup input")
	}

	return m.RevertLegacyFenceKey, nil
}

func requiresAtomicOutcomeBackup(m mmodel.TransactionRedisQueue) bool {
	return m.AttemptOwner != "" || m.ExpectedOutcome != ""
}

func balanceFromBackup(balance mmodel.BalanceRedis, organizationID, ledgerID uuid.UUID) (*mmodel.Balance, error) {
	balanceKey := balance.Key
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	// An empty OverdraftUsed is the expected legacy shape and means zero. A
	// malformed non-empty value is corrupt financial evidence and must fail
	// closed instead of silently reporting that no overdraft was consumed.
	overdraftUsed := decimal.Zero

	if balance.OverdraftUsed != "" {
		parsed, err := decimal.NewFromString(balance.OverdraftUsed)
		if err != nil {
			return nil, fmt.Errorf("decode backup overdraft used for balance %s: %w", balance.ID, err)
		}

		overdraftUsed = parsed
	}

	var settings *mmodel.BalanceSettings

	if balance.AllowOverdraft != 0 || balance.OverdraftLimitEnabled != 0 ||
		(balance.BalanceScope != "" && balance.BalanceScope != mmodel.BalanceScopeTransactional) ||
		(balance.OverdraftLimit != "" && balance.OverdraftLimit != "0") {
		scope := balance.BalanceScope
		if scope == "" {
			scope = mmodel.BalanceScopeTransactional
		}

		settings = &mmodel.BalanceSettings{
			BalanceScope:          scope,
			AllowOverdraft:        balance.AllowOverdraft == 1,
			OverdraftLimitEnabled: balance.OverdraftLimitEnabled == 1,
		}
		if balance.OverdraftLimitEnabled == 1 && balance.OverdraftLimit != "" {
			limit := balance.OverdraftLimit
			settings.OverdraftLimit = &limit
		}
	}

	return &mmodel.Balance{
		Alias:          balance.Alias,
		ID:             balance.ID,
		AccountID:      balance.AccountID,
		Key:            balanceKey,
		Available:      balance.Available,
		OnHold:         balance.OnHold,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending == 1,
		AllowReceiving: balance.AllowReceiving == 1,
		AssetCode:      balance.AssetCode,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Direction:      balance.Direction,
		OverdraftUsed:  overdraftUsed,
		Settings:       settings,
	}, nil
}

func (r *RedisQueueConsumer) resolveBackupParentTransactionID(ctx context.Context, m mmodel.TransactionRedisQueue) (*string, string, error) {
	isRevert := m.Action == constant.ActionRevert || m.ParentTransactionID != nil
	if !isRevert {
		return nil, "", nil
	}

	claim, err := r.TransactionHandler.Command.GetRevertClaimByReverseID(ctx, m.OrganizationID, m.LedgerID, m.TransactionID)
	if err != nil {
		return nil, "", err
	}

	if claim == nil {
		// Every phase-zero-capable request has a claim before H1. A claim-less
		// explicit-parent record therefore belongs to a genuinely old compatible
		// pod. Its backup and Lua-authored BalancesAfter are sufficient to persist
		// the exact reverse; WriteTransactionAsync adopts and completes a durable
		// claim only after the child and every operation are durable. Parent-less
		// old-pod records remain untrusted.
		if m.ParentTransactionID != nil && *m.ParentTransactionID != uuid.Nil {
			if len(m.BalancesAfter) == 0 {
				drained, phaseErr := r.phaseZeroRequestsDrained(ctx)
				if phaseErr != nil {
					return nil, "", phaseErr
				}

				if drained {
					return nil, "phase_zero_pre_movement_seed_drained", nil
				}

				return nil, "revert_balance_outcome_missing", nil
			}

			parentTransactionID := m.ParentTransactionID.String()

			return &parentTransactionID, "", nil
		}

		return nil, "revert_claim_missing", nil
	}

	if m.ParentTransactionID != nil && *m.ParentTransactionID != claim.OriginTransactionID {
		return nil, "revert_origin_claim_mismatch", nil
	}

	if len(m.BalancesAfter) == 0 {
		// A revert backup is seeded before balance Lua dispatch. Only the Lua
		// command writes BalancesAfter atomically with the movement, so a seed
		// without that outcome must never be persisted as a completed reverse.
		return nil, "revert_balance_outcome_missing", nil
	}

	parentTransactionID := claim.OriginTransactionID.String()

	return &parentTransactionID, "", nil
}

func (r *RedisQueueConsumer) phaseZeroRequestsDrained(ctx context.Context) (bool, error) {
	phaseReader, ok := r.TransactionHandler.RevertUpdateFreeze.(interface {
		Phase(context.Context) (string, error)
	})
	if !ok {
		return false, nil
	}

	phase, err := phaseReader.Phase(ctx)
	if err != nil {
		return false, fmt.Errorf("read revert rollout phase for backup recovery: %w", err)
	}

	return phase == transactionredis.RevertUpdateFreezeDrained || phase == transactionredis.RevertUpdateFreezeFinalized, nil
}

// quarantinePoisonRecord enforces THE INVARIANT for poison backup records: a
// poison record (the only durable copy of an authorized transaction) is never
// deleted from Redis without prior confirmed persistence to the Postgres
// quarantine table, and never skipped silently forever.
//
// Flow per poison classification:
//  1. Increment the per-record attempts counter (parallel attempts hash).
//  2. Below QuarantineThreshold: leave the record in Redis to retry next cycle.
//  3. At/above the threshold: Insert into the durable quarantine table FIRST.
//     - On Insert failure: leave BOTH the record and the attempts counter in
//     place so the next cycle retries — never delete on a failed persist.
//     - On Insert success: remove the record (HDel) then the attempts counter
//     (HDel). Order is load-bearing: the durable copy must exist before the
//     Redis copy is deleted.
//
// The payload (raw financial copy) is never logged or attached to a span (T9).
func (r *RedisQueueConsumer) quarantinePoisonRecord(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	key string,
	organizationID, ledgerID, transactionID uuid.UUID,
	payload []byte,
	failureReason string,
) {
	if r.quarantineRepo == nil {
		// No quarantine sink wired: leave the record in Redis untouched so the
		// invariant (never delete without confirmed persistence) holds. This is
		// a misconfiguration, surfaced loudly.
		logger.Log(ctx, libLog.LevelError, "Quarantine repository not configured; poison record left in backup queue",
			libLog.String("redis_key", key), libLog.String("failure_reason", failureReason))

		return
	}

	attempts, err := r.TransactionHandler.Command.TransactionRedisRepo.IncrementBackupAttempt(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to increment backup attempt", err)
		logger.Log(ctx, libLog.LevelError, "Failed to increment backup attempt; record left in backup queue",
			libLog.String("redis_key", key), libLog.Err(err))

		return
	}

	if attempts < QuarantineThreshold {
		logger.Log(ctx, libLog.LevelWarn, "Poison backup record failed; will retry next cycle",
			libLog.String("redis_key", key),
			libLog.String("failure_reason", failureReason),
			libLog.Int("attempts", int(attempts)))

		return
	}

	record := &transactionquarantine.QuarantineRecord{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  transactionID,
		RedisKey:       key,
		Payload:        payload,
		FailureReason:  failureReason,
		Attempts:       int(attempts),
		FirstFailedAt:  time.Now(),
		QuarantinedAt:  time.Now(),
	}

	if err := r.quarantineRepo.Insert(ctx, record); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to quarantine poison backup record", err)
		logger.Log(ctx, libLog.LevelError, "Failed to quarantine poison backup record; record left in backup queue for retry",
			libLog.String("redis_key", key),
			libLog.String("failure_reason", failureReason),
			libLog.Int("attempts", int(attempts)),
			libLog.Err(err))

		return
	}

	// Persistence confirmed. Now (and only now) remove the Redis copy, then the
	// attempts counter.
	removed, err := r.TransactionHandler.Command.TransactionRedisRepo.RemoveMessageFromQueueIfValue(ctx, key, payload)
	if err != nil || !removed {
		libOpentelemetry.HandleSpanError(span, "Failed to remove quarantined record from backup queue", err)
		logger.Log(ctx, libLog.LevelError, "Quarantined record persisted but failed to remove from backup queue",
			libLog.String("redis_key", key), libLog.Err(err))

		return
	}

	r.clearBackupAttempt(ctx, logger, key)

	r.emitQuarantineMetric(ctx, logger)

	logger.Log(ctx, libLog.LevelError, "Poison backup record quarantined and removed from backup queue",
		libLog.String("redis_key", key),
		libLog.String("failure_reason", failureReason),
		libLog.Int("attempts", int(attempts)))
}

// clearBackupAttempt removes the per-record attempts counter, logging at Warn on
// failure (best-effort cleanup; a stale counter only delays re-accrual).
func (r *RedisQueueConsumer) clearBackupAttempt(ctx context.Context, logger libLog.Logger, key string) {
	if err := r.TransactionHandler.Command.TransactionRedisRepo.ClearBackupAttempt(ctx, key); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to clear backup attempt counter",
			libLog.String("redis_key", key), libLog.Err(err))
	}
}

// parsePoisonKeyIDs extracts the organization, ledger, and transaction UUIDs
// from a backup-queue field key. The key format is produced by
// utils.TransactionInternalKey: "transaction:{transactions}:<org>:<ledger>:<tx>".
// It scans colon-separated segments and returns the first three that parse as
// UUIDs, in order. Used on the unmarshal-failure path where the payload itself
// did not parse, so the IDs must come from the key.
func parsePoisonKeyIDs(key string) (organizationID, ledgerID, transactionID uuid.UUID, ok bool) {
	var found []uuid.UUID

	for _, seg := range strings.Split(key, ":") {
		if u, err := uuid.Parse(seg); err == nil {
			found = append(found, u)
			if len(found) == 3 {
				break
			}
		}
	}

	if len(found) < 3 {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}

	return found[0], found[1], found[2], true
}

// emitDepthGauge sets the backup-queue depth gauge from the record count read
// this cycle. Best-effort: a nil factory or a metric emit error never affects
// processing (emit errors logged at Debug per T11).
func (r *RedisQueueConsumer) emitDepthGauge(ctx context.Context, depth int64) {
	if r.metricsFactory == nil {
		return
	}

	depthGauge, err := r.metricsFactory.Gauge(utils.RedisBackupQueueDepth)
	if err != nil {
		r.Logger.Log(ctx, libLog.LevelDebug, "Failed to create backup queue depth gauge", libLog.Err(err))
	} else if setErr := depthGauge.Set(ctx, depth); setErr != nil {
		r.Logger.Log(ctx, libLog.LevelDebug, "Failed to set backup queue depth gauge", libLog.Err(setErr))
	}
}

// emitOldestAgeGauge sets the backup-queue oldest-age gauge from the earliest
// record TTL tracked during the dispatch pass. A zero oldestTTL (no records
// parsed) is a no-op. Best-effort: a nil factory or emit error never affects
// processing (emit errors logged at Debug per T11).
func (r *RedisQueueConsumer) emitOldestAgeGauge(ctx context.Context, oldestTTL time.Time) {
	if r.metricsFactory == nil || oldestTTL.IsZero() {
		return
	}

	ageSeconds := int64(time.Since(oldestTTL).Seconds())
	if ageSeconds < 0 {
		ageSeconds = 0
	}

	ageGauge, err := r.metricsFactory.Gauge(utils.RedisBackupQueueOldestAgeSeconds)
	if err != nil {
		r.Logger.Log(ctx, libLog.LevelDebug, "Failed to create backup queue oldest-age gauge", libLog.Err(err))

		return
	}

	if setErr := ageGauge.Set(ctx, ageSeconds); setErr != nil {
		r.Logger.Log(ctx, libLog.LevelDebug, "Failed to set backup queue oldest-age gauge", libLog.Err(setErr))
	}
}

// emitQuarantineMetric increments the quarantine counter. Best-effort: a nil
// factory or emit error never affects the quarantine flow (emit errors at Debug).
func (r *RedisQueueConsumer) emitQuarantineMetric(ctx context.Context, logger libLog.Logger) {
	if r.metricsFactory == nil {
		return
	}

	counter, err := r.metricsFactory.Counter(utils.RedisBackupQuarantineTotal)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to create backup quarantine counter", libLog.Err(err))

		return
	}

	if addErr := counter.AddOne(ctx); addErr != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit backup quarantine counter", libLog.Err(addErr))
	}
}

// emitReplayRecomputedBalancesAfterMetric increments the counter that tracks
// backup-replay records rebuilt without Lua's authoritative after-balances.
// Best-effort: a nil factory or emit error never affects processing (emit errors
// at Debug per T11).
func (r *RedisQueueConsumer) emitReplayRecomputedBalancesAfterMetric(ctx context.Context, logger libLog.Logger) {
	if r.metricsFactory == nil {
		return
	}

	counter, err := r.metricsFactory.Counter(utils.RedisBackupReplayRecomputedBalancesAfterTotal)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to create replay recomputed-balances-after counter", libLog.Err(err))

		return
	}

	if addErr := counter.AddOne(ctx); addErr != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit replay recomputed-balances-after counter", libLog.Err(addErr))
	}
}
