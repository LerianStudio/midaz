//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// seedRecoveryRecord writes one legacy recovery record naming the account, which is
// what an execution still owed a completion leaves behind. The field is returned so
// a test can acknowledge it exactly as the completer does.
func (h *accountClosingHarness) seedRecoveryRecord(t *testing.T, accountID uuid.UUID) string {
	t.Helper()

	record := mmodel.TransactionRedisQueue{
		TransactionID:  uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID: h.organizationID,
		LedgerID:       h.ledgerID,
		Action:         constant.ActionDirect,
		Balances:       []mmodel.BalanceRedis{{ID: uuid.NewString(), AccountID: accountID.String(), Key: constant.DefaultBalanceKey}},
	}

	raw, err := json.Marshal(record)
	require.NoError(t, err)

	field := record.TransactionID.String()
	require.NoError(t, h.client.HSet(context.Background(), txRedis.TransactionBackupQueue, field, string(raw)).Err())

	return field
}

// acknowledgeRecoveryRecord removes one recovery record, which under the current
// acknowledgment contract is what the completer does once persistence concluded.
func (h *accountClosingHarness) acknowledgeRecoveryRecord(t *testing.T, field string) {
	t.Helper()

	require.NoError(t, h.client.HDel(context.Background(), txRedis.TransactionBackupQueue, field).Err())
}

// seedPendingTransaction writes a PENDING transaction and the source-side operation
// row that ties the account to it, which is the shape a hold projects.
func (h *accountClosingHarness) seedPendingTransaction(t *testing.T, accountID uuid.UUID, side string) uuid.UUID {
	t.Helper()

	transactionID := pgtestutil.CreateTestTransactionWithStatus(t, h.transactionDB, h.organizationID, h.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")

	pgtestutil.CreateTestOperation(t, h.transactionDB, h.organizationID, h.ledgerID, pgtestutil.OperationParams{
		TransactionID: transactionID,
		Description:   "Account closing pending leg",
		Type:          side,
		AccountID:     accountID,
		AccountAlias:  "@closing-pending",
		BalanceID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AssetCode:     "USD",
		Amount:        decimal.NewFromInt(10),
	})

	return transactionID
}

// TestIntegrationAccountClosingRefusesUntilTheBalanceSyncCaughtUp is AS-08: a
// credit and a debit of the same amount left the live balance at zero while the row
// is still at the version the sync worker has not reached. Zero is not enough — the
// closing refuses temporarily, discards nothing, and closes only once the row
// carries the same version, without any movement being repeated.
func TestIntegrationAccountClosingRefusesUntilTheBalanceSyncCaughtUp(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-sync", "deposit")
	row := accountClosingBalanceSeed{alias: "@closing-sync", key: "default"}
	balanceID := h.seedBalance(t, accountID, row)

	// The credit and the debit both ran: the live state is zero at version 2 while
	// the row is still the untouched version 0.
	settled := accountClosingBalanceSeed{alias: "@closing-sync", key: "default", version: 2}
	cacheKey := h.cacheBalance(t, accountID, balanceID, settled)

	transactionsBefore, operationsBefore := h.countRows(t, "transaction"), h.countRows(t, "operation")

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)

	require.False(t, h.closedAt(t, accountID).Valid, "a temporary refusal records no instant")
	require.Equal(t, int64(1), h.exists(t, cacheKey), "the cache the sync worker still needs is never removed")

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership),
		"a known refusal gives its own protection back so the workers can converge")

	// The existing sync worker finishes its job.
	h.syncBalanceRow(t, balanceID, settled)

	closedAt, err := h.close(ctx, accountID)
	require.NoError(t, err, "with the persistence proven the same account closes")
	require.True(t, closedAt.Equal(h.closedAt(t, accountID).Time))

	require.Equal(t, transactionsBefore, h.countRows(t, "transaction"), "the closing writes no transaction")
	require.Equal(t, operationsBefore, h.countRows(t, "operation"), "the closing writes no operation")
	require.Equal(t, settled.version, h.balanceRow(t, balanceID).version, "no movement is repeated and no version is duplicated")
}

// TestIntegrationAccountClosingRefusesUntilTheCompletionBacklogCleared walks AS-09
// end to end: while an execution of this account is still in the recovery backlog
// the closing is refused temporarily, because the SQL rows of a pending transaction
// it may yet project are simply not there to be read. Once the completion is
// acknowledged and the rows land, the same account is refused again — now as the
// pending transaction it turned out to hold.
func TestIntegrationAccountClosingRefusesUntilTheCompletionBacklogCleared(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-backlog", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-backlog", key: "default"})

	field := h.seedRecoveryRecord(t, accountID)

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)
	require.False(t, h.closedAt(t, accountID).Valid)

	stored, err := h.client.HGet(ctx, txRedis.TransactionBackupQueue, field).Result()
	require.NoError(t, err)
	require.NotEmpty(t, stored, "the closing reads the recovery backlog without consuming it")

	// The completion concludes and projects the pending transaction it owed.
	h.acknowledgeRecoveryRecord(t, field)
	h.seedPendingTransaction(t, accountID, "debit")

	_, err = h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountHasPendingTransactions)
	require.False(t, h.closedAt(t, accountID).Valid, "the pending transaction keeps the account open")
}

