// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// EngineRecoveryCleanupSchedule is tenant-global. The transaction hash tag
// keeps it co-located with every scoped receipt, guard, protection coordinator,
// and recovery hash used by the cleanup script.
const EngineRecoveryCleanupSchedule = "engine:" + cachepolicy.HashTag + ":recovery-cleanup"

const (
	recoveryCleanupNoop        int64 = 0
	recoveryCleanupDeleted     int64 = 1
	recoveryCleanupStale       int64 = 2
	recoveryCleanupRescheduled int64 = 3
)

// recoveryCleanupRetryDelay matches the delay the cleanup script applies to an
// execution blocked by a pending predecessor.
const recoveryCleanupRetryDelay = time.Minute

//go:embed scripts/cleanup_engine_recovery.lua
var cleanupEngineRecoveryLua string

var cleanupEngineRecoveryScript = redis.NewScript(cleanupEngineRecoveryLua)

// RecoveryCleanupResult reports one bounded cleanup pass.
type RecoveryCleanupResult struct {
	Scanned     int
	Cleaned     int
	Stale       int
	Rescheduled int
	// Failed counts executions whose cleanup proof was rejected. They keep every
	// artifact and are retried after recoveryCleanupRetryDelay.
	Failed int
	// FirstFailure is the rejection of the first execution counted in Failed.
	FirstFailure error
}

// RecoveryCleanupBacklog describes the executions whose cleanup is due.
type RecoveryCleanupBacklog struct {
	Due int64
	// OldestDueMs is the earliest due deadline in unix milliseconds, or zero
	// when nothing is due.
	OldestDueMs int64
}

// errRecoveryCleanupRejected marks a failure confined to one scheduled
// execution: its receipt or the cleanup script rejected the proof.
var errRecoveryCleanupRejected = errors.New("engine recovery cleanup proof rejected")

// CleanupEngineRecovery removes only execution artifacts whose frozen cleanup
// proof is due and still agrees with every transaction coordinator. An
// execution whose proof is rejected keeps its artifacts and is rescheduled, so
// it never blocks the executions behind it. Any other failure ends the pass and
// returns the work done so far.
func (rr *RedisConsumerRepository) CleanupEngineRecovery(ctx context.Context, now time.Time, limit int) (RecoveryCleanupResult, error) {
	var result RecoveryCleanupResult
	if err := ctx.Err(); err != nil {
		return result, err
	}

	if now.IsZero() || now.UnixMilli() < 1 || limit < 1 || limit > maxRedisBatchSize {
		return result, fmt.Errorf("invalid engine recovery cleanup request")
	}

	dueKey, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	if err != nil {
		return result, fmt.Errorf("resolve engine recovery cleanup schedule: %w", err)
	}

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return result, fmt.Errorf("get engine recovery cleanup client: %w", err)
	}

	due, err := client.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key: dueKey, Start: "-inf", Stop: strconv.FormatInt(now.UnixMilli(), 10),
		ByScore: true, Offset: 0, Count: int64(limit),
	}).Result()
	if err != nil {
		return result, fmt.Errorf("read engine recovery cleanup schedule: %w", err)
	}

	result.Scanned = len(due)
	for _, entry := range due {
		status, cleanupErr := rr.cleanupEngineRecoveryEntry(ctx, client, dueKey, entry, now)
		if errors.Is(cleanupErr, errRecoveryCleanupRejected) {
			member, _ := entry.Member.(string)

			rescheduled, rescheduleErr := rescheduleRejectedRecoveryCleanup(ctx, client, dueKey, member, now)
			if rescheduleErr != nil {
				return result, rescheduleErr
			}

			if rescheduled {
				result.Failed++
				if result.FirstFailure == nil {
					result.FirstFailure = cleanupErr
				}
			}

			continue
		}

		if cleanupErr != nil {
			return result, cleanupErr
		}

		if err := result.count(status); err != nil {
			return result, err
		}
	}

	return result, nil
}

func (result *RecoveryCleanupResult) count(status int64) error {
	switch status {
	case recoveryCleanupNoop:
	case recoveryCleanupDeleted:
		result.Cleaned++
	case recoveryCleanupStale:
		result.Stale++
	case recoveryCleanupRescheduled:
		result.Rescheduled++
	default:
		return fmt.Errorf("invalid engine recovery cleanup result")
	}

	return nil
}

