// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionquarantine"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func consumerRecoveryFixture(t *testing.T) (string, string, *command.TransactionCompletionRecord) {
	t.Helper()
	organization := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledger := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transaction := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	execution := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	balanceID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	account := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	date := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	balance := command.OperationBalanceContext{}
	balance.ID, balance.AccountID = balanceID.String(), account.String()
	balance.OrganizationID, balance.LedgerID = organization.String(), ledger.String()
	balance.Alias, balance.Key, balance.AssetCode, balance.AccountType = "@source", "default", "BRL", "deposit"
	balance.Available, balance.Version = decimal.NewFromInt(100), 7
	projection := command.OperationRecordSpec{
		TransactionID: transaction, PostingRef: "posting", BalanceRef: "@source#default", Role: accounting.RolePrimary,
		Side: command.OperationSpecSideFrom, RowType: "DEBIT", Direction: "debit", Balance: balance,
		RequestedAmount: decimal.NewFromInt(30), CompatibilityPath: command.OperationRecordStandard,
	}
	payload := command.TransactionCompletionPlan{
		FormatVersion: 2, TransactionID: transaction, OrganizationID: organization, LedgerID: ledger, ExecutionID: execution,
		TTL: date, TransactionDate: date, Action: "direct", TransactionStatus: "APPROVED", OperationSpecs: []command.OperationRecordSpec{projection},
		TransactionCreatedAt: date, TransactionUpdatedAt: date, OperationUpdatedAt: date,
	}
	payload.TransactionInput.Send.Asset, payload.TransactionInput.Send.Value = "BRL", decimal.NewFromInt(30)
	fingerprint, err := command.ComputeEngineIntentFingerprint(command.EngineIntent{
		OrganizationID: organization, LedgerID: ledger, ExecutionID: execution,
		Transactions: []command.EngineTransactionIntent{{
			TransactionID: transaction, Action: payload.Action,
			TransactionStatus: payload.TransactionStatus, TransactionDate: date, Input: payload.TransactionInput,
			TransactionCreatedAt: payload.TransactionCreatedAt, TransactionUpdatedAt: payload.TransactionUpdatedAt, OperationUpdatedAt: payload.OperationUpdatedAt,
			PostingRefs: []string{"posting"}, OperationSpecs: []command.OperationRecordIntent{projection.Intent()},
		}},
	})
	require.NoError(t, err)
	payload.IntentFingerprint = fingerprint
	rawPayload, err := command.EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	before := accounting.BalanceState{Available: decimal.NewFromInt(100), Version: 7}
	after := accounting.BalanceState{Available: decimal.NewFromInt(70), Version: 8}
	envelope := &command.TransactionCompletionRecord{
		FormatVersion: 2, OrganizationID: organization, LedgerID: ledger, TransactionID: transaction, ExecutionID: execution,
		IntentFingerprint: fingerprint, Payload: string(rawPayload),
		Result: accounting.ExecutionResult{
			Movements: []accounting.Movement{{
				Ref: transaction.String() + ":7:posting:primary:0", TransactionID: transaction, PostingRef: "posting", Role: accounting.RolePrimary,
				BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30), Before: before, After: after,
			}},
			Final: []accounting.BalanceSnapshot{{
				ID: balanceID, AccountID: account, BalanceRef: "@source#default", Alias: "@source", Key: "default", AssetCode: "BRL",
				AccountType: "deposit", Direction: "credit", BalanceScope: "transactional", Available: after.Available, Version: 8,
			}},
		},
	}
	raw, err := command.EncodeTransactionCompletionRecord(*envelope)
	require.NoError(t, err)
	return transaction.String() + ":" + execution.String(), string(raw), envelope
}

