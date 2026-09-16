//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type countingAtomicBatchRecoveryEngine struct {
	delegate command.Engine
	calls    int
}

func (engine *countingAtomicBatchRecoveryEngine) Execute(
	ctx context.Context,
	execution command.EngineExecution,
) (*accounting.ExecutionResult, error) {
	engine.calls++

	return engine.delegate.Execute(ctx, execution)
}

type failureInjectedBatchProjectionStore struct {
	mu                     sync.Mutex
	transactions           map[uuid.UUID]*transaction.Transaction
	attempts               map[uuid.UUID]int
	durableWrites          map[uuid.UUID]int
	failCompletion         map[uuid.UUID]int
	projectionReadFailures int
}

func newFailureInjectedBatchProjectionStore() *failureInjectedBatchProjectionStore {
	return &failureInjectedBatchProjectionStore{
		transactions:   make(map[uuid.UUID]*transaction.Transaction),
		attempts:       make(map[uuid.UUID]int),
		durableWrites:  make(map[uuid.UUID]int),
		failCompletion: make(map[uuid.UUID]int),
	}
}

func (store *failureInjectedBatchProjectionStore) Complete(
	_ context.Context,
	record *command.TransactionCompletionRecord,
) (command.TransactionCompletionResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.attempts[record.TransactionID]++
	if store.failCompletion[record.TransactionID] > 0 {
		store.failCompletion[record.TransactionID]--

		return command.TransactionCompletionResult{}, errors.New("injected projection failure")
	}

	if durable, found := store.transactions[record.TransactionID]; found {
		return atomicBatchRecoveryCompletionResult(durable), nil
	}

	plan, err := command.DecodeTransactionCompletionPlan([]byte(record.Payload))
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}
	writeSet, err := command.BuildTransactionWriteSet(*plan, record.Result)
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}
	store.transactions[record.TransactionID] = writeSet.Transaction
	store.durableWrites[record.TransactionID]++

	return command.TransactionCompletionResult{
		Record:  writeSet,
		Outcome: command.TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
	}, nil
}

func atomicBatchRecoveryCompletionResult(tran *transaction.Transaction) command.TransactionCompletionResult {
	return command.TransactionCompletionResult{
		Record: command.TransactionWriteSet{
			Transaction: tran,
			Action:      constant.ActionDirect,
		},
		Outcome: command.TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
	}
}

func (store *failureInjectedBatchProjectionStore) GetAtomicTransactionBatchProjections(
	_ context.Context,
	_, _ uuid.UUID,
	transactionIDs []uuid.UUID,
) ([]*transaction.Transaction, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.projectionReadFailures > 0 {
		store.projectionReadFailures--

		return nil, errors.New("injected terminal projection read failure")
	}

	transactions := make([]*transaction.Transaction, 0, len(transactionIDs))
	for index := len(transactionIDs) - 1; index >= 0; index-- {
		if tran, found := store.transactions[transactionIDs[index]]; found {
			transactions = append(transactions, tran)
		}
	}

	return transactions, nil
}

func (store *failureInjectedBatchProjectionStore) durableWriteCount(transactionID uuid.UUID) int {
	store.mu.Lock()
	defer store.mu.Unlock()

	return store.durableWrites[transactionID]
}

func (store *failureInjectedBatchProjectionStore) projectionCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()

	return len(store.transactions)
}

