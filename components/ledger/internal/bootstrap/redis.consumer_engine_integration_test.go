//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type recoveryEngineClientProvider struct {
	client *redis.Client
}

func (provider recoveryEngineClientProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	return provider.client, nil
}

func recoveryEngineValkey(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "valkey/valkey:8", ExposedPorts: []string{"6379/tcp"}, WaitingFor: wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
		}, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(host, port.Port()), DB: 2, Protocol: 2, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	info, err := client.Info(ctx, "server").Result()
	require.NoError(t, err)
	require.Contains(t, info, "valkey_version:")

	return client
}

func recoveryEngineID(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("consumer-engine:"+name))
}

func recoveryEngineExecution(t *testing.T) command.EngineExecution {
	t.Helper()
	id := func(suffix string) uuid.UUID { return recoveryEngineID(t.Name() + ":" + suffix) }
	date := time.Date(2024, time.January, 2, 12, 0, 0, 0, time.UTC)
	balance := accounting.BalanceSnapshot{
		BalanceRef: "@source#default", ID: id("balance"), AccountID: id("account"), Alias: "@source", Key: constant.DefaultBalanceKey,
		AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit, BalanceScope: "transactional",
		Available: decimal.NewFromInt(100), Version: 7, AllowSending: true, AllowReceiving: true,
	}
	request := accounting.Execution{
		OrganizationID: id("organization"), LedgerID: id("ledger"), ExecutionID: id("execution"), Balances: []accounting.BalanceSnapshot{balance},
		Transactions: []accounting.Transaction{{ID: id("transaction"), Postings: []accounting.Posting{{Ref: "source:0", BalanceRef: balance.BalanceRef, Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30), DrawPolicy: accounting.DrawForbidden}}}},
	}
	projection := command.OperationRecordSpec{
		TransactionID: id("transaction"), PostingRef: "source:0", BalanceRef: balance.BalanceRef, Role: accounting.RolePrimary, Side: command.OperationSpecSideFrom,
		RowType: constant.DEBIT, Direction: constant.DirectionDebit, Description: "frozen operation", RouteCode: "FROZEN", ChartOfAccounts: "frozen chart",
		Metadata: map[string]any{"purpose": "frozen operation metadata"}, RequestedAmount: decimal.NewFromInt(30), CompatibilityPath: command.OperationRecordStandard,
		Balance: command.OperationBalanceContext(mmodel.Balance{
			ID: balance.ID.String(), AccountID: balance.AccountID.String(), OrganizationID: request.OrganizationID.String(), LedgerID: request.LedgerID.String(),
			Alias: balance.Alias, Key: balance.Key, AssetCode: balance.AssetCode, AccountType: balance.AccountType, Direction: balance.Direction,
			Available: balance.Available, Version: balance.Version, AllowSending: true, AllowReceiving: true,
		}),
	}
	payload := command.TransactionCompletionPlan{
		FormatVersion: command.TransactionCompletionFormatVersion, HeaderID: "recovery-engine-integration", OrganizationID: request.OrganizationID, LedgerID: request.LedgerID,
		TransactionID: id("transaction"), ExecutionID: request.ExecutionID, TTL: date, TransactionDate: date,
		TransactionCreatedAt: date.Add(-24 * time.Hour), TransactionUpdatedAt: date.Add(time.Millisecond), OperationUpdatedAt: date.Add(2 * time.Millisecond),
		TransactionStatus: constant.APPROVED, Action: constant.ActionDirect, OperationSpecs: []command.OperationRecordSpec{projection},
		TransactionInput: mtransaction.Transaction{
			Description: "frozen transaction", Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(30)},
			Metadata: map[string]any{"sequence": json.Number("9007199254740993"), "purpose": "frozen transaction metadata"},
		},
		Validate: &mtransaction.Responses{Sources: []string{"@source"}, Destinations: []string{"@destination"}},
	}
	fingerprint, err := command.ComputeBalanceEngineIntentFingerprint(command.BalanceEngineIntent{
		OrganizationID: request.OrganizationID, LedgerID: request.LedgerID, ExecutionID: request.ExecutionID,
		Transactions: []command.BalanceEngineTransactionIntent{{
			TransactionID: payload.TransactionID, Action: payload.Action, TransactionStatus: payload.TransactionStatus,
			TransactionDate: payload.TransactionDate, TransactionCreatedAt: payload.TransactionCreatedAt, TransactionUpdatedAt: payload.TransactionUpdatedAt, OperationUpdatedAt: payload.OperationUpdatedAt,
			Input: payload.TransactionInput, PostingRefs: []string{projection.PostingRef}, OperationSpecs: []command.OperationRecordIntent{projection.Intent()},
		}},
	})
	require.NoError(t, err)
	payload.IntentFingerprint = fingerprint
	raw, err := command.EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	input := command.EngineExecution{
		Execution: request, IntentFingerprint: fingerprint,
		Guards:          []command.ExecutionGuard{{TransactionID: payload.TransactionID, NextToken: "frozen-execution-completed"}},
		CompletionPlans: []command.CompletionPlanRecord{{TransactionID: payload.TransactionID, Payload: raw}},
	}
	require.NoError(t, command.ValidateTransactionCompletion(input))

	return input
}

