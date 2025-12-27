//go:build integration

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
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

// assertInsufficientFundsError verifies the error is an UnprocessableOperationError with code "0018".
func assertInsufficientFundsError(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err, "expected an error")

	var unprocessableErr pkg.UnprocessableOperationError
	if assert.ErrorAs(t, err, &unprocessableErr, "error should be UnprocessableOperationError") {
		assert.Equal(t, constant.ErrInsufficientFunds.Error(), unprocessableErr.Code, "error code should be 0018")
	}
}

// createTestBalanceOperation creates a BalanceOperation for testing purposes.
func createTestBalanceOperation(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount decimal.Decimal) mmodel.BalanceOperation {
	return createTestBalanceOperationWithAvailable(organizationID, ledgerID, alias, assetCode, operation, amount, decimal.NewFromInt(1000), "deposit")
}

// createTestBalanceOperationWithAvailable creates a BalanceOperation with custom available balance and account type.
func createTestBalanceOperationWithAvailable(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount, available decimal.Decimal, accountType string) mmodel.BalanceOperation {
	balanceID := libCommons.GenerateUUIDv7().String()
	accountID := libCommons.GenerateUUIDv7().String()
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
			Available:      available,
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    accountType,
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias,
		Amount: pkgTransaction.Amount{
			Asset:     assetCode,
			Value:     amount,
			Operation: operation,
		},
		InternalKey: internalKey,
	}
}

func TestIntegration_AddSumBalancesRedis_WithSyncEnabled_SchedulesBalanceSync(t *testing.T) {
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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

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

func TestIntegration_AddSumBalancesRedis_WithSyncDisabled_DoesNotScheduleBalanceSync(t *testing.T) {
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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

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

func TestIntegration_AddSumBalancesRedis_ProcessesBalancesCorrectly_RegardlessOfSyncFlag(t *testing.T) {
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

			organizationID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			transactionID := libCommons.GenerateUUIDv7()

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

func TestIntegration_AddSumBalancesRedis_MultipleOperations_AllScheduledWhenEnabled(t *testing.T) {
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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

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

	// Verify the exact keys scheduled match both operations
	members, err := client.ZRange(ctx, scheduleKey, 0, -1).Result()
	require.NoError(t, err, "failed to get ZRANGE for schedule key")
	assert.ElementsMatch(t, []string{balanceOp1.InternalKey, balanceOp2.InternalKey}, members,
		"scheduled members should match both balance internal keys")
}

func TestIntegration_AddSumBalancesRedis_InsufficientFunds_ReturnsError(t *testing.T) {
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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	// Create a balance with 100 available, but try to debit 500
	balanceOp := createTestBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@sender", "USD",
		constant.DEBIT,
		decimal.NewFromInt(500), // amount to debit
		decimal.NewFromInt(100), // available balance (insufficient)
		"deposit",               // internal account type
	)

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.CREATED, false, []mmodel.BalanceOperation{balanceOp})

	// Assert
	assertInsufficientFundsError(t, err)
	assert.Nil(t, balances, "balances should be nil on error")
}

func TestIntegration_AddSumBalancesRedis_InsufficientFunds_RollsBackAllBalances(t *testing.T) {
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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	// First operation: valid debit (100 from 1000 available)
	balanceOp1 := createTestBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@sender1", "USD",
		constant.DEBIT,
		decimal.NewFromInt(100),
		decimal.NewFromInt(1000),
		"deposit",
	)

	// Second operation: invalid debit (500 from 100 available) - will fail
	balanceOp2 := createTestBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@sender2", "USD",
		constant.DEBIT,
		decimal.NewFromInt(500),
		decimal.NewFromInt(100),
		"deposit",
	)
	// Use different key to avoid collision
	balanceOp2.Balance.Key = "secondary"
	balanceOp2.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "secondary")

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.CREATED, false, []mmodel.BalanceOperation{balanceOp1, balanceOp2})

	// Assert
	assertInsufficientFundsError(t, err)
	assert.Nil(t, balances, "balances should be nil on error")

	// Verify rollback: first balance should be restored to original state
	val, err := client.Get(ctx, balanceOp1.InternalKey).Result()
	require.NoError(t, err, "rolled-back balance key should exist in Redis")

	var restored mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(val), &restored), "should unmarshal balance")
	assert.True(t, restored.Available.Equal(decimal.NewFromInt(1000)),
		"first balance should be rolled back to original 1000, got %s", restored.Available)
}

func TestIntegration_AddSumBalancesRedis_NonNegativeBalance_BoundaryConditions(t *testing.T) {
	// Single container for all subtests - avoids 4× container startup overhead
	client, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()

	addr := client.Options().Addr
	conn := createTestRedisConnection(t, addr)

	testCases := []struct {
		name        string
		available   decimal.Decimal
		debitAmount decimal.Decimal
		accountType string
		expectError bool
	}{
		{
			name:        "exact balance debit succeeds",
			available:   decimal.NewFromInt(100),
			debitAmount: decimal.NewFromInt(100),
			accountType: "deposit",
			expectError: false,
		},
		{
			name:        "one cent over fails",
			available:   decimal.NewFromFloat(100.00),
			debitAmount: decimal.NewFromFloat(100.01),
			accountType: "deposit",
			expectError: true,
		},
		{
			name:        "zero balance debit fails",
			available:   decimal.Zero,
			debitAmount: decimal.NewFromInt(1),
			accountType: "deposit",
			expectError: true,
		},
		{
			name:        "external account allows overdraft",
			available:   decimal.NewFromInt(100),
			debitAmount: decimal.NewFromInt(1000),
			accountType: "external",
			expectError: false,
		},
		{
			name:        "sub-cent precision debit fails",
			available:   decimal.NewFromFloat(100.001),
			debitAmount: decimal.NewFromFloat(100.002),
			accountType: "deposit",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Flush Redis between subtests for isolation
			require.NoError(t, client.FlushDB(ctx).Err(), "failed to flush Redis")

			repo := &RedisConsumerRepository{
				conn:               conn,
				balanceSyncEnabled: false,
			}

			organizationID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			transactionID := libCommons.GenerateUUIDv7()

			balanceOp := createTestBalanceOperationWithAvailable(
				organizationID, ledgerID,
				"@account", "USD",
				constant.DEBIT,
				tc.debitAmount,
				tc.available,
				tc.accountType,
			)

			// Act
			balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.CREATED, false, []mmodel.BalanceOperation{balanceOp})

			// Assert
			if tc.expectError {
				assertInsufficientFundsError(t, err)
				assert.Nil(t, balances, "balances should be nil on error")
			} else {
				require.NoError(t, err, "should not return error")
				require.NotNil(t, balances, "balances should not be nil")
				assert.Len(t, balances, 1, "should return one balance")
			}
		})
	}
}
