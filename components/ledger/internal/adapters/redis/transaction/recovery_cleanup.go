// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
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

//go:embed scripts/cleanup_engine_recovery.lua
var cleanupEngineRecoveryLua string

var cleanupEngineRecoveryScript = redis.NewScript(cleanupEngineRecoveryLua)

// RecoveryCleanupResult reports one bounded cleanup pass.
type RecoveryCleanupResult struct {
	Scanned     int
	Cleaned     int
	Stale       int
	Rescheduled int
}

// CleanupEngineRecovery removes only execution artifacts whose frozen cleanup
// proof is due and still agrees with every transaction coordinator.
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
		if cleanupErr != nil {
			return result, cleanupErr
		}

		switch status {
		case recoveryCleanupNoop:
		case recoveryCleanupDeleted:
			result.Cleaned++
		case recoveryCleanupStale:
			result.Stale++
		case recoveryCleanupRescheduled:
			result.Rescheduled++
		default:
			return result, fmt.Errorf("invalid engine recovery cleanup result")
		}
	}

	return result, nil
}

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
	})
	if err != nil {
		return recoveryCleanupNoop, fmt.Errorf("resolve engine recovery cleanup keys: %w", err)
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
	if err != nil {
		return recoveryCleanupNoop, fmt.Errorf("cleanup engine recovery execution: %w", err)
	}

	return status, nil
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
