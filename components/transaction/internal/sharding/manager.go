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
	"sync"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	RouteCacheTTL      time.Duration
	MigrationLockTTL   time.Duration
	MigrationDrainWait time.Duration
	MigrationWaitMax   time.Duration
	MetricsWindow      time.Duration
	IsolationTTL       time.Duration

	ShardMigrationCooldown   time.Duration
	AccountMigrationCooldown time.Duration
}

func defaultConfig() Config {
	return Config{
		RouteCacheTTL:            0,
		MigrationLockTTL:         30 * time.Second,
		MigrationDrainWait:       10 * time.Millisecond,
		MigrationWaitMax:         75 * time.Millisecond,
		MetricsWindow:            60 * time.Second,
		IsolationTTL:             30 * time.Minute,
		ShardMigrationCooldown:   10 * time.Second,
		AccountMigrationCooldown: 5 * time.Minute,
	}
}

type routeCacheEntry struct {
	shardID   int
	expiresAt time.Time
}

type Manager struct {
	conn   *libRedis.RedisConnection
	router *shard.Router
	logger libLog.Logger

	cfg Config

	cacheMu    sync.RWMutex
	routeCache map[string]routeCacheEntry
}

type ShardLoad struct {
	ShardID int
	Load    int64
}

type HotAccount struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Alias          string
	Load           int64
}

type MigrationResult struct {
	Alias        string
	SourceShard  int
	TargetShard  int
	MigratedKeys int
}

func NewManager(conn *libRedis.RedisConnection, router *shard.Router, logger libLog.Logger, cfg Config) *Manager {
	if conn == nil || router == nil {
		return nil
	}

	defaults := defaultConfig()

	if cfg.RouteCacheTTL < 0 {
		cfg.RouteCacheTTL = defaults.RouteCacheTTL
	}

	if cfg.MigrationLockTTL <= 0 {
		cfg.MigrationLockTTL = defaults.MigrationLockTTL
	}

	if cfg.MigrationDrainWait < 0 {
		cfg.MigrationDrainWait = defaults.MigrationDrainWait
	}

	if cfg.MigrationWaitMax <= 0 {
		cfg.MigrationWaitMax = defaults.MigrationWaitMax
	}

	if cfg.MetricsWindow <= 0 {
		cfg.MetricsWindow = defaults.MetricsWindow
	}

	if cfg.IsolationTTL < 0 {
		cfg.IsolationTTL = defaults.IsolationTTL
	}

	if cfg.ShardMigrationCooldown <= 0 {
		cfg.ShardMigrationCooldown = defaults.ShardMigrationCooldown
	}

	if cfg.AccountMigrationCooldown <= 0 {
		cfg.AccountMigrationCooldown = defaults.AccountMigrationCooldown
	}

	return &Manager{
		conn:       conn,
		router:     router,
		logger:     logger,
		cfg:        cfg,
		routeCache: make(map[string]routeCacheEntry),
	}
}

func (m *Manager) Enabled() bool {
	return m != nil && m.router != nil && m.conn != nil
}

func (m *Manager) ResolveBalanceShard(ctx context.Context, organizationID, ledgerID uuid.UUID, alias, balanceKey string) (int, error) {
	if !m.Enabled() {
		return 0, nil
	}

	shardCount := m.router.ShardCount()
	if shardCount <= 0 {
		return 0, fmt.Errorf("invalid shard count")
	}

	if shard.IsExternal(alias) && shard.IsExternalBalanceKey(balanceKey) {
		return m.router.ResolveBalance(alias, balanceKey), nil
	}

	if shardID, ok := m.getRouteCache(organizationID, ledgerID, alias); ok {
		return shardID, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return m.router.ResolveBalance(alias, balanceKey), err
	}

	raw, err := rds.HGet(ctx, utils.ShardRoutingKey(organizationID, ledgerID), alias).Result()
	if err != nil {
		if err == redis.Nil {
			return m.router.ResolveBalance(alias, balanceKey), nil
		}

		return m.router.ResolveBalance(alias, balanceKey), err
	}

	shardID, err := strconv.Atoi(raw)
	if err != nil {
		return m.router.ResolveBalance(alias, balanceKey), err
	}

	if shardID < 0 || shardID >= shardCount {
		return m.router.ResolveBalance(alias, balanceKey), nil
	}

	m.setRouteCache(organizationID, ledgerID, alias, shardID)

	return shardID, nil
}