func TestIntegrationAtomicTransactionBatchFailureRecoveryConverges(t *testing.T) {
	ctx := context.Background()
	client := recoveryEngineValkey(t)

	tests := []struct {
		name                   string
		failSecondCompletion   bool
		failTerminalProjection bool
	}{
		{name: "between individual projection completions", failSecondCompletion: true},
		{name: "before terminal idempotency storage", failTerminalProjection: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tenantID := "atomic-batch-recovery-" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name())).String()
			testCtx := tmcore.ContextWithTenantID(ctx, tenantID)
			provider := recoveryEngineClientProvider{client: client}
			repository, err := txredis.NewConsumerRedis(provider)
			require.NoError(t, err)
			adapter, err := redisengine.NewAdapter(provider)
			require.NoError(t, err)
			engine := &countingAtomicBatchRecoveryEngine{delegate: adapter}
			execution := atomicBatchRecoveryExecution(t, tenantID)
			effectiveKey, appliedRecord := seedAtomicBatchRecoveryIdempotency(t, testCtx, repository, execution)

			result, err := engine.Execute(testCtx, execution)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, engine.calls)
			require.Len(t, result.Final, 1)
			assert.True(t, result.Final[0].Available.Equal(decimal.NewFromInt(50)))
			assert.Equal(t, int64(9), result.Final[0].Version)

			messages, err := repository.ReadAllRecoveryMessages(testCtx, txredis.RecoveryQueueSourceEngineRecover)
			require.NoError(t, err)
			require.Len(t, messages, len(execution.Execution.Transactions))
			fields := atomicBatchRecoveryFields(execution)
			for _, field := range fields {
				require.NotEmpty(t, messages[field])
			}

			_, balanceKey := recoveryEngineKeys(
				t,
				client,
				fields[0],
				messages[fields[0]],
				execution.Execution.Balances[0].ID,
			)
			monetaryState, err := client.Get(testCtx, balanceKey).Result()
			require.NoError(t, err)
			artifactKeys := atomicBatchRecoveryArtifactKeys(t, testCtx, execution)
			assertAtomicBatchRecoveryProtection(t, testCtx, client, artifactKeys, 2)

			store := newFailureInjectedBatchProjectionStore()
			secondID := execution.Execution.Transactions[1].ID
			if test.failSecondCompletion {
				store.failCompletion[secondID] = 1
			}
			if test.failTerminalProjection {
				store.projectionReadFailures = 1
			}
			finalizer := &command.UseCase{
				AtomicTransactionBatchIdempotencyRepo:  repository,
				AtomicTransactionBatchProjectionReader: store,
			}
			completedAt := time.Date(2026, time.September, 16, 18, 30, 0, 0, time.UTC)
			coordinator := &recoveryRecordCompleter{
				logger:         recoveryQuietLogger{},
				queue:          repository,
				completer:      store,
				batchFinalizer: finalizer,
				clock:          func() time.Time { return completedAt },
			}

			firstEnvelope, err := command.DecodeTransactionCompletionRecord([]byte(messages[fields[0]]))
			require.NoError(t, err)
			require.NoError(t, coordinator.complete(
				testCtx,
				txredis.RecoveryQueueSourceEngineRecover,
				fields[0],
				messages[fields[0]],
				firstEnvelope,
			))
			assert.Equal(t, 1, store.durableWrites[firstEnvelope.TransactionID])
			assertAtomicBatchRecoveryMember(t, testCtx, repository, fields[0], "")
			assertAtomicBatchRecoveryMember(t, testCtx, repository, fields[1], messages[fields[1]])
			assertAtomicBatchRecoveryProtection(t, testCtx, client, artifactKeys, 2)

			secondEnvelope, err := command.DecodeTransactionCompletionRecord([]byte(messages[fields[1]]))
			require.NoError(t, err)
			err = coordinator.complete(
				testCtx,
				txredis.RecoveryQueueSourceEngineRecover,
				fields[1],
				messages[fields[1]],
				secondEnvelope,
			)
			require.Error(t, err)
			if test.failSecondCompletion {
				require.ErrorContains(t, err, "injected projection failure")
				assert.Len(t, store.transactions, 1)
			} else {
				require.ErrorContains(t, err, "injected terminal projection read failure")
				assert.Len(t, store.transactions, 2)
			}
			assertAtomicBatchRecoveryMember(t, testCtx, repository, fields[1], messages[fields[1]])
			lookup, err := repository.GetAtomicTransactionBatchByExecutionID(
				testCtx,
				execution.Execution.OrganizationID,
				execution.Execution.LedgerID,
				execution.Execution.ExecutionID,
			)
			require.NoError(t, err)
			require.NotNil(t, lookup)
			assert.Equal(t, txredis.AtomicTransactionBatchStateApplied, lookup.Record.State)
			assert.Equal(t, monetaryState, client.Get(testCtx, balanceKey).Val())
			assert.Equal(t, 1, engine.calls, "recovery must never invoke accounting")

			require.NoError(t, coordinator.complete(
				testCtx,
				txredis.RecoveryQueueSourceEngineRecover,
				fields[1],
				messages[fields[1]],
				secondEnvelope,
			))
			assertAtomicBatchRecoveryMember(t, testCtx, repository, fields[1], "")
			assert.Len(t, store.transactions, 2)
			for _, transactionID := range appliedRecord.TransactionIDs {
				assert.Equal(t, 1, store.durableWrites[transactionID], "each projection must become durable exactly once")
			}
			assert.Equal(t, 1, store.attempts[appliedRecord.TransactionIDs[0]])
			assert.Equal(t, 2, store.attempts[appliedRecord.TransactionIDs[1]],
				"only the retained member may be retried")
			assert.Equal(t, 1, engine.calls, "completion retry and finalization must not re-execute accounting")
			assert.Equal(t, monetaryState, client.Get(testCtx, balanceKey).Val())
			assertAtomicBatchRecoveryProtection(t, testCtx, client, artifactKeys, 2)

			expectedResponse := atomicBatchRecoveryExpectedResponse(t, appliedRecord.BatchID, appliedRecord.TransactionIDs, store)
			claim := txredis.AtomicTransactionBatchIdempotencyRecord{
				FormatVersion:      txredis.AtomicTransactionBatchIdempotencyFormatVersion,
				State:              txredis.AtomicTransactionBatchStateClaimed,
				RequestFingerprint: appliedRecord.RequestFingerprint,
				OwnerToken:         "replay-owner",
				BatchID:            uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":replay-batch")),
			}
			replay, err := repository.ClaimAtomicTransactionBatch(
				testCtx,
				execution.Execution.OrganizationID,
				execution.Execution.LedgerID,
				effectiveKey,
				claim,
			)
			require.NoError(t, err)
			require.NotNil(t, replay)
			assert.Equal(t, txredis.AtomicTransactionBatchReplayed, replay.Outcome)
			assert.Equal(t, expectedResponse, []byte(replay.Record.Response), "replay must preserve the original ordered response bytes")
			assert.Equal(t, 1, engine.calls)

			cleanup, err := repository.CleanupEngineRecovery(testCtx, completedAt.Add(38*time.Second), 1)
			require.NoError(t, err)
			assert.Equal(t, 1, cleanup.Scanned)
			assert.Equal(t, 1, cleanup.Cleaned)
			assertAtomicBatchRecoveryProtection(t, testCtx, client, artifactKeys, 0)
		})
	}
}