type recoveryEngineMetadataFault struct {
	*mongodb.MetadataMongoDBRepository
	fail bool
}

func (repo *recoveryEngineMetadataFault) Create(ctx context.Context, collection string, value *mongodb.Metadata) error {
	if repo.fail && collection == constant.EntityOperation {
		repo.fail = false

		return errors.New("operation metadata unavailable after SQL commit")
	}

	return repo.MetadataMongoDBRepository.Create(ctx, collection, value)
}

type recoveryEngineStoredKey struct {
	Dump   string
	Expiry int64
}

func captureRecoveryEngineState(t *testing.T, client *redis.Client, recoverKey string) map[string]recoveryEngineStoredKey {
	t.Helper()
	keys, err := client.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	state := make(map[string]recoveryEngineStoredKey)
	for _, key := range keys {
		if key == recoverKey {
			continue
		}

		dump, err := client.Dump(context.Background(), key).Result()
		require.NoError(t, err)
		expiry, err := client.Do(context.Background(), "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		state[key] = recoveryEngineStoredKey{Dump: dump, Expiry: expiry}
	}

	return state
}

func captureRecoveryEngineFinancialState(t *testing.T, client *redis.Client, recoverKey string) map[string]recoveryEngineStoredKey {
	t.Helper()
	state := captureRecoveryEngineState(t, client, recoverKey)
	for key := range state {
		if strings.HasPrefix(key, "engine:"+cachepolicy.HashTag+":receipts:") ||
			strings.HasPrefix(key, "engine:"+cachepolicy.HashTag+":guards:") ||
			strings.HasPrefix(key, "engine:"+cachepolicy.HashTag+":protection:") ||
			key == txredis.EngineRecoveryCleanupSchedule {
			delete(state, key)
		}
	}
	return state
}

func recoveryEngineKeys(t *testing.T, client *redis.Client, field, raw string, balanceID uuid.UUID) (string, string) {
	t.Helper()
	keys, err := client.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	var recoverKey, balanceKey string
	for _, key := range keys {
		kind, err := client.Type(context.Background(), key).Result()
		require.NoError(t, err)
		switch kind {
		case "hash":
			value, err := client.HGet(context.Background(), key, field).Result()
			if errors.Is(err, redis.Nil) {
				continue
			}

			require.NoError(t, err)
			if value == raw {
				recoverKey = key
			}
		case "string":
			value, err := client.Get(context.Background(), key).Bytes()
			require.NoError(t, err)
			decoded, err := balancecache.Decode(value)
			if err == nil && decoded.ID == balanceID {
				balanceKey = key
			}
		}
	}

	require.NotEmpty(t, recoverKey)
	require.NotEmpty(t, balanceKey)

	return recoverKey, balanceKey
}

func assertRecoveryEngineSQL(t *testing.T, db *sql.DB, payload *command.TransactionCompletionPlan, expected *operation.Operation) {
	t.Helper()
	ctx := context.Background()
	var rootCount, rowCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM transaction WHERE id = $1", payload.TransactionID.String()).Scan(&rootCount))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM operation WHERE transaction_id = $1", payload.TransactionID.String()).Scan(&rowCount))
	assert.Equal(t, 1, rootCount)
	assert.Equal(t, 1, rowCount)
	var rootCreatedAt, rootUpdatedAt time.Time
	require.NoError(t, db.QueryRowContext(ctx, "SELECT created_at, updated_at FROM transaction WHERE id = $1", payload.TransactionID.String()).Scan(&rootCreatedAt, &rootUpdatedAt))
	assert.True(t, payload.TransactionCreatedAt.Equal(rootCreatedAt))
	assert.True(t, payload.TransactionUpdatedAt.Equal(rootUpdatedAt))
	var id, kind, amount, before, after, holdBefore, holdAfter, direction, routeCode, snapshot string
	var beforeVersion, afterVersion int64
	var createdAt, updatedAt time.Time
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id, type, amount::text, available_balance::text, available_balance_after::text,
on_hold_balance::text, on_hold_balance_after::text, balance_version_before, balance_version_after, direction, route_code, created_at, updated_at, snapshot::text
FROM operation WHERE id = $1`, expected.ID).Scan(&id, &kind, &amount, &before, &after, &holdBefore, &holdAfter, &beforeVersion, &afterVersion, &direction, &routeCode, &createdAt, &updatedAt, &snapshot))
	assert.Equal(t, expected.ID, id)
	assert.Equal(t, expected.Type, kind)
	assert.True(t, expected.Amount.Value.Equal(decimal.RequireFromString(amount)))
	assert.True(t, expected.Balance.Available.Equal(decimal.RequireFromString(before)))
	assert.True(t, expected.BalanceAfter.Available.Equal(decimal.RequireFromString(after)))
	assert.True(t, expected.Balance.OnHold.Equal(decimal.RequireFromString(holdBefore)))
	assert.True(t, expected.BalanceAfter.OnHold.Equal(decimal.RequireFromString(holdAfter)))
	assert.Equal(t, *expected.Balance.Version, beforeVersion)
	assert.Equal(t, *expected.BalanceAfter.Version, afterVersion)
	assert.Equal(t, expected.Direction, direction)
	assert.Equal(t, *expected.RouteCode, routeCode)
	assert.True(t, expected.CreatedAt.Equal(createdAt))
	assert.True(t, expected.UpdatedAt.Equal(updatedAt))
	expectedSnapshot, err := json.Marshal(expected.Snapshot)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedSnapshot), snapshot)
}

func TestIntegrationRedisEngineCrashRecoveryConsumer(t *testing.T) {
	ctx := context.Background()
	client := recoveryEngineValkey(t)
	provider := recoveryEngineClientProvider{client: client}
	queue, err := txredis.NewConsumerRedis(provider)
	require.NoError(t, err)
	adapter, err := redisengine.NewAdapter(provider, redisengine.Limits{
		MaxTransactions: 4, MaxPostings: 16, MaxBalances: 16, MaxCompletionPlanBytes: 1 << 20, MaxRequestBytes: 1 << 20, MaxPreparedBytes: 1 << 20,
	})
	require.NoError(t, err)
	pgConfig := pgtestutil.DefaultContainerConfig()
	pgConfig.Image = "postgres:17"
	pg := pgtestutil.SetupContainerWithConfig(t, pgConfig)
	dsn := pgtestutil.BuildConnectionString(pg.Host, pg.Port, pg.Config)
	pgConnection := pgtestutil.CreatePostgresClient(t, dsn, dsn, pg.Config.DBName, pgtestutil.FindMigrationsPath(t, "transaction"))
	store := completion.NewStore(transaction.NewTransactionPostgreSQLRepository(pgConnection), operation.NewOperationPostgreSQLRepository(pgConnection))
	mongoConfig := mongotestutil.DefaultContainerConfig()
	mongoConfig.Image = "mongo:8"
	mongoContainer := mongotestutil.SetupContainerWithConfig(t, mongoConfig)
	mongoConnection := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)
	metadata := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	for _, metadataFailure := range []bool{false, true} {
		name := "crash before finalization"
		if metadataFailure {
			name = "metadata failure retains recover record"
		}

		t.Run(name, func(t *testing.T) {
			input := recoveryEngineExecution(t)
			result, err := adapter.Execute(ctx, input)
			require.NoError(t, err)
			require.NotNil(t, result)
			messages, err := queue.ReadAllRecoveryMessages(ctx, txredis.RecoveryQueueSourceEngineRecover)
			require.NoError(t, err)
			field := input.Execution.Transactions[0].ID.String() + ":" + input.Execution.ExecutionID.String()
			raw, exists := messages[field]
			require.True(t, exists, "the accounting execution must durably write recovery before its caller finalizes")
			envelope, err := command.DecodeTransactionCompletionRecord([]byte(raw))
			require.NoError(t, err)
			payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
			require.NoError(t, err)
			rows, err := command.BuildOperationRecordsFromMovements(*payload, *result)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			recoverKey, balanceKey := recoveryEngineKeys(t, client, field, raw, input.Execution.Balances[0].ID)
			changed := result.Final[0]
			changed.Available, changed.OnHold, changed.Version = decimal.NewFromInt(999), decimal.NewFromInt(12), 99
			changed.AllowSending, changed.AllowReceiving, changed.Direction = false, false, constant.DirectionDebit
			currentBalance, err := balancecache.Encode(changed, balancecache.FormatDual)
			require.NoError(t, err)
			require.NoError(t, client.Set(ctx, balanceKey, currentBalance, 24*time.Hour).Err())
			accountingState := captureRecoveryEngineState(t, client, recoverKey)
			financialState := captureRecoveryEngineFinancialState(t, client, recoverKey)
			fault := &recoveryEngineMetadataFault{MetadataMongoDBRepository: metadata, fail: metadataFailure}
			finalizer := command.NewTransactionCompletionService(store, fault)
			consumer := NewRedisQueueConsumer(recoveryQuietLogger{}, &command.UseCase{TransactionRedisRepo: queue}, nil).WithTransactionCompleter(finalizer)
			require.Nil(t, consumer.Query)
			consumer.readMessagesAndProcess(ctx)
			assertRecoveryEngineSQL(t, pg.DB, payload, rows[0])
			if metadataFailure {
				retained, err := queue.ReadAllRecoveryMessages(ctx, txredis.RecoveryQueueSourceEngineRecover)
				require.NoError(t, err)
				require.Equal(t, raw, retained[field], "metadata failure must retain the exact engine-written recover record")
				operationMetadata, err := metadata.FindByEntity(ctx, constant.EntityOperation, rows[0].ID)
				require.NoError(t, err)
				require.Nil(t, operationMetadata)
				require.Equal(t, accountingState, captureRecoveryEngineState(t, client, recoverKey))
				consumer.readMessagesAndProcess(ctx)
				assertRecoveryEngineSQL(t, pg.DB, payload, rows[0])
			}

			remaining, err := queue.ReadAllRecoveryMessages(ctx, txredis.RecoveryQueueSourceEngineRecover)
			require.NoError(t, err)
			assert.NotContains(t, remaining, field)
			require.Equal(t, financialState, captureRecoveryEngineFinancialState(t, client, recoverKey), "recovery must not mutate live balances, settings, schedule, or expiry")
			transactionMetadata, err := metadata.FindByEntity(ctx, constant.EntityTransaction, payload.TransactionID.String())
			require.NoError(t, err)
			require.NotNil(t, transactionMetadata)
			assert.Equal(t, int64(9007199254740993), transactionMetadata.Data["sequence"])
			operationMetadata, err := metadata.FindByEntity(ctx, constant.EntityOperation, rows[0].ID)
			require.NoError(t, err)
			require.NotNil(t, operationMetadata)
			assert.Equal(t, "frozen operation metadata", operationMetadata.Data["purpose"])
		})
	}
}
