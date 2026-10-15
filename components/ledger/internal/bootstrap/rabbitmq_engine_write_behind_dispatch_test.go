// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"testing/synctest"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	postgresOperation "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

type rabbitCompletionStub struct {
	records     []*command.TransactionCompletionRecord
	bulkRecords []*command.TransactionCompletionRecord
	bulkErr     error
}

type rabbitIndividualCompletionStub struct {
	records []*command.TransactionCompletionRecord
}

func (stub *rabbitIndividualCompletionStub) Complete(_ context.Context, record *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	stub.records = append(stub.records, record)

	return rabbitCompletionResult(record), nil
}

func requireRabbitTransactionDispatcher(t testing.TB, useCase *command.UseCase, policy rabbitEngineTenantPolicy, bulkRequired bool) *rabbitTransactionDispatcher {
	t.Helper()

	dispatcher, err := newRabbitTransactionDispatcher(useCase, policy, bulkRequired)
	require.NoError(t, err)

	return dispatcher
}

func (stub *rabbitCompletionStub) Complete(_ context.Context, record *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	stub.records = append(stub.records, record)
	return rabbitCompletionResult(record), nil
}

func (stub *rabbitCompletionStub) CompleteBulk(_ context.Context, records []*command.TransactionCompletionRecord) ([]command.TransactionCompletionResult, error) {
	stub.bulkRecords = append(stub.bulkRecords, records...)
	if stub.bulkErr != nil {
		return nil, stub.bulkErr
	}
	results := make([]command.TransactionCompletionResult, len(records))
	for index, record := range records {
		results[index] = rabbitCompletionResult(record)
	}
	return results, nil
}

func rabbitCompletionResult(record *command.TransactionCompletionRecord) command.TransactionCompletionResult {
	return command.TransactionCompletionResult{Outcome: command.TransactionPersistenceOutcome{
		TransactionStatus: constant.APPROVED,
		LifecyclePhase:    command.TransactionLifecyclePhaseCreated,
	}}
}

type rabbitRecoveryAcknowledgerStub struct {
	records []*command.TransactionCompletionRecord
}

func (stub *rabbitRecoveryAcknowledgerStub) AcknowledgeEngineRecovery(_ context.Context, record *command.TransactionCompletionRecord, _ command.TransactionCompletionResult) error {
	stub.records = append(stub.records, record)
	return nil
}

func rabbitEngineMessages(t testing.TB) ([]byte, []byte, *command.TransactionCompletionRecord) {
	t.Helper()
	_, rawCompletion, record := consumerRecoveryFixture(t)
	writeBehind, err := command.EncodeTransactionWriteBehindEnvelope(command.TransactionWriteBehindEnvelope{
		FormatVersion: command.TransactionWriteBehindFormatVersion, ApplicationState: command.TransactionApplicationConfirmed,
		ReplayState: command.TransactionReplayReconstructible, DurabilityState: command.TransactionDurabilityPending, Record: *record,
	})
	require.NoError(t, err)
	return writeBehind, []byte(rawCompletion), record
}

func rabbitEngineMessageForTenant(t testing.TB, tenantID string) ([]byte, *command.TransactionCompletionRecord) {
	t.Helper()
	_, _, source := consumerRecoveryFixture(t)
	record := *source
	plan, err := command.DecodeTransactionCompletionPlan([]byte(record.Payload))
	require.NoError(t, err)
	plan.TenantID = tenantID
	rawPlan, err := command.EncodeTransactionCompletionPlan(*plan)
	require.NoError(t, err)
	record.TenantID = tenantID
	record.Payload = string(rawPlan)
	writeBehind, err := command.EncodeTransactionWriteBehindEnvelope(command.TransactionWriteBehindEnvelope{
		FormatVersion: command.TransactionWriteBehindFormatVersion, ApplicationState: command.TransactionApplicationConfirmed,
		ReplayState: command.TransactionReplayReconstructible, DurabilityState: command.TransactionDurabilityPending, Record: record,
	})
	require.NoError(t, err)

	return writeBehind, &record
}