func TestDecodeRecoveryRecord_ExactScopeAndFrozenEnvelope(t *testing.T) {
	field, raw, expected := consumerRecoveryFixture(t)
	envelope, ttl, err := decodeRecoveryRecord(context.Background(), field, raw)
	require.NoError(t, err)
	require.Equal(t, expected.TransactionID, envelope.TransactionID)
	require.Equal(t, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), ttl)
	for _, key := range []string{field + ":extra", "tenant:" + field, strings.ToUpper(field), "wrong"} {
		if key == field {
			continue
		}
		_, _, err := decodeRecoveryRecord(context.Background(), key, raw)
		require.Error(t, err)
	}
	_, _, err = decodeRecoveryRecord(tmcore.ContextWithTenantID(context.Background(), "unrelated"), field, raw)
	require.ErrorContains(t, err, "authenticated scope")
	for _, replacement := range []any{nil, "{}", `{"formatVersion":999}`} {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &fields))
		fields["payload"], err = json.Marshal(replacement)
		require.NoError(t, err)
		corrupted, err := json.Marshal(fields)
		require.NoError(t, err)
		_, _, err = decodeRecoveryRecord(context.Background(), field, string(corrupted))
		require.Error(t, err)
	}
}

func TestRecoveryRecordEligible_FixedBoundary(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	require.True(t, recoveryRecordEligible(now.Add(-31*time.Minute), now))
	require.True(t, recoveryRecordEligible(now.Add(-30*time.Minute), now))
	require.False(t, recoveryRecordEligible(now.Add(-30*time.Minute+time.Second), now))
	require.False(t, recoveryRecordEligible(now.Add(time.Hour), now))
}

func TestRecoveryRecordVersion_AbsentOnlyLegacy(t *testing.T) {
	for _, raw := range []string{`{}`, `{"ttl":"2026-01-01T00:00:00Z"}`, `{"extension":{"formatVersion":2}}`, `{"legacy":1,"legacy":2}`} {
		version, err := recoveryRecordVersion(raw)
		require.NoError(t, err)
		require.Zero(t, version)
	}
	for _, raw := range []string{`{"formatVersion":2}`, `{"formatVersion": 2, "payload":"opaque"}`, `{"format\u0056ersion":2}`} {
		version, err := recoveryRecordVersion(raw)
		require.NoError(t, err)
		require.Equal(t, 2, version)
	}
	for _, raw := range []string{
		`null`, `[]`, `{`, `{} {}`, `{"formatVersion":null}`, `{"formatVersion":0}`, `{"formatVersion":1}`, `{"formatVersion":3}`,
		`{"formatVersion":"2"}`, `{"formatVersion":2.0}`, `{"formatVersion":2e0}`, `{"formatVersion":true}`,
		`{"formatVersion":2,"formatVersion":2}`, `{"formatVersion":2,"format\u0056ersion":2}`, `{"FormatVersion":2}`, `{"formatVersion":2,"FORMATVERSION":2}`,
	} {
		_, err := recoveryRecordVersion(raw)
		require.Error(t, err, raw)
	}
}

func TestTrustedLegacyBackupScope(t *testing.T) {
	const field = "transaction:{transactions}:11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:33333333-3333-4333-8333-333333333333"
	ctx := context.Background()
	_, _, _, trusted := trustedLegacyBackupScope(ctx, field)
	require.True(t, trusted)
	tenantCtx := tmcore.ContextWithTenantID(ctx, "tenant-a")
	tenantField, err := tmvalkey.GetKeyContext(tenantCtx, field)
	require.NoError(t, err)
	_, _, _, trusted = trustedLegacyBackupScope(tenantCtx, tenantField)
	require.True(t, trusted)
	for _, invalid := range []string{field + ":extra", "arbitrary:" + field, "11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222", tenantField} {
		_, _, _, trusted := trustedLegacyBackupScope(ctx, invalid)
		require.False(t, trusted, invalid)
	}
	_, _, _, trusted = trustedLegacyBackupScope(tmcore.ContextWithTenantID(ctx, "tenant-b"), tenantField)
	require.False(t, trusted)
}

type recoveryQuarantineStub struct {
	transactionquarantine.Repository
}

type recoveryPoisonQueueStub struct {
	recoveryQueueStub
	attempts int
}

