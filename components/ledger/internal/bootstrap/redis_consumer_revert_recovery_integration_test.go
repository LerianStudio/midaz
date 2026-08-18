//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
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
	postgresContainer := postgrestestutil.SetupContainer(t)
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN, postgresContainer.Config.DBName,
		postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupContainer(t)
	redisConnection := redistestutil.CreateConnection(t, redisContainer.Addr)
	redisRepo, err := transactionredis.NewConsumerRedis(redisConnection)
	require.NoError(t, err)

	transactionRepo := postgreTransaction.NewTransactionPostgreSQLRepository(postgresClient)
	operationRepo := operation.NewOperationPostgreSQLRepository(postgresClient)
	claimRepo := revertclaim.NewPostgreSQLRepository(postgresClient)
	ctrl := gomock.NewController(t)
	rabbitRepo := rabbitmq.NewMockProducerRepository(ctrl)
	rabbitRepo.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("force synchronous recovery persistence"))

	commandUC := &command.UseCase{
		TransactionRepo:      transactionRepo,
		OperationRepo:        operationRepo,
		RevertClaimRepo:      claimRepo,
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
	queue := mmodel.TransactionRedisQueue{
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
		TransactionInput: mtransaction.Transaction{
			Description: "recovered reverse",
			Send:        mtransaction.Send{Value: amount, Asset: "USD"},
		},
		Validate:          &mtransaction.Responses{},
		TransactionStatus: constant.CREATED,
		Action:            constant.ActionRevert,
		TransactionDate:   fixedTime,
		TTL:               fixedTime,
		BalancesAfter: []mmodel.BalanceRedis{
			{ID: operations[0].BalanceID, Alias: operations[0].AccountAlias, Available: amount},
			{ID: operations[1].BalanceID, Alias: operations[1].AccountAlias, Available: zero},
		},
		Operations: []mmodel.OperationRedis{
			operations[0].ToRedis(),
			operations[1].ToRedis(),
		},
	}
	raw, err := json.Marshal(queue)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, raw))
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(queue.TransactionInput)
	require.NoError(t, err)
	expectedLegacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	legacyAcquired, err := redisRepo.SetNX(ctx, expectedLegacyKey, "", 0)
	require.NoError(t, err)
	require.True(t, legacyAcquired, "the phase-zero request must leave its old-compatible empty H1 fence")

	consumer.processMessage(ctx, backupKey, string(raw), queue)

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
}

func TestIntegration_LifecycleBackupRecoveryAfterLeaseExpiryPersistsExactOperationsAndCleansOutcome(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	ctx := context.Background()
	postgresContainer := postgrestestutil.SetupContainer(t)
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN, postgresContainer.Config.DBName,
		postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupContainer(t)
	redisConnection := redistestutil.CreateConnection(t, redisContainer.Addr)
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
	input := mtransaction.Transaction{
		Description: "pending transfer",
		Send:        mtransaction.Send{Value: amount, Asset: "USD"},
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
	before := []mmodel.BalanceRedis{{ID: operations[0].BalanceID, Available: amount}, {ID: operations[1].BalanceID, Available: zero}}
	after := []mmodel.BalanceRedis{{ID: operations[0].BalanceID, Available: zero}, {ID: operations[1].BalanceID, Available: amount}}
	queue := mmodel.TransactionRedisQueue{
		TransactionID:     transactionID,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionInput:  input,
		Validate:          &mtransaction.Responses{Pending: true},
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
	raw, err := json.Marshal(queue)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, raw))
	outcomeRaw, err := json.Marshal(mmodel.BalanceExecutionOutcome{
		Identity: transactionID,
		Outcome:  mmodel.TransactionOutcomeCommitted,
		Owner:    owner,
		Before:   before,
		After:    after,
	})
	require.NoError(t, err)
	require.NoError(t, redisRepo.Set(ctx, outcomeKey, string(outcomeRaw), 0))

	consumer.processMessage(ctx, backupKey, string(raw), queue)

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
	postgresContainer := postgrestestutil.SetupContainer(t)
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN, postgresContainer.Config.DBName,
		postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupContainer(t)
	redisConnection := redistestutil.CreateConnection(t, redisContainer.Addr)
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
	_, acquired, err := claimRepo.Claim(ctx, organizationID, ledgerID, originID, reverseID, nil, nil)
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
			AssetCode: "USD", Available: amount, Version: 1, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Direction: constant.DirectionDebit, OverdraftUsed: decimal.NewFromInt(125),
			Settings: &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal, AllowOverdraft: true,
				OverdraftLimitEnabled: true, OverdraftLimit: func() *string { value := "500"; return &value }()}},
		{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: zero, Version: 1, OrganizationID: organizationID.String(), LedgerID: ledgerID.String()},
	}
	after := []*mmodel.Balance{
		{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: zero, Version: 7, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Direction: constant.DirectionDebit, OverdraftUsed: decimal.NewFromInt(25),
			Settings: &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal, AllowOverdraft: true,
				OverdraftLimitEnabled: true, OverdraftLimit: func() *string { value := "500"; return &value }()}},
		{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: amount, Version: 9, OrganizationID: organizationID.String(), LedgerID: ledgerID.String()},
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
		Balances: []mmodel.BalanceRedis{
			{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: amount, Version: 1,
				Direction: constant.DirectionDebit, OverdraftUsed: "125", AllowOverdraft: 1, OverdraftLimitEnabled: 1,
				OverdraftLimit: "500", BalanceScope: mmodel.BalanceScopeInternal},
			{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: zero, Version: 1},
		},
		BalancesAfter: []mmodel.BalanceRedis{
			{ID: fromBalanceID.String(), AccountID: fromAccountID.String(), Alias: fromAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: zero, Version: 7,
				Direction: constant.DirectionDebit, OverdraftUsed: "25", AllowOverdraft: 1, OverdraftLimitEnabled: 1,
				OverdraftLimit: "500", BalanceScope: mmodel.BalanceScopeInternal},
			{ID: toBalanceID.String(), AccountID: toAccountID.String(), Alias: toAlias, Key: constant.DefaultBalanceKey, AssetCode: "USD", Available: amount, Version: 9},
		},
		// Operations intentionally absent: this is the lost best-effort
		// materialization window that used to mint new IDs on every replay.
	}
	raw, err := json.Marshal(queue)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, raw))

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
