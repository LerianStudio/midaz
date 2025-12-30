//go:build integration

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupRedisContainer starts a Redis container for integration testing and returns
// a cleanup function that should be called when the test is done.
func setupRedisContainer(t *testing.T) (*redis.Client, func()) {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "valkey/valkey:8",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
	}

	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Redis container")

	host, err := redisContainer.Host(ctx)
	require.NoError(t, err, "failed to get Redis container host")

	port, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err, "failed to get Redis container port")

	client := redis.NewClient(&redis.Options{
		Addr: host + ":" + port.Port(),
	})

	// Verify connection
	_, err = client.Ping(ctx).Result()
	require.NoError(t, err, "failed to ping Redis container")

	cleanup := func() {
		client.Close()
		if err := redisContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Redis container: %v", err)
		}
	}

	return client, cleanup
}

// createTestRedisConnection creates a libRedis.RedisConnection wrapper for testing
// using the provided Redis client address.
func createTestRedisConnection(t *testing.T, addr string) *libRedis.RedisConnection {
	t.Helper()

	logger := libZap.InitializeLogger()

	conn := &libRedis.RedisConnection{
		Address: []string{addr},
		Logger:  logger,
	}

	return conn
}

// createTestBalanceOperation creates a BalanceOperation for testing purposes.
func createTestBalanceOperation(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount decimal.Decimal) mmodel.BalanceOperation {
	balanceID := uuid.New().String()
	accountID := uuid.New().String()
	balanceKey := "default"

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceKey)

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             balanceID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID,
			Alias:          alias,
			Key:            balanceKey,
			AssetCode:      assetCode,
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias,
		Amount: libTransaction.Amount{
			Asset:     assetCode,
			Value:     amount,
			Operation: operation,
		},
		InternalKey: internalKey,
	}
}

func TestAddSumBalancesRedis_WithSyncEnabled_SchedulesBalanceSync(t *testing.T) {
	// Arrange
	client, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()

	// Get container address for connection
	addr := client.Options().Addr

	// Create Redis connection with lib-commons wrapper
	conn := createTestRedisConnection(t, addr)

	// Create repository with balanceSyncEnabled = true
	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balanceOp := createTestBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp})

	// Assert
	require.NoError(t, err, "AddSumBalancesRedis should not return error")
	require.NotNil(t, balances, "balances should not be nil")
	assert.Len(t, balances, 1, "should return one balance")

	// Verify the balance sync schedule key was populated (ZADD was executed)
	scheduleKey := utils.BalanceSyncScheduleKey
	count, err := client.ZCard(ctx, scheduleKey).Result()
	require.NoError(t, err, "failed to get ZCARD for schedule key")
	assert.Equal(t, int64(1), count, "schedule key should have 1 member when sync is enabled")

	// Verify the scheduled member is the balance key
	members, err := client.ZRange(ctx, scheduleKey, 0, -1).Result()
	require.NoError(t, err, "failed to get ZRANGE for schedule key")
	assert.Contains(t, members, balanceOp.InternalKey, "schedule should contain the balance internal key")
}

func TestAddSumBalancesRedis_WithSyncDisabled_DoesNotScheduleBalanceSync(t *testing.T) {
	// Arrange
	client, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()

	// Get container address for connection
	addr := client.Options().Addr

	// Create Redis connection with lib-commons wrapper
	conn := createTestRedisConnection(t, addr)

	// Create repository with balanceSyncEnabled = false
	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balanceOp := createTestBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp})

	// Assert
	require.NoError(t, err, "AddSumBalancesRedis should not return error")
	require.NotNil(t, balances, "balances should not be nil")
	assert.Len(t, balances, 1, "should return one balance")

	// Verify the balance sync schedule key is EMPTY (ZADD was NOT executed)
	scheduleKey := utils.BalanceSyncScheduleKey
	count, err := client.ZCard(ctx, scheduleKey).Result()
	require.NoError(t, err, "failed to get ZCARD for schedule key")
	assert.Equal(t, int64(0), count, "schedule key should have 0 members when sync is disabled")
}

