// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package sharding

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

var (
	// ErrInvalidShardCount is returned when the shard count is zero or negative.
	ErrInvalidShardCount = errors.New("invalid shard count")
	// ErrAliasRequired is returned when the alias is empty.
	ErrAliasRequired = errors.New("alias is required")
	// ErrInvalidIsolationMemberFormat is returned when an isolation set member cannot be parsed.
	ErrInvalidIsolationMemberFormat = errors.New("invalid isolation member format")
	// ErrEmptyAlias is returned when the parsed alias component of an isolation member is empty.
	ErrEmptyAlias = errors.New("empty alias")
	// ErrInvalidShardID is returned when a shard ID is out of range.
	ErrInvalidShardID = errors.New("invalid shard id")
	// ErrMigrationInProgress is returned when a migration lock is already held for the alias.
	ErrMigrationInProgress = errors.New("account migration in progress")
)

// Config holds configuration for the sharding Manager.
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

// Default configuration constants for the sharding Manager.
const (
	defaultMigrationLockTTLSec         = 30
	defaultMigrationDrainWaitMs        = 10
	defaultMigrationWaitMaxMs          = 75
	defaultMetricsWindowSec            = 60
	defaultIsolationTTLMin             = 30
	defaultShardMigrationCooldownSec   = 10
	defaultAccountMigrationCooldownMin = 5
)

func defaultConfig() Config {
	return Config{
		RouteCacheTTL:            0,
		MigrationLockTTL:         defaultMigrationLockTTLSec * time.Second,
		MigrationDrainWait:       defaultMigrationDrainWaitMs * time.Millisecond,
		MigrationWaitMax:         defaultMigrationWaitMaxMs * time.Millisecond,
		MetricsWindow:            defaultMetricsWindowSec * time.Second,
		IsolationTTL:             defaultIsolationTTLMin * time.Minute,
		ShardMigrationCooldown:   defaultShardMigrationCooldownSec * time.Second,
		AccountMigrationCooldown: defaultAccountMigrationCooldownMin * time.Minute,
	}
}

type routeCacheEntry struct {
	shardID   int
	expiresAt time.Time
}

// Manager provides shard routing, migration, and load tracking functionality.
type Manager struct {
	conn   *libRedis.RedisConnection
	router *shard.Router
	logger libLog.Logger

	cfg Config

	cacheMu    sync.RWMutex
	routeCache map[string]routeCacheEntry
}

// ShardLoad represents the observed load on a single shard.
type ShardLoad struct {
	ShardID int
	Load    int64
}

// HotAccount represents an account that has generated significant load on a shard.
type HotAccount struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Alias          string
	Load           int64
}

// MigrationResult describes the outcome of a shard account migration.
type MigrationResult struct {
	Alias        string
	SourceShard  int
	TargetShard  int
	MigratedKeys int
}

// NewManager creates a new sharding Manager. Returns nil if conn or router is nil.
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

// Enabled reports whether the Manager has been initialized with valid dependencies.
func (m *Manager) Enabled() bool {
	return m != nil && m.router != nil && m.conn != nil
}

// ResolveBalanceShard resolves the shard ID for a given balance key.
func (m *Manager) ResolveBalanceShard(ctx context.Context, organizationID, ledgerID uuid.UUID, alias, balanceKey string) (int, error) {
	if !m.Enabled() {
		return 0, nil
	}

	shardCount := m.router.ShardCount()
	if shardCount <= 0 {
		return 0, ErrInvalidShardCount
	}

	if shard.IsExternal(alias) && shard.IsExternalBalanceKey(balanceKey) {
		return m.router.ResolveBalance(alias, balanceKey), nil
	}

	if shardID, ok := m.getRouteCache(organizationID, ledgerID, alias); ok {
		return shardID, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return m.router.ResolveBalance(alias, balanceKey), fmt.Errorf("resolve balance shard: get redis client: %w", err)
	}

	raw, err := rds.HGet(ctx, utils.ShardRoutingKey(organizationID, ledgerID), alias).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return m.router.ResolveBalance(alias, balanceKey), nil
		}

		return m.router.ResolveBalance(alias, balanceKey), fmt.Errorf("resolve balance shard: hget routing key: %w", err)
	}

	shardID, err := strconv.Atoi(raw)
	if err != nil {
		return m.router.ResolveBalance(alias, balanceKey), fmt.Errorf("resolve balance shard: parse shard id: %w", err)
	}

	if shardID < 0 || shardID >= shardCount {
		return m.router.ResolveBalance(alias, balanceKey), nil
	}

	m.setRouteCache(organizationID, ledgerID, alias, shardID)

	return shardID, nil
}