func rabbitEngineWrapper(t testing.TB, body []byte, record *command.TransactionCompletionRecord) []byte {
	t.Helper()
	wrapper, err := msgpack.Marshal(mmodel.Queue{
		OrganizationID: record.OrganizationID,
		LedgerID:       record.LedgerID,
		QueueData:      []mmodel.QueueData{{ID: record.TransactionID, Value: body}},
	})
	require.NoError(t, err)

	return wrapper
}

func TestEngineWriteBehindRabbitDispatchProcessesDirectAndEveryWrapperEntry(t *testing.T) {
	writeBehind, completionV2, record := rabbitEngineMessages(t)
	completer := &rabbitCompletionStub{}
	acknowledger := &rabbitRecoveryAcknowledgerStub{}
	dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{
		AppliedTransactionCompleter: completer,
		EngineRecoveryAcknowledger:  acknowledger,
	}, rabbitEngineSingleTenant, false)

	require.NoError(t, dispatcher.handle(context.Background(), writeBehind))
	wrapper, err := msgpack.Marshal(mmodel.Queue{
		OrganizationID: record.OrganizationID,
		LedgerID:       record.LedgerID,
		QueueData: []mmodel.QueueData{
			{ID: record.TransactionID, Value: writeBehind},
			{ID: record.ExecutionID, Value: completionV2},
		},
	})
	require.NoError(t, err)
	require.NoError(t, dispatcher.handle(context.Background(), wrapper))

	assert.Len(t, completer.records, 3, "direct, version-one wrapper, and version-two wrapper entries must all be projected")
	assert.Len(t, acknowledger.records, 3)
	assert.Nil(t, dispatcher.useCase.Engine, "dispatching applied evidence must not require or invoke accounting")
}

func TestEngineWriteBehindRabbitBulkCorrelatesMessagesAndDeduplicatesEvidence(t *testing.T) {
	writeBehind, completionV2, record := rabbitEngineMessages(t)
	completer := &rabbitCompletionStub{}
	acknowledger := &rabbitRecoveryAcknowledgerStub{}
	dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{
		AppliedTransactionCompleter: completer,
		EngineRecoveryAcknowledger:  acknowledger,
	}, rabbitEngineSingleTenant, true)
	wrapper, err := msgpack.Marshal(mmodel.Queue{
		OrganizationID: record.OrganizationID,
		LedgerID:       record.LedgerID,
		QueueData: []mmodel.QueueData{
			{ID: record.TransactionID, Value: writeBehind},
			{ID: record.ExecutionID, Value: completionV2},
		},
	})
	require.NoError(t, err)

	results, err := dispatcher.handleBulk(context.Background(), []amqp.Delivery{{Body: wrapper}, {Body: writeBehind}})

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, 0, results[0].Index)
	assert.Equal(t, 1, results[1].Index)
	assert.True(t, results[0].Success)
	assert.True(t, results[1].Success)
	assert.Len(t, completer.bulkRecords, 1, "the same immutable execution is persisted once per SQL group")
	assert.Len(t, acknowledger.records, 3, "each broker entry keeps its own completion/ACK correlation")
}

func TestEngineWriteBehindRabbitBulkIsolatesMalformedMessage(t *testing.T) {
	writeBehind, _, _ := rabbitEngineMessages(t)
	completer := &rabbitCompletionStub{}
	dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{AppliedTransactionCompleter: completer}, rabbitEngineSingleTenant, true)

	results, err := dispatcher.handleBulk(context.Background(), []amqp.Delivery{{Body: writeBehind}, {Body: []byte{0xff}}})

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
	assert.Error(t, results[1].Error)
	assert.Len(t, completer.bulkRecords, 1)
}

func TestEngineWriteBehindRabbitBulkReportsGroupFailurePerMessage(t *testing.T) {
	writeBehind, _, _ := rabbitEngineMessages(t)
	failure := errors.New("SQL unavailable")
	completer := &rabbitCompletionStub{bulkErr: failure}
	dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{AppliedTransactionCompleter: completer}, rabbitEngineSingleTenant, true)

	results, err := dispatcher.handleBulk(context.Background(), []amqp.Delivery{{Body: writeBehind}, {Body: writeBehind}})

	require.NoError(t, err)
	require.Len(t, results, 2)
	for index, result := range results {
		assert.Equal(t, index, result.Index)
		assert.False(t, result.Success)
		assert.ErrorIs(t, result.Error, failure)
	}
}