func TestIntegrationAtomicTransactionBatchCompatibleRecoveryUsesEngineOnly(t *testing.T) {
	ctx := context.Background()
	client := recoveryEngineValkey(t)
	tenantID := "atomic-batch-compatible-" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name())).String()
	testCtx := tmcore.ContextWithTenantID(ctx, tenantID)
	provider := recoveryEngineClientProvider{client: client}
	repository, err := txredis.NewConsumerRedis(provider)
	require.NoError(t, err)
	adapter, err := redisengine.NewAdapter(provider)
	require.NoError(t, err)
	engine := &countingAtomicBatchRecoveryEngine{delegate: adapter}
	execution := atomicBatchRecoveryExecution(t, tenantID)
	_, _ = seedAtomicBatchRecoveryIdempotency(t, testCtx, repository, execution)

	result, err := engine.Execute(testCtx, execution)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, engine.calls)

	legacyMessages, err := repository.ReadAllRecoveryMessages(testCtx, txredis.RecoveryQueueSourceLegacyBackup)
	require.NoError(t, err)
	assert.Empty(t, legacyMessages, "the batch must not enter the legacy backup queue")
	backupKey, err := tmvalkey.GetKeyContext(testCtx, txredis.TransactionBackupQueue)
	require.NoError(t, err)
	assert.Zero(t, client.HLen(testCtx, backupKey).Val())

	engineMessages, err := repository.ReadAllRecoveryMessages(testCtx, txredis.RecoveryQueueSourceEngineRecover)
	require.NoError(t, err)
	require.Len(t, engineMessages, len(execution.Execution.Transactions))
	for _, field := range atomicBatchRecoveryFields(execution) {
		raw := engineMessages[field]
		require.NotEmpty(t, raw)
		record, decodeErr := command.DecodeTransactionCompletionRecord([]byte(raw))
		require.NoError(t, decodeErr)
		assert.Equal(t, command.TransactionCompletionFormatVersion, record.FormatVersion)
	}
	artifactKeys := atomicBatchRecoveryArtifactKeys(t, testCtx, execution)
	assertAtomicBatchRecoveryProtection(t, testCtx, client, artifactKeys, int64(len(execution.Execution.Transactions)))
	for _, tran := range execution.Execution.Transactions {
		writeBehindKey, resolveErr := tmvalkey.GetKeyContext(
			testCtx,
			utils.TransactionInternalKey(execution.Execution.OrganizationID, execution.Execution.LedgerID, tran.ID.String()),
		)
		require.NoError(t, resolveErr)
		assert.Zero(t, client.Exists(testCtx, writeBehindKey).Val(),
			"the batch must not create a transaction write-behind entry")
	}

	store := newFailureInjectedBatchProjectionStore()
	commandUseCase := &command.UseCase{
		TransactionRedisRepo:                   repository,
		AtomicTransactionBatchIdempotencyRepo:  repository,
		AtomicTransactionBatchProjectionReader: store,
	}
	completedAt := time.Date(2026, time.September, 16, 19, 0, 0, 0, time.UTC)
	runner := NewRedisQueueConsumer(recoveryQuietLogger{}, commandUseCase, nil).
		WithAppliedTransactionCompleter(store).
		WithRecoveryClock(func() time.Time { return completedAt })

	// The runner deliberately enables the legacy consumer first and the engine
	// consumer second. A concurrent final-member race may retain one trigger, so
	// the next bounded cycle must converge without another durable projection.
	runner.readMessagesAndProcess(testCtx)
	runner.readMessagesAndProcess(testCtx)

	remainingEngine, err := repository.ReadAllRecoveryMessages(testCtx, txredis.RecoveryQueueSourceEngineRecover)
	require.NoError(t, err)
	assert.Empty(t, remainingEngine)
	legacyMessages, err = repository.ReadAllRecoveryMessages(testCtx, txredis.RecoveryQueueSourceLegacyBackup)
	require.NoError(t, err)
	assert.Empty(t, legacyMessages)
	assert.Equal(t, len(execution.Execution.Transactions), store.projectionCount())
	for _, tran := range execution.Execution.Transactions {
		assert.Equal(t, 1, store.durableWriteCount(tran.ID),
			"only the engine consumer may durably project each batch member")
	}
	assert.Equal(t, 1, engine.calls, "neither recovery consumer may invoke accounting")
}