// WaitForAliasesUnlocked blocks until all provided aliases are no longer locked for migration.
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
		return fmt.Errorf("wait for aliases unlocked: get redis client: %w", err)
	}

	deadline := time.Now().UTC().Add(m.cfg.MigrationWaitMax)

	for {
		lockedAlias := ""

		for alias := range unique {
			exists, existsErr := rds.Exists(ctx, utils.MigrationLockKey(organizationID, ledgerID, alias)).Result()
			if existsErr != nil {
				return fmt.Errorf("wait for aliases unlocked: exists check: %w", existsErr)
			}

			if exists > 0 {
				lockedAlias = alias
				break
			}
		}

		if lockedAlias == "" {
			return nil
		}

		if time.Now().UTC().After(deadline) {
			return fmt.Errorf("%w: alias %s", ErrMigrationInProgress, lockedAlias)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for aliases unlocked: %w", ctx.Err())
		case <-time.After(1 * time.Millisecond):
		}
	}
}

// SetRoutingOverride explicitly routes an alias to a specific shard, overriding the default hash assignment.
func (m *Manager) SetRoutingOverride(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, shardID int) error {
	if !m.Enabled() {
		return nil
	}

	if alias == "" {
		return ErrAliasRequired
	}

	if shardID < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidShardID, shardID)
	}

	shardCount := m.router.ShardCount()
	if shardCount <= 0 {
		return ErrInvalidShardCount
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("set routing override: get redis client: %w", err)
	}

	if shardID >= shardCount {
		return fmt.Errorf("%w: %d", ErrInvalidShardID, shardID)
	}

	normalized := shardID

	if err := rds.HSet(ctx, utils.ShardRoutingKey(organizationID, ledgerID), alias, strconv.Itoa(normalized)).Err(); err != nil {
		return fmt.Errorf("set routing override: hset: %w", err)
	}

	_, _ = rds.Publish(ctx, utils.ShardRoutingUpdatesChannel(organizationID, ledgerID), fmt.Sprintf("%s:%d", alias, normalized)).Result()

	m.setRouteCache(organizationID, ledgerID, alias, normalized)

	return nil
}

// MigrateAccount moves an alias from its current shard to the specified target shard.
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
		return nil, fmt.Errorf("migrate account: get redis client: %w", err)
	}

	migrationKey := utils.MigrationLockKey(organizationID, ledgerID, alias)
	lockValue := fmt.Sprintf("%d->%d", sourceShard, targetShard)

	locked, err := rds.SetNX(ctx, migrationKey, lockValue, m.cfg.MigrationLockTTL).Result()
	if err != nil {
		return nil, fmt.Errorf("migrate account: acquire lock: %w", err)
	}

	if !locked {
		return nil, fmt.Errorf("%w: alias %s", ErrMigrationInProgress, alias)
	}

	defer m.clearMigrationLock(ctx, rds, migrationKey, alias)

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

