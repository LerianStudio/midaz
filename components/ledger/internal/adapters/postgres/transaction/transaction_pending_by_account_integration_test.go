//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// accountClosingPendingLeg writes one operation row tying an account to a
// transaction. side is what the account was in that transaction: the debit leg is
// the source, the credit leg the destination.
func accountClosingPendingLeg(t *testing.T, infra *integrationTestInfra, transactionID, accountID uuid.UUID, side string) {
	t.Helper()

	pgtestutil.CreateTestOperation(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, pgtestutil.OperationParams{
		TransactionID: transactionID,
		Description:   "Account closing pending fixture",
		Type:          side,
		AccountID:     accountID,
		AccountAlias:  "@closing-" + side,
		BalanceID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AssetCode:     "USD",
		Amount:        decimal.NewFromInt(10),
	})
}

// TestIntegration_AccountClosingPendingByAccountSeesBothSides proves the query
// answers on participation, whichever side the account was on, and that it refuses
// to answer for a transaction that is not pending or for another scope.
func TestIntegration_AccountClosingPendingByAccountSeesBothSides(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	source := uuid.Must(libCommons.GenerateUUIDv7())
	destination := uuid.Must(libCommons.GenerateUUIDv7())
	settled := uuid.Must(libCommons.GenerateUUIDv7())
	untouched := uuid.Must(libCommons.GenerateUUIDv7())

	pending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")
	accountClosingPendingLeg(t, infra, pending, source, "debit")
	accountClosingPendingLeg(t, infra, pending, destination, "credit")

	approved := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountClosingPendingLeg(t, infra, approved, settled, "debit")

	tests := []struct {
		name      string
		accountID uuid.UUID
		ledgerID  uuid.UUID
		found     bool
	}{
		{name: "the source of a pending transaction", accountID: source, ledgerID: infra.ledgerID, found: true},
		{name: "the destination of a pending transaction", accountID: destination, ledgerID: infra.ledgerID, found: true},
		{name: "an account whose only transaction settled", accountID: settled, ledgerID: infra.ledgerID},
		{name: "an account with no operation at all", accountID: untouched, ledgerID: infra.ledgerID},
		{name: "the same account read from another ledger", accountID: source, ledgerID: uuid.Must(libCommons.GenerateUUIDv7())},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := infra.repo.HasPendingByAccount(ctx, infra.orgID, test.ledgerID, test.accountID)
			require.NoError(t, err)
			assert.Equal(t, test.found, found)
		})
	}
}

// TestIntegration_AccountClosingPendingByAccountIgnoresDeletedRows proves both
// soft-delete filters hold: a deleted operation stops tying the account to the
// transaction, and a deleted transaction is no longer pending work.
func TestIntegration_AccountClosingPendingByAccountIgnoresDeletedRows(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	deletedLeg := uuid.Must(libCommons.GenerateUUIDv7())
	deletedTransactionAccount := uuid.Must(libCommons.GenerateUUIDv7())

	pending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")
	accountClosingPendingLeg(t, infra, pending, deletedLeg, "debit")

	_, err := infra.pgContainer.DB.Exec(`UPDATE operation SET deleted_at = now() WHERE account_id = $1`, deletedLeg)
	require.NoError(t, err)

	deletedPending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")
	accountClosingPendingLeg(t, infra, deletedPending, deletedTransactionAccount, "debit")

	_, err = infra.pgContainer.DB.Exec(`UPDATE "transaction" SET deleted_at = now() WHERE id = $1`, deletedPending)
	require.NoError(t, err)

	found, err := infra.repo.HasPendingByAccount(ctx, infra.orgID, infra.ledgerID, deletedLeg)
	require.NoError(t, err)
	assert.False(t, found, "a deleted operation no longer ties the account to the transaction")

	found, err = infra.repo.HasPendingByAccount(ctx, infra.orgID, infra.ledgerID, deletedTransactionAccount)
	require.NoError(t, err)
	assert.False(t, found, "a deleted transaction is not pending work")
}

// TestIntegration_AccountClosingPendingByAccountIsInvisibleUntilProjected covers
// AS-09: a pending transaction whose operations are not persisted yet cannot be
// seen here. The refusal of that window belongs to the completion evidence, and
// once the rows land the same query reports the pending transaction.
func TestIntegration_AccountClosingPendingByAccountIsInvisibleUntilProjected(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	pending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")

	found, err := infra.repo.HasPendingByAccount(ctx, infra.orgID, infra.ledgerID, accountID)
	require.NoError(t, err)
	assert.False(t, found, "an unprojected pending transaction is invisible to the persisted query")

	accountClosingPendingLeg(t, infra, pending, accountID, "debit")

	found, err = infra.repo.HasPendingByAccount(ctx, infra.orgID, infra.ledgerID, accountID)
	require.NoError(t, err)
	assert.True(t, found, "once projected, the pending transaction is reported")
}

// TestIntegration_AccountClosingPendingByAccountUsesTheExistingIndexes runs the
// query's own SQL through EXPLAIN over a populated operation table and records the
// plan. It is the evidence behind adding NO index for the closing: the scoped
// account predicate is answered by an index the ledger already has —
// (organization_id, ledger_id, account_id, ...) WHERE deleted_at IS NULL, of which
// idx_operation_account_id and idx_operation_account_balance_pit are the two
// current shapes, either of which the planner may pick — and the operation side is
// never scanned sequentially. The transaction side of the join is left
// unasserted: which access path it gets depends on a table size a fixture cannot
// reproduce, and it is reached by its primary key.
func TestIntegration_AccountClosingPendingByAccountUsesTheExistingIndexes(t *testing.T) {
	infra := setupIntegrationInfra(t)

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	pending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")
	accountClosingPendingLeg(t, infra, pending, accountID, "debit")

	// The plan only describes production if the account predicate is selective, so
	// the table is filled with operations of other accounts in the same scope.
	_, err := infra.pgContainer.DB.Exec(`
		INSERT INTO operation (
			id, transaction_id, description, type, account_id, account_alias, balance_id, balance_key,
			asset_code, chart_of_accounts, amount, available_balance, on_hold_balance,
			available_balance_after, on_hold_balance_after, balance_version_before, balance_version_after,
			status, balance_affected, organization_id, ledger_id, created_at, updated_at
		)
		SELECT gen_random_uuid(), $1, 'Account closing plan fixture', 'debit', gen_random_uuid(), '@closing-seed',
			gen_random_uuid(), 'default', 'USD', 'default', 1, 0, 0, 0, 0, 0, 1,
			'APPROVED', true, $2, $3, now(), now()
		FROM generate_series(1, 20000)
	`, pending, infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	_, err = infra.pgContainer.DB.Exec(`ANALYZE operation; ANALYZE "transaction";`)
	require.NoError(t, err)

	query, args, err := pendingByAccountQuery("transaction", infra.orgID, infra.ledgerID, accountID).ToSql()
	require.NoError(t, err)

	rows, err := infra.pgContainer.DB.Query("EXPLAIN (ANALYZE, BUFFERS) "+query, args...)
	require.NoError(t, err)

	defer func() { require.NoError(t, rows.Close()) }()

	var plan strings.Builder

	for rows.Next() {
		var line string

		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}

	require.NoError(t, rows.Err())

	t.Logf("EXPLAIN plan for the pending-by-account query:\n%s", plan.String())

	assert.Regexp(t, `Index Scan using idx_operation_account(_id|_balance_pit) on operation`, plan.String(),
		"the scoped account predicate must be served by an existing operation index")
	assert.NotContains(t, plan.String(), "Seq Scan on operation",
		"the operation side must never be answered by a sequential scan")
}
