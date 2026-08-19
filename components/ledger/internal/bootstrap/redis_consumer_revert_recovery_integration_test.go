//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	postgreTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegration_RevertBackupRecoveryPersistsExactParentAndCompletesClaim(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	ctx := context.Background()
	postgresContainer := postgrestestutil.SetupMigratedContainer(t, "transaction")
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN, postgresContainer.Config.DBName,
		postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupReusableContainerWithConfig(t, redistestutil.FinancialContainerConfig())
	redisConnection := redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB)
	redisRepo, err := transactionredis.NewConsumerRedis(redisConnection)
	require.NoError(t, err)
	claimRepo := revertclaim.NewPostgreSQLRepository(postgresClient)
	const expectedRedisGeneration = "645439df-1837-421e-9607-f60b091542c9"
	const initializationRequestID = "52c85247-b684-4ff7-a45e-41d8f437e4f1"
	initializer := transactionredis.NewRevertUpdateFreezeGuard(redisConnection,
		transactionredis.RevertUpdateFreezeInitialize, expectedRedisGeneration).
		WithRolloutInitializationWitness(claimRepo, initializationRequestID)
	require.Eventually(t, func() bool { return initializer.FinancialDurability(ctx) == nil },
		10*time.Second, 50*time.Millisecond)
	require.NoError(t, initializer.InitializeFinancialDatasetGeneration(ctx))
	rolloutGuard := transactionredis.NewRevertUpdateFreezeGuard(redisConnection,
		transactionredis.RevertUpdateFreezePrepared, expectedRedisGeneration).
		WithRolloutInitializationWitness(claimRepo, "")
	redisGeneration, err := rolloutGuard.FinancialDatasetGeneration(ctx)
	require.NoError(t, err)
	rolloutToken := uuid.NewString()
	admitted, leaseHeld, phase, err := rolloutGuard.AcquireRevert(ctx, "legacy", rolloutToken, "consumer-recovery-attempt")
	require.NoError(t, err)
	require.True(t, admitted)
	require.True(t, leaseHeld)
	require.Equal(t, transactionredis.RevertUpdateFreezePrepared, phase)
	require.Error(t, rolloutGuard.Activate(ctx),
		"a pre-activation reverse remains a rollout blocker until terminal persistence")

	transactionRepo := postgreTransaction.NewTransactionPostgreSQLRepository(postgresClient)
	operationRepo := operation.NewOperationPostgreSQLRepository(postgresClient)
	ctrl := gomock.NewController(t)
	rabbitRepo := rabbitmq.NewMockProducerRepository(ctrl)
	rabbitRepo.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("force synchronous recovery persistence"))

	commandUC := &command.UseCase{
		TransactionRepo:      transactionRepo,
		OperationRepo:        operationRepo,
		RevertClaimRepo:      claimRepo,
		RevertRolloutLease:   rolloutGuard,
		TransactionRedisRepo: redisRepo,
		RabbitMQRepo:         rabbitRepo,
	}
	handler := in.TransactionHandler{Command: commandUC, Query: &query.UseCase{TransactionRepo: transactionRepo}}
	consumer := NewRedisQueueConsumer(newTestLogger(), handler)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	fixedTime := time.Date(2026, time.August, 18, 0, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(100)
	approved := constant.APPROVED
	_, err = transactionRepo.Create(ctx, &postgreTransaction.Transaction{
		ID:             originID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "origin transaction",
		Amount:         &amount,
		AssetCode:      "USD",
		Status:         postgreTransaction.Status{Code: approved, Description: &approved},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	})
	require.NoError(t, err)
	zero := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)
	statusDescription := constant.CREATED
	operations := []*operation.Operation{
		recoveryOperation(reverseID, organizationID, ledgerID, uuid.New(), uuid.New(), "@source", constant.CREDIT,
			constant.DirectionCredit, &amount, &zero, &amount, &versionBefore, &versionAfter, &statusDescription, fixedTime),
		recoveryOperation(reverseID, organizationID, ledgerID, uuid.New(), uuid.New(), "@destination", constant.DEBIT,
			constant.DirectionDebit, &amount, &amount, &zero, &versionBefore, &versionAfter, &statusDescription, fixedTime),
	}
	debit := mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.DEBIT, Direction: constant.DirectionDebit,
	}
	credit := mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.CREDIT, Direction: constant.DirectionCredit,
	}
	input := mtransaction.Transaction{
		Description: "recovered reverse",
		Send: mtransaction.Send{
			Value: amount, Asset: "USD",
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@destination", BalanceKey: constant.DefaultBalanceKey, Amount: &debit, IsFrom: true,
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@source", BalanceKey: constant.DefaultBalanceKey, Amount: &credit,
			}}},
		},
	}
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			mtransaction.ConcatAlias(0, mtransaction.AliasKey("@destination", constant.DefaultBalanceKey)): debit,
		},
		To: map[string]mtransaction.Amount{
			mtransaction.ConcatAlias(0, mtransaction.AliasKey("@source", constant.DefaultBalanceKey)): credit,
		},
	}
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(input)
	require.NoError(t, err)
	expectedLegacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	queue := mmodel.TransactionRedisQueue{
		TransactionID:        reverseID,
		ParentTransactionID:  &originID,
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		TransactionInput:     input,
		Validate:             validate,
		TransactionStatus:    constant.CREATED,
		Action:               constant.ActionRevert,
		AttemptOwner:         reverseID.String(),
		ExpectedOutcome:      mmodel.TransactionOutcomeCommitted,
		RevertRolloutMode:    "legacy",
		RevertRolloutToken:   rolloutToken,
		RevertLegacyFenceKey: expectedLegacyKey,
		RedisGeneration:      redisGeneration,
		TransactionDate:      fixedTime,
		TTL:                  fixedTime,
		Balances: []mmodel.BalanceRedis{
			{ID: operations[0].BalanceID, Alias: operations[0].AccountAlias, Key: operations[0].BalanceKey,
				AccountID: operations[0].AccountID, AssetCode: operations[0].AssetCode, Available: zero,
				OnHold: zero, Version: versionBefore, AccountType: "deposit", Direction: operations[0].Direction, OverdraftUsed: "0",
				OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional},
			{ID: operations[1].BalanceID, Alias: operations[1].AccountAlias, Key: operations[1].BalanceKey,
				AccountID: operations[1].AccountID, AssetCode: operations[1].AssetCode, Available: amount,
				OnHold: zero, Version: versionBefore, AccountType: "deposit", Direction: operations[1].Direction, OverdraftUsed: "0",
				OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional},
		},
		BalancesAfter: []mmodel.BalanceRedis{
			{ID: operations[0].BalanceID, Alias: operations[0].AccountAlias, Key: operations[0].BalanceKey,
				AccountID: operations[0].AccountID, AssetCode: operations[0].AssetCode, Available: amount,
				OnHold: zero, Version: versionAfter, AccountType: "deposit", Direction: operations[0].Direction, OverdraftUsed: "0",
				OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional},
			{ID: operations[1].BalanceID, Alias: operations[1].AccountAlias, Key: operations[1].BalanceKey,
				AccountID: operations[1].AccountID, AssetCode: operations[1].AssetCode, Available: zero,
				OnHold: zero, Version: versionAfter, AccountType: "deposit", Direction: operations[1].Direction, OverdraftUsed: "0",
				OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional},
		},
		Operations: []mmodel.OperationRedis{
			operations[0].ToRedis(),
			operations[1].ToRedis(),
		},
	}
	queue.ExpectedEconomicPlan = expectedEconomicPlanForRecovery(t, organizationID, ledgerID, queue.Validate,
		[]*mmodel.Balance{
			balanceFromBackup(queue.Balances[0], organizationID, ledgerID),
			balanceFromBackup(queue.Balances[1], organizationID, ledgerID),
		}, queue.TransactionStatus, false)
	raw, err := json.Marshal(queue)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, raw))
	legacyAcquired, err := redisRepo.AcquireOwnedKey(ctx, expectedLegacyKey, reverseID.String(), 0)
	require.NoError(t, err)
	require.True(t, legacyAcquired, "the phase-zero request must leave its old-compatible empty H1 fence")
	outcomeRaw, err := json.Marshal(mmodel.BalanceExecutionOutcome{
		Identity:            reverseID,
		Owner:               reverseID.String(),
		Outcome:             mmodel.TransactionOutcomeCommitted,
		EconomicPlanVersion: strconv.Itoa(queue.ExpectedEconomicPlan.Version),
		EconomicPlanDigest:  queue.ExpectedEconomicPlan.Digest,
		Before:              queue.Balances,
		After:               queue.BalancesAfter,
	})
	require.NoError(t, err)
	require.NoError(t, redisRepo.Set(ctx,
		utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, reverseID), string(outcomeRaw), 0))

	decodedQueue := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(raw, &decodedQueue))
	require.NoError(t, redisContainer.Client.Set(ctx, transactionredis.FinancialDatasetGenerationKey,
		uuid.NewString(), 0).Err())
	parentID := originID.String()
	bulkPayload := postgreTransaction.TransactionProcessingPayload{
		Transaction: &postgreTransaction.Transaction{
			ID: reverseID.String(), ParentTransactionID: &parentID,
			OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Description: queue.TransactionInput.Description, Amount: &amount, AssetCode: "USD",
			Status:     postgreTransaction.Status{Code: constant.CREATED, Description: &statusDescription},
			Operations: operations, CreatedAt: fixedTime, UpdatedAt: fixedTime,
		},
		Input: &queue.TransactionInput, Validate: queue.Validate, Version: "v2",
		Balances: []*mmodel.Balance{
			balanceFromBackup(queue.Balances[0], organizationID, ledgerID),
			balanceFromBackup(queue.Balances[1], organizationID, ledgerID),
		},
		BalancesAfter: []*mmodel.Balance{
			balanceFromBackup(queue.BalancesAfter[0], organizationID, ledgerID),
			balanceFromBackup(queue.BalancesAfter[1], organizationID, ledgerID),
		},
		AttemptOwner: queue.AttemptOwner, ExpectedOutcome: queue.ExpectedOutcome,
		RevertRolloutMode: queue.RevertRolloutMode, RevertRolloutToken: queue.RevertRolloutToken,
		RedisGeneration: queue.RedisGeneration,
	}
	_, err = commandUC.CreateBulkTransactionOperationsAsync(ctx,
		[]postgreTransaction.TransactionProcessingPayload{bulkPayload})
	require.ErrorContains(t, err, "validate bulk Redis economic outcome")
	var bulkTransactionRows, bulkOperationRows int
	require.NoError(t, postgresContainer.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction WHERE id = $1`, reverseID).Scan(&bulkTransactionRows))
	require.NoError(t, postgresContainer.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM operation WHERE transaction_id = $1`, reverseID).Scan(&bulkOperationRows))
	assert.Zero(t, bulkTransactionRows,
		"a stale bulk consumer must stop before inserting the reverse transaction")
	assert.Zero(t, bulkOperationRows,
		"a stale bulk consumer must stop before inserting any reverse operation")

	consumer.processMessage(ctx, backupKey, string(raw), decodedQueue)
	persistedBeforeGenerationRecovery, err := transactionRepo.FindWithOperations(ctx,
		organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, persistedBeforeGenerationRecovery)
	assert.Empty(t, persistedBeforeGenerationRecovery.ID,
		"a delayed consumer from another Redis generation must stop before any PostgreSQL write")
	claimBeforeGenerationRecovery, err := claimRepo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	assert.Nil(t, claimBeforeGenerationRecovery)
	require.Error(t, rolloutGuard.Activate(ctx),
		"generation rejection must preserve the admitted rollout lease for reconciliation")
	require.NoError(t, redisContainer.Client.Set(ctx, transactionredis.FinancialDatasetGenerationKey,
		redisGeneration, 0).Err())
	consumer.processMessage(ctx, backupKey, string(raw), decodedQueue)
	require.NoError(t, rolloutGuard.Activate(ctx),
		"terminal handoff must release the exact pre-activation origin only after PostgreSQL and Redis agree")

	persisted, err := transactionRepo.FindWithOperations(ctx, organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, reverseID.String(), persisted.ID, "backup consumer must persist the reserved reverse")
	require.NotNil(t, persisted.ParentTransactionID)
	assert.Equal(t, originID.String(), *persisted.ParentTransactionID)
	require.Len(t, persisted.Operations, 2)

	completed, err := claimRepo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, reverseID, completed.ReverseTransactionID,
		"phase-zero recovery must adopt the exact backup reverse instead of minting a replacement")
	assert.Equal(t, revertclaim.StateCompleted, completed.State)
	require.NotNil(t, completed.LegacyFenceKey,
		"phase-zero adoption must persist the exact legacy fence derived from its immutable backup")
	assert.Equal(t, expectedLegacyKey, *completed.LegacyFenceKey)
	legacyReplay, err := redisRepo.Get(ctx, expectedLegacyKey)
	require.NoError(t, err)
	require.NotEmpty(t, legacyReplay, "terminal phase-zero adoption must publish H1 without waiting for another HTTP retry")
	decodedReplay := postgreTransaction.Transaction{}
	require.NoError(t, json.Unmarshal([]byte(legacyReplay), &decodedReplay))
	assert.Equal(t, reverseID.String(), decodedReplay.ID)
	require.NotNil(t, decodedReplay.ParentTransactionID)
	assert.Equal(t, originID.String(), *decodedReplay.ParentTransactionID)
	mutatedPayload := queue.TransactionInput
	mutatedPayload.Description = "payload changed after the phase-zero backup"
	mutatedHash, err := utils.LegacyTransactionIdempotencyHash(mutatedPayload)
	require.NoError(t, err)
	assert.NotEqual(t, utils.IdempotencyInternalKey(organizationID, ledgerID, mutatedHash), *completed.LegacyFenceKey,
		"later payload changes must never rewrite the H1 key recovered from the phase-zero backup")

	var reverseCount int
	require.NoError(t, postgresContainer.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transaction
		WHERE organization_id = $1 AND ledger_id = $2 AND parent_transaction_id = $3`,
		organizationID, ledgerID, originID).Scan(&reverseCount))
	assert.Equal(t, 1, reverseCount)
	require.Eventually(t, func() bool {
		_, readErr := redisRepo.ReadMessageFromQueue(ctx, backupKey)
		return errors.Is(readErr, redislib.Nil)
	}, 2*time.Second, 20*time.Millisecond, "completed recovery must remove the Redis backup")
	lostAckReplay, err := commandUC.CreateBulkTransactionOperationsAsync(ctx,
		[]postgreTransaction.TransactionProcessingPayload{bulkPayload})
	require.NoError(t, err,
		"redelivery after PostgreSQL commit and Redis cleanup must prove the terminal receipt and acknowledge")
	assert.Zero(t, lostAckReplay.TransactionsAttempted,
		"terminal redelivery must not enter any PostgreSQL write branch")
	require.NoError(t, postgresContainer.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transaction
		WHERE organization_id = $1 AND ledger_id = $2 AND parent_transaction_id = $3`,
		organizationID, ledgerID, originID).Scan(&reverseCount))
	assert.Equal(t, 1, reverseCount)
	require.NoError(t, redisContainer.Client.Del(ctx, transactionredis.FinancialDatasetGenerationKey,
		transactionredis.RevertUpdateFreezeKey, transactionredis.RevertRolloutGenerationKey).Err())
	require.Error(t, initializer.InitializeFinancialDatasetGeneration(ctx),
		"the real PostgreSQL PREPARED birth certificate must prevent total Redis loss from becoming first install")
	assert.Zero(t, redisContainer.Client.Exists(ctx, transactionredis.FinancialDatasetGenerationKey,
		transactionredis.RevertUpdateFreezeKey, transactionredis.RevertRolloutGenerationKey).Val())
}