// RecordShardAliasLoad records a load metric for the given alias on the given shard.
func (m *Manager) RecordShardAliasLoad(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, shardID int, weight int64) error {
	if !m.Enabled() || alias == "" || weight <= 0 {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("record shard alias load: get redis client: %w", err)
	}

	nowSec := time.Now().UTC().Unix()
	bucket := strconv.FormatInt(nowSec, 10)
	accountBucketField := fmt.Sprintf("%d|%s|%s|%s", nowSec, organizationID.String(), ledgerID.String(), alias)

	metricsKey := utils.ShardMetricsKey(shardID)
	hotAccountsBucketKey := utils.ShardHotAccountsBucketKey(shardID)

	const metricsRetentionMultiplier = 2

	pipe := rds.Pipeline()
	pipe.HIncrBy(ctx, metricsKey, bucket, weight)
	pipe.Expire(ctx, metricsKey, metricsRetentionMultiplier*m.cfg.MetricsWindow)
	pipe.HIncrBy(ctx, hotAccountsBucketKey, accountBucketField, weight)
	pipe.Expire(ctx, hotAccountsBucketKey, metricsRetentionMultiplier*m.cfg.MetricsWindow)

	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("record shard alias load: exec pipeline: %w", err)
	}

	return nil
}

// GetShardLoads returns the total request load observed on each shard within the given window.
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
		return nil, fmt.Errorf("get shard loads: get redis client: %w", err)
	}

	nowSec := time.Now().UTC().Unix()
	minSec := nowSec - int64(window.Seconds())

	loads := make([]ShardLoad, 0, shardCount)

	for shardID := 0; shardID < shardCount; shardID++ {
		key := utils.ShardMetricsKey(shardID)

		buckets, getErr := rds.HGetAll(ctx, key).Result()
		if getErr != nil {
			if errors.Is(getErr, redis.Nil) {
				loads = append(loads, ShardLoad{ShardID: shardID, Load: 0})
				continue
			}

			return nil, fmt.Errorf("get shard loads: hgetall: %w", getErr)
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

// TopHotAccounts returns the top-N accounts by load on the given shard within the window.
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
		return nil, fmt.Errorf("top hot accounts: get redis client: %w", err)
	}

	buckets, err := rds.HGetAll(ctx, utils.ShardHotAccountsBucketKey(shardID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		return nil, fmt.Errorf("top hot accounts: hgetall: %w", err)
	}

	if len(buckets) == 0 {
		return nil, nil
	}

	minSec := time.Now().UTC().Unix() - int64(window.Seconds())
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

// SetRebalancerPaused sets the rebalancer paused flag in Redis.
func (m *Manager) SetRebalancerPaused(ctx context.Context, paused bool) error {
	if !m.Enabled() {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("set rebalancer paused: get redis client: %w", err)
	}

	value := "0"
	if paused {
		value = "1"
	}

	if err := rds.Set(ctx, utils.ShardRebalanceStateKey(), value, 0).Err(); err != nil {
		return fmt.Errorf("set rebalancer paused: set: %w", err)
	}

	return nil
}

// IsRebalancerPaused returns true if the rebalancer has been paused via SetRebalancerPaused.
func (m *Manager) IsRebalancerPaused(ctx context.Context) (bool, error) {
	if !m.Enabled() {
		return false, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return false, fmt.Errorf("is rebalancer paused: get redis client: %w", err)
	}

	value, err := rds.Get(ctx, utils.ShardRebalanceStateKey()).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}

		return false, fmt.Errorf("is rebalancer paused: get: %w", err)
	}

	return value == "1", nil
}

// TryAcquireRebalancePermits atomically acquires the source shard, target shard,
// and account cooldown locks. Returns false (without error) when any lock is held.
func (m *Manager) TryAcquireRebalancePermits(ctx context.Context, sourceShard, targetShard int, account HotAccount) (bool, error) {
	if !m.Enabled() {
		return false, nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return false, fmt.Errorf("try acquire rebalance permits: get redis client: %w", err)
	}

	sourceKey := utils.ShardRebalanceShardCooldownKey(sourceShard)
	targetKey := utils.ShardRebalanceShardCooldownKey(targetShard)
	accountKey := utils.ShardRebalanceAccountCooldownKey(account.OrganizationID, account.LedgerID, account.Alias)

	sourceOK, err := rds.SetNX(ctx, sourceKey, "1", m.cfg.ShardMigrationCooldown).Result()
	if err != nil {
		return false, fmt.Errorf("try acquire rebalance permits: source setnx: %w", err)
	}

	if !sourceOK {
		return false, nil
	}

	targetOK, err := rds.SetNX(ctx, targetKey, "1", m.cfg.ShardMigrationCooldown).Result()
	if err != nil {
		_, _ = rds.Del(ctx, sourceKey).Result()

		return false, fmt.Errorf("try acquire rebalance permits: target setnx: %w", err)
	}

	if !targetOK {
		_, _ = rds.Del(ctx, sourceKey).Result()

		return false, nil
	}

	accountOK, err := rds.SetNX(ctx, accountKey, "1", m.cfg.AccountMigrationCooldown).Result()
	if err != nil {
		_, _ = rds.Del(ctx, sourceKey, targetKey).Result()

		return false, fmt.Errorf("try acquire rebalance permits: account setnx: %w", err)
	}

	if !accountOK {
		_, _ = rds.Del(ctx, sourceKey, targetKey).Result()

		return false, nil
	}

	return true, nil
}