// TestIntegrationAccountClosingRefusesTechnicallyOnInconclusiveEvidence is AS-10:
// when the evidence cannot be established the closing is neither confirmed nor
// guessed, and nothing the recovery needs is discarded.
func TestIntegrationAccountClosingRefusesTechnicallyOnInconclusiveEvidence(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	t.Run("a recovery record that cannot be read", func(t *testing.T) {
		accountID := h.seedAccount(t, "@closing-unreadable-record", "deposit")
		h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-unreadable-record", key: "default"})

		field := uuid.NewString()
		require.NoError(t, h.client.HSet(ctx, txRedis.TransactionBackupQueue, field, "{not-json").Err())

		t.Cleanup(func() { h.client.HDel(context.Background(), txRedis.TransactionBackupQueue, field) })

		_, err := h.close(ctx, accountID)
		requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
		require.False(t, h.closedAt(t, accountID).Valid)

		stored, err := h.client.HGet(ctx, txRedis.TransactionBackupQueue, field).Result()
		require.NoError(t, err)
		require.Equal(t, "{not-json", stored, "an unreadable record is left exactly where the recovery expects it")
	})

	t.Run("a live balance behind its row", func(t *testing.T) {
		accountID := h.seedAccount(t, "@closing-forked", "deposit")
		row := accountClosingBalanceSeed{alias: "@closing-forked", key: "default", version: 4}
		balanceID := h.seedBalance(t, accountID, row)
		cacheKey := h.cacheBalance(t, accountID, balanceID, accountClosingBalanceSeed{alias: "@closing-forked", key: "default", version: 1})

		_, err := h.close(ctx, accountID)
		requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
		require.False(t, h.closedAt(t, accountID).Valid)
		require.Equal(t, int64(1), h.exists(t, cacheKey), "inconclusive evidence is never resolved by deleting it")
	})

	t.Run("the balance store cannot answer", func(t *testing.T) {
		accountID := h.seedAccount(t, "@closing-store-down", "deposit")
		balanceID := h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-store-down", key: "default"})
		cacheKey := h.cacheBalance(t, accountID, balanceID, accountClosingBalanceSeed{alias: "@closing-store-down", key: "default"})

		_, err := h.transactionDB.Exec(`ALTER TABLE balance RENAME TO balance_unavailable`)
		require.NoError(t, err)

		// Registered immediately, so a failing assertion below cannot leave the
		// table renamed for the cases that follow.
		t.Cleanup(func() {
			_, restoreErr := h.transactionDB.Exec(`ALTER TABLE balance_unavailable RENAME TO balance`)
			require.NoError(t, restoreErr)
		})

		_, closeErr := h.close(ctx, accountID)
		requireClosingCode(t, closeErr, constant.ErrAccountClosingProtectionIndeterminate)
		require.NotContains(t, closeErr.Error(), "balance_unavailable",
			"the driver's message never reaches the caller")
		require.False(t, h.closedAt(t, accountID).Valid)
		require.Equal(t, int64(1), h.exists(t, cacheKey), "nothing the recovery needs is discarded")
	})
}

// TestIntegrationAccountClosingLeavesTheAccountMetadataUntouched pins the MongoDB
// side of AC-13: the closing is a state of the account row, so the metadata
// document of that account is neither rewritten nor removed, and no history
// document is created for the transition.
func TestIntegrationAccountClosingLeavesTheAccountMetadataUntouched(t *testing.T) {
	h := newAccountClosingHarness(t)
	database := h.withOnboardingMetadata(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-metadata", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-metadata", key: "default"})

	mongotestutil.InsertMetadata(t, database, constant.EntityAccount, mongotestutil.MetadataFixture{
		EntityID:   accountID.String(),
		EntityName: constant.EntityAccount,
		Data:       map[string]any{"owner": "treasury"},
		CreatedAt:  accountClosingSeedInstant,
		UpdatedAt:  accountClosingSeedInstant,
	})

	before := accountClosingMetadataDocuments(t, database)

	_, err := h.close(ctx, accountID)
	require.NoError(t, err)

	require.Equal(t, before, accountClosingMetadataDocuments(t, database),
		"the closing writes no metadata and removes none")
}

// accountClosingMetadataDocuments reads every metadata document of the account
// collection, without the fields a driver would reorder.
func accountClosingMetadataDocuments(t *testing.T, database *mongo.Database) []bson.M {
	t.Helper()

	cursor, err := database.Collection(constant.EntityAccount).Find(context.Background(), bson.M{})
	require.NoError(t, err)

	var documents []bson.M

	require.NoError(t, cursor.All(context.Background(), &documents))

	return documents
}
