package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	balanceSyncIdleWaitSeconds = 600
	uuidStringLength           = 36
)

// ErrBalanceSyncKeyMissingUUIDs is returned when balance sync key is missing required UUIDs
var ErrBalanceSyncKeyMissingUUIDs = errors.New("balance sync key missing required UUIDs")

// ErrBalanceSyncPanicRecovered is returned when a panic is recovered during balance sync processing
var ErrBalanceSyncPanicRecovered = errors.New("panic recovered during balance sync")

// BalanceSyncWorker continuously processes keys scheduled for pre-expiry actions.
// Ensures that the balance is synced before the key expires.
type BalanceSyncWorker struct {
	redisConn  *libRedis.RedisConnection
	logger     libLog.Logger
	idleWait   time.Duration
	batchSize  int64
	maxWorkers int
	useCase    *command.UseCase
}

// NewBalanceSyncWorker creates a new BalanceSyncWorker with the specified Redis connection and configuration.
// The maxWorkers parameter controls the concurrency of balance sync operations.
func NewBalanceSyncWorker(conn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase, maxWorkers int) *BalanceSyncWorker {
	assert.NotNil(conn, "Redis connection required for BalanceSyncWorker")
	assert.NotNil(logger, "Logger required for BalanceSyncWorker")
	assert.NotNil(useCase, "UseCase required for BalanceSyncWorker")

	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	assert.That(maxWorkers > 0, "maxWorkers must be greater than zero", "maxWorkers", maxWorkers)

	return &BalanceSyncWorker{
		redisConn:  conn,
		logger:     logger,
		idleWait:   balanceSyncIdleWaitSeconds * time.Second,
		batchSize:  int64(maxWorkers),
		maxWorkers: maxWorkers,
		useCase:    useCase,
	}
}

// Run starts the balance sync worker loop that continuously processes scheduled balance syncs.
// It blocks until the context is cancelled or an interrupt signal is received.
func (w *BalanceSyncWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Info("BalanceSyncWorker started")

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: failed to get redis client: %v", err)

		return pkg.ValidateInternalError(err, "BalanceSyncWorker")
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

		mruntime.SafeGo(w.logger, "balance_sync_worker", mruntime.KeepRunning, func() {
			defer func() { <-sem }()

			defer wg.Done()

			if w.shouldShutdown(ctx) {
				return
			}

			w.processBalanceToExpire(ctx, rds, member)
		})
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
	_, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "balance.worker.process_balance_to_expire")
	defer span.End()

	// Panic recovery with span event recording
	// Records custom span fields for debugging, then re-panics so the outer mruntime.SafeGo
	// wrapper (line 147) can observe the panic for metrics and error reporting.
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			span.AddEvent("panic.recovered", trace.WithAttributes(
				attribute.String("panic.value", fmt.Sprintf("%v", rec)),
				attribute.String("panic.stack", string(stack)),
				attribute.String("member", member),
			))
			libOpentelemetry.HandleSpanError(&span, "Panic during balance sync processing", w.panicAsError(rec))
			// Logger.Errorf removed - outer mruntime.SafeGo wrapper logs with full context
			// Re-panic so outer mruntime.SafeGo wrapper can record metrics and invoke error reporter
			//nolint:panicguardwarn // Intentional re-panic for observability chain
			panic(rec)
		}
	}()

	if member == "" {
		return
	}

	if w.checkAndHandleExpiredKey(ctx, rds, member) {
		return
	}

	val, shouldReturn := w.getBalanceValue(ctx, rds, member)
	if shouldReturn {
		return
	}

	organizationID, ledgerID, shouldReturn := w.extractAndValidateMember(ctx, member)
	if shouldReturn {
		return
	}

	balance, shouldReturn := w.unmarshalBalance(ctx, member, val)
	if shouldReturn {
		return
	}

	w.syncAndRecordBalance(ctx, member, organizationID, ledgerID, balance, metricFactory)
}

// checkAndHandleExpiredKey checks TTL and removes expired keys
func (w *BalanceSyncWorker) checkAndHandleExpiredKey(ctx context.Context, rds redis.UniversalClient, member string) bool {
	ttl, err := rds.TTL(ctx, member).Result()
	if err != nil {
		w.logger.Warnf("BalanceSyncWorker: TTL error for %s: %v", member, err)
		return true
	}

	if ttl == -2 || ttl == -2*time.Second {
		w.logger.Warnf("BalanceSyncWorker: already-gone key: %s, removing from schedule", member)
		w.removeBalanceSyncKey(ctx, member)

		return true
	}

	return false
}

// getBalanceValue retrieves the balance value from Redis
func (w *BalanceSyncWorker) getBalanceValue(ctx context.Context, rds redis.UniversalClient, member string) (string, bool) {
	val, err := rds.Get(ctx, member).Result()
	if err != nil {
		w.handleGetError(ctx, member, err)
		return "", true
	}

	return val, false
}

