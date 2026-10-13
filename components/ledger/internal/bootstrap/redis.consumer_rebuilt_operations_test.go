// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionquarantine"
	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestWithRebuiltOperationsReplacesOnlyTheOperationsKey(t *testing.T) {
	t.Parallel()

	amount := decimal.RequireFromString("2.50")
	rebuilt := []*operation.Operation{
		{ID: "01a136f4-0000-7000-8000-000000000001", Type: constant.DEBIT, Amount: operation.Amount{Value: &amount}},
		{ID: "01a136f4-0000-7000-8000-000000000002", Type: constant.CREDIT, Amount: operation.Amount{Value: &amount}},
	}

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{
			name: "record without operations",
			raw:  `{"header_id":"h-1", "ttl":"2026-01-01T00:00:00Z", "futureField":{"b":[1, 2.50, "<x>&"], "a":null}, "transaction_status":"APPROVED"}`,
		},
		{
			name: "record with null operations",
			raw:  `{"ttl":"2026-01-01T00:00:00Z","operations":null,"futureField":"kept"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			replaced, err := withRebuiltOperations(tc.raw, rebuilt)
			require.NoError(t, err)

			var original, result map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &original))
			require.NoError(t, json.Unmarshal([]byte(replaced), &result))

			assert.NotContains(t, result, "formatVersion", "the record must keep selecting the legacy replay path")
			expectedKeys := len(original)
			if _, recorded := original["operations"]; !recorded {
				expectedKeys++
			}

			assert.Len(t, result, expectedKeys, "only the operations key may be added")

			for key, value := range original {
				if key == "operations" {
					continue
				}

				var compacted bytes.Buffer
				require.NoError(t, json.Compact(&compacted, value))
				assert.Equalf(t, compacted.String(), string(result[key]), "key %q must keep its recorded value", key)
			}

			var recorded []mmodel.OperationRedis
			require.NoError(t, json.Unmarshal(result["operations"], &recorded))
			require.Len(t, recorded, len(rebuilt))

			for i, op := range recorded {
				assert.Equal(t, rebuilt[i].ID, op.ID)
				assert.Equal(t, rebuilt[i].Type, op.Type)
				assert.True(t, amount.Equal(op.AmountValue))
			}
		})
	}
}

func TestWithRebuiltOperationsRejectsARecordThatIsNotAnObject(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{`null`, `[]`, `not json`} {
		_, err := withRebuiltOperations(raw, nil)
		assert.Errorf(t, err, "raw %q", raw)
	}
}

// TestProcessMessageWritesNothingWhenRebuiltOperationsCannotBeStored replays a
// record whose operations are rebuilt and fails to store them back in the
// record: the record was removed, rewritten by someone else, or Redis failed.
// None of these may write the transaction or count toward quarantine. The
// transaction repository and the attempt counter register no expectation, so
// any write or quarantine attempt fails the test.
func TestProcessMessageWritesNothingWhenRebuiltOperationsCannotBeStored(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		outcome redisTransaction.BackupRecordReplacement
		err     error
	}{
		{name: "record removed after it was read", outcome: redisTransaction.BackupRecordMissing},
		{name: "record rewritten after it was read", outcome: redisTransaction.BackupRecordChanged},
		{name: "redis failure", err: errors.New("redis unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			replay := newRebuildReplay(t)
			mockRedis := replay.redis
			consumer, message, key, raw := replay.consumer, replay.message, replay.key, replay.raw

			replay.operations.EXPECT().
				ListByTransactionPrimary(gomock.Any(), message.OrganizationID, message.LedgerID, message.TransactionID).
				Return(nil, nil)
			mockRedis.EXPECT().
				ReplaceMessageInQueueIfValue(gomock.Any(), key, raw, gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _, replacement string) (redisTransaction.BackupRecordReplacement, error) {
					var stored mmodel.TransactionRedisQueue
					require.NoError(t, json.Unmarshal([]byte(replacement), &stored))
					assert.Len(t, stored.Operations, 2, "the replacement must carry the rebuilt operations")

					return tc.outcome, tc.err
				})

			consumer.processMessage(context.Background(), key, raw, message)
		})
	}
}

// TestProcessMessageWritesNothingWhenPersistedOperationsCannotBeRead replays a
// rebuilt record while the primary read of the transaction's persisted
// operations fails. Whether a leg is persisted is unknown, so nothing is stored
// in the record, written, or counted toward quarantine: no Redis, transaction
// or quarantine call is expected.
func TestProcessMessageWritesNothingWhenPersistedOperationsCannotBeRead(t *testing.T) {
	t.Parallel()

	replay := newRebuildReplay(t)
	replay.operations.EXPECT().
		ListByTransactionPrimary(gomock.Any(), replay.message.OrganizationID, replay.message.LedgerID, replay.message.TransactionID).
		Return(nil, errors.New("primary unavailable"))

	replay.consumer.processMessage(context.Background(), replay.key, replay.raw, replay.message)
}

// TestProcessMessageQuarantinesARebuildThatDiffersFromThePersistedLegs replays
// a rebuilt record whose source leg is persisted with another amount, the shape
// of an amount clipped by overdraft. The record must not be stored or written;
// at the threshold it reaches quarantine under its own reason with the payload
// exactly as read, and leaves the backup queue.
func TestProcessMessageQuarantinesARebuildThatDiffersFromThePersistedLegs(t *testing.T) {
	t.Parallel()

	replay := newRebuildReplay(t)
	source := replay.message.Balances[0]
	clipped := decimal.NewFromInt(50)
	version := source.Version

	replay.operations.EXPECT().
		ListByTransactionPrimary(gomock.Any(), replay.message.OrganizationID, replay.message.LedgerID, replay.message.TransactionID).
		Return([]*operation.Operation{{
			ID: uuid.NewString(), TransactionID: replay.message.TransactionID.String(), BalanceID: source.ID, BalanceKey: source.Key,
			Type: constant.DEBIT, Amount: operation.Amount{Value: &clipped}, Balance: operation.Balance{Version: &version},
		}}, nil)

	gomock.InOrder(
		replay.redis.EXPECT().IncrementBackupAttempt(gomock.Any(), replay.key).Return(int64(QuarantineThreshold), nil),
		replay.quarantine.EXPECT().Insert(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, record *transactionquarantine.QuarantineRecord) error {
				assert.Equal(t, "persisted_operations_mismatch", record.FailureReason)
				assert.Equal(t, []byte(replay.raw), record.Payload, "the quarantine keeps the record as read")

				return nil
			}),
		replay.redis.EXPECT().RemoveMessageFromQueue(gomock.Any(), replay.key).Return(nil),
		replay.redis.EXPECT().ClearBackupAttempt(gomock.Any(), replay.key).Return(nil),
	)

	replay.consumer.processMessage(context.Background(), replay.key, replay.raw, replay.message)
}

// rebuildReplay wires a consumer whose ledger settings, Redis queue, operation
// repository and quarantine are mocks, around a record the replay rebuilds. The
// transaction repository registers no expectation, so any write fails the test.
type rebuildReplay struct {
	consumer   *RedisQueueConsumer
	redis      *redisTransaction.MockRedisRepository
	operations *operation.MockRepository
	quarantine *transactionquarantine.MockRepository
	message    mmodel.TransactionRedisQueue
	key        string
	raw        string
}

func newRebuildReplay(t *testing.T) rebuildReplay {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	replay := rebuildReplay{
		redis:      redisTransaction.NewMockRedisRepository(ctrl),
		operations: operation.NewMockRepository(ctrl),
		quarantine: transactionquarantine.NewMockRepository(ctrl),
		message:    rebuildableLegacyRecord(),
	}

	mockLedger := ledger.NewMockRepository(ctrl)
	replay.consumer = NewRedisQueueConsumer(newTestLogger(),
		&command.UseCase{TransactionRedisRepo: replay.redis, TransactionRepo: transaction.NewMockRepository(ctrl), OperationRepo: replay.operations},
		&query.UseCase{LedgerRepo: mockLedger},
	).WithQuarantineRepository(replay.quarantine)

	replay.key = "transaction:{transactions}:" + replay.message.OrganizationID.String() + ":" + replay.message.LedgerID.String() + ":" + replay.message.TransactionID.String()
	raw, err := json.Marshal(replay.message)
	require.NoError(t, err)
	replay.raw = string(raw)

	mockLedger.EXPECT().GetSettings(gomock.Any(), replay.message.OrganizationID, replay.message.LedgerID).Return(map[string]any{}, nil)

	return replay
}

// rebuildableLegacyRecord is a direct transfer of 100 USD between two accounts
// whose entry carries balance snapshots but no materialized operations.
func rebuildableLegacyRecord() mmodel.TransactionRedisQueue {
	amount := decimal.NewFromInt(100)
	source := mtransaction.FromTo{
		AccountAlias: "@source", BalanceKey: constant.DefaultBalanceKey, IsFrom: true,
		Amount: &mtransaction.Amount{Asset: "USD", Value: amount, Operation: constant.DEBIT},
	}
	destination := mtransaction.FromTo{
		AccountAlias: "@destination", BalanceKey: constant.DefaultBalanceKey,
		Amount: &mtransaction.Amount{Asset: "USD", Value: amount, Operation: constant.CREDIT},
	}
	sourceAlias, destinationAlias := source.ConcatAlias(0), destination.ConcatAlias(0)

	snapshot := func(alias string) mmodel.BalanceRedis {
		return mmodel.BalanceRedis{
			ID: uuid.NewString(), AccountID: uuid.NewString(), Alias: alias, Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: decimal.NewFromInt(1000), Version: 1, AllowSending: 1, AllowReceiving: 1,
			Direction: constant.DirectionCredit,
		}
	}

	return mmodel.TransactionRedisQueue{
		OrganizationID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		LedgerID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TransactionID:     uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		TransactionStatus: constant.APPROVED,
		Action:            constant.ActionDirect,
		Balances:          []mmodel.BalanceRedis{snapshot(sourceAlias), snapshot(destinationAlias)},
		Validate: &mtransaction.Responses{
			From: map[string]mtransaction.Amount{sourceAlias: *source.Amount},
			To:   map[string]mtransaction.Amount{destinationAlias: *destination.Amount},
		},
		TransactionInput: mtransaction.Transaction{
			Send: mtransaction.Send{
				Asset: "USD", Value: amount,
				Source:     mtransaction.Source{From: []mtransaction.FromTo{source}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{destination}},
			},
		},
	}
}