func (queue *recoveryPoisonQueueStub) IncrementBackupAttempt(context.Context, string) (int64, error) {
	queue.attempts++
	return 1, nil
}

func TestReadMessagesAndProcess_InvalidLegacyRequiresTrustedScope(t *testing.T) {
	const canonical = "transaction:{transactions}:11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:33333333-3333-4333-8333-333333333333"
	ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-a")
	trusted, err := tmvalkey.GetKeyContext(ctx, canonical)
	require.NoError(t, err)
	foreign, err := tmvalkey.GetKeyContext(tmcore.ContextWithTenantID(t.Context(), "tenant-b"), canonical)
	require.NoError(t, err)
	for _, raw := range []string{`{`, `{"ttl":42}`} {
		for _, field := range []string{trusted, canonical, foreign, "arbitrary:" + canonical, trusted + ":extra"} {
			t.Run(raw+"/"+field, func(t *testing.T) {
				queue := &recoveryPoisonQueueStub{recoveryQueueStub: recoveryQueueStub{messages: map[string]string{field: raw}}}
				consumer := &RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}, quarantineRepo: &recoveryQuarantineStub{}}
				consumer.readMessagesAndProcess(ctx)
				if field == trusted {
					require.Equal(t, 1, queue.attempts)
				} else {
					require.Zero(t, queue.attempts, "untrusted fields must not enter quarantine")
				}
			})
		}
	}
}

type recoveryCompleterStub struct {
	err   error
	calls int
	order *[]string
}

func (f *recoveryCompleterStub) Complete(context.Context, *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	f.calls++
	*f.order = append(*f.order, "durable-finalization")
	return command.TransactionCompletionResult{Outcome: command.TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}, f.err
}

type recoveryQueueStub struct {
	txRedis.RedisRepository
	status         int64
	err            error
	calls          int
	field, payload string
	order          *[]string
	messages       map[string]string
}

type originRecoveryQueueStub struct {
	recoveryQueueStub
	messagesBySource map[txRedis.RecoveryQueueSource]map[string]string
	readErrBySource  map[txRedis.RecoveryQueueSource]error
	reads            []txRedis.RecoveryQueueSource
	ackSources       []txRedis.RecoveryQueueSource
}

type synchronousRecoveryQueueStub struct {
	txRedis.RedisRepository
	raw                     string
	readErr, acknowledgeErr error
	status                  int64
	readSource, ackSource   txRedis.RecoveryQueueSource
	readField, ackField     string
	expectedPayload         string
	organizationID          uuid.UUID
	ledgerID                uuid.UUID
	terminal                bool
	completedAt             time.Time
	acknowledgments         int
}

func (q *synchronousRecoveryQueueStub) ReadRecoveryMessage(_ context.Context, source txRedis.RecoveryQueueSource, field string) (string, error) {
	q.readSource, q.readField = source, field
	return q.raw, q.readErr
}

func (q *synchronousRecoveryQueueStub) CompareAndDeleteRecoveryWithProtectionFrom(
	_ context.Context,
	source txRedis.RecoveryQueueSource,
	organizationID, ledgerID uuid.UUID,
	field, expectedPayload string,
	terminal bool,
	completedAt time.Time,
) (int64, error) {
	q.acknowledgments++
	q.ackSource, q.organizationID, q.ledgerID = source, organizationID, ledgerID
	q.ackField, q.expectedPayload = field, expectedPayload
	q.terminal, q.completedAt = terminal, completedAt

	return q.status, q.acknowledgeErr
}