// GetShardIsolationCounts returns the number of isolated accounts per shard.
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
		return nil, fmt.Errorf("get shard isolation counts: get redis client: %w", err)
	}

	for shardID := 0; shardID < shardCount; shardID++ {
		setKey := utils.ShardIsolationSetKey(shardID)

		members, membersErr := rds.SMembers(ctx, setKey).Result()
		if membersErr != nil && !errors.Is(membersErr, redis.Nil) {
			return nil, fmt.Errorf("get shard isolation counts: smembers: %w", membersErr)
		}

		activeCount, countErr := m.countActiveIsolationMembers(ctx, rds, setKey, shardID, members)
		if countErr != nil {
			return nil, countErr
		}

		counts[shardID] = activeCount
	}

	return counts, nil
}

// MarkAccountIsolated records that the given account has been isolated to the specified shard.
func (m *Manager) MarkAccountIsolated(ctx context.Context, account HotAccount, shardID int) error {
	if !m.Enabled() {
		return nil
	}

	if account.Alias == "" {
		return nil
	}

	rds, err := m.conn.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("mark account isolated: get redis client: %w", err)
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

	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("mark account isolated: exec pipeline: %w", err)
	}

	return nil
}

func (m *Manager) getRouteCache(organizationID, ledgerID uuid.UUID, alias string) (int, bool) {
	if m.cfg.RouteCacheTTL <= 0 {
		return 0, false
	}

	cacheKey := routeCacheKey(organizationID, ledgerID, alias)

	m.cacheMu.RLock()
	entry, ok := m.routeCache[cacheKey]
	m.cacheMu.RUnlock()

	if !ok || time.Now().UTC().After(entry.expiresAt) {
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
		expiresAt: time.Now().UTC().Add(m.cfg.RouteCacheTTL),
	}
	m.cacheMu.Unlock()
}

func parseIsolationMember(member string) (uuid.UUID, uuid.UUID, string, error) {
	const isolationMemberParts = 3

	parts := strings.SplitN(member, ":", isolationMemberParts)
	if len(parts) != isolationMemberParts {
		return uuid.Nil, uuid.Nil, "", ErrInvalidIsolationMemberFormat
	}

	organizationID, orgErr := uuid.Parse(parts[0])
	if orgErr != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("parse isolation member org id: %w", orgErr)
	}

	ledgerID, ledgerErr := uuid.Parse(parts[1])
	if ledgerErr != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("parse isolation member ledger id: %w", ledgerErr)
	}

	if parts[2] == "" {
		return uuid.Nil, uuid.Nil, "", ErrEmptyAlias
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

	const scanCount = 500

	pattern := utils.BalanceShardKey(sourceShard, organizationID, ledgerID, alias+"#*")

	iter := rds.Scan(ctx, 0, pattern, scanCount).Iterator()
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
		return nil, fmt.Errorf("collect balance keys: scan: %w", err)
	}

	balanceKeys := make([]string, 0, len(keySet))
	for balanceKey := range keySet {
		balanceKeys = append(balanceKeys, balanceKey)
	}

	sort.Strings(balanceKeys)

	return balanceKeys, nil
}