// handleGetError handles errors when getting value from Redis
func (w *BalanceSyncWorker) handleGetError(ctx context.Context, member string, err error) {
	if errors.Is(err, redis.Nil) {
		w.logger.Warnf("BalanceSyncWorker: missing key on GET: %s, removing from schedule", member)
		w.removeBalanceSyncKey(ctx, member)

		return
	}

	w.logger.Warnf("BalanceSyncWorker: GET error for %s: %v", member, err)
}

// extractAndValidateMember extracts and validates IDs from member key
func (w *BalanceSyncWorker) extractAndValidateMember(ctx context.Context, member string) (uuid.UUID, uuid.UUID, bool) {
	organizationID, ledgerID, err := w.extractIDsFromMember(member)
	if err != nil {
		w.logger.Warnf("BalanceSyncWorker: extractIDsFromMember error for %s: %v", member, err)
		w.removeBalanceSyncKey(ctx, member)

		return uuid.UUID{}, uuid.UUID{}, true
	}

	return organizationID, ledgerID, false
}

// unmarshalBalance unmarshals balance from JSON string
func (w *BalanceSyncWorker) unmarshalBalance(ctx context.Context, member, val string) (mmodel.BalanceRedis, bool) {
	var balance mmodel.BalanceRedis
	if err := json.Unmarshal([]byte(val), &balance); err != nil {
		w.logger.Warnf("BalanceSyncWorker: Unmarshal error for %s: %v", member, err)
		w.removeBalanceSyncKey(ctx, member)

		return mmodel.BalanceRedis{}, true
	}

	return balance, false
}

// syncAndRecordBalance syncs balance and records metrics
func (w *BalanceSyncWorker) syncAndRecordBalance(ctx context.Context, member string, organizationID, ledgerID uuid.UUID, balance mmodel.BalanceRedis, metricFactory any) {
	synced, err := w.useCase.SyncBalance(ctx, organizationID, ledgerID, balance)
	if err != nil {
		w.logger.Errorf("BalanceSyncWorker: SyncBalance error for member %s with content %+v: %v", member, balance, err)
		return
	}

	if synced {
		if factory, ok := metricFactory.(interface {
			Counter(metric any) interface {
				WithLabels(labels map[string]string) interface {
					AddOne(ctx context.Context)
				}
			}
		}); ok {
			factory.Counter(utils.BalanceSynced).WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
			}).AddOne(ctx)
		}

		w.logger.Infof("BalanceSyncWorker: Synced key %s", member)
	}

	w.removeBalanceSyncKey(ctx, member)
}

// removeBalanceSyncKey removes a balance sync key and logs any errors
func (w *BalanceSyncWorker) removeBalanceSyncKey(ctx context.Context, member string) {
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
		if !w.isDelimiter(i, len(member), member) {
			continue
		}

		seg := w.extractSegment(start, i, member)
		if seg == "" {
			start = i + 1
			continue
		}

		u, ok := w.tryParseUUID(seg)
		if !ok {
			start = i + 1
			continue
		}

		if !haveFirst {
			first = u
			haveFirst = true
			start = i + 1

			continue
		}

		return first, u, nil
	}

	return uuid.UUID{}, uuid.UUID{}, pkg.ValidateInternalError(ErrBalanceSyncKeyMissingUUIDs, "BalanceSyncWorker")
}

// isDelimiter checks if position i is at a delimiter (end of string or colon)
func (w *BalanceSyncWorker) isDelimiter(i, length int, member string) bool {
	return i == length || member[i] == ':'
}

// extractSegment extracts a segment from member between start and i
func (w *BalanceSyncWorker) extractSegment(start, i int, member string) string {
	if i <= start {
		return ""
	}

	return member[start:i]
}

// tryParseUUID attempts to parse a segment as UUID if it has the correct length
func (w *BalanceSyncWorker) tryParseUUID(seg string) (uuid.UUID, bool) {
	if len(seg) != uuidStringLength {
		return uuid.UUID{}, false
	}

	u, err := uuid.Parse(seg)
	if err != nil {
		return uuid.UUID{}, false
	}

	return u, true
}

// panicAsError converts a recovered panic value to an error
func (w *BalanceSyncWorker) panicAsError(rec any) error {
	var panicErr error

	if err, ok := rec.(error); ok {
		panicErr = fmt.Errorf("%w: %w", ErrBalanceSyncPanicRecovered, err)
	} else {
		panicErr = fmt.Errorf("%w: %s", ErrBalanceSyncPanicRecovered, fmt.Sprint(rec))
	}

	return pkg.ValidateInternalError(panicErr, "BalanceSyncWorker")
}
