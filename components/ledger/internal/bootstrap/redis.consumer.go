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
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

const (
	CronTimeToRun     = 30 * time.Minute
	MessageTimeOfLife = 30
	MaxWorkers        = 100
	ConsumerLockTTL   = 1500 // 25 minutes in seconds
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
			r.readMessagesAndProcess(ctx)
		}
	}
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
			tenantIDs := r.tenantCache.TenantIDs()
			if len(tenantIDs) == 0 {
				r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer: no tenants in cache, skipping cycle")

				continue
			}

			for _, tenantID := range tenantIDs {
				if ctx.Err() != nil {
					r.Logger.Log(ctx, libLog.LevelInfo, "RedisQueueConsumer: shutting down...")
					return nil
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

				tenantCtx = tmcore.ContextWithTenantPGConnection(tenantCtx, db)

				r.readMessagesAndProcess(tenantCtx)
			}
		}
	}
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

// processMessage handles a single Redis backup queue message: acquires a distributed lock,
// rebuilds balances and operations, and writes the transaction via the async path.
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

	// Acquire distributed lock to prevent duplicate processing across pods
	lockKey := utils.RedisConsumerLockKey(m.OrganizationID, m.LedgerID, m.TransactionID.String())

	_, spanLock := tracer.Start(msgCtxWithSpan, "redis.consumer.acquire_lock")

	success, err := r.TransactionHandler.Command.TransactionRedisRepo.SetNX(msgCtxWithSpan, lockKey, "", ConsumerLockTTL)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanLock, "Failed to acquire lock", err)
		spanLock.End()

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to acquire lock for message %s: %v", key, err))

		return
	}

	if !success {
		libOpentelemetry.HandleSpanEvent(spanLock, "Lock already held by another pod")
		spanLock.End()

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Message %s already being processed by another pod, skipping", key))

		return
	}

	spanLock.End()

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

	tran := &postgreTransaction.Transaction{
		ID:                       m.TransactionID.String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           m.OrganizationID.String(),
		LedgerID:                 m.LedgerID.String(),
		Description:              m.TransactionInput.Description,
		Amount:                   &m.TransactionInput.Send.Value,
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
		var fromTo []pkgTransaction.FromTo

		fromTo = append(fromTo, r.TransactionHandler.HandleAccountFields(m.TransactionInput.Send.Source.From, true)...)
		to := r.TransactionHandler.HandleAccountFields(m.TransactionInput.Send.Distribute.To, true)

		if m.TransactionStatus != constant.PENDING && m.TransactionStatus != constant.CANCELED {
			fromTo = append(fromTo, to...)
		}

		ledgerSettings := r.TransactionHandler.Query.GetParsedLedgerSettings(msgCtxWithSpan, m.OrganizationID, m.LedgerID)

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

		var buildErr error

		operations, _, buildErr = r.TransactionHandler.BuildOperations(
			msgCtxWithSpan, balances, fromTo, m.TransactionInput, *tran, m.Validate, m.TransactionDate, m.TransactionStatus == constant.NOTED, ledgerSettings.Accounting.ValidateRoutes, routeCache,
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