func TestIntegration_LifecycleBackupRecoveryAfterLeaseExpiryPersistsExactOperationsAndCleansOutcome(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	ctx := context.Background()
	postgresContainer := postgrestestutil.SetupMigratedContainer(t, "transaction")
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN, postgresContainer.Config.DBName,
		postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupReusableContainer(t)
	redisConnection := redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB)
	redisRepo, err := transactionredis.NewConsumerRedis(redisConnection)
	require.NoError(t, err)

	transactionRepo := postgreTransaction.NewTransactionPostgreSQLRepository(postgresClient)
	operationRepo := operation.NewOperationPostgreSQLRepository(postgresClient)
	ctrl := gomock.NewController(t)
	rabbitRepo := rabbitmq.NewMockProducerRepository(ctrl)
	rabbitRepo.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("force synchronous recovery persistence"))
	commandUC := &command.UseCase{
		TransactionRepo:      transactionRepo,
		OperationRepo:        operationRepo,
		TransactionRedisRepo: redisRepo,
		RabbitMQRepo:         rabbitRepo,
	}
	handler := in.TransactionHandler{Command: commandUC, Query: &query.UseCase{TransactionRepo: transactionRepo}}
	consumer := NewRedisQueueConsumer(newTestLogger(), handler)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	owner := uuid.NewString()
	fixedTime := time.Date(2026, time.August, 18, 0, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(100)
	pending := constant.PENDING
	debit := mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.DEBIT, Direction: constant.DirectionDebit,
	}
	credit := mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.CREDIT, Direction: constant.DirectionCredit,
	}
	input := mtransaction.Transaction{
		Description: "pending transfer",
		Send: mtransaction.Send{
			Value: amount, Asset: "USD",
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@source", BalanceKey: constant.DefaultBalanceKey, Amount: &debit, IsFrom: true,
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@destination", BalanceKey: constant.DefaultBalanceKey, Amount: &credit,
			}}},
		},
	}
	_, err = transactionRepo.Create(ctx, &postgreTransaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Description:    input.Description,
		Amount:         &amount,
		AssetCode:      "USD",
		Body:           input,
		Status:         postgreTransaction.Status{Code: pending, Description: &pending},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	})
	require.NoError(t, err)

	zero := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)
	approved := constant.APPROVED
	operations := []*operation.Operation{
		recoveryOperation(transactionID, organizationID, ledgerID, uuid.New(), uuid.New(), "@source", constant.DEBIT,
			constant.DirectionDebit, &amount, &amount, &zero, &versionBefore, &versionAfter, &approved, fixedTime),
		recoveryOperation(transactionID, organizationID, ledgerID, uuid.New(), uuid.New(), "@destination", constant.CREDIT,
			constant.DirectionCredit, &amount, &zero, &amount, &versionBefore, &versionAfter, &approved, fixedTime),
	}
	completeSnapshot := func(operation *operation.Operation, available decimal.Decimal, version int64) mmodel.BalanceRedis {
		return mmodel.BalanceRedis{
			ID: operation.BalanceID, Alias: operation.AccountAlias, Key: operation.BalanceKey,
			AccountID: operation.AccountID, AssetCode: operation.AssetCode, Available: available,
			OnHold: zero, Version: version, AccountType: "deposit", Direction: operation.Direction,
			OverdraftUsed: "0", OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional,
		}
	}
	before := []mmodel.BalanceRedis{
		completeSnapshot(operations[0], amount, versionBefore),
		completeSnapshot(operations[1], zero, versionBefore),
	}
	after := []mmodel.BalanceRedis{
		completeSnapshot(operations[0], zero, versionAfter),
		completeSnapshot(operations[1], amount, versionAfter),
	}
	queue := mmodel.TransactionRedisQueue{
		TransactionID:    transactionID,
		OrganizationID:   organizationID,
		LedgerID:         ledgerID,
		TransactionInput: input,
		Validate: &mtransaction.Responses{
			Pending: true,
			From: map[string]mtransaction.Amount{
				mtransaction.ConcatAlias(0, mtransaction.AliasKey("@source", constant.DefaultBalanceKey)): debit,
			},
			To: map[string]mtransaction.Amount{
				mtransaction.ConcatAlias(0, mtransaction.AliasKey("@destination", constant.DefaultBalanceKey)): credit,
			},
		},
		TransactionStatus: constant.APPROVED,
		Action:            constant.ActionCommit,
		TransactionDate:   fixedTime,
		TTL:               fixedTime.Add(-10 * time.Minute),
		Balances:          before,
		BalancesAfter:     after,
		AttemptOwner:      owner,
		ExpectedOutcome:   mmodel.TransactionOutcomeCommitted,
		Operations:        []mmodel.OperationRedis{operations[0].ToRedis(), operations[1].ToRedis()},
	}
	queue.ExpectedEconomicPlan = expectedEconomicPlanForRecovery(t, organizationID, ledgerID, queue.Validate,
		[]*mmodel.Balance{
			balanceFromBackup(queue.Balances[0], organizationID, ledgerID),
			balanceFromBackup(queue.Balances[1], organizationID, ledgerID),
		}, queue.TransactionStatus, true)
	raw, err := json.Marshal(queue)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, raw))
	outcomeRaw, err := json.Marshal(mmodel.BalanceExecutionOutcome{
		Identity:            transactionID,
		Outcome:             mmodel.TransactionOutcomeCommitted,
		Owner:               owner,
		EconomicPlanVersion: strconv.Itoa(queue.ExpectedEconomicPlan.Version),
		EconomicPlanDigest:  queue.ExpectedEconomicPlan.Digest,
		Before:              before,
		After:               after,
	})
	require.NoError(t, err)
	require.NoError(t, redisRepo.Set(ctx, outcomeKey, string(outcomeRaw), 0))

	decodedQueue := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(raw, &decodedQueue))
	consumer.processMessage(ctx, backupKey, string(raw), decodedQueue)

	assert.Equal(t, constant.APPROVED, postgrestestutil.GetTransactionStatus(t, postgresContainer.DB, transactionID))
	persisted, err := transactionRepo.FindWithOperations(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Len(t, persisted.Operations, 2)
	assert.ElementsMatch(t, []string{operations[0].ID, operations[1].ID},
		[]string{persisted.Operations[0].ID, persisted.Operations[1].ID})
	_, err = redisRepo.ReadMessageFromQueue(ctx, backupKey)
	require.ErrorIs(t, err, redislib.Nil)
	outcomeValue, err := redisRepo.Get(ctx, outcomeKey)
	require.NoError(t, err)
	assert.Empty(t, outcomeValue, "exact outcome remains until delayed recovery is fully durable, then is removed")
}

