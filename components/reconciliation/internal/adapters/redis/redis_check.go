package redis

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultRedisSampleSize = 250
	balanceSyncScheduleKey = "schedule:{transactions}:balance-sync"
)

type balanceRow struct {
	ID             string
	OrganizationID string
	LedgerID       string
	AccountID      string
	Alias          string
	Key            string
	AssetCode      string
	Available      decimal.Decimal
	OnHold         decimal.Decimal
	Version        int64
}

type balanceRedis struct {
	ID        string          `json:"id"`
	Alias     string          `json:"alias"`
	AccountID string          `json:"accountId"`
	AssetCode string          `json:"assetCode"`
	Available decimal.Decimal `json:"available"`
	OnHold    decimal.Decimal `json:"onHold"`
	Version   int64           `json:"version"`
	Key       string          `json:"key"`
}

// RedisChecker validates Redis vs Postgres balance snapshots.
type RedisChecker struct {
	db        *sql.DB
	redisConn *libRedis.RedisConnection
	logger    libLog.Logger
}

// NewRedisChecker creates a new Redis checker.
func NewRedisChecker(db *sql.DB, redisConn *libRedis.RedisConnection, logger libLog.Logger) *RedisChecker {
	return &RedisChecker{
		db:        db,
		redisConn: redisConn,
		logger:    logger,
	}
}

// Name returns the unique name of this checker.
func (c *RedisChecker) Name() string {
	return postgres.CheckerNameRedis
}

// Check validates Redis balances against PostgreSQL.
func (c *RedisChecker) Check(ctx context.Context, config postgres.CheckerConfig) (postgres.CheckResult, error) {
	if c.redisConn == nil || c.db == nil {
		return &domain.RedisCheckResult{Status: domain.StatusSkipped}, nil
	}

	sampleSize := config.RedisSampleSize
	if sampleSize <= 0 {
		sampleSize = defaultRedisSampleSize
	}

	rows, err := c.db.QueryContext(ctx, `
		SELECT
			id::text,
			organization_id::text,
			ledger_id::text,
			account_id::text,
			alias,
			key,
			asset_code,
			available::DECIMAL,
			on_hold::DECIMAL,
			version
		FROM balance
		WHERE deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT $1
	`, sampleSize)
	if err != nil {
		return nil, fmt.Errorf("redis check query failed: %w", err)
	}
	defer rows.Close()

	var balances []balanceRow

	for rows.Next() {
		var b balanceRow
		if err := rows.Scan(
			&b.ID,
			&b.OrganizationID,
			&b.LedgerID,
			&b.AccountID,
			&b.Alias,
			&b.Key,
			&b.AssetCode,
			&b.Available,
			&b.OnHold,
			&b.Version,
		); err != nil {
			return nil, fmt.Errorf("redis check scan failed: %w", err)
		}

		balances = append(balances, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("redis check iteration failed: %w", err)
	}

	rds, err := c.redisConn.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("redis check client failed: %w", err)
	}

	result := &domain.RedisCheckResult{
		SampledBalances: len(balances),
	}

	if len(balances) == 0 {
		result.Status = domain.StatusHealthy
		return result, nil
	}

	keys := make([]string, 0, len(balances))
	for _, b := range balances {
		balanceKey := b.Alias + "#" + b.Key
		internalKey := balanceInternalKey(b.OrganizationID, b.LedgerID, balanceKey)
		keys = append(keys, internalKey)
	}

	values, err := rds.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis check mget failed: %w", err)
	}

	for i, val := range values {
		b := balances[i]

		if val == nil {
			// Balance not in Redis cache - this is expected/normal behavior
			// (balance never transacted or TTL expired). NOT a discrepancy.
			result.MissingRedis++

			continue
		}

		var data []byte

		switch v := val.(type) {
		case string:
			data = []byte(v)
		case []byte:
			data = v
		default:
			result.ValueMismatches++
			c.appendRedisDiscrepancy(result, b, balanceRedis{}, config.MaxResults)
			if c.logger != nil {
				sc := trace.SpanContextFromContext(ctx)
				traceID := ""
				if sc.HasTraceID() {
					traceID = sc.TraceID().String()
				}

				// Keep log lightweight and avoid dumping potentially sensitive content.
				c.logger.Debugf(
					"redis check unexpected MGET value type (trace_id=%s, index=%d, org_id=%s, ledger_id=%s, balance_id=%s, redis_key=%s, value_type=%s)",
					traceID,
					i,
					b.OrganizationID,
					b.LedgerID,
					b.ID,
					redactRedisKey(keys[i]),
					fmt.Sprintf("%T", val),
				)
			}

			continue
		}

		var rb balanceRedis
		if err := json.Unmarshal(data, &rb); err != nil {
			result.ValueMismatches++
			c.appendRedisDiscrepancy(result, b, balanceRedis{}, config.MaxResults)

			continue
		}

		valueMismatch := b.Available.Cmp(rb.Available) != 0 || b.OnHold.Cmp(rb.OnHold) != 0
		versionMismatch := b.Version != rb.Version

		if valueMismatch {
			result.ValueMismatches++
		}

		if versionMismatch {
			result.VersionMismatches++
		}

		if valueMismatch || versionMismatch {
			c.appendRedisDiscrepancy(result, b, rb, config.MaxResults)
		}
	}

	result.SyncQueueDepth, result.OldestSyncScore, result.OldestSyncAt = getSyncQueueStats(ctx, rds)

	// Only count actual mismatches (value/version) as issues for status determination.
	// MissingRedis is informational only - balances not in cache is expected behavior.
	issueCount := result.ValueMismatches + result.VersionMismatches
	result.Status = postgres.DetermineStatus(issueCount, postgres.StatusThresholds{
		WarningThreshold:          10,
		WarningThresholdExclusive: true,
	})

	return result, nil
}

