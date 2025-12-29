//go:build integration

package redis

import (
	"context"
	"encoding/json"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	redistestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_AddSumBalancesRedis_WithSyncEnabled_SchedulesBalanceSync(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()

	// Create Redis connection with lib-commons wrapper
	conn := redistestutil.CreateConnection(t, container.Addr)

	// Create repository with balanceSyncEnabled = true
	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	balanceOp := redistestutil.CreateBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp})

	// Assert
	require.NoError(t, err, "AddSumBalancesRedis should not return error")
	require.NotNil(t, balances, "balances should not be nil")
	assert.Len(t, balances, 1, "should return one balance")

	// Verify the balance sync schedule key was populated (ZADD was executed)
	scheduleKey := utils.BalanceSyncScheduleKey
	count, err := container.Client.ZCard(ctx, scheduleKey).Result()
	require.NoError(t, err, "failed to get ZCARD for schedule key")
	assert.Equal(t, int64(1), count, "schedule key should have 1 member when sync is enabled")

	// Verify the scheduled member is the balance key
	members, err := container.Client.ZRange(ctx, scheduleKey, 0, -1).Result()
	require.NoError(t, err, "failed to get ZRANGE for schedule key")
	assert.Contains(t, members, balanceOp.InternalKey, "schedule should contain the balance internal key")
}

func TestIntegration_AddSumBalancesRedis_WithSyncDisabled_DoesNotScheduleBalanceSync(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()

	// Create Redis connection with lib-commons wrapper
	conn := redistestutil.CreateConnection(t, container.Addr)

	// Create repository with balanceSyncEnabled = false
	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	balanceOp := redistestutil.CreateBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))

	// Act
	balances, err := repo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, constant.APPROVED, false, []mmodel.BalanceOperation{balanceOp})

	// Assert
	require.NoError(t, err, "AddSumBalancesRedis should not return error")
	require.NotNil(t, balances, "balances should not be nil")
	assert.Len(t, balances, 1, "should return one balance")

	// Verify the balance sync schedule key is EMPTY (ZADD was NOT executed)
	scheduleKey := utils.BalanceSyncScheduleKey
	count, err := container.Client.ZCard(ctx, scheduleKey).Result()
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
			container := redistestutil.SetupContainer(t)
			defer container.Cleanup()

			ctx := context.Background()

			conn := redistestutil.CreateConnection(t, container.Addr)

			repo := &RedisConsumerRepository{
				conn:               conn,
				balanceSyncEnabled: tc.balanceSyncEnabled,
			}

			organizationID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			transactionID := libCommons.GenerateUUIDv7()

			// Create a CREDIT operation that adds 500 to balance
			balanceOp := redistestutil.CreateBalanceOperation(organizationID, ledgerID, "@receiver", "USD", constant.CREDIT, decimal.NewFromInt(500))

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
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	// Create two balance operations with different keys
	balanceOp1 := redistestutil.CreateBalanceOperation(organizationID, ledgerID, "@sender", "USD", constant.DEBIT, decimal.NewFromInt(100))
	// Modify the second operation to use a different internal key
	balanceOp2 := redistestutil.CreateBalanceOperation(organizationID, ledgerID, "@receiver", "USD", constant.CREDIT, decimal.NewFromInt(100))
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
	count, err := container.Client.ZCard(ctx, scheduleKey).Result()
	require.NoError(t, err, "failed to get ZCARD for schedule key")
	assert.Equal(t, int64(2), count, "schedule key should have 2 members for 2 operations")

	// Verify the exact keys scheduled match both operations
	members, err := container.Client.ZRange(ctx, scheduleKey, 0, -1).Result()
	require.NoError(t, err, "failed to get ZRANGE for schedule key")
	assert.ElementsMatch(t, []string{balanceOp1.InternalKey, balanceOp2.InternalKey}, members,
		"scheduled members should match both balance internal keys")
}

func TestIntegration_AddSumBalancesRedis_InsufficientFunds_ReturnsError(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	// Create a balance with 100 available, but try to debit 500
	balanceOp := redistestutil.CreateBalanceOperationWithAvailable(
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
	redistestutil.AssertInsufficientFundsError(t, err)
	assert.Nil(t, balances, "balances should be nil on error")
}

func TestIntegration_AddSumBalancesRedis_InsufficientFunds_RollsBackAllBalances(t *testing.T) {
	// Arrange
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	// First operation: valid debit (100 from 1000 available)
	balanceOp1 := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@sender1", "USD",
		constant.DEBIT,
		decimal.NewFromInt(100),
		decimal.NewFromInt(1000),
		"deposit",
	)

	// Second operation: invalid debit (500 from 100 available) - will fail
	balanceOp2 := redistestutil.CreateBalanceOperationWithAvailable(
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
	redistestutil.AssertInsufficientFundsError(t, err)
	assert.Nil(t, balances, "balances should be nil on error")

	// Verify rollback: first balance should be restored to original state
	val, err := container.Client.Get(ctx, balanceOp1.InternalKey).Result()
	require.NoError(t, err, "rolled-back balance key should exist in Redis")

	var restored mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(val), &restored), "should unmarshal balance")
	assert.True(t, restored.Available.Equal(decimal.NewFromInt(1000)),
		"first balance should be rolled back to original 1000, got %s", restored.Available)
}