func TestEngineWriteBehindRabbitDispatchRejectsUnsupportedVersion(t *testing.T) {
	dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{AppliedTransactionCompleter: &rabbitCompletionStub{}}, rabbitEngineSingleTenant, false)
	err := dispatcher.handle(context.Background(), []byte(`{"formatVersion":99}`))
	require.EqualError(t, err, "unsupported RabbitMQ transaction version 99")
}

func TestEngineWriteBehindRabbitDispatchEnforcesTenantPolicyBeforeCompletion(t *testing.T) {
	testCases := []struct {
		name          string
		policy        rabbitEngineTenantPolicy
		contextTenant string
		messageTenant string
		wantErr       error
	}{
		{name: "single tenant accepts empty scope", policy: rabbitEngineSingleTenant},
		{name: "single tenant rejects context tenant", policy: rabbitEngineSingleTenant, contextTenant: "tenant-a", wantErr: rabbitmq.ErrEngineWriteBehindTenantUnexpected},
		{name: "single tenant rejects envelope tenant", policy: rabbitEngineSingleTenant, messageTenant: "tenant-a", wantErr: rabbitmq.ErrEngineWriteBehindTenantUnexpected},
		{name: "multi tenant accepts matching trusted scope", policy: rabbitEngineMultiTenant, contextTenant: "tenant-a", messageTenant: "tenant-a"},
		{name: "multi tenant rejects missing context tenant", policy: rabbitEngineMultiTenant, messageTenant: "tenant-a", wantErr: rabbitmq.ErrEngineWriteBehindTenantRequired},
		{name: "multi tenant rejects missing envelope tenant", policy: rabbitEngineMultiTenant, contextTenant: "tenant-a", wantErr: rabbitmq.ErrEngineWriteBehindTenantRequired},
		{name: "multi tenant rejects mismatched tenant", policy: rabbitEngineMultiTenant, contextTenant: "tenant-a", messageTenant: "tenant-b", wantErr: rabbitmq.ErrEngineWriteBehindTenantMismatch},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body, record := rabbitEngineMessageForTenant(t, testCase.messageTenant)
			ctx := context.Background()
			if testCase.contextTenant != "" {
				ctx = tmcore.ContextWithTenantID(ctx, testCase.contextTenant)
			}

			for _, delivery := range []struct {
				name string
				body []byte
			}{
				{name: "direct", body: body},
				{name: "wrapper", body: rabbitEngineWrapper(t, body, record)},
			} {
				t.Run(delivery.name, func(t *testing.T) {
					completer := &rabbitCompletionStub{}
					acknowledger := &rabbitRecoveryAcknowledgerStub{}
					dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{
						AppliedTransactionCompleter: completer,
						EngineRecoveryAcknowledger:  acknowledger,
					}, testCase.policy, false)

					err := dispatcher.handle(ctx, delivery.body)
					if testCase.wantErr != nil {
						require.ErrorIs(t, err, testCase.wantErr)
						assert.Empty(t, completer.records)
						assert.Empty(t, acknowledger.records, "rejected evidence must not acknowledge recovery")

						return
					}

					require.NoError(t, err)
					assert.Len(t, completer.records, 1)
					assert.Len(t, acknowledger.records, 1)
				})
			}
		})
	}
}

func TestEngineWriteBehindRabbitBulkIsolatesInvalidTenantWithoutRecoveryAck(t *testing.T) {
	valid, _ := rabbitEngineMessageForTenant(t, "tenant-a")
	invalid, _ := rabbitEngineMessageForTenant(t, "tenant-b")
	completer := &rabbitCompletionStub{}
	acknowledger := &rabbitRecoveryAcknowledgerStub{}
	dispatcher := requireRabbitTransactionDispatcher(t, &command.UseCase{
		AppliedTransactionCompleter: completer,
		EngineRecoveryAcknowledger:  acknowledger,
	}, rabbitEngineMultiTenant, true)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	results, err := dispatcher.handleBulk(ctx, []amqp.Delivery{{Body: valid}, {Body: invalid}})

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
	assert.ErrorIs(t, results[1].Error, rabbitmq.ErrEngineWriteBehindTenantMismatch)
	assert.Len(t, completer.bulkRecords, 1, "valid evidence must continue independently")
	assert.Len(t, acknowledger.records, 1, "rejected evidence must not acknowledge recovery")
}