func (m *Manager) WaitForAliasesUnlocked(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) error {
	if !m.Enabled() || len(aliases) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if alias == "" {
			continue
		}

		unique[alias] = struct{}{}
	}

	if len(unique) == 0 {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(m.cfg.MigrationWaitMax)

	for {
		lockedAlias := ""

		for alias := range unique {
			exists, existsErr := rds.Exists(ctx, utils.MigrationLockKey(organizationID, ledgerID, alias)).Result()
			if existsErr != nil {
				return existsErr
			}

			if exists > 0 {
				lockedAlias = alias
				break
			}
		}

		if lockedAlias == "" {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("account migration in progress for alias %s", lockedAlias)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Millisecond):
		}
	}
}

func (m *Manager) SetRoutingOverride(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, shardID int) error {
	if !m.Enabled() {
		return nil
	}

	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	if shardID < 0 {
		return fmt.Errorf("invalid shard id %d", shardID)
	}

	shardCount := m.router.ShardCount()
	if shardCount <= 0 {
		return fmt.Errorf("invalid shard count")
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	if shardID >= shardCount {
		return fmt.Errorf("invalid shard id %d", shardID)
	}

	normalized := shardID

	if err := rds.HSet(ctx, utils.ShardRoutingKey(organizationID, ledgerID), alias, strconv.Itoa(normalized)).Err(); err != nil {
		return err
	}

	_, _ = rds.Publish(ctx, utils.ShardRoutingUpdatesChannel(organizationID, ledgerID), fmt.Sprintf("%s:%d", alias, normalized)).Result()

	m.setRouteCache(organizationID, ledgerID, alias, normalized)

	return nil
}

func (m *Manager) MigrateAccount(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	alias string,
	targetShard int,
	knownBalanceKeys []string,
) (*MigrationResult, error) {
	if err := m.validateMigrationRequest(alias, targetShard); err != nil {
		return nil, err
	}

	sourceShard, err := m.ResolveBalanceShard(ctx, organizationID, ledgerID, alias, constant.DefaultBalanceKey)
	if err != nil {
		return nil, err
	}

	if sourceShard == targetShard {
		return &MigrationResult{Alias: alias, SourceShard: sourceShard, TargetShard: targetShard, MigratedKeys: 0}, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	migrationKey := utils.MigrationLockKey(organizationID, ledgerID, alias)
	lockValue := fmt.Sprintf("%d->%d", sourceShard, targetShard)

	locked, err := rds.SetNX(ctx, migrationKey, lockValue, m.cfg.MigrationLockTTL).Result()
	if err != nil {
		return nil, err
	}

	if !locked {
		return nil, fmt.Errorf("migration already in progress for alias %s", alias)
	}

	defer m.clearMigrationLock(rds, migrationKey, alias)

	migratedKeys, err := m.executeMigration(ctx, rds, sourceShard, targetShard, organizationID, ledgerID, alias, knownBalanceKeys)
	if err != nil {
		return nil, err
	}

	return &MigrationResult{
		Alias:        alias,
		SourceShard:  sourceShard,
		TargetShard:  targetShard,
		MigratedKeys: migratedKeys,
	}, nil
}

func (m *Manager) RecordShardAliasLoad(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, shardID int, weight int64) error {
	if !m.Enabled() || alias == "" || weight <= 0 {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	nowSec := time.Now().Unix()
	bucket := strconv.FormatInt(nowSec, 10)
	accountBucketField := fmt.Sprintf("%d|%s|%s|%s", nowSec, organizationID.String(), ledgerID.String(), alias)

	metricsKey := utils.ShardMetricsKey(shardID)
	hotAccountsBucketKey := utils.ShardHotAccountsBucketKey(shardID)

	pipe := rds.Pipeline()
	pipe.HIncrBy(ctx, metricsKey, bucket, weight)
	pipe.Expire(ctx, metricsKey, 2*m.cfg.MetricsWindow)
	pipe.HIncrBy(ctx, hotAccountsBucketKey, accountBucketField, weight)
	pipe.Expire(ctx, hotAccountsBucketKey, 2*m.cfg.MetricsWindow)

	_, err = pipe.Exec(ctx)

	return err
}

func (m *Manager) GetShardLoads(ctx context.Context, shardCount int, window time.Duration) ([]ShardLoad, error) {
	if !m.Enabled() {
		return nil, nil
	}

	if shardCount <= 0 {
		shardCount = m.router.ShardCount()
	}

	if window <= 0 {
		window = m.cfg.MetricsWindow
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	nowSec := time.Now().Unix()
	minSec := nowSec - int64(window.Seconds())

	loads := make([]ShardLoad, 0, shardCount)

	for shardID := 0; shardID < shardCount; shardID++ {
		key := utils.ShardMetricsKey(shardID)

		buckets, getErr := rds.HGetAll(ctx, key).Result()
		if getErr != nil {
			if getErr == redis.Nil {
				loads = append(loads, ShardLoad{ShardID: shardID, Load: 0})
				continue
			}

			return nil, getErr
		}

		var total int64

		for rawBucket, rawValue := range buckets {
			bucketSec, parseErr := strconv.ParseInt(rawBucket, 10, 64)
			if parseErr != nil || bucketSec < minSec {
				continue
			}

			count, countErr := strconv.ParseInt(rawValue, 10, 64)
			if countErr != nil {
				continue
			}

			total += count
		}

		loads = append(loads, ShardLoad{ShardID: shardID, Load: total})
	}

	sort.Slice(loads, func(i, j int) bool {
		if loads[i].Load == loads[j].Load {
			return loads[i].ShardID < loads[j].ShardID
		}

		return loads[i].Load > loads[j].Load
	})

	return loads, nil
}

func (m *Manager) TopHotAccounts(ctx context.Context, shardID int, window time.Duration, limit int) ([]HotAccount, error) {
	if !m.Enabled() {
		return nil, nil
	}

	if window <= 0 {
		window = m.cfg.MetricsWindow
	}

	if limit <= 0 {
		limit = 1
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	buckets, err := rds.HGetAll(ctx, utils.ShardHotAccountsBucketKey(shardID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}

		return nil, err
	}

	if len(buckets) == 0 {
		return nil, nil
	}

	minSec := time.Now().Unix() - int64(window.Seconds())
	totals := aggregateHotAccountBuckets(buckets, minSec)

	if len(totals) == 0 {
		return nil, nil
	}

	hotAccounts := sortedHotAccounts(totals)

	if limit >= len(hotAccounts) {
		return hotAccounts, nil
	}

	return hotAccounts[:limit], nil
}

func (m *Manager) SetRebalancerPaused(ctx context.Context, paused bool) error {
	if !m.Enabled() {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	value := "0"
	if paused {
		value = "1"
	}

	return rds.Set(ctx, utils.ShardRebalanceStateKey(), value, 0).Err()
}

func (m *Manager) IsRebalancerPaused(ctx context.Context) (bool, error) {
	if !m.Enabled() {
		return false, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	value, err := rds.Get(ctx, utils.ShardRebalanceStateKey()).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}

		return false, err
	}

	return value == "1", nil
}

func (m *Manager) TryAcquireRebalancePermits(ctx context.Context, sourceShard, targetShard int, account HotAccount) (bool, error) {
	if !m.Enabled() {
		return false, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	sourceKey := utils.ShardRebalanceShardCooldownKey(sourceShard)
	targetKey := utils.ShardRebalanceShardCooldownKey(targetShard)
	accountKey := utils.ShardRebalanceAccountCooldownKey(account.OrganizationID, account.LedgerID, account.Alias)

	sourceOK, err := rds.SetNX(ctx, sourceKey, "1", m.cfg.ShardMigrationCooldown).Result()
	if err != nil {
		return false, err
	}

	if !sourceOK {
		return false, nil
	}

	targetOK, err := rds.SetNX(ctx, targetKey, "1", m.cfg.ShardMigrationCooldown).Result()
	if err != nil {
		_, _ = rds.Del(ctx, sourceKey).Result()

		return false, err
	}

	if !targetOK {
		_, _ = rds.Del(ctx, sourceKey).Result()

		return false, nil
	}

	accountOK, err := rds.SetNX(ctx, accountKey, "1", m.cfg.AccountMigrationCooldown).Result()
	if err != nil {
		_, _ = rds.Del(ctx, sourceKey, targetKey).Result()

		return false, err
	}

	if !accountOK {
		_, _ = rds.Del(ctx, sourceKey, targetKey).Result()

		return false, nil
	}

	return true, nil
}

func (m *Manager) GetShardIsolationCounts(ctx context.Context, shardCount int) (map[int]int64, error) {
	counts := make(map[int]int64)

	if !m.Enabled() {
		return counts, nil
	}

	if shardCount <= 0 {
		shardCount = m.router.ShardCount()
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	for shardID := 0; shardID < shardCount; shardID++ {
		setKey := utils.ShardIsolationSetKey(shardID)

		members, membersErr := rds.SMembers(ctx, setKey).Result()
		if membersErr != nil && membersErr != redis.Nil {
			return nil, membersErr
		}

		activeCount, countErr := m.countActiveIsolationMembers(ctx, rds, setKey, shardID, members)
		if countErr != nil {
			return nil, countErr
		}

		counts[shardID] = activeCount
	}

	return counts, nil
}

func (m *Manager) MarkAccountIsolated(ctx context.Context, account HotAccount, shardID int) error {
	if !m.Enabled() {
		return nil
	}

	if account.Alias == "" {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	member := fmt.Sprintf("%s:%s:%s", account.OrganizationID.String(), account.LedgerID.String(), account.Alias)
	accountKey := utils.ShardIsolationAccountKey(account.OrganizationID, account.LedgerID, account.Alias)

	prevShard, prevErr := rds.Get(ctx, accountKey).Result()
	if prevErr == nil {
		if prevID, parseErr := strconv.Atoi(prevShard); parseErr == nil && prevID >= 0 && prevID != shardID {
			_ = rds.SRem(ctx, utils.ShardIsolationSetKey(prevID), member).Err()
		}
	}

	pipe := rds.Pipeline()
	pipe.SAdd(ctx, utils.ShardIsolationSetKey(shardID), member)
	pipe.Set(ctx, accountKey, strconv.Itoa(shardID), m.cfg.IsolationTTL)
	_, err = pipe.Exec(ctx)

	return err
}

func (m *Manager) getRouteCache(organizationID, ledgerID uuid.UUID, alias string) (int, bool) {
	if m.cfg.RouteCacheTTL <= 0 {
		return 0, false
	}

	cacheKey := routeCacheKey(organizationID, ledgerID, alias)

	m.cacheMu.RLock()
	entry, ok := m.routeCache[cacheKey]
	m.cacheMu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			m.cacheMu.Lock()
			delete(m.routeCache, cacheKey)
			m.cacheMu.Unlock()
		}

		return 0, false
	}

	return entry.shardID, true
}

func (m *Manager) setRouteCache(organizationID, ledgerID uuid.UUID, alias string, shardID int) {
	if m.cfg.RouteCacheTTL <= 0 {
		return
	}

	m.cacheMu.Lock()
	m.routeCache[routeCacheKey(organizationID, ledgerID, alias)] = routeCacheEntry{
		shardID:   shardID,
		expiresAt: time.Now().Add(m.cfg.RouteCacheTTL),
	}
	m.cacheMu.Unlock()
}

func parseIsolationMember(member string) (uuid.UUID, uuid.UUID, string, error) {
	parts := strings.SplitN(member, ":", 3)
	if len(parts) != 3 {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("invalid isolation member format")
	}

	organizationID, orgErr := uuid.Parse(parts[0])
	if orgErr != nil {
		return uuid.Nil, uuid.Nil, "", orgErr
	}

	ledgerID, ledgerErr := uuid.Parse(parts[1])
	if ledgerErr != nil {
		return uuid.Nil, uuid.Nil, "", ledgerErr
	}

	if parts[2] == "" {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("empty alias")
	}

	return organizationID, ledgerID, parts[2], nil
}

func routeCacheKey(organizationID, ledgerID uuid.UUID, alias string) string {
	return organizationID.String() + ":" + ledgerID.String() + ":" + alias
}

func (m *Manager) collectBalanceKeys(
	ctx context.Context,
	rds redis.UniversalClient,
	sourceShard int,
	organizationID, ledgerID uuid.UUID,
	alias string,
	knownBalanceKeys []string,
) ([]string, error) {
	keySet := make(map[string]struct{})

	for _, known := range knownBalanceKeys {
		if known == "" {
			continue
		}

		keySet[known] = struct{}{}
	}

	if len(keySet) == 0 {
		keySet[constant.DefaultBalanceKey] = struct{}{}
	}

	pattern := utils.BalanceShardKey(sourceShard, organizationID, ledgerID, alias+"#*")

	iter := rds.Scan(ctx, 0, pattern, 500).Iterator()
	for iter.Next(ctx) {
		rawKey := iter.Val()

		idx := strings.LastIndexByte(rawKey, ':')
		if idx < 0 || idx+1 >= len(rawKey) {
			continue
		}

		aliasWithKey := rawKey[idx+1:]

		prefix := alias + "#"
		if !strings.HasPrefix(aliasWithKey, prefix) {
			continue
		}

		balanceKey := strings.TrimPrefix(aliasWithKey, prefix)
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		keySet[balanceKey] = struct{}{}
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	balanceKeys := make([]string, 0, len(keySet))
	for balanceKey := range keySet {
		balanceKeys = append(balanceKeys, balanceKey)
	}

	sort.Strings(balanceKeys)

	return balanceKeys, nil
}
