// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
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
