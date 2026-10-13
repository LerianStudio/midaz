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
	"slices"
	"testing"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestIntegrationLegacyBackupReplayCompletesAPartialWriteWithoutDuplicatingThePersistedLeg
// replays a legacy entry without materialized operations for a transaction an
// older, non-atomic writer left with one of its two legs. The replay must insert
// only the missing leg and keep the persisted one under its own ID.
func TestIntegrationLegacyBackupReplayCompletesAPartialWriteWithoutDuplicatingThePersistedLeg(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	for _, tc := range rebuiltReplayCases {
		t.Run(tc.name, func(t *testing.T) {
			infra := setupRebuiltReplayIntegration(t)
			ctx := context.Background()
			field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, tc)
			raw := infra.legacyEntryRaw(t, ctx, field)

			replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
				WithQuarantineRepository(infra.quarantine).
				newLegacyBackupConsumer()

			replay.Consume(ctx)
			infra.waitForEmptyLegacyBackupQueue(t, ctx)
			written := infra.operationIDs(t, transactionID)
			require.Len(t, written, 2)

			_, err := infra.db.Exec(`DELETE FROM operation WHERE id = $1`, written[0])
			require.NoError(t, err)

			infra.restoreLegacyEntry(t, ctx, field, raw)
			replay.Consume(ctx)
			infra.waitForEmptyLegacyBackupQueue(t, ctx)

			require.EqualValues(t, 2, infra.operationCount(t, transactionID), "the replay must complete the missing leg without duplicating the persisted one")
			assert.Contains(t, infra.operationIDs(t, transactionID), written[1], "the persisted leg keeps its ID")
			assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
		})
	}
}

// TestIntegrationLegacyBackupReplayOfACompleteTransactionAdoptsThePersistedIDs
// replays a legacy entry without materialized operations for a transaction whose
// legs are all persisted under IDs the replay never generates, the shape of
// engine-written UUIDv5 operations. The replay writes nothing new and stores the
// persisted IDs in the entry.
func TestIntegrationLegacyBackupReplayOfACompleteTransactionAdoptsThePersistedIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	infra := setupRebuiltReplayIntegration(t)
	ctx := context.Background()
	field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, rebuiltReplayCases[1])
	raw := infra.legacyEntryRaw(t, ctx, field)

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer()

	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)

	engineIDs := make([]string, 0, 2)
	for _, id := range infra.operationIDs(t, transactionID) {
		engineID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
		_, err := infra.db.Exec(`UPDATE operation SET id = $1 WHERE id = $2`, engineID, id)
		require.NoError(t, err)

		engineIDs = append(engineIDs, engineID)
	}

	slices.Sort(engineIDs)

	infra.restoreLegacyEntry(t, ctx, field, raw)

	lost := lostCleanupRedisRepository{RedisConsumerRepository: infra.redisRepo, dropped: make(chan string, 1)}
	infra.command.TransactionRedisRepo = lost
	replay.Consume(ctx)
	infra.awaitDroppedCleanup(t, lost)
	infra.command.TransactionRedisRepo = infra.redisRepo

	assert.Equal(t, engineIDs, infra.operationIDs(t, transactionID), "the replay must add no operation")
	infra.requireEntryCarriesPersistedOperations(t, ctx, field, transactionID)
}

// TestIntegrationLegacyBackupReplayOfACommitKeepsThePersistedHold replays a
// legacy commit entry, without materialized operations, of a pending
// transaction whose hold is already persisted. The commit legs are inserted
// once, the hold legs stay as persisted, and replaying the entry again adds
// nothing.
func TestIntegrationLegacyBackupReplayOfACommitKeepsThePersistedHold(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	infra := setupRebuiltReplayIntegration(t)
	ctx := context.Background()
	field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, rebuiltReplayCase{name: "hold", status: constant.PENDING, pending: true})
	holdRaw := infra.legacyEntryRaw(t, ctx, field)

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer()

	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)
	hold := infra.operationIDs(t, transactionID)
	require.Len(t, hold, 1, "the hold replay writes the source's ON_HOLD leg")

	var entry mmodel.TransactionRedisQueue
	require.NoError(t, json.Unmarshal([]byte(holdRaw), &entry))
	infra.validateAsLifecycleStage(t, ctx, &entry, constant.APPROVED)
	entry.Balances = infra.snapshotsAfterTransaction(t, transactionID, entry.Balances)
	commitRaw, err := json.Marshal(entry)
	require.NoError(t, err)

	infra.restoreLegacyEntry(t, ctx, field, string(commitRaw))
	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)

	afterCommit := infra.operationIDs(t, transactionID)
	require.Len(t, afterCommit, 3, "the commit inserts exactly its DEBIT and CREDIT legs next to the hold")
	assert.ElementsMatch(t, []string{constant.ONHOLD, constant.DEBIT, constant.CREDIT}, infra.operationTypes(t, transactionID))

	for _, id := range hold {
		assert.Contains(t, afterCommit, id, "the hold legs stay as persisted")
	}

	infra.restoreLegacyEntry(t, ctx, field, string(commitRaw))
	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)

	assert.Equal(t, afterCommit, infra.operationIDs(t, transactionID), "replaying the commit again adds nothing")
	assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
}