func TestAddSumBalancesRedis_ProcessesBalancesCorrectly_RegardlessOfSyncFlag(t *testing.T) {
	testCases := []struct {
		name               string
		balanceSyncEnabled bool
	}{
		{
			name:               "sync enabled processes balance correctly",
			balanceSyncEnabled: true,
		},
		{
			name:               "sync disabled processes balance correctly",
			balanceSyncEnabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			client, cleanup := setupRedisContainer(t)
			defer cleanup()

			ctx := context.Background()

			addr := client.Options().Addr
			conn := createTestRedisConnection(t, addr)

			repo := &RedisConsumerRepository{
				conn:               conn,
				balanceSyncEnabled: tc.balanceSyncEnabled,
			}

			organizationID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			// Create a CREDIT operation that adds 500 to balance
			balanceOp := createTestBalanceOperation(organizationID, ledgerID, "@receiver", "USD", constant.CREDIT, decimal.NewFromInt(500))

			// Act
			balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp})

			// Assert
			require.NoError(t, err, "AddSumBalancesRedis should not return error")
			require.NotNil(t, balances, "balances should not be nil")
			require.Len(t, balances, 1, "should return one balance")

			// Verify balance was processed - initial was 1000, credit 500 should result in 1500
			assert.Equal(t, "@receiver", balances[0].Alias, "alias should match")
			assert.Equal(t, "USD", balances[0].AssetCode, "asset code should match")

			// The returned balance should reflect the ORIGINAL state before operation
			// (the Lua script returns the balance state before modification in returnBalances)
			assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(1000)), "available should be original value")
		})
	}
}

func TestAddSumBalancesRedis_MultipleOperations_AllScheduledWhenEnabled(t *testing.T) {
	// Arrange
	client, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()

	addr := client.Options().Addr
	conn := createTestRedisConnection(t, addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Create two balance operations with different keys
	balanceOp1 := createTestBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))
	// Modify the second operation to use a different internal key
	balanceOp2 := createTestBalanceOperation(organizationID, ledgerID, "@receiver", "USD", constant.CREDIT, decimal.NewFromInt(100))
	balanceOp2.Balance.Key = "secondary"
	balanceOp2.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "secondary")

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp1, balanceOp2})

	// Assert
	require.NoError(t, err, "AddSumBalancesRedis should not return error")
	require.NotNil(t, balances, "balances should not be nil")
	assert.Len(t, balances, 2, "should return two balances")

	// Verify both balance keys were scheduled
	scheduleKey := utils.BalanceSyncScheduleKey
	count, err := client.ZCard(ctx, scheduleKey).Result()
	require.NoError(t, err, "failed to get ZCARD for schedule key")
	assert.Equal(t, int64(2), count, "schedule key should have 2 members for 2 operations")
}

func TestAddSumBalancesRedis_NotedStatus_SkipsLuaScript(t *testing.T) {
	// Arrange
	client, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()

	addr := client.Options().Addr
	conn := createTestRedisConnection(t, addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balanceOp := createTestBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))

	// Act - with NOTED status, Lua script should be skipped
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.NOTED, false, []mmodel.BalanceOperation{balanceOp})

	// Assert
	require.NoError(t, err, "AddSumBalancesRedis should not return error")
	require.NotNil(t, balances, "balances should not be nil")
	assert.Len(t, balances, 1, "should return one balance")

	// With NOTED status, the Lua script is skipped entirely, so no scheduling happens
	scheduleKey := utils.BalanceSyncScheduleKey
	count, err := client.ZCard(ctx, scheduleKey).Result()
	require.NoError(t, err, "failed to get ZCARD for schedule key")
	assert.Equal(t, int64(0), count, "schedule key should be empty for NOTED transactions")
}

func TestAddSumBalancesRedis_LargeNegativeCredit_NoPrecisionLoss(t *testing.T) {
	// Arrange
	client, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()

	addr := client.Options().Addr
	conn := createTestRedisConnection(t, addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	amount, err := decimal.NewFromString("9999999999999999999999")
	require.NoError(t, err, "failed to parse amount")

	available, err := decimal.NewFromString("-10000000000000000000047.765432109876543211")
	require.NoError(t, err, "failed to parse available balance")

	expected, err := decimal.NewFromString("-48.765432109876543211")
	require.NoError(t, err, "failed to parse expected balance")

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, "@external/USD#default")

	balanceOp := mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Alias:          "@external/USD",
			Key:            "default",
			AssetCode:      "USD",
			Available:      available,
			OnHold:         decimal.Zero,
			Version:        3,
			AccountType:    "external",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: "@external/USD",
		Amount: libTransaction.Amount{
			Asset:     "USD",
			Value:     amount,
			Operation: constant.CREDIT,
		},
		InternalKey: internalKey,
	}

	// Act
	_, err = repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp})
	require.NoError(t, err, "AddSumBalancesRedis should not return error")

	// Assert: verify stored Redis balance reflects correct negative result
	raw, err := client.Get(ctx, internalKey).Result()
	require.NoError(t, err, "failed to get balance from Redis")

	var cached mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(raw), &cached), "failed to unmarshal cached balance")
	assert.True(t, cached.Available.Equal(expected), "available should match expected negative value")
}