func TestIntegration_RevertBackupRecoveryAdoptsPartialDeterministicOperationSet(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	ctx := context.Background()
	postgresContainer := postgrestestutil.SetupMigratedContainer(t, "transaction")
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN, postgresContainer.Config.DBName,
		postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupReusableContainer(t)
	redisConnection := redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB)
	redisRepo, err := transactionredis.NewConsumerRedis(redisConnection)
	require.NoError(t, err)

	transactionRepo := postgreTransaction.NewTransactionPostgreSQLRepository(postgresClient)
	operationRepo := operation.NewOperationPostgreSQLRepository(postgresClient)
	claimRepo := revertclaim.NewPostgreSQLRepository(postgresClient)
	ctrl := gomock.NewController(t)
	rabbitRepo := rabbitmq.NewMockProducerRepository(ctrl)
	rabbitRepo.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("force synchronous recovery persistence"))
	ledgerRepo := ledger.NewMockRepository(ctrl)
	ledgerRepo.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).Return(map[string]any{}, nil)

	commandUC := &command.UseCase{
		TransactionRepo: transactionRepo, OperationRepo: operationRepo, RevertClaimRepo: claimRepo,
		TransactionRedisRepo: redisRepo, RabbitMQRepo: rabbitRepo,
	}
	queryUC := &query.UseCase{TransactionRepo: transactionRepo, LedgerRepo: ledgerRepo}
	handler := in.TransactionHandler{Command: commandUC, Query: queryUC}
	consumer := NewRedisQueueConsumer(newTestLogger(), handler)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	fixedTime := time.Date(2026, time.August, 18, 1, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(100)
	zero := decimal.Zero
	approved := constant.APPROVED
	_, err = transactionRepo.Create(ctx, &postgreTransaction.Transaction{
		ID: originID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		Description: "origin", Amount: &amount, AssetCode: "USD",
		Status: postgreTransaction.Status{Code: approved, Description: &approved}, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
	require.NoError(t, err)
	parent := originID.String()
	_, err = transactionRepo.Create(ctx, &postgreTransaction.Transaction{
		ID: reverseID.String(), ParentTransactionID: &parent, OrganizationID: organizationID.String(),
		LedgerID: ledgerID.String(), Description: "partially persisted reverse", Amount: &amount,
		AssetCode: "USD", Status: postgreTransaction.Status{Code: approved, Description: &approved},
		CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
	require.NoError(t, err)
	_, acquired, err := claimRepo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	reason := "post_balance_persistence_incomplete"
	require.NoError(t, claimRepo.Transition(ctx, organizationID, ledgerID, originID, reverseID,
		revertclaim.StateReconciliationRequired, &reason))

	fromAlias := "0#@destination#default"
	toAlias := "0#@source#default"
	fromAccountID, fromBalanceID := uuid.New(), uuid.New()
	toAccountID, toBalanceID := uuid.New(), uuid.New()
	before := []*mmodel.Balance{
		{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: amount, Version: 1, AccountType: "deposit", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Direction: constant.DirectionDebit, OverdraftUsed: decimal.NewFromInt(125),
			Settings: &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal, AllowOverdraft: true,
				OverdraftLimitEnabled: true, OverdraftLimit: func() *string { value := "500"; return &value }()}},
		{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: zero, Version: 1, AccountType: "deposit", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Direction: constant.DirectionCredit},
	}
	after := []*mmodel.Balance{
		{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: zero, Version: 7, AccountType: "deposit", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Direction: constant.DirectionDebit, OverdraftUsed: decimal.NewFromInt(25),
			Settings: &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal, AllowOverdraft: true,
				OverdraftLimitEnabled: true, OverdraftLimit: func() *string { value := "500"; return &value }()}},
		{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: amount, Version: 9, AccountType: "deposit", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Direction: constant.DirectionCredit},
	}
	debit := mtransaction.Amount{Asset: "USD", Value: amount, Operation: constant.DEBIT, Direction: constant.DirectionDebit}
	credit := mtransaction.Amount{Asset: "USD", Value: amount, Operation: constant.CREDIT, Direction: constant.DirectionCredit}
	input := mtransaction.Transaction{
		Description: "partially persisted reverse",
		Send: mtransaction.Send{Asset: "USD", Value: amount,
			Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "@destination", BalanceKey: constant.DefaultBalanceKey, Amount: &debit, IsFrom: true}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "@source", BalanceKey: constant.DefaultBalanceKey, Amount: &credit}}}},
	}
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{fromAlias: debit}, To: map[string]mtransaction.Amount{toAlias: credit},
		Sources: []string{fromAlias}, Destinations: []string{toAlias}, Aliases: []string{fromAlias, toAlias},
	}
	fromTo := []mtransaction.FromTo{
		{AccountAlias: fromAlias, BalanceKey: constant.DefaultBalanceKey, Amount: &debit, IsFrom: true},
		{AccountAlias: toAlias, BalanceKey: constant.DefaultBalanceKey, Amount: &credit},
	}
	expected, _, err := handler.BuildOperations(ctx, before, after, fromTo, input, postgreTransaction.Transaction{
		ID: reverseID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
	}, validate, fixedTime, false, false, nil, constant.ActionRevert)
	require.NoError(t, err)
	require.Len(t, expected, 2)
	_, err = operationRepo.Create(ctx, expected[1])
	require.NoError(t, err, "simulate a crash after only the first operation became durable")

	queue := mmodel.TransactionRedisQueue{
		TransactionID: reverseID, ParentTransactionID: &originID, OrganizationID: organizationID, LedgerID: ledgerID,
		TransactionInput: input, Validate: validate, TransactionStatus: constant.CREATED, Action: constant.ActionRevert,
		TransactionDate: fixedTime, TTL: fixedTime,
		EffectModeVersion: mmodel.TransactionEffectModeVersion,
		EffectMode:        mmodel.TransactionEffectBalanceMutation,
		AttemptOwner:      reverseID.String(),
		ExpectedOutcome:   mmodel.TransactionOutcomeCommitted,
		Balances: []mmodel.BalanceRedis{
			{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: amount, Version: 1,
				AccountType: "deposit", Direction: constant.DirectionDebit, OverdraftUsed: "125", AllowOverdraft: 1, OverdraftLimitEnabled: 1,
				OverdraftLimit: "500", BalanceScope: mmodel.BalanceScopeInternal},
			{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: zero, Version: 1,
				AccountType: "deposit", Direction: constant.DirectionCredit, OverdraftUsed: "0", OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional},
		},
		BalancesAfter: []mmodel.BalanceRedis{
			{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: zero, Version: 7,
				AccountType: "deposit", Direction: constant.DirectionDebit, OverdraftUsed: "25", AllowOverdraft: 1, OverdraftLimitEnabled: 1,
				OverdraftLimit: "500", BalanceScope: mmodel.BalanceScopeInternal},
			{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: amount, Version: 9,
				AccountType: "deposit", Direction: constant.DirectionCredit, OverdraftUsed: "0", OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional},
		},
		// Operations intentionally absent: this is the lost best-effort
		// materialization window that used to mint new IDs on every replay.
	}
	queue.ExpectedEconomicPlan = expectedEconomicPlanForRecovery(t, organizationID, ledgerID, queue.Validate,
		[]*mmodel.Balance{
			balanceFromBackup(queue.Balances[0], organizationID, ledgerID),
			balanceFromBackup(queue.Balances[1], organizationID, ledgerID),
		}, queue.TransactionStatus, false)
	rebuiltEconomicOperations := make([]mmodel.OperationRedis, 0, len(expected))
	for _, candidate := range expected {
		rebuiltEconomicOperations = append(rebuiltEconomicOperations, candidate.ToRedis())
	}
	require.NoError(t, mmodel.ValidateRedisTransactionEconomicEffect(&queue, rebuiltEconomicOperations),
		"the deterministic recovery candidate must prove every immutable input leg before Redis enrichment")
	raw, err := json.Marshal(queue)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, raw))
	outcomeRaw, err := json.Marshal(mmodel.BalanceExecutionOutcome{
		Identity:            reverseID,
		Owner:               reverseID.String(),
		Outcome:             mmodel.TransactionOutcomeCommitted,
		EconomicPlanVersion: strconv.Itoa(queue.ExpectedEconomicPlan.Version),
		EconomicPlanDigest:  queue.ExpectedEconomicPlan.Digest,
		Before:              queue.Balances,
		After:               queue.BalancesAfter,
	})
	require.NoError(t, err)
	require.NoError(t, redisRepo.Set(ctx,
		utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, reverseID), string(outcomeRaw), 0))

	consumer.processMessage(ctx, backupKey, string(raw), queue)

	persisted, err := transactionRepo.FindWithOperations(ctx, organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Len(t, persisted.Operations, 2, "partial replay must fill the missing operation without duplicating the first")
	operationIDs := map[string]struct{}{persisted.Operations[0].ID: {}, persisted.Operations[1].ID: {}}
	assert.Contains(t, operationIDs, expected[0].ID)
	assert.Contains(t, operationIDs, expected[1].ID)
	for _, op := range persisted.Operations {
		if op.BalanceID == fromBalanceID.String() {
			require.NotNil(t, op.BalanceAfter.Version)
			assert.Equal(t, int64(7), *op.BalanceAfter.Version, "rebuild must persist Lua's authoritative after-version")
			assert.Equal(t, constant.DirectionDebit, op.Direction)
			assert.Equal(t, "125", op.Snapshot.OverdraftUsedBefore)
			assert.Equal(t, "25", op.Snapshot.OverdraftUsedAfter,
				"rebuild must preserve Lua's authoritative overdraft audit state")
		}
		if op.BalanceID == toBalanceID.String() {
			require.NotNil(t, op.BalanceAfter.Version)
			assert.Equal(t, int64(9), *op.BalanceAfter.Version, "rebuild must persist Lua's authoritative after-version")
		}
	}
	completed, err := claimRepo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, revertclaim.StateCompleted, completed.State)
}