type atomicBatchRecoveryKeys struct {
	receipt    string
	guards     string
	protection string
}

func atomicBatchRecoveryArtifactKeys(
	t *testing.T,
	ctx context.Context,
	execution command.EngineExecution,
) atomicBatchRecoveryKeys {
	t.Helper()
	scope := execution.Execution.OrganizationID.String() + ":" + execution.Execution.LedgerID.String()
	resolve := func(raw string) string {
		key, err := tmvalkey.GetKeyContext(ctx, raw)
		require.NoError(t, err)

		return key
	}

	return atomicBatchRecoveryKeys{
		receipt:    resolve("engine:" + cachepolicy.HashTag + ":receipts:" + scope),
		guards:     resolve("engine:" + cachepolicy.HashTag + ":guards:" + scope),
		protection: resolve("engine:" + cachepolicy.HashTag + ":protection:" + scope),
	}
}

func assertAtomicBatchRecoveryProtection(
	t *testing.T,
	ctx context.Context,
	client *redis.Client,
	keys atomicBatchRecoveryKeys,
	wantMembers int64,
) {
	t.Helper()
	if wantMembers == 0 {
		assert.Zero(t, client.HLen(ctx, keys.receipt).Val())
		assert.Zero(t, client.HLen(ctx, keys.guards).Val())
		assert.Zero(t, client.HLen(ctx, keys.protection).Val())

		return
	}
	assert.Equal(t, int64(1), client.HLen(ctx, keys.receipt).Val())
	assert.Equal(t, wantMembers, client.HLen(ctx, keys.guards).Val())
	assert.Equal(t, wantMembers, client.HLen(ctx, keys.protection).Val())
}

func assertAtomicBatchRecoveryMember(
	t *testing.T,
	ctx context.Context,
	repository *txredis.RedisConsumerRepository,
	field, want string,
) {
	t.Helper()
	actual, err := repository.ReadRecoveryMessage(ctx, txredis.RecoveryQueueSourceEngineRecover, field)
	require.NoError(t, err)
	assert.Equal(t, want, actual)
}

func atomicBatchRecoveryFields(execution command.EngineExecution) []string {
	fields := make([]string, len(execution.Execution.Transactions))
	for index, transaction := range execution.Execution.Transactions {
		fields[index] = transaction.ID.String() + ":" + execution.Execution.ExecutionID.String()
	}

	return fields
}