func TestAcknowledgeEngineRecoveryUsesExactProtectedRecord(t *testing.T) {
	field, raw, envelope := consumerRecoveryFixture(t)
	completedAt := time.Date(2026, time.September, 11, 15, 30, 0, 0, time.UTC)

	for _, test := range []struct {
		status   string
		terminal bool
	}{
		{status: constant.APPROVED, terminal: true},
		{status: constant.CANCELED, terminal: true},
		{status: constant.PENDING, terminal: false},
	} {
		t.Run(test.status, func(t *testing.T) {
			queue := &synchronousRecoveryQueueStub{raw: raw, status: txRedis.RecoveryAckDeleted}
			coordinator := &recoveryRecordCompleter{queue: queue, clock: func() time.Time { return completedAt }}

			err := coordinator.AcknowledgeEngineRecovery(t.Context(), envelope, command.TransactionCompletionResult{
				Outcome: command.TransactionPersistenceOutcome{TransactionStatus: test.status},
			})

			require.NoError(t, err)
			require.Equal(t, 1, queue.acknowledgments)
			require.Equal(t, txRedis.RecoveryQueueSourceEngineRecover, queue.readSource)
			require.Equal(t, txRedis.RecoveryQueueSourceEngineRecover, queue.ackSource)
			require.Equal(t, field, queue.readField)
			require.Equal(t, field, queue.ackField)
			require.Equal(t, raw, queue.expectedPayload)
			require.Equal(t, envelope.OrganizationID, queue.organizationID)
			require.Equal(t, envelope.LedgerID, queue.ledgerID)
			require.Equal(t, test.terminal, queue.terminal)
			require.Equal(t, completedAt, queue.completedAt)
		})
	}
}

func TestAcknowledgeEngineRecoveryRetainsUncertainRecords(t *testing.T) {
	_, raw, envelope := consumerRecoveryFixture(t)
	completedAt := time.Date(2026, time.September, 11, 15, 30, 0, 0, time.UTC)

	t.Run("missing is already acknowledged", func(t *testing.T) {
		queue := &synchronousRecoveryQueueStub{status: txRedis.RecoveryAckDeleted}
		coordinator := &recoveryRecordCompleter{queue: queue, clock: func() time.Time { return completedAt }}
		require.NoError(t, coordinator.AcknowledgeEngineRecovery(t.Context(), envelope, command.TransactionCompletionResult{
			Outcome: command.TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
		}))
		require.Zero(t, queue.acknowledgments)
	})

	t.Run("replacement is retained", func(t *testing.T) {
		queue := &synchronousRecoveryQueueStub{raw: raw, status: txRedis.RecoveryAckReplaced}
		coordinator := &recoveryRecordCompleter{queue: queue, clock: func() time.Time { return completedAt }}
		err := coordinator.AcknowledgeEngineRecovery(t.Context(), envelope, command.TransactionCompletionResult{
			Outcome: command.TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
		})
		require.ErrorContains(t, err, "replacement retained")
	})

	t.Run("different envelope is retained", func(t *testing.T) {
		replacement := *envelope
		payload, err := command.DecodeTransactionCompletionPlan([]byte(replacement.Payload))
		require.NoError(t, err)
		payload.HeaderID = "different-request"
		payloadRaw, err := command.EncodeTransactionCompletionPlan(*payload)
		require.NoError(t, err)
		replacement.Payload = string(payloadRaw)
		replacementRaw, err := command.EncodeTransactionCompletionRecord(replacement)
		require.NoError(t, err)
		queue := &synchronousRecoveryQueueStub{raw: string(replacementRaw), status: txRedis.RecoveryAckDeleted}
		coordinator := &recoveryRecordCompleter{queue: queue, clock: func() time.Time { return completedAt }}
		err = coordinator.AcknowledgeEngineRecovery(t.Context(), envelope, command.TransactionCompletionResult{
			Outcome: command.TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
		})
		require.ErrorContains(t, err, "does not match")
		require.Zero(t, queue.acknowledgments)
	})

	t.Run("unsupported durable status is retained", func(t *testing.T) {
		queue := &synchronousRecoveryQueueStub{raw: raw, status: txRedis.RecoveryAckDeleted}
		coordinator := &recoveryRecordCompleter{queue: queue, clock: func() time.Time { return completedAt }}
		err := coordinator.AcknowledgeEngineRecovery(t.Context(), envelope, command.TransactionCompletionResult{
			Outcome: command.TransactionPersistenceOutcome{TransactionStatus: "CREATED"},
		})
		require.ErrorContains(t, err, "unsupported status")
		require.Zero(t, queue.acknowledgments)
	})
}