func TestRabbitTransactionDispatcherReadiness(t *testing.T) {
	policies := []struct {
		name   string
		policy rabbitEngineTenantPolicy
	}{
		{name: "single tenant", policy: rabbitEngineSingleTenant},
		{name: "multi tenant", policy: rabbitEngineMultiTenant},
	}

	for _, policy := range policies {
		t.Run(policy.name, func(t *testing.T) {
			tenantID := ""
			ctx := context.Background()
			if policy.policy == rabbitEngineMultiTenant {
				tenantID = "tenant-a"
				ctx = tmcore.ContextWithTenantID(ctx, tenantID)
			}
			body, _ := rabbitEngineMessageForTenant(t, tenantID)

			t.Run("missing use case fails before registration", func(t *testing.T) {
				dispatcher, err := newRabbitTransactionDispatcher(nil, policy.policy, false)

				require.ErrorIs(t, err, errRabbitTransactionUseCaseNotConfigured)
				assert.Nil(t, dispatcher)
			})

			t.Run("missing completer fails before registration", func(t *testing.T) {
				dispatcher, err := newRabbitTransactionDispatcher(&command.UseCase{}, policy.policy, false)

				require.ErrorIs(t, err, errRabbitTransactionCompleterNotConfigured)
				assert.Nil(t, dispatcher)
			})

			t.Run("individual completer is sufficient for individual delivery", func(t *testing.T) {
				completer := &rabbitIndividualCompletionStub{}
				dispatcher, err := newRabbitTransactionDispatcher(&command.UseCase{
					AppliedTransactionCompleter: completer,
				}, policy.policy, false)

				require.NoError(t, err)
				require.NotNil(t, dispatcher)
				assert.NotNil(t, dispatcher.completer)
				assert.Nil(t, dispatcher.bulkCompleter)
				require.NoError(t, dispatcher.handle(ctx, body))
				assert.Len(t, completer.records, 1)
			})

			t.Run("bulk delivery requires bulk capability", func(t *testing.T) {
				dispatcher, err := newRabbitTransactionDispatcher(&command.UseCase{
					AppliedTransactionCompleter: &rabbitIndividualCompletionStub{},
				}, policy.policy, true)

				require.ErrorIs(t, err, errRabbitTransactionBulkCompleterNotConfigured)
				assert.Nil(t, dispatcher)
			})

			t.Run("bulk completer is ready for the first delivery", func(t *testing.T) {
				completer := &rabbitCompletionStub{}
				dispatcher, err := newRabbitTransactionDispatcher(&command.UseCase{
					AppliedTransactionCompleter: completer,
				}, policy.policy, true)

				require.NoError(t, err)
				require.NotNil(t, dispatcher)
				assert.NotNil(t, dispatcher.completer)
				assert.NotNil(t, dispatcher.bulkCompleter)
				results, err := dispatcher.handleBulk(ctx, []amqp.Delivery{{Body: body}})
				require.NoError(t, err)
				require.Len(t, results, 1)
				assert.True(t, results[0].Success)
				assert.Len(t, completer.bulkRecords, 1)
			})
		})
	}
}