func seedAtomicBatchRecoveryIdempotency(
	t *testing.T,
	ctx context.Context,
	repository *txredis.RedisConsumerRepository,
	execution command.EngineExecution,
) (string, txredis.AtomicTransactionBatchIdempotencyRecord) {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":request"))
	fingerprint := hex.EncodeToString(digest[:])
	effectiveKey := t.Name() + ":idempotency"
	ownerToken := t.Name() + ":owner"
	batchID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":batch"))
	transactionIDs := make([]uuid.UUID, len(execution.Execution.Transactions))
	for index, tran := range execution.Execution.Transactions {
		transactionIDs[index] = tran.ID
	}
	claim := txredis.AtomicTransactionBatchIdempotencyRecord{
		FormatVersion:      txredis.AtomicTransactionBatchIdempotencyFormatVersion,
		State:              txredis.AtomicTransactionBatchStateClaimed,
		RequestFingerprint: fingerprint,
		OwnerToken:         ownerToken,
		BatchID:            batchID,
	}
	claimed, err := repository.ClaimAtomicTransactionBatch(
		ctx,
		execution.Execution.OrganizationID,
		execution.Execution.LedgerID,
		effectiveKey,
		claim,
	)
	require.NoError(t, err)
	require.Equal(t, txredis.AtomicTransactionBatchClaimed, claimed.Outcome)

	prepared := claim
	prepared.State = txredis.AtomicTransactionBatchStatePrepared
	prepared.TransactionIDs = transactionIDs
	transitioned, err := repository.TransitionAtomicTransactionBatch(
		ctx,
		execution.Execution.OrganizationID,
		execution.Execution.LedgerID,
		effectiveKey,
		ownerToken,
		txredis.AtomicTransactionBatchStateClaimed,
		prepared,
		0,
	)
	require.NoError(t, err)
	require.Equal(t, txredis.AtomicTransactionBatchTransitionUpdated, transitioned.Outcome)

	applied := prepared
	applied.State = txredis.AtomicTransactionBatchStateApplied
	executionID := execution.Execution.ExecutionID
	applied.ExecutionID = &executionID
	handedOff, err := repository.HandoffAtomicTransactionBatchExecution(
		ctx,
		execution.Execution.OrganizationID,
		execution.Execution.LedgerID,
		effectiveKey,
		ownerToken,
		applied,
	)
	require.NoError(t, err)
	require.Equal(t, txredis.AtomicTransactionBatchTransitionUpdated, handedOff.Outcome)

	return effectiveKey, applied
}

func atomicBatchRecoveryExpectedResponse(
	t *testing.T,
	batchID uuid.UUID,
	transactionIDs []uuid.UUID,
	store *failureInjectedBatchProjectionStore,
) []byte {
	t.Helper()
	transactions := make([]*transaction.Transaction, len(transactionIDs))
	for index, transactionID := range transactionIDs {
		stored := store.transactions[transactionID]
		require.NotNil(t, stored)
		public := *stored
		created := constant.CREATED
		public.Status = transaction.Status{Code: created, Description: &created}
		transactions[index] = &public
	}
	response := struct {
		BatchID      uuid.UUID                  `json:"batchId"`
		Transactions []*transaction.Transaction `json:"transactions"`
	}{BatchID: batchID, Transactions: transactions}
	encoded, err := json.Marshal(response)
	require.NoError(t, err)

	return encoded
}

