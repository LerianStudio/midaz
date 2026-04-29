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
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	postgreTransaction "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

const (
	CronTimeToRun     = 30 * time.Minute
	MessageTimeOfLife = 30
	MaxWorkers        = 100
	CycleLockTTL      = 1800 // 30 minutes in seconds — matches CronTimeToRun
)

type RedisQueueConsumer struct {
	Logger             libLog.Logger
	TransactionHandler in.TransactionHandler
	multiTenantEnabled bool
	tenantCache        *tenantcache.TenantCache
	pgManager          *tmpostgres.Manager
	serviceName        string
}

func NewRedisQueueConsumer(logger libLog.Logger, handler in.TransactionHandler) *RedisQueueConsumer {
	return &RedisQueueConsumer{
		Logger:             logger,
		TransactionHandler: handler,
	}
}

// NewRedisQueueConsumerMultiTenant creates a RedisQueueConsumer with multi-tenant fields populated.
// When multiTenantEnabled is true, both tenantCache and pgManager must be non-nil for the consumer
// to be considered ready (isMultiTenantReady). The consumer reads tenant IDs from the shared
// TenantCache (populated by the TenantEventListener) and uses pgManager to resolve per-tenant
// PostgreSQL connections.
// serviceName is the service identifier for logging purposes.
func NewRedisQueueConsumerMultiTenant(
	logger libLog.Logger,
	handler in.TransactionHandler,
	multiTenantEnabled bool,
	cache *tenantcache.TenantCache,
	pgManager *tmpostgres.Manager,
	serviceName string,
) *RedisQueueConsumer {
	c := NewRedisQueueConsumer(logger, handler)
	c.multiTenantEnabled = multiTenantEnabled
	c.tenantCache = cache
	c.pgManager = pgManager
	c.serviceName = serviceName

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
		r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer: no tenants in cache, skipping cycle")

		return
	}

	for _, tenantID := range tenantIDs {
		if ctx.Err() != nil {
			r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer: context cancelled, stopping tenant iteration")

			return
		}

		tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)

		conn, err := r.pgManager.GetConnection(tenantCtx, tenantID)
		if err != nil {
			r.Logger.Log(ctx, libLog.LevelError, fmt.Sprintf("RedisQueueConsumer: failed to get PG connection for tenant %s: %v", tenantID, err))

			continue
		}

		db, err := conn.GetDB()
		if err != nil {
			r.Logger.Log(ctx, libLog.LevelError, fmt.Sprintf("RedisQueueConsumer: failed to get DB for tenant %s: %v", tenantID, err))

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
		r.Logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to acquire backup consumer cycle lock: %v", err))

		return false, nil
	}

	if !success {
		r.Logger.Log(ctx, libLog.LevelInfo, "Another pod holds the backup consumer lock, skipping cycle")

		return false, nil
	}

	r.Logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Cycle lock acquired by pod %s", podID))

	release := func() {
		if delErr := r.TransactionHandler.Command.TransactionRedisRepo.Del(ctx, cycleLockKey); delErr != nil {
			r.Logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to release backup consumer cycle lock: %v", delErr))
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
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "redis.consumer.read_messages_from_queue")
	defer span.End()

	r.Logger.Log(ctx, libLog.LevelInfo, "Init cron to read messages from redis...")

	messages, err := r.TransactionHandler.Command.TransactionRedisRepo.ReadAllMessagesFromQueue(ctx)
	if err != nil {
		r.Logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Err to read messages from redis: %v", err))
		return
	}

	r.Logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Total of read %d messages from queue", len(messages)))

	if len(messages) == 0 {
		return
	}

	sem := make(chan struct{}, MaxWorkers)

	var wg sync.WaitGroup

	totalMessagesLessThanOneHour := 0

Outer:
	for key, message := range messages {
		if ctx.Err() != nil {
			r.Logger.Log(ctx, libLog.LevelWarn, "Shutdown in progress: skipping remaining messages")
			break Outer
		}

		var transaction mmodel.TransactionRedisQueue
		if err := json.Unmarshal([]byte(message), &transaction); err != nil {
			r.Logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error unmarshalling message from Redis: %v", err))
			continue
		}

		if transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix() {
			totalMessagesLessThanOneHour++
			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		go func(key string, m mmodel.TransactionRedisQueue) {
			defer func() {
				if rec := recover(); rec != nil {
					r.Logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Panic recovered while processing message (key: %s): %v. Message will remain in queue.", key, rec))
				}

				<-sem
				wg.Done()
			}()

			r.processMessage(ctx, key, m)
		}(key, transaction)
	}

	wg.Wait()

	r.Logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Total of messagens under %d minute(s) : %d", MessageTimeOfLife, totalMessagesLessThanOneHour))
	r.Logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Finished processing total of %d eligible messages", len(messages)-totalMessagesLessThanOneHour))
}

// processMessage handles a single Redis backup queue message: rebuilds balances
// and operations, and writes the transaction via the async path.
// Duplicate-processing prevention is handled at the cycle level by acquireCycleLock;
// only the leader pod reaches this method.
func (r *RedisQueueConsumer) processMessage(ctx context.Context, key string, m mmodel.TransactionRedisQueue) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled

	msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	logger := r.Logger.With(libLog.String(libConstants.HeaderID, m.HeaderID))

	ctxWithLogger := libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(msgCtx, m.HeaderID),
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
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Message (key: %s) has nil Validate field, skipping. Message will remain in queue.", key))

		return
	}

	balances := make([]*mmodel.Balance, 0, len(m.Balances))
	for _, balance := range m.Balances {
		balanceKey := balance.Key
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		balances = append(balances, &mmodel.Balance{
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
			OrganizationID: m.OrganizationID.String(),
			LedgerID:       m.LedgerID.String(),
		})
	}

	// Parse AFTER balances from backup queue (nil for legacy entries written by old pods)
	var balancesAfter []*mmodel.Balance
	if len(m.BalancesAfter) > 0 {
		balancesAfter = make([]*mmodel.Balance, 0, len(m.BalancesAfter))
		for _, balance := range m.BalancesAfter {
			balanceKey := balance.Key
			if balanceKey == "" {
				balanceKey = constant.DefaultBalanceKey
			}

			balancesAfter = append(balancesAfter, &mmodel.Balance{
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
				OrganizationID: m.OrganizationID.String(),
				LedgerID:       m.LedgerID.String(),
			})
		}

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Using %d AFTER balances from backup for direct persistence", len(balancesAfter)))
	}

	var parentTransactionID *string

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
		Route:                    m.TransactionInput.Route,
		RouteID:                  m.TransactionInput.RouteID,
		Metadata:                 m.TransactionInput.Metadata,
		Status: postgreTransaction.Status{
			Code:        m.TransactionStatus,
			Description: &m.TransactionStatus,
		},
	}

	var operations []*operation.Operation

	if len(m.Operations) > 0 {
		operations = make([]*operation.Operation, 0, len(m.Operations))
		for _, r := range m.Operations {
			operations = append(operations, operation.OperationFromRedis(r))
		}

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Using %d materialized operations from backup", len(operations)))
	} else {
		var fromTo []mtransaction.FromTo

		fromTo = append(fromTo, mtransaction.MutateConcatAliases(m.TransactionInput.Send.Source.From)...)
		to := mtransaction.MutateConcatAliases(m.TransactionInput.Send.Distribute.To)

		if m.TransactionStatus != constant.PENDING && m.TransactionStatus != constant.CANCELED {
			fromTo = append(fromTo, to...)
		}

		ledgerSettings, err := r.TransactionHandler.Query.GetParsedLedgerSettings(msgCtxWithSpan, m.OrganizationID, m.LedgerID)
		if err != nil {
			logger.Log(msgCtxWithSpan, libLog.LevelError, "Failed to get ledger settings, aborting backup consumer message", libLog.String("transactionId", m.TransactionID.String()), libLog.Err(err))

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
					logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Failed to parse TransactionRouteID %s: %v", *m.Validate.TransactionRouteID, parseErr))
				}
			} else if m.Validate.TransactionRoute != "" {
				trID, parseErr = uuid.Parse(m.Validate.TransactionRoute)
				if parseErr != nil {
					logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Failed to parse TransactionRoute UUID %s: %v", m.Validate.TransactionRoute, parseErr))
				}
			}

			if parseErr == nil && trID != uuid.Nil {
				cache, cacheErr := r.TransactionHandler.Query.GetOrCreateTransactionRouteCache(msgCtxWithSpan, m.OrganizationID, m.LedgerID, trID)
				if cacheErr != nil {
					logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Failed to get route cache for org=%s ledger=%s route=%s: %v", m.OrganizationID, m.LedgerID, trID, cacheErr))
				} else {
					routeCache = &cache
				}
			}
		}

		// Prefer persisted action from backup payload (e.g. revert), then
		// fall back to status-derived action only for statuses with an
		// unambiguous mapping. CREATED is intentionally left empty because
		// legacy payloads may be either "direct" or "revert".
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

		var buildErr error

		// Replay path: Lua's authoritative `balancesAfter` is not captured
		// in the backup envelope, so BuildOperations falls back to the
		// OperateBalances-recomputation branch. For non-overdraft
		// transactions this is correct; for overdraft transactions the
		// operation records will carry the naive `before - amount`
		// arithmetic — the balance table remains consistent (Lua already
		// flushed it) but the audit trail may diverge. Capturing Lua's
		// after-state in the backup envelope is tracked under T-006.1 /
		// T-009 hardening items.
		operations, _, buildErr = r.TransactionHandler.BuildOperations(
			msgCtxWithSpan, balances, nil /* balancesAfter */, fromTo, m.TransactionInput, *tran, m.Validate, m.TransactionDate, m.TransactionStatus == constant.NOTED, ledgerSettings.Accounting.ValidateRoutes, routeCache, action,
		)
		if buildErr != nil {
			libOpentelemetry.HandleSpanError(msgSpan, "Failed to validate balances", buildErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", buildErr.Error()))

			return
		}
	}

	tran.Source = m.Validate.Sources
	tran.Destination = m.Validate.Destinations
	tran.Operations = operations

	utils.SanitizeAccountAliases(&m.TransactionInput)

	if err := r.TransactionHandler.Command.WriteTransactionAsync(
		msgCtxWithSpan, m.OrganizationID, m.LedgerID, &m.TransactionInput, m.Validate, balances, balancesAfter, tran,
	); err != nil {
		libOpentelemetry.HandleSpanError(msgSpan, "Failed sending message to queue", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed sending message: %s to queue: %v", key, err.Error()))

		return
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction message processed: %s", key))
}
