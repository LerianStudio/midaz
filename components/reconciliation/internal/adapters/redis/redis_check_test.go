package redis

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisChecker_Name(t *testing.T) {
	t.Parallel()

	checker := NewRedisChecker(nil, nil, nil)
	assert.Equal(t, postgres.CheckerNameRedis, checker.Name())
}

func TestRedisChecker_Check_SkipsWhenRedisNil(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	checker := NewRedisChecker(db, nil, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)
	assert.Equal(t, domain.StatusSkipped, typedResult.Status)
}

func TestRedisChecker_Check_SkipsWhenDBNil(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(nil, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)
	assert.Equal(t, domain.StatusSkipped, typedResult.Status)
}

func TestRedisChecker_Check_HealthyWhenNoBalances(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	})
	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 10})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 0, typedResult.SampledBalances)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Check_MissingRedisDoesNotAffectStatus(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Setup 5 balances in DB, none in Redis
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	})
	for i := 0; i < 5; i++ {
		rows.AddRow(
			"bal-id-"+strconv.Itoa(i),
			"org-id",
			"ledger-id",
			"acc-id",
			"@alias"+strconv.Itoa(i),
			"default",
			"USD",
			decimal.NewFromInt(1000),
			decimal.Zero,
			int64(1),
		)
	}
	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 10, MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)

	// Key assertion: Missing Redis entries should NOT affect status
	assert.Equal(t, domain.StatusHealthy, typedResult.Status, "missing Redis entries should not cause unhealthy status")
	assert.Equal(t, 5, typedResult.MissingRedis, "should count 5 missing Redis entries")
	assert.Equal(t, 0, typedResult.ValueMismatches, "should have no value mismatches")
	assert.Equal(t, 0, typedResult.VersionMismatches, "should have no version mismatches")
	assert.Empty(t, typedResult.Discrepancies, "missing Redis should NOT be added to discrepancies")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Check_ValueMismatchCausesWarning(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Setup 1 balance in DB
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	}).AddRow(
		"bal-id-1",
		"org-id",
		"ledger-id",
		"acc-id",
		"@alias1",
		"default",
		"USD",
		decimal.NewFromInt(1000), // DB available = 1000
		decimal.Zero,
		int64(1),
	)
	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	// Setup Redis with DIFFERENT value
	redisKey := balanceInternalKey("org-id", "ledger-id", "@alias1#default")
	redisValue := balanceRedis{
		ID:        "bal-id-1",
		Alias:     "@alias1",
		AccountID: "acc-id",
		AssetCode: "USD",
		Available: decimal.NewFromInt(900), // Redis available = 900 (mismatch!)
		OnHold:    decimal.Zero,
		Version:   1,
		Key:       "default",
	}
	data, _ := json.Marshal(redisValue)
	mr.Set(redisKey, string(data))

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 10, MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)

	assert.Equal(t, domain.StatusWarning, typedResult.Status, "value mismatch should cause warning")
	assert.Equal(t, 0, typedResult.MissingRedis)
	assert.Equal(t, 1, typedResult.ValueMismatches, "should count 1 value mismatch")
	assert.Equal(t, 0, typedResult.VersionMismatches)
	require.Len(t, typedResult.Discrepancies, 1, "should have 1 discrepancy")
	assert.Equal(t, "bal-id-1", typedResult.Discrepancies[0].BalanceID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Check_VersionMismatchCausesWarning(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Setup 1 balance in DB
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	}).AddRow(
		"bal-id-1",
		"org-id",
		"ledger-id",
		"acc-id",
		"@alias1",
		"default",
		"USD",
		decimal.NewFromInt(1000),
		decimal.Zero,
		int64(5), // DB version = 5
	)
	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	// Setup Redis with same value but DIFFERENT version
	redisKey := balanceInternalKey("org-id", "ledger-id", "@alias1#default")
	redisValue := balanceRedis{
		ID:        "bal-id-1",
		Alias:     "@alias1",
		AccountID: "acc-id",
		AssetCode: "USD",
		Available: decimal.NewFromInt(1000), // Same value
		OnHold:    decimal.Zero,
		Version:   3, // Redis version = 3 (mismatch!)
		Key:       "default",
	}
	data, _ := json.Marshal(redisValue)
	mr.Set(redisKey, string(data))

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 10, MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)

	assert.Equal(t, domain.StatusWarning, typedResult.Status, "version mismatch should cause warning")
	assert.Equal(t, 0, typedResult.MissingRedis)
	assert.Equal(t, 0, typedResult.ValueMismatches)
	assert.Equal(t, 1, typedResult.VersionMismatches, "should count 1 version mismatch")
	require.Len(t, typedResult.Discrepancies, 1, "should have 1 discrepancy")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Check_MatchingBalancesHealthy(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Setup 3 balances in DB
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	})
	for i := 0; i < 3; i++ {
		alias := "@alias" + strconv.Itoa(i)
		rows.AddRow(
			"bal-id-"+strconv.Itoa(i),
			"org-id",
			"ledger-id",
			"acc-id",
			alias,
			"default",
			"USD",
			decimal.NewFromInt(1000),
			decimal.Zero,
			int64(1),
		)

		// Setup matching Redis entry
		redisKey := balanceInternalKey("org-id", "ledger-id", alias+"#default")
		redisValue := balanceRedis{
			ID:        "bal-id-" + strconv.Itoa(i),
			Alias:     alias,
			AccountID: "acc-id",
			AssetCode: "USD",
			Available: decimal.NewFromInt(1000),
			OnHold:    decimal.Zero,
			Version:   1,
			Key:       "default",
		}
		data, _ := json.Marshal(redisValue)
		mr.Set(redisKey, string(data))
	}
	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 10, MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)

	assert.Equal(t, domain.StatusHealthy, typedResult.Status, "matching balances should be healthy")
	assert.Equal(t, 3, typedResult.SampledBalances)
	assert.Equal(t, 0, typedResult.MissingRedis)
	assert.Equal(t, 0, typedResult.ValueMismatches)
	assert.Equal(t, 0, typedResult.VersionMismatches)
	assert.Empty(t, typedResult.Discrepancies)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Check_CriticalWhenManyMismatches(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Setup 15 balances with value mismatches (>10 threshold = CRITICAL)
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	})
	for i := 0; i < 15; i++ {
		alias := "@alias" + strconv.Itoa(i)
		rows.AddRow(
			"bal-id-"+strconv.Itoa(i),
			"org-id",
			"ledger-id",
			"acc-id",
			alias,
			"default",
			"USD",
			decimal.NewFromInt(1000), // DB = 1000
			decimal.Zero,
			int64(1),
		)

		// Setup Redis with DIFFERENT value
		redisKey := balanceInternalKey("org-id", "ledger-id", alias+"#default")
		redisValue := balanceRedis{
			ID:        "bal-id-" + strconv.Itoa(i),
			Alias:     alias,
			AccountID: "acc-id",
			AssetCode: "USD",
			Available: decimal.NewFromInt(500), // Redis = 500 (mismatch!)
			OnHold:    decimal.Zero,
			Version:   1,
			Key:       "default",
		}
		data, _ := json.Marshal(redisValue)
		mr.Set(redisKey, string(data))
	}
	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 20, MaxResults: 20})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)

	// >10 mismatches should be CRITICAL (threshold is exclusive: >=10 is CRITICAL)
	assert.Equal(t, domain.StatusCritical, typedResult.Status, "many mismatches should be critical")
	assert.Equal(t, 15, typedResult.ValueMismatches)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Check_MixedScenario(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version",
	})

	// Balance 1: Missing from Redis (should NOT affect status)
	rows.AddRow("bal-1", "org", "ledger", "acc", "@missing", "default", "USD",
		decimal.NewFromInt(1000), decimal.Zero, int64(1))

	// Balance 2: Value mismatch (should affect status)
	rows.AddRow("bal-2", "org", "ledger", "acc", "@mismatch", "default", "USD",
		decimal.NewFromInt(1000), decimal.Zero, int64(1))
	mr.Set(balanceInternalKey("org", "ledger", "@mismatch#default"),
		mustMarshal(balanceRedis{Available: decimal.NewFromInt(500), Version: 1}))

	// Balance 3: Matching (healthy)
	rows.AddRow("bal-3", "org", "ledger", "acc", "@match", "default", "USD",
		decimal.NewFromInt(1000), decimal.Zero, int64(1))
	mr.Set(balanceInternalKey("org", "ledger", "@match#default"),
		mustMarshal(balanceRedis{Available: decimal.NewFromInt(1000), Version: 1}))

	mock.ExpectQuery(`FROM balance`).WillReturnRows(rows)

	conn := createTestRedisConn(mr.Addr())
	checker := NewRedisChecker(db, conn, nil)
	result, err := checker.Check(context.Background(), postgres.CheckerConfig{RedisSampleSize: 10, MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireRedisResult(t, result)

	assert.Equal(t, domain.StatusWarning, typedResult.Status, "should be warning due to 1 mismatch")
	assert.Equal(t, 3, typedResult.SampledBalances)
	assert.Equal(t, 1, typedResult.MissingRedis, "1 missing from Redis")
	assert.Equal(t, 1, typedResult.ValueMismatches, "1 value mismatch")
	assert.Equal(t, 0, typedResult.VersionMismatches)
	// Only the value mismatch should be in discrepancies, NOT the missing one
	require.Len(t, typedResult.Discrepancies, 1, "only actual mismatches in discrepancies")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceInternalKey(t *testing.T) {
	t.Parallel()

	key := balanceInternalKey("org-123", "ledger-456", "@alias#default")
	assert.Equal(t, "balance:{transactions}:org-123:ledger-456:@alias#default", key)
}

func TestAppendRedisDiscrepancy_RespectsMaxResults(t *testing.T) {
	t.Parallel()

	checker := &RedisChecker{}
	result := &domain.RedisCheckResult{}

	// Add 5 discrepancies with max 3
	for i := 0; i < 5; i++ {
		checker.appendRedisDiscrepancy(result, balanceRow{ID: "bal-" + strconv.Itoa(i)}, balanceRedis{}, 3)
	}

	assert.Len(t, result.Discrepancies, 3, "should respect maxResults limit")
}

func TestAppendRedisDiscrepancy_ZeroMaxResults(t *testing.T) {
	t.Parallel()

	checker := &RedisChecker{}
	result := &domain.RedisCheckResult{}

	checker.appendRedisDiscrepancy(result, balanceRow{ID: "bal-1"}, balanceRedis{}, 0)

	assert.Empty(t, result.Discrepancies, "should not add when maxResults is 0")
}

// Helper functions

func createTestRedisConn(addr string) *libRedis.RedisConnection {
	logger := libZap.InitializeLogger()
	return &libRedis.RedisConnection{
		Address: []string{addr},
		Logger:  logger,
	}
}

func requireRedisResult(t *testing.T, result postgres.CheckResult) *domain.RedisCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.RedisCheckResult)
	require.True(t, ok, "result should be *domain.RedisCheckResult")

	return typedResult
}

func mustMarshal(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return string(data)
}
