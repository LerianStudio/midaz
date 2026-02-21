// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package sharding

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// validateMigrationRequest checks all preconditions for a migration request
// before any Redis operations are performed.
func (m *Manager) validateMigrationRequest(alias string, targetShard int) error {
	if !m.Enabled() {
		return fmt.Errorf("sharding manager not enabled")
	}

	if m.router.ShardCount() <= 0 {
		return fmt.Errorf("invalid shard count")
	}

	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	if strings.ContainsAny(alias, "*?[]") {
		return fmt.Errorf("alias contains invalid wildcard characters")
	}

	if shard.IsExternal(alias) {
		return fmt.Errorf("external aliases cannot be migrated")
	}

	if targetShard < 0 || targetShard >= m.router.ShardCount() {
		return fmt.Errorf("target shard %d out of range", targetShard)
	}

	return nil
}

// clearMigrationLock removes the migration lock key from Redis. It is intended
// to be called via defer to ensure the lock is always released.
func (m *Manager) clearMigrationLock(rds redis.UniversalClient, migrationKey, alias string) {
	if derr := rds.Del(context.Background(), migrationKey).Err(); derr != nil && m.logger != nil {
		m.logger.Warnf("failed to clear migration lock for alias %s: %v", alias, derr)
	}
}

// executeMigration performs the actual data migration once the lock has been
// acquired. It drains in-flight operations, copies balance keys from source
// to target shard, updates routing, and cleans up the source keys.
func (m *Manager) executeMigration(
	ctx context.Context,
	rds redis.UniversalClient,
	sourceShard, targetShard int,
	organizationID, ledgerID uuid.UUID,
	alias string,
	knownBalanceKeys []string,
) (int, error) {
	if err := m.waitForDrain(ctx); err != nil {
		return 0, err
	}

	balanceKeys, err := m.collectBalanceKeys(ctx, rds, sourceShard, organizationID, ledgerID, alias, knownBalanceKeys)
	if err != nil {
		return 0, err
	}

	sourceKeys, err := m.migrateBalanceKeys(ctx, rds, sourceShard, targetShard, organizationID, ledgerID, alias, balanceKeys)
	if err != nil {
		return 0, err
	}

	if err := m.SetRoutingOverride(ctx, organizationID, ledgerID, alias, targetShard); err != nil {
		return 0, err
	}

	if len(sourceKeys) > 0 {
		if err := rds.Del(ctx, sourceKeys...).Err(); err != nil {
			return 0, err
		}
	}

	return len(sourceKeys), nil
}

// waitForDrain pauses for the configured MigrationDrainWait duration to let
// in-flight operations on the source shard complete before migration proceeds.
func (m *Manager) waitForDrain(ctx context.Context) error {
	if m.cfg.MigrationDrainWait <= 0 {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(m.cfg.MigrationDrainWait):
		return nil
	}
}

// migrateBalanceKeys copies each balance key from sourceShard to targetShard,
// preserving the original TTL (persistent keys stay persistent). It returns the
// list of source Redis keys that were successfully copied so the caller can
// delete them after routing is updated.
func (m *Manager) migrateBalanceKeys(
	ctx context.Context,
	rds redis.UniversalClient,
	sourceShard, targetShard int,
	organizationID, ledgerID uuid.UUID,
	alias string,
	balanceKeys []string,
) ([]string, error) {
	sourceKeys := make([]string, 0, len(balanceKeys))

	for _, balanceKey := range balanceKeys {
		aliasKey := alias + "#" + balanceKey

		sourceRedisKey := utils.BalanceShardKey(sourceShard, organizationID, ledgerID, aliasKey)
		targetRedisKey := utils.BalanceShardKey(targetShard, organizationID, ledgerID, aliasKey)

		value, err := rds.Get(ctx, sourceRedisKey).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}

			return nil, err
		}

		ttl, err := rds.TTL(ctx, sourceRedisKey).Result()
		if err != nil {
			return nil, err
		}

		// TTL of -1 means the key has no expiration (persistent).
		// TTL of -2 means the key does not exist (already handled above).
		// Any positive value is preserved as-is.
		if ttl < 0 {
			ttl = 0
		}

		if err := rds.Set(ctx, targetRedisKey, value, ttl).Err(); err != nil {
			return nil, err
		}

		sourceKeys = append(sourceKeys, sourceRedisKey)
	}

	return sourceKeys, nil
}

// countActiveIsolationMembers counts how many members of the isolation set
// still have a valid (non-expired) per-account isolation marker key. Stale
// members whose TTL-based marker has expired are removed from the set.
func (m *Manager) countActiveIsolationMembers(
	ctx context.Context,
	rds redis.UniversalClient,
	setKey string,
	_ int,
	members []string,
) (int64, error) {
	var activeCount int64

	for _, member := range members {
		orgID, ledgerID, alias, parseErr := parseIsolationMember(member)
		if parseErr != nil {
			_ = rds.SRem(ctx, setKey, member).Err()

			continue
		}

		accountKey := utils.ShardIsolationAccountKey(orgID, ledgerID, alias)

		exists, existsErr := rds.Exists(ctx, accountKey).Result()
		if existsErr != nil {
			return 0, existsErr
		}

		if exists > 0 {
			activeCount++
		} else {
			_ = rds.SRem(ctx, setKey, member).Err()
		}
	}

	return activeCount, nil
}

// hotAccountKey is a composite key used to aggregate per-second hot-account
// buckets into a single total across the metrics window.
type hotAccountKey struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Alias          string
}

// aggregateHotAccountBuckets takes the raw hash map from Redis
// (field: "epoch|orgID|ledgerID|alias", value: count) and sums the values
// per unique account, filtering out buckets older than minSec.
func aggregateHotAccountBuckets(buckets map[string]string, minSec int64) map[hotAccountKey]int64 {
	totals := make(map[hotAccountKey]int64)

	for field, rawValue := range buckets {
		// Field format: "epoch|organizationID|ledgerID|alias"
		parts := strings.SplitN(field, "|", 4)
		if len(parts) != 4 {
			continue
		}

		bucketSec, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil || bucketSec < minSec {
			continue
		}

		orgID, orgErr := uuid.Parse(parts[1])
		if orgErr != nil {
			continue
		}

		ledgerID, ledgerErr := uuid.Parse(parts[2])
		if ledgerErr != nil {
			continue
		}

		alias := parts[3]
		if alias == "" {
			continue
		}

		count, countErr := strconv.ParseInt(rawValue, 10, 64)
		if countErr != nil {
			continue
		}

		key := hotAccountKey{
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Alias:          alias,
		}

		totals[key] += count
	}

	return totals
}

// sortedHotAccounts converts the aggregated totals map into a sorted slice of
// HotAccount values, ordered by load descending (ties broken by alias ascending).
func sortedHotAccounts(totals map[hotAccountKey]int64) []HotAccount {
	accounts := make([]HotAccount, 0, len(totals))

	for key, load := range totals {
		accounts = append(accounts, HotAccount{
			OrganizationID: key.OrganizationID,
			LedgerID:       key.LedgerID,
			Alias:          key.Alias,
			Load:           load,
		})
	}

	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Load == accounts[j].Load {
			return accounts[i].Alias < accounts[j].Alias
		}

		return accounts[i].Load > accounts[j].Load
	})

	return accounts
}