func (q *originRecoveryQueueStub) ReadAllRecoveryMessages(_ context.Context, source txRedis.RecoveryQueueSource) (map[string]string, error) {
	q.reads = append(q.reads, source)
	if err := q.readErrBySource[source]; err != nil {
		return nil, err
	}
	return q.messagesBySource[source], nil
}

func (q *originRecoveryQueueStub) CompareAndDeleteRecoveryFrom(_ context.Context, source txRedis.RecoveryQueueSource, field, payload string) (int64, error) {
	q.calls++
	q.field, q.payload = field, payload
	q.ackSources = append(q.ackSources, source)
	*q.order = append(*q.order, "conditional-ack")
	return q.status, q.err
}

func (q *recoveryQueueStub) ReadAllMessagesFromQueue(context.Context) (map[string]string, error) {
	return q.messages, nil
}

type recoveryQuietLogger struct{ libLog.Logger }

func (recoveryQuietLogger) Log(context.Context, int, string, ...any) {}

func TestReadMessagesAndProcess_VersionTwoNeverUsesLegacyDependencies(t *testing.T) {
	field, raw, _ := consumerRecoveryFixture(t)
	order := []string{}
	finalizer := &recoveryCompleterStub{order: &order}
	queue := &recoveryQueueStub{status: 1, order: &order, messages: map[string]string{field: raw}}
	consumer := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}}).WithAppliedTransactionCompleter(finalizer)
	consumer.readMessagesAndProcess(context.Background())
	require.Equal(t, []string{"durable-finalization", "conditional-ack"}, order)
	require.Equal(t, field, queue.field)
	require.Equal(t, raw, queue.payload)
	require.Nil(t, consumer.Command)
	require.Nil(t, consumer.Query)
}

func TestReadMessagesAndProcess_UnknownVersionDoesNotFinalizeOrAcknowledge(t *testing.T) {
	field, _, _ := consumerRecoveryFixture(t)
	for _, raw := range []string{`{"formatVersion":null}`, `{"formatVersion":3}`, `{"formatVersion":2}`, `{`} {
		order := []string{}
		finalizer := &recoveryCompleterStub{order: &order}
		queue := &recoveryQueueStub{status: 1, order: &order, messages: map[string]string{field: raw}}
		consumer := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}}).WithAppliedTransactionCompleter(finalizer)
		consumer.readMessagesAndProcess(context.Background())
		require.Zero(t, finalizer.calls)
		require.Zero(t, queue.calls)
	}
}

func TestReadMessagesAndProcess_OriginsRemainIndependent(t *testing.T) {
	field, raw, _ := consumerRecoveryFixture(t)
	order := []string{}
	finalizer := &recoveryCompleterStub{order: &order}
	queue := &originRecoveryQueueStub{
		recoveryQueueStub: recoveryQueueStub{status: txRedis.RecoveryAckDeleted, order: &order},
		messagesBySource: map[txRedis.RecoveryQueueSource]map[string]string{
			txRedis.RecoveryQueueSourceLegacyBackup:  {field: raw},
			txRedis.RecoveryQueueSourceEngineRecover: {field: raw},
		},
		readErrBySource: map[txRedis.RecoveryQueueSource]error{},
	}
	consumer := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}}).WithAppliedTransactionCompleter(finalizer)
	consumer.readMessagesAndProcess(t.Context())

	require.Equal(t, 2, finalizer.calls)
	require.Equal(t, []txRedis.RecoveryQueueSource{
		txRedis.RecoveryQueueSourceLegacyBackup,
		txRedis.RecoveryQueueSourceEngineRecover,
	}, queue.reads)
	require.ElementsMatch(t, []txRedis.RecoveryQueueSource{
		txRedis.RecoveryQueueSourceLegacyBackup,
		txRedis.RecoveryQueueSourceEngineRecover,
	}, queue.ackSources)
}