func TestIntegration_AddSumBalancesRedis_NonNegativeBalance_BoundaryConditions(t *testing.T) {
	// Single container for all subtests - avoids 4× container startup overhead
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()

	conn := redistestutil.CreateConnection(t, container.Addr)

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
			require.NoError(t, container.Client.FlushDB(ctx).Err(), "failed to flush Redis")

			repo := &RedisConsumerRepository{
				conn:               conn,
				balanceSyncEnabled: false,
			}

			organizationID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			transactionID := libCommons.GenerateUUIDv7()

			balanceOp := redistestutil.CreateBalanceOperationWithAvailable(
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
				redistestutil.AssertInsufficientFundsError(t, err)
				assert.Nil(t, balances, "balances should be nil on error")
			} else {
				require.NoError(t, err, "should not return error")
				require.NotNil(t, balances, "balances should not be nil")
				assert.Len(t, balances, 1, "should return one balance")
			}
		})
	}
}

func TestIntegration_AddSumBalancesRedis_NtoN_InsufficientFunds_RollsBackAllBalances(t *testing.T) {
	// This test validates the atomic rollback behavior for N:N transactions (multiple sources, multiple destinations).
	// When one balance operation fails validation (insufficient funds), ALL previously modified balances
	// must be restored to their original state by the Lua script's rollback function.
	//
	// Scenario:
	// - 3 sources: @source1 (1000), @source2 (1000), @source3 (1000)
	// - 3 destinations: @dest1 (0), @dest2 (0), @dest3 (0)
	// - Operations: DEBIT 100 from each source, CREDIT 100 to each dest
	// - @source3 will try to DEBIT 5000 (invalid - only 1000 available)
	// - Expected: All 5 previous balance modifications should be rolled back

	// Arrange
	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()
	conn := redistestutil.CreateConnection(t, container.Addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	// Create source balance operations (DEBIT)
	source1 := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@source1", "USD",
		constant.DEBIT,
		decimal.NewFromInt(100),  // amount to debit
		decimal.NewFromInt(1000), // available balance
		"deposit",
	)
	source1.Balance.Key = "source1-key"
	source1.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "source1-key")

	source2 := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@source2", "USD",
		constant.DEBIT,
		decimal.NewFromInt(100),
		decimal.NewFromInt(1000),
		"deposit",
	)
	source2.Balance.Key = "source2-key"
	source2.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "source2-key")

	// source3 will FAIL - trying to debit 5000 from 1000 available
	source3Invalid := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@source3", "USD",
		constant.DEBIT,
		decimal.NewFromInt(5000), // amount to debit (INVALID - exceeds available)
		decimal.NewFromInt(1000), // available balance
		"deposit",
	)
	source3Invalid.Balance.Key = "source3-key"
	source3Invalid.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "source3-key")

	// Create destination balance operations (CREDIT)
	dest1 := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@dest1", "USD",
		constant.CREDIT,
		decimal.NewFromInt(100), // amount to credit
		decimal.Zero,            // available balance (starts at 0)
		"deposit",
	)
	dest1.Balance.Key = "dest1-key"
	dest1.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "dest1-key")

	dest2 := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@dest2", "USD",
		constant.CREDIT,
		decimal.NewFromInt(100),
		decimal.Zero,
		"deposit",
	)
	dest2.Balance.Key = "dest2-key"
	dest2.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "dest2-key")

	dest3 := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID,
		"@dest3", "USD",
		constant.CREDIT,
		decimal.NewFromInt(100),
		decimal.Zero,
		"deposit",
	)
	dest3.Balance.Key = "dest3-key"
	dest3.InternalKey = utils.BalanceInternalKey(organizationID, ledgerID, "dest3-key")

	// Order matters: source3Invalid is last, so all previous balances should be modified then rolled back
	balanceOps := []mmodel.BalanceOperation{
		source1,        // Valid: 1000 - 100 = 900
		source2,        // Valid: 1000 - 100 = 900
		dest1,          // Valid: 0 + 100 = 100
		dest2,          // Valid: 0 + 100 = 100
		dest3,          // Valid: 0 + 100 = 100
		source3Invalid, // INVALID: 1000 - 5000 = -4000 (fails validation)
	}

	// Store original values for assertion
	originalBalances := map[string]decimal.Decimal{
		source1.InternalKey:        decimal.NewFromInt(1000),
		source2.InternalKey:        decimal.NewFromInt(1000),
		dest1.InternalKey:          decimal.Zero,
		dest2.InternalKey:          decimal.Zero,
		dest3.InternalKey:          decimal.Zero,
		source3Invalid.InternalKey: decimal.NewFromInt(1000),
	}

	// Act
	balances, err := repo.AddSumBalancesRedis(
		ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, balanceOps,
	)

	// Assert: Error should be returned (insufficient funds - error code 0018)
	redistestutil.AssertInsufficientFundsError(t, err)
	assert.Nil(t, balances, "balances should be nil on error")

	// Assert: ALL balances should be rolled back to their original state
	for key, expectedAvailable := range originalBalances {
		val, err := container.Client.Get(ctx, key).Result()
		require.NoError(t, err, "balance key %s should exist in Redis after rollback", key)

		var restored mmodel.BalanceRedis
		require.NoError(t, json.Unmarshal([]byte(val), &restored),
			"should unmarshal balance for key %s", key)

		assert.True(t, restored.Available.Equal(expectedAvailable),
			"balance %s should be rolled back to %s, got %s",
			key, expectedAvailable, restored.Available)
	}
}

