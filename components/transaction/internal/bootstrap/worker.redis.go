package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/commons/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/redis/go-redis/v9"
)

// BalancePreExpireWorker continuously processes keys scheduled for pre-expiry actions.
// WHY: Ensures we can run a custom action shortly before a key (balance cache) expires.
type BalancePreExpireWorker struct {
	redisConn   *libRedis.RedisConnection
	logger      libLog.Logger
	scheduleKey string
	idleWait    time.Duration
	batchSize   int64
	useCase     *command.UseCase
}

func NewBalancePreExpireWorker(conn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase) *BalancePreExpireWorker {
	return &BalancePreExpireWorker{
		redisConn:   conn,
		logger:      logger,
		scheduleKey: "schedule:{balance-pre-expire}",
		idleWait:    15 * time.Second,
		batchSize:   100,
	}
}

// fetchAndRemoveDueScript atomically fetches and removes due items up to now.
var fetchAndRemoveDueScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2]) or 100
local due = redis.call('ZRANGEBYSCORE', key, '-inf', now, 'WITHSCORES', 'LIMIT', 0, limit)
if #due == 0 then return {} end
local members = {}
for i = 1, #due, 2 do table.insert(members, due[i]) end
if #members > 0 then redis.call('ZREM', key, unpack(members)) end
return due
`)

func (w *BalancePreExpireWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Errorf("PreExpireWorker: failed to get redis client: %v", err)
		return err
	}

	w.logger.Info("PreExpireWorker started")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("PreExpireWorker: shutting down...")
			return nil
		default:
		}

		now := time.Now().Unix()
		raw, err := fetchAndRemoveDueScript.Run(ctx, rds, []string{w.scheduleKey}, now, w.batchSize).Result()

		if err == nil {
			vals, _ := raw.([]any)
			if len(vals) > 0 {
				// Lua returned [member1, score1, member2, score2, ...]
				for i := 0; i+1 < len(vals); i += 2 {
					select {
					case <-ctx.Done():
						w.logger.Info("PreExpireWorker: shutting down...")

						return nil
					default:
					}

					member, _ := vals[i].(string)
					if member == "" {
						continue
					}

					// Check TTL > 0
					ttl, err := rds.TTL(ctx, member).Result()
					if err != nil {
						w.logger.Warnf("PreExpireWorker: TTL error for %s: %v", member, err)

						continue
					}

					if ttl == -2*time.Second { // Redis sentinel value: -2 means already-gone
						w.logger.Warnf("PreExpireWorker: already-gone key: %s", member)

						continue
					}

					val, err := rds.Get(ctx, member).Result()
					if err != nil {
						w.logger.Warnf("PreExpireWorker: GET error for %s: %v", member, err)

						continue
					}

					w.logger.Infof("PreExpireWorker: pre-expire action for %s (ttl=%s) value-size=%d", member, ttl.String(), len(val))

					organizationID, ledgerID, err := w.extractIDsFromMember(member)
					if err != nil {
						w.logger.Warnf("PreExpireWorker: extractIDsFromMember error for %s: %v", member, err)

						continue
					}

					w.logger.Infof("PreExpireWorker: organizationID: %s, ledgerID: %s", organizationID, ledgerID)

					var balance mmodel.BalanceRedis

					err = json.Unmarshal([]byte(val), &balance)
					if err != nil {
						w.logger.Warnf("PreExpireWorker: Unmarshal error for %s: %v", member, err)

						continue
					}

					// i think i need to create a use case for this particular scenario

				}

				continue
			}
		} else if !errors.Is(err, redis.Nil) {
			w.logger.Warnf("PreExpireWorker: fetchAndRemoveDueScript error: %v", err)
		}

		// Look up next scheduled and wait/back off
		next, err := rds.ZRangeWithScores(ctx, w.scheduleKey, 0, 0).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			w.logger.Warnf("PreExpireWorker: ZRangeWithScores error: %v", err)

			if waitOrDone(ctx, w.idleWait) {
				w.logger.Info("PreExpireWorker: shutting down...")

				return nil
			}

			continue
		}

		if len(next) == 0 {
			// Nothing scheduled; back off.
			if waitOrDone(ctx, w.idleWait) {
				w.logger.Info("PreExpireWorker: shutting down...")
				return nil
			}

			continue
		}

		w.logger.Infof("PreExpireWorker: next: %+v", next[0])

		if w.waitUntilDue(ctx, int64(next[0].Score)) {
			w.logger.Info("PreExpireWorker: shutting down...")
			return nil
		}
	}
}

// waitOrDone waits for d or returns true if ctx is done first.
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

// waitUntilDue waits until the given dueAtUnix time.
// Returns true if the context was cancelled while waiting.
func (w *BalancePreExpireWorker) waitUntilDue(ctx context.Context, dueAtUnix int64) bool {
	nowUnix := time.Now().Unix()
	if dueAtUnix <= nowUnix {
		return false
	}

	waitFor := time.Duration(dueAtUnix-nowUnix) * time.Second
	if waitFor <= 0 {
		return false
	}

	return waitOrDone(ctx, waitFor)
}

// extractIDsFromMember parses a Redis member key that follows the pattern
// "lock:<organizationID>:<ledgerID>:@<suffix>" and returns organizationID and ledgerID.
// WHY: Organization and ledger identifiers are required to route balance updates.
func (w *BalancePreExpireWorker) extractIDsFromMember(member string) (organizationID string, ledgerID string, err error) {
	if member == "" {
		return "", "", fmt.Errorf("empty member")
	}

	parts := strings.Split(member, ":")

	if len(parts) < 4 { // expect: lock, orgID, ledgerID, suffix
		return "", "", fmt.Errorf("invalid member format: %q", member)
	}

	organizationID = parts[1]
	ledgerID = parts[2]

	if organizationID == "" || ledgerID == "" {
		return "", "", fmt.Errorf("missing ids in member: %q", member)
	}

	return organizationID, ledgerID, nil
}