// EngineRecoveryCleanupBacklog reads how many executions are due for cleanup
// at now and the earliest due deadline, without changing the schedule.
func (rr *RedisConsumerRepository) EngineRecoveryCleanupBacklog(ctx context.Context, now time.Time) (RecoveryCleanupBacklog, error) {
	var backlog RecoveryCleanupBacklog
	if err := ctx.Err(); err != nil {
		return backlog, err
	}

	if now.IsZero() || now.UnixMilli() < 1 {
		return backlog, fmt.Errorf("invalid engine recovery cleanup backlog request")
	}

	dueKey, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	if err != nil {
		return backlog, fmt.Errorf("resolve engine recovery cleanup schedule: %w", err)
	}

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return backlog, fmt.Errorf("get engine recovery cleanup client: %w", err)
	}

	var (
		dueCount *redis.IntCmd
		oldest   *redis.ZSliceCmd
	)

	if _, err := client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		dueCount = pipe.ZCount(ctx, dueKey, "-inf", strconv.FormatInt(now.UnixMilli(), 10))
		oldest = pipe.ZRangeWithScores(ctx, dueKey, 0, 0)

		return nil
	}); err != nil {
		return backlog, fmt.Errorf("read engine recovery cleanup backlog: %w", err)
	}

	backlog.Due = dueCount.Val()
	if backlog.Due > 0 && len(oldest.Val()) == 1 {
		backlog.OldestDueMs = int64(oldest.Val()[0].Score)
	}

	return backlog, nil
}

// rescheduleRejectedRecoveryCleanup moves a rejected execution forward by
// recoveryCleanupRetryDelay. XX never revives a member another cleaner already
// removed, and GT never pulls an already later retry backward; either case
// reports false.
func rescheduleRejectedRecoveryCleanup(
	ctx context.Context,
	client redis.UniversalClient,
	dueKey, member string,
	now time.Time,
) (bool, error) {
	changed, err := client.ZAddArgs(ctx, dueKey, redis.ZAddArgs{
		XX: true, GT: true, Ch: true,
		Members: []redis.Z{{Score: float64(now.Add(recoveryCleanupRetryDelay).UnixMilli()), Member: member}},
	}).Result()
	if err != nil {
		return false, fmt.Errorf("reschedule rejected engine recovery cleanup: %w", err)
	}

	return changed == 1, nil
}