func expectedEconomicPlanForRecovery(
	t *testing.T,
	organizationID, ledgerID uuid.UUID,
	validate *mtransaction.Responses,
	balances []*mmodel.Balance,
	transactionStatus string,
	pending bool,
) *mmodel.ExpectedEconomicPlan {
	t.Helper()

	balancesByAlias := make(map[string]*mmodel.Balance, len(balances))
	for _, balance := range balances {
		key := balance.Key
		if key == "" {
			key = constant.DefaultBalanceKey
		}
		balancesByAlias[mtransaction.AliasKey(balance.Alias, key)] = balance
		balancesByAlias[mtransaction.SplitAliasWithKey(balance.Alias)] = balance
	}

	operations := make([]mmodel.BalanceOperation, 0, len(validate.From)+len(validate.To))
	appendOperations := func(amounts map[string]mtransaction.Amount, side string) {
		for alias, amount := range amounts {
			resolvedAlias := mtransaction.SplitAliasWithKey(alias)
			balance := balancesByAlias[resolvedAlias]
			require.NotNil(t, balance, "expected economic plan balance for %s", alias)
			operations = append(operations, mmodel.BalanceOperation{
				Balance: balance,
				Alias:   alias,
				Amount:  amount,
				InternalKey: utils.BalanceInternalKey(
					organizationID, ledgerID, resolvedAlias,
				),
				EconomicSide: side,
				EconomicRole: amount.EconomicRole,
			})
		}
	}
	appendOperations(validate.From, mmodel.EconomicSideSource)
	appendOperations(validate.To, mmodel.EconomicSideDestination)

	plan, err := mmodel.BuildExpectedEconomicPlan(operations, transactionStatus, pending, "")
	require.NoError(t, err)

	return plan
}