func (c *RedisChecker) appendRedisDiscrepancy(result *domain.RedisCheckResult, db balanceRow, rb balanceRedis, maxResults int) {
	if maxResults <= 0 || len(result.Discrepancies) >= maxResults {
		return
	}

	result.Discrepancies = append(result.Discrepancies, domain.RedisBalanceDiscrepancy{
		BalanceID:      db.ID,
		AccountID:      db.AccountID,
		Alias:          db.Alias,
		AssetCode:      db.AssetCode,
		Key:            db.Key,
		DBAvailable:    db.Available,
		DBOnHold:       db.OnHold,
		DBVersion:      db.Version,
		RedisAvailable: rb.Available,
		RedisOnHold:    rb.OnHold,
		RedisVersion:   rb.Version,
	})
}

func balanceInternalKey(organizationID, ledgerID, key string) string {
	return fmt.Sprintf("balance:{transactions}:%s:%s:%s", organizationID, ledgerID, key)
}

func redactRedisKey(k string) string {
	// Keys can contain aliases/account keys. Log only a short prefix+suffix to aid debugging.
	const max = 40
	if k == "" {
		return ""
	}
	if len(k) <= max {
		return k
	}
	const prefix = 18
	const suffix = 10
	if len(k) <= prefix+suffix+1 {
		return k[:max]
	}
	return k[:prefix] + "…" + k[len(k)-suffix:]
}

func getSyncQueueStats(ctx context.Context, rds redis.UniversalClient) (int64, int64, *time.Time) {
	depth, err := rds.ZCard(ctx, balanceSyncScheduleKey).Result()
	if err != nil {
		return 0, 0, nil
	}

	entries, err := rds.ZRangeWithScores(ctx, balanceSyncScheduleKey, 0, 0).Result()
	if err != nil || len(entries) == 0 {
		return depth, 0, nil
	}

	score := int64(entries[0].Score)
	ts := time.Unix(score, 0).UTC()

	return depth, score, &ts
}