// newRabbitLegacyBulkUseCase wires a command.UseCase whose bulk path inserts every
// transaction in txIDs and fails the transaction metadata write of every ID in
// metadataFailedTxIDs. It must be built inside a synctest bubble so the test waits for the
// cleanup and event goroutines the use case starts.
func newRabbitLegacyBulkUseCase(t *testing.T, txIDs []string, metadataFailedTxIDs map[string]struct{}) *command.UseCase {
	t.Helper()

	ctrl := gomock.NewController(t)

	transactionRepo := postgresTransaction.NewMockRepository(ctrl)
	operationRepo := postgresOperation.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	dbTx := &rabbitLegacyDBTransaction{}
	transactionRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil).Times(1)
	transactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
		Return(&repository.BulkInsertResult{Attempted: int64(len(txIDs)), Inserted: int64(len(txIDs)), InsertedIDs: txIDs}, nil).
		Times(1)
	operationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
		Return(&repository.BulkInsertResult{Attempted: int64(len(txIDs)), Inserted: int64(len(txIDs))}, nil).
		Times(1)

	// A document-level bulk error routes the entries through the individual fallback.
	metadataRepo.EXPECT().
		CreateBulk(gomock.Any(), constant.EntityTransaction, gomock.Any()).
		Return(nil, errors.New("bulk insert failed")).
		AnyTimes()
	metadataRepo.EXPECT().
		Create(gomock.Any(), constant.EntityTransaction, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
			if _, fail := metadataFailedTxIDs[meta.EntityID]; fail {
				return errors.New("metadata create failed")
			}

			return nil
		}).
		AnyTimes()

	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return &command.UseCase{
		TransactionRepo:             transactionRepo,
		OperationRepo:               operationRepo,
		TransactionMetadataRepo:     metadataRepo,
		TransactionRedisRepo:        redisRepo,
		AppliedTransactionCompleter: &rabbitCompletionStub{},
	}
}

// rabbitLegacyDBTransaction is a no-op repository.DBTransaction for the bulk insert.
type rabbitLegacyDBTransaction struct{}

func (rabbitLegacyDBTransaction) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (rabbitLegacyDBTransaction) Commit() error   { return nil }
func (rabbitLegacyDBTransaction) Rollback() error { return nil }

// rabbitLegacyPayload builds an APPROVED legacy payload carrying transaction metadata.
func rabbitLegacyPayload(orgID, ledgerID uuid.UUID, txID string) postgresTransaction.TransactionProcessingPayload {
	return postgresTransaction.TransactionProcessingPayload{
		Transaction: &postgresTransaction.Transaction{
			ID:             txID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Status:         postgresTransaction.Status{Code: constant.APPROVED},
			Metadata:       map[string]any{"reference": "legacy"},
			Operations: []*postgresOperation.Operation{
				{ID: uuid.New().String(), TransactionID: txID},
			},
		},
		Version: "v2",
	}
}

// rabbitLegacyMessage encodes payloads as one legacy queue wrapper message.
func rabbitLegacyMessage(t testing.TB, orgID, ledgerID uuid.UUID, payloads ...postgresTransaction.TransactionProcessingPayload) []byte {
	t.Helper()

	queueData := make([]mmodel.QueueData, 0, len(payloads))

	for _, payload := range payloads {
		value, err := msgpack.Marshal(payload)
		require.NoError(t, err)

		queueData = append(queueData, mmodel.QueueData{ID: uuid.MustParse(payload.Transaction.ID), Value: value})
	}

	body, err := msgpack.Marshal(mmodel.Queue{OrganizationID: orgID, LedgerID: ledgerID, QueueData: queueData})
	require.NoError(t, err)

	return body
}

func TestEngineWriteBehindRabbitBulkReportsLegacyMetadataFailurePerMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		orgID, ledgerID := uuid.New(), uuid.New()
		confirmedTxID, failedTxID := uuid.New().String(), uuid.New().String()

		useCase := newRabbitLegacyBulkUseCase(t, []string{confirmedTxID, failedTxID}, map[string]struct{}{failedTxID: {}})
		dispatcher := requireRabbitTransactionDispatcher(t, useCase, rabbitEngineSingleTenant, true)

		results, err := dispatcher.handleBulk(context.Background(), []amqp.Delivery{
			{Body: rabbitLegacyMessage(t, orgID, ledgerID, rabbitLegacyPayload(orgID, ledgerID, confirmedTxID))},
			{Body: rabbitLegacyMessage(t, orgID, ledgerID, rabbitLegacyPayload(orgID, ledgerID, failedTxID))},
		})

		synctest.Wait()

		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, 0, results[0].Index)
		assert.True(t, results[0].Success)
		assert.NoError(t, results[0].Error)
		assert.Equal(t, 1, results[1].Index)
		assert.False(t, results[1].Success)
		require.Error(t, results[1].Error)
		assert.Contains(t, results[1].Error.Error(), failedTxID)
	})
}

