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

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const MaxBalanceSyncWorkers = 25

// BalanceSyncWorker continuously processes keys scheduled for pre-expiry actions.
// Ensures that the balance is synced before the key expires.
type BalanceSyncWorker struct {
	redisConn *libRedis.RedisConnection
	logger    libLog.Logger
	idleWait  time.Duration
	batchSize int64
	useCase   *command.UseCase
}

func NewBalanceSyncWorker(conn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase) *BalanceSyncWorker {
	return &BalanceSyncWorker{
		redisConn: conn,
		logger:    logger,
		idleWait:  600 * time.Second,
		batchSize: 25,
		useCase:   useCase,
	}
}

func (w *BalanceSyncWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Info("BalanceSyncWorker started")

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

func (w *BalanceSyncWorker) shouldShutdown(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func (w *BalanceSyncWorker) processBalancesToExpire(ctx context.Context, rds redis.UniversalClient) bool {
	now := time.Now().Unix()

	members, err := w.useCase.RedisRepo.GetBalanceSyncKeys(ctx, now, w.batchSize)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			w.logger.Warnf("BalanceSyncWorker: get balance sync keys error: %v", err)
		}

		return false
	}

	if len(members) == 0 {
		return false
	}

	workers := MaxBalanceSyncWorkers
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

	if ttl == -2 {
		w.logger.Warnf("BalanceSyncWorker: already-gone key: %s, removing from schedule", member)

		if remErr := w.useCase.RedisRepo.RemoveBalanceSyncKey(ctx, member); remErr != nil {
			w.logger.Warnf("BalanceSyncWorker: failed to remove expired balance sync key %s: %v", member, remErr)
		}

		return
	}

	val, err := rds.Get(ctx, member).Result()
	if err != nil {
		w.logger.Warnf("BalanceSyncWorker: GET error for %s: %v", member, err)

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