func TestIntegration_AddSumBalancesRedis_OperationsSumEqualsBalance(t *testing.T) {
	// Validates invariant: Σ(operations) == balance.Available
	// Uses sequence of CREDIT/DEBIT ops and verifies Redis state after each

	container := redistestutil.SetupContainer(t)
	defer container.Cleanup()

	ctx := context.Background()
	conn := redistestutil.CreateConnection(t, container.Addr)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: false,
	}

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Fixed account for the sequence
	alias := "@ops-sum-account"
	initialBalance := decimal.NewFromInt(1000)

	// Create initial balance in Redis
	balanceOp := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, alias, "USD",
		constant.CREDIT, decimal.Zero, // Initial credit of 0 just to set up
		initialBalance, "deposit",
	)
	internalKey := balanceOp.InternalKey

	// Sequence of operations to apply
	operations := []struct {
		name      string
		operation string
		amount    decimal.Decimal
	}{
		{"credit 500", constant.CREDIT, decimal.NewFromInt(500)},
		{"debit 200", constant.DEBIT, decimal.NewFromInt(200)},
		{"credit 150", constant.CREDIT, decimal.NewFromInt(150)},
		{"debit 450", constant.DEBIT, decimal.NewFromInt(450)},
		{"credit 1000", constant.CREDIT, decimal.NewFromInt(1000)},
	}

	expectedSum := initialBalance

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			// Calculate expected sum before operation
			if op.operation == constant.CREDIT {
				expectedSum = expectedSum.Add(op.amount)
			} else {
				expectedSum = expectedSum.Sub(op.amount)
			}

			// Create balance operation with current expected sum as available
			// (simulating the balance state before this operation)
			currentAvailable := expectedSum
			if op.operation == constant.CREDIT {
				currentAvailable = expectedSum.Sub(op.amount) // Before credit
			} else {
				currentAvailable = expectedSum.Add(op.amount) // Before debit
			}

			balanceOp := redistestutil.CreateBalanceOperationWithAvailable(
				organizationID, ledgerID, alias, "USD",
				op.operation, op.amount, currentAvailable, "deposit",
			)
			// Use same internal key for consistency
			balanceOp.InternalKey = internalKey
			balanceOp.Balance.Key = "default"

			transactionID := libCommons.GenerateUUIDv7()
			balances, err := repo.AddSumBalancesRedis(
				ctx, organizationID, ledgerID, transactionID,
				constant.CREATED, false, []mmodel.BalanceOperation{balanceOp},
			)

			require.NoError(t, err, "AddSumBalancesRedis should not error for %s", op.name)
			require.Len(t, balances, 1)

			// Verify Redis state matches expected sum
			val, err := container.Client.Get(ctx, internalKey).Result()
			require.NoError(t, err, "balance key should exist in Redis")

			var stored mmodel.BalanceRedis
			require.NoError(t, json.Unmarshal([]byte(val), &stored), "should unmarshal balance")

			assert.True(t, stored.Available.Equal(expectedSum),
				"After %s: expected=%s got=%s", op.name, expectedSum, stored.Available)
		})
	}

	// Final assertion: verify cumulative sum matches Redis state after all operations
	// Expected: 1000 + 500 - 200 + 150 - 450 + 1000 = 2000
	finalExpected := decimal.NewFromInt(2000)
	require.True(t, expectedSum.Equal(finalExpected),
		"Final expectedSum should be 2000, got %s", expectedSum)

	val, err := container.Client.Get(ctx, internalKey).Result()
	require.NoError(t, err, "balance key should exist after all operations")

	var finalStored mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(val), &finalStored), "should unmarshal final balance")

	assert.True(t, finalStored.Available.Equal(finalExpected),
		"Final Redis balance should equal cumulative sum: expected=%s got=%s",
		finalExpected, finalStored.Available)
}