func atomicBatchRecoveryExecution(t *testing.T, tenantID string) command.EngineExecution {
	t.Helper()
	id := func(suffix string) uuid.UUID {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":"+suffix))
	}
	date := time.Date(2024, time.January, 2, 18, 0, 0, 0, time.UTC)
	balance := accounting.BalanceSnapshot{
		BalanceRef:     "@source#default",
		ID:             id("balance"),
		AccountID:      id("account"),
		Alias:          "@source",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "USD",
		AccountType:    "deposit",
		Direction:      constant.DirectionCredit,
		BalanceScope:   "transactional",
		Available:      decimal.NewFromInt(100),
		Version:        7,
		AllowSending:   true,
		AllowReceiving: true,
	}
	amounts := []decimal.Decimal{decimal.NewFromInt(30), decimal.NewFromInt(20)}
	request := accounting.Execution{
		OrganizationID: id("organization"),
		LedgerID:       id("ledger"),
		ExecutionID:    id("execution"),
		Balances:       []accounting.BalanceSnapshot{balance},
		Transactions:   make([]accounting.Transaction, len(amounts)),
	}
	plans := make([]command.TransactionCompletionPlan, len(amounts))
	intents := make([]command.EngineTransactionIntent, len(amounts))
	for index, amount := range amounts {
		transactionID := id(fmt.Sprintf("transaction-%d", index))
		postingRef := fmt.Sprintf("source:%d", index)
		request.Transactions[index] = accounting.Transaction{
			ID: transactionID,
			Postings: []accounting.Posting{{
				Ref:        postingRef,
				BalanceRef: balance.BalanceRef,
				Type:       accounting.PostingDebit,
				Amount:     amount,
				DrawPolicy: accounting.DrawForbidden,
			}},
		}
		projection := command.OperationRecordSpec{
			TransactionID:     transactionID,
			PostingRef:        postingRef,
			BalanceRef:        balance.BalanceRef,
			Role:              accounting.RolePrimary,
			Side:              command.OperationSpecSideFrom,
			RowType:           constant.DEBIT,
			Direction:         constant.DirectionDebit,
			Description:       fmt.Sprintf("projection %d", index),
			RequestedAmount:   amount,
			CompatibilityPath: command.OperationRecordStandard,
			Balance: command.OperationBalanceContext(mmodel.Balance{
				ID:             balance.ID.String(),
				AccountID:      balance.AccountID.String(),
				OrganizationID: request.OrganizationID.String(),
				LedgerID:       request.LedgerID.String(),
				Alias:          balance.Alias,
				Key:            balance.Key,
				AssetCode:      balance.AssetCode,
				AccountType:    balance.AccountType,
				Direction:      balance.Direction,
				Available:      balance.Available,
				Version:        balance.Version,
				AllowSending:   true,
				AllowReceiving: true,
			}),
		}
		input := mtransaction.Transaction{
			Description: fmt.Sprintf("atomic recovery %d", index),
			Send: mtransaction.Send{
				Asset: "USD",
				Value: amount,
			},
		}
		plans[index] = command.TransactionCompletionPlan{
			FormatVersion:        command.TransactionCompletionFormatVersion,
			TenantID:             tenantID,
			HeaderID:             "atomic-batch-failure-integration",
			OrganizationID:       request.OrganizationID,
			LedgerID:             request.LedgerID,
			TransactionID:        transactionID,
			ExecutionID:          request.ExecutionID,
			TTL:                  date,
			TransactionDate:      date,
			TransactionCreatedAt: date,
			TransactionUpdatedAt: date,
			OperationUpdatedAt:   date,
			TransactionStatus:    constant.APPROVED,
			Action:               constant.ActionDirect,
			TracerSkipped:        true,
			OperationSpecs:       []command.OperationRecordSpec{projection},
			TransactionInput:     input,
			Validate: &mtransaction.Responses{
				Sources:      []string{"@source"},
				Destinations: []string{"@destination"},
			},
		}
		intents[index] = command.EngineTransactionIntent{
			TransactionID:        transactionID,
			TracerSkipped:        true,
			Action:               constant.ActionDirect,
			TransactionStatus:    constant.APPROVED,
			TransactionDate:      date,
			TransactionCreatedAt: date,
			TransactionUpdatedAt: date,
			OperationUpdatedAt:   date,
			Input:                input,
			PostingRefs:          []string{postingRef},
			OperationSpecs:       []command.OperationRecordIntent{projection.Intent()},
		}
	}
	fingerprint, err := command.ComputeEngineIntentFingerprint(command.EngineIntent{
		TenantID:       tenantID,
		OrganizationID: request.OrganizationID,
		LedgerID:       request.LedgerID,
		ExecutionID:    request.ExecutionID,
		Transactions:   intents,
	})
	require.NoError(t, err)
	execution := command.EngineExecution{
		Execution:         request,
		IntentFingerprint: fingerprint,
		RetentionSeconds:  37,
		Guards:            make([]command.ExecutionGuard, len(plans)),
		CompletionPlans:   make([]command.CompletionPlanRecord, len(plans)),
	}
	for index := range plans {
		plans[index].IntentFingerprint = fingerprint
		payload, encodeErr := command.EncodeTransactionCompletionPlan(plans[index])
		require.NoError(t, encodeErr)
		execution.Guards[index] = command.ExecutionGuard{
			TransactionID: plans[index].TransactionID,
			NextToken:     fmt.Sprintf("atomic-batch-guard-%d", index),
		}
		execution.CompletionPlans[index] = command.CompletionPlanRecord{
			TransactionID: plans[index].TransactionID,
			Payload:       payload,
		}
	}
	require.NoError(t, command.ValidateTransactionCompletion(execution))

	return execution
}