// TestIntegrationLegacyBackupReplayQuarantinesARecordWhoseLegsDifferFromThePersistedOnes
// replays a legacy entry without materialized operations for a transaction
// whose persisted leg moved another amount than the rebuild, the shape of an
// amount clipped by overdraft. The replay cannot prove which leg is missing:
// no cycle writes anything or touches the entry, and at the threshold the entry
// moves to the quarantine table under its own reason.
func TestIntegrationLegacyBackupReplayQuarantinesARecordWhoseLegsDifferFromThePersistedOnes(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	infra := setupRebuiltReplayIntegration(t)
	ctx := context.Background()
	field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, rebuiltReplayCases[1])
	raw := infra.legacyEntryRaw(t, ctx, field)

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer()

	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)
	written := infra.operationIDs(t, transactionID)
	require.Len(t, written, 2)

	_, err := infra.db.Exec(`UPDATE operation SET amount = amount / 2 WHERE id = $1`, written[0])
	require.NoError(t, err)

	infra.restoreLegacyEntry(t, ctx, field, raw)

	attempts, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)

	for cycle := int64(1); cycle < QuarantineThreshold; cycle++ {
		replay.Consume(ctx)

		assert.Equal(t, written, infra.operationIDs(t, transactionID), "an ambiguous replay must write nothing")
		assert.Equal(t, raw, infra.legacyEntryRaw(t, ctx, field), "an ambiguous replay must leave the entry as read")
		counted, err := infra.redis.HGet(ctx, attempts, field).Int64()
		require.NoError(t, err, "an ambiguous replay must count toward quarantine")
		assert.Equal(t, cycle, counted)
	}

	replay.Consume(ctx)

	reason, quarantinedAttempts := infra.requireSingleQuarantineRow(t, transactionID, field, raw)
	assert.Equal(t, "persisted_operations_mismatch", reason)
	assert.Equal(t, QuarantineThreshold, quarantinedAttempts)

	messages, err := readRecoveryMessages(ctx, infra.redisRepo, txRedis.RecoveryQueueSourceLegacyBackup)
	require.NoError(t, err)
	assert.Empty(t, messages, "a quarantined record must leave the backup queue")
	assert.Equal(t, written, infra.operationIDs(t, transactionID))
}

func (infra *engineWriteBehindHTTPIntegration) operationTypes(tb testing.TB, transactionID uuid.UUID) []string {
	tb.Helper()

	rows, err := infra.db.Query(`SELECT type FROM operation WHERE transaction_id = $1 AND deleted_at IS NULL`, transactionID)
	require.NoError(tb, err)

	defer rows.Close()

	types := []string{}

	for rows.Next() {
		var opType string
		require.NoError(tb, rows.Scan(&opType))
		types = append(types, opType)
	}

	require.NoError(tb, rows.Err())

	return types
}

// snapshotsAfterTransaction advances each balance snapshot to the state the
// transaction's persisted legs left on that balance, as a later lifecycle
// stage of the same transaction carries in its entry.
func (infra *engineWriteBehindHTTPIntegration) snapshotsAfterTransaction(tb testing.TB, transactionID uuid.UUID, snapshots []mmodel.BalanceRedis) []mmodel.BalanceRedis {
	tb.Helper()

	advanced := slices.Clone(snapshots)

	for i := range advanced {
		var available, onHold string

		var version int64

		err := infra.db.QueryRow(
			`SELECT available_balance_after::text, on_hold_balance_after::text, balance_version_after FROM operation
			 WHERE transaction_id = $1 AND balance_id = $2 ORDER BY balance_version_after DESC LIMIT 1`,
			transactionID, advanced[i].ID,
		).Scan(&available, &onHold, &version)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}

		require.NoError(tb, err)

		advanced[i].Available = decimal.RequireFromString(available)
		advanced[i].OnHold = decimal.RequireFromString(onHold)
		advanced[i].Version = version
	}

	return advanced
}