//nolint:gocognit,gocyclo // cleanup validates the complete scoped receipt and key inventory before one atomic script
func (rr *RedisConsumerRepository) cleanupEngineRecoveryEntry(
	ctx context.Context,
	client redis.UniversalClient,
	dueKey string,
	entry redis.Z,
	now time.Time,
) (int64, error) {
	member, ok := entry.Member.(string)
	if !ok {
		return recoveryCleanupNoop, fmt.Errorf("invalid engine recovery cleanup schedule member")
	}

	validScore := entry.Score >= 1 && entry.Score == math.Trunc(entry.Score) && entry.Score <= float64(now.UnixMilli())

	organizationID, ledgerID, executionID, parseErr := parseRecoveryCleanupMember(member)
	if !validScore || parseErr != nil {
		removed, err := client.ZRem(ctx, dueKey, member).Result()
		if err != nil {
			return recoveryCleanupNoop, fmt.Errorf("remove invalid engine recovery cleanup member: %w", err)
		}

		if removed == 0 {
			return recoveryCleanupNoop, nil
		}

		return recoveryCleanupStale, nil
	}

	scope := organizationID.String() + ":" + ledgerID.String()

	keys, err := tenantKeysFromContext(ctx, []string{
		EngineRecoveryCleanupSchedule,
		TransactionBackupQueue,
		cachepolicy.EngineRecoverQueue,
		"engine:" + cachepolicy.HashTag + ":receipts:" + scope,
		"engine:" + cachepolicy.HashTag + ":guards:" + scope,
		"engine:" + cachepolicy.HashTag + ":protection:" + scope,
		"engine:" + cachepolicy.HashTag + ":evidence:" + scope,
		"engine:" + cachepolicy.HashTag + ":transaction-index:" + scope,
	})
	if err != nil {
		return recoveryCleanupNoop, fmt.Errorf("resolve engine recovery cleanup keys: %w", err)
	}

	rawReceipt, readErr := client.HGet(ctx, keys[3], executionID.String()).Bytes()
	if readErr != nil && !errors.Is(readErr, redis.Nil) {
		return recoveryCleanupNoop, fmt.Errorf("read engine recovery cleanup receipt: %w", readErr)
	}

	if readErr == nil {
		var receipt struct {
			Protection struct {
				Transactions []uuid.UUID `json:"transactions"`
				Scopes       []struct {
					OrganizationID uuid.UUID `json:"organizationId"`
					LedgerID       uuid.UUID `json:"ledgerId"`
				} `json:"scopes"`
			} `json:"protection"`
		}
		if json.Unmarshal(rawReceipt, &receipt) == nil {
			if len(receipt.Protection.Scopes) > 0 && len(receipt.Protection.Scopes) != len(receipt.Protection.Transactions) {
				return recoveryCleanupNoop, fmt.Errorf("%w: invalid receipt scopes", errRecoveryCleanupRejected)
			}

			for index, transactionID := range receipt.Protection.Transactions {
				if transactionID == uuid.Nil {
					break
				}

				partScope := scope

				if len(receipt.Protection.Scopes) > 0 {
					part := receipt.Protection.Scopes[index]
					if part.OrganizationID == uuid.Nil || part.LedgerID == uuid.Nil {
						return recoveryCleanupNoop, fmt.Errorf("%w: invalid transaction scope", errRecoveryCleanupRejected)
					}

					partScope = part.OrganizationID.String() + ":" + part.LedgerID.String()
				}

				materialized, keyErr := tenantKeyFromContextOrError(ctx,
					"engine:"+cachepolicy.HashTag+":materialized:"+partScope+":"+transactionID.String())
				if keyErr != nil {
					return recoveryCleanupNoop, fmt.Errorf("resolve materialized transaction cleanup key: %w", keyErr)
				}

				keys = append(keys, materialized)
			}

			if len(receipt.Protection.Scopes) > 0 {
				for _, part := range receipt.Protection.Scopes {
					partScope := part.OrganizationID.String() + ":" + part.LedgerID.String()

					partKeys, keyErr := tenantKeysFromContext(ctx, []string{
						"engine:" + cachepolicy.HashTag + ":guards:" + partScope,
						"engine:" + cachepolicy.HashTag + ":protection:" + partScope,
						"engine:" + cachepolicy.HashTag + ":evidence:" + partScope,
						"engine:" + cachepolicy.HashTag + ":transaction-index:" + partScope,
					})
					if keyErr != nil {
						return recoveryCleanupNoop, fmt.Errorf("resolve scoped engine recovery cleanup keys: %w", keyErr)
					}

					keys = append(keys, partKeys...)
				}
			}
		}
	}

	status, err := cleanupEngineRecoveryScript.Run(
		ctx, client, keys,
		member,
		strconv.FormatInt(int64(entry.Score), 10),
		strconv.FormatInt(now.UnixMilli(), 10),
		tmcore.GetTenantIDContext(ctx),
		organizationID.String(),
		ledgerID.String(),
		executionID.String(),
	).Int64()
	if isRecoveryCleanupScriptRejection(err) {
		return recoveryCleanupNoop, fmt.Errorf("%w: %w", errRecoveryCleanupRejected, err)
	}

	if err != nil {
		return recoveryCleanupNoop, fmt.Errorf("cleanup engine recovery execution: %w", err)
	}

	return status, nil
}

// isRecoveryCleanupScriptRejection reports whether the cleanup script itself
// refused the execution: one of its proof replies, a type mismatch, or a Lua
// runtime error while decoding the execution's artifacts. Transport failures,
// cancellation, and server states such as LOADING, READONLY, BUSY, or OOM are
// not entry failures and must end the pass.
func isRecoveryCleanupScriptRejection(err error) bool {
	var reply redis.Error
	if err == nil || errors.Is(err, redis.Nil) || !errors.As(err, &reply) {
		return false
	}

	message := reply.Error()

	return strings.HasPrefix(message, "ERR ") || strings.HasPrefix(message, "WRONGTYPE ")
}

func recoveryCleanupMember(organizationID, ledgerID, executionID uuid.UUID) string {
	return organizationID.String() + ":" + ledgerID.String() + ":" + executionID.String()
}

func parseRecoveryCleanupMember(member string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	parts := strings.Split(member, ":")
	if len(parts) != 3 {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("invalid recovery cleanup member")
	}

	ids := make([]uuid.UUID, len(parts))
	for index, raw := range parts {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("invalid canonical recovery cleanup member")
		}

		ids[index] = id
	}

	if recoveryCleanupMember(ids[0], ids[1], ids[2]) != member {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("invalid canonical recovery cleanup member")
	}

	return ids[0], ids[1], ids[2], nil
}