func recoveryOperation(
	transactionID, organizationID, ledgerID, accountID, balanceID uuid.UUID,
	alias, operationType, direction string,
	amount, availableBefore, availableAfter *decimal.Decimal,
	versionBefore, versionAfter *int64,
	statusDescription *string,
	createdAt time.Time,
) *operation.Operation {
	zero := decimal.Zero

	return &operation.Operation{
		ID:              uuid.NewString(),
		TransactionID:   transactionID.String(),
		Description:     "recovered reverse operation",
		Type:            operationType,
		AssetCode:       "USD",
		ChartOfAccounts: "recovery",
		Amount:          operation.Amount{Value: amount},
		Balance: operation.Balance{
			Available: availableBefore,
			OnHold:    &zero,
			Version:   versionBefore,
		},
		BalanceAfter: operation.Balance{
			Available: availableAfter,
			OnHold:    &zero,
			Version:   versionAfter,
		},
		Status:          operation.Status{Code: constant.CREATED, Description: statusDescription},
		AccountID:       accountID.String(),
		AccountAlias:    alias,
		BalanceKey:      constant.DefaultBalanceKey,
		BalanceID:       balanceID.String(),
		OrganizationID:  organizationID.String(),
		LedgerID:        ledgerID.String(),
		BalanceAffected: true,
		Direction:       direction,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  "0",
		},
	}
}