func TestLegacyBackupConsumerReadsOnlyLegacyOrigin(t *testing.T) {
	field, raw, _ := consumerRecoveryFixture(t)
	order := []string{}
	queue := &originRecoveryQueueStub{
		recoveryQueueStub: recoveryQueueStub{status: txRedis.RecoveryAckDeleted, order: &order},
		messagesBySource: map[txRedis.RecoveryQueueSource]map[string]string{
			txRedis.RecoveryQueueSourceLegacyBackup:  {field: raw},
			txRedis.RecoveryQueueSourceEngineRecover: {"must-not-be-read": raw},
		},
		readErrBySource: map[txRedis.RecoveryQueueSource]error{},
	}
	runner := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}}).
		WithAppliedTransactionCompleter(&recoveryCompleterStub{order: &order})

	stats := runner.newLegacyBackupConsumer().Consume(t.Context())

	require.True(t, stats.read)
	require.Equal(t, 1, stats.messageCount)
	require.Equal(t, []txRedis.RecoveryQueueSource{txRedis.RecoveryQueueSourceLegacyBackup}, queue.reads)
	require.Equal(t, []txRedis.RecoveryQueueSource{txRedis.RecoveryQueueSourceLegacyBackup}, queue.ackSources)
}

func TestEngineRecoveryConsumerReadsOnlyEngineOrigin(t *testing.T) {
	field, raw, _ := consumerRecoveryFixture(t)
	order := []string{}
	queue := &originRecoveryQueueStub{
		recoveryQueueStub: recoveryQueueStub{status: txRedis.RecoveryAckDeleted, order: &order},
		messagesBySource: map[txRedis.RecoveryQueueSource]map[string]string{
			txRedis.RecoveryQueueSourceLegacyBackup:  {"must-not-be-read": raw},
			txRedis.RecoveryQueueSourceEngineRecover: {field: raw},
		},
		readErrBySource: map[txRedis.RecoveryQueueSource]error{},
	}
	runner := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}}).
		WithAppliedTransactionCompleter(&recoveryCompleterStub{order: &order})

	stats := runner.newEngineRecoveryConsumer().Consume(t.Context())

	require.True(t, stats.read)
	require.Equal(t, 1, stats.messageCount)
	require.Equal(t, []txRedis.RecoveryQueueSource{txRedis.RecoveryQueueSourceEngineRecover}, queue.reads)
	require.Equal(t, []txRedis.RecoveryQueueSource{txRedis.RecoveryQueueSourceEngineRecover}, queue.ackSources)
}

func TestReadMessagesAndProcess_OneOriginFailureDoesNotBlockOther(t *testing.T) {
	field, raw, _ := consumerRecoveryFixture(t)
	for _, failedSource := range []txRedis.RecoveryQueueSource{
		txRedis.RecoveryQueueSourceLegacyBackup,
		txRedis.RecoveryQueueSourceEngineRecover,
	} {
		t.Run(string(failedSource), func(t *testing.T) {
			order := []string{}
			finalizer := &recoveryCompleterStub{order: &order}
			queue := &originRecoveryQueueStub{
				recoveryQueueStub: recoveryQueueStub{status: txRedis.RecoveryAckDeleted, order: &order},
				messagesBySource: map[txRedis.RecoveryQueueSource]map[string]string{
					txRedis.RecoveryQueueSourceLegacyBackup:  {field: raw},
					txRedis.RecoveryQueueSourceEngineRecover: {field: raw},
				},
				readErrBySource: map[txRedis.RecoveryQueueSource]error{failedSource: errors.New("read failed")},
			}
			consumer := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}}).WithAppliedTransactionCompleter(finalizer)
			consumer.readMessagesAndProcess(t.Context())
			require.Equal(t, 1, finalizer.calls)
			require.Len(t, queue.ackSources, 1)
			require.NotEqual(t, failedSource, queue.ackSources[0])
		})
	}
}