func TestEngineWriteBehindRabbitBulkFailsLegacyMessageWhenOneEntryFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		orgID, ledgerID := uuid.New(), uuid.New()
		confirmedTxID, failedTxID, otherTxID := uuid.New().String(), uuid.New().String(), uuid.New().String()

		useCase := newRabbitLegacyBulkUseCase(t, []string{confirmedTxID, failedTxID, otherTxID}, map[string]struct{}{failedTxID: {}})
		dispatcher := requireRabbitTransactionDispatcher(t, useCase, rabbitEngineSingleTenant, true)

		results, err := dispatcher.handleBulk(context.Background(), []amqp.Delivery{
			{Body: rabbitLegacyMessage(t, orgID, ledgerID,
				rabbitLegacyPayload(orgID, ledgerID, confirmedTxID),
				rabbitLegacyPayload(orgID, ledgerID, failedTxID))},
			{Body: rabbitLegacyMessage(t, orgID, ledgerID, rabbitLegacyPayload(orgID, ledgerID, otherTxID))},
		})

		synctest.Wait()

		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.False(t, results[0].Success, "a message is failed when any of its entries has unconfirmed metadata")
		require.Error(t, results[0].Error)
		assert.Contains(t, results[0].Error.Error(), failedTxID)
		assert.True(t, results[1].Success)
	})
}

func TestEngineWriteBehindRabbitBulkAcknowledgesLegacyMessagesWithConfirmedMetadata(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		orgID, ledgerID := uuid.New(), uuid.New()
		firstTxID, secondTxID := uuid.New().String(), uuid.New().String()

		useCase := newRabbitLegacyBulkUseCase(t, []string{firstTxID, secondTxID}, nil)
		dispatcher := requireRabbitTransactionDispatcher(t, useCase, rabbitEngineSingleTenant, true)

		results, err := dispatcher.handleBulk(context.Background(), []amqp.Delivery{
			{Body: rabbitLegacyMessage(t, orgID, ledgerID, rabbitLegacyPayload(orgID, ledgerID, firstTxID))},
			{Body: rabbitLegacyMessage(t, orgID, ledgerID, rabbitLegacyPayload(orgID, ledgerID, secondTxID))},
		})

		synctest.Wait()

		require.NoError(t, err)
		require.Len(t, results, 2)

		for index, result := range results {
			assert.Equal(t, index, result.Index)
			assert.True(t, result.Success)
			assert.NoError(t, result.Error)
		}
	})
}

func TestMarkRabbitLegacyMetadataFailedNamesEveryFailedTransactionOfAMessage(t *testing.T) {
	t.Parallel()

	firstTxID := "11111111-1111-1111-1111-111111111111"
	secondTxID := "22222222-2222-2222-2222-222222222222"
	results := []rabbitmq.BulkMessageResult{{Index: 0, Success: true}}

	markRabbitLegacyMetadataFailed(results,
		map[string][]int{secondTxID: {0}, firstTxID: {0}},
		map[string]struct{}{secondTxID: {}, firstTxID: {}})

	assert.False(t, results[0].Success)
	assert.EqualError(t, results[0].Error, "legacy transactions "+firstTxID+", "+secondTxID+" metadata not confirmed",
		"the error names every failed transaction of the message in sorted order")
	assert.True(t, pkgRabbitmq.NewDefaultClassifier().IsRetryable(results[0].Error))
}

func TestMarkRabbitLegacyMetadataFailedReportsRetryableTechnicalError(t *testing.T) {
	t.Parallel()

	failedTxID := uuid.New().String()
	results := []rabbitmq.BulkMessageResult{{Index: 0, Success: true}, {Index: 1, Success: true}}

	markRabbitLegacyMetadataFailed(results, map[string][]int{failedTxID: {1}}, map[string]struct{}{failedTxID: {}})

	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
	require.Error(t, results[1].Error)
	assert.EqualError(t, results[1].Error, "legacy transaction "+failedTxID+" metadata not confirmed")
	assert.False(t, pkg.IsBusinessError(results[1].Error), "metadata failure must not be a business error")
	assert.True(t, pkgRabbitmq.NewDefaultClassifier().IsRetryable(results[1].Error), "metadata failure must be retried")
}