func TestReadMessagesAndProcess_EngineRecoverNeverFallsBackToLegacyOrQuarantine(t *testing.T) {
	order := []string{}
	finalizer := &recoveryCompleterStub{order: &order}
	queue := &originRecoveryQueueStub{
		recoveryQueueStub: recoveryQueueStub{status: txRedis.RecoveryAckDeleted, order: &order},
		messagesBySource: map[txRedis.RecoveryQueueSource]map[string]string{
			txRedis.RecoveryQueueSourceEngineRecover: {
				"unversioned": `{}`,
				"unknown":     `{"formatVersion":3}`,
				"malformed":   `{`,
			},
		},
		readErrBySource: map[txRedis.RecoveryQueueSource]error{},
	}
	consumer := (&RedisQueueConsumer{queue: queue, Logger: recoveryQuietLogger{}, quarantineRepo: &recoveryQuarantineStub{}}).WithAppliedTransactionCompleter(finalizer)
	consumer.readMessagesAndProcess(t.Context())
	require.Zero(t, finalizer.calls)
	require.Zero(t, queue.calls)
}

func (q *recoveryQueueStub) CompareAndDeleteRecovery(_ context.Context, field, payload string) (int64, error) {
	q.calls++
	q.field, q.payload = field, payload
	*q.order = append(*q.order, "conditional-ack")
	return q.status, q.err
}

func TestCompleteRecoveryRecord_RequiresDurableCompletionAndExactAck(t *testing.T) {
	for _, test := range []struct {
		name                string
		finalizeErr, ackErr error
		status              int64
		wantErr             bool
	}{
		{name: "deleted", status: 1},
		{name: "already absent", status: 0},
		{name: "replacement retained", status: 2, wantErr: true},
		{name: "invalid status", status: 3, wantErr: true},
		{name: "SQL or metadata uncertain", finalizeErr: errors.New("durability uncertain"), wantErr: true},
		{name: "ack uncertain", ackErr: errors.New("response lost"), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			finalizer := &recoveryCompleterStub{err: test.finalizeErr, order: &order}
			queue := &recoveryQueueStub{status: test.status, err: test.ackErr, order: &order}
			consumer := (&RedisQueueConsumer{queue: queue}).WithAppliedTransactionCompleter(finalizer)
			err := consumer.newRecoveryRecordCompleter().complete(context.Background(), txRedis.RecoveryQueueSourceLegacyBackup, "transaction:execution", "exact original JSON bytes", &command.TransactionCompletionRecord{})
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, finalizer.calls)
			if test.finalizeErr != nil {
				require.ErrorIs(t, err, test.finalizeErr)
				require.Zero(t, queue.calls)
				require.Equal(t, []string{"durable-finalization"}, order)
			} else {
				require.Equal(t, []string{"durable-finalization", "conditional-ack"}, order)
				require.Equal(t, "transaction:execution", queue.field)
				require.Equal(t, "exact original JSON bytes", queue.payload)
				if test.ackErr != nil {
					require.ErrorIs(t, err, test.ackErr)
				}
			}
		})
	}
}

func TestCompleteRecoveryRecord_MissingCapabilityAndCancellationRetain(t *testing.T) {
	order := []string{}
	finalizer := &recoveryCompleterStub{order: &order}
	consumer := (&RedisQueueConsumer{}).WithAppliedTransactionCompleter(finalizer)
	require.Error(t, consumer.newRecoveryRecordCompleter().complete(context.Background(), txRedis.RecoveryQueueSourceLegacyBackup, "field", "raw", nil))
	require.Zero(t, finalizer.calls)
	queue := &recoveryQueueStub{order: &order}
	consumer.queue = queue
	consumer.appliedTransactionCompleter = nil
	require.Error(t, consumer.newRecoveryRecordCompleter().complete(context.Background(), txRedis.RecoveryQueueSourceLegacyBackup, "field", "raw", nil))
	require.Zero(t, queue.calls)
	consumer.appliedTransactionCompleter = finalizer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, consumer.newRecoveryRecordCompleter().complete(ctx, txRedis.RecoveryQueueSourceLegacyBackup, "field", "raw", nil), context.Canceled)
	require.Zero(t, finalizer.calls)
	require.Zero(t, queue.calls)
}
