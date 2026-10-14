//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// balanceTrailBase is the fixed clock every trail fixture is stamped against.
var balanceTrailBase = time.Date(2026, time.March, 2, 10, 0, 0, 0, time.UTC)

type balanceTrailFixture struct {
	container *pgtestutil.ContainerResult
	repo      *BalancePostgreSQLRepository
	orgID     uuid.UUID
	ledgerID  uuid.UUID
	txID      uuid.UUID
}

func newBalanceTrailFixture(t *testing.T) *balanceTrailFixture {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	return &balanceTrailFixture{
		container: container,
		repo:      createRepository(t, container),
		orgID:     orgID,
		ledgerID:  ledgerID,
		txID:      pgtestutil.CreateTestTransactionWithStatus(t, container.DB, orgID, ledgerID, "APPROVED", decimal.NewFromInt(100), "USD"),
	}
}

func (f *balanceTrailFixture) balance(t *testing.T, alias string) (balanceID, accountID uuid.UUID) {
	t.Helper()

	accountID = createTestAccountID()
	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias

	return pgtestutil.CreateTestBalance(t, f.container.DB, f.orgID, f.ledgerID, accountID, params), accountID
}

func (f *balanceTrailFixture) operation(t *testing.T, balanceID, accountID uuid.UUID, version int64, available int64, affected bool, createdAt time.Time) {
	t.Helper()

	f.operationIn(t, f.txID, "CREDIT", balanceID, accountID, version, available, affected, createdAt, nil)
}

// operationIn writes one trail row under txID. recordedAt nil leaves recorded_at NULL, as
// on rows written before the column existed.
func (f *balanceTrailFixture) operationIn(t *testing.T, txID uuid.UUID, opType string, balanceID, accountID uuid.UUID, version int64, available int64, affected bool, createdAt time.Time, recordedAt *time.Time) {
	t.Helper()

	pgtestutil.CreateTestOperation(t, f.container.DB, f.orgID, f.ledgerID, pgtestutil.OperationParams{
		TransactionID: txID, Description: "trail", Type: opType, AccountID: accountID, AccountAlias: "@trail",
		BalanceID: balanceID, BalanceKey: "default", AssetCode: "USD", Amount: decimal.NewFromInt(1),
		AvailableBalance: decimal.Zero, OnHoldBalance: decimal.Zero,
		AvailableBalanceAfter: decimal.NewFromInt(available), OnHoldBalanceAfter: decimal.NewFromInt(version),
		BalanceVersionBefore: version - 1, BalanceVersionAfter: version,
		Status: "APPROVED", BalanceAffected: affected, CreatedAt: createdAt, RecordedAt: recordedAt,
	})
}

func TestListBalanceTrailPairs(t *testing.T) {
	t.Run("pages by id with chained cursors", func(t *testing.T) {
		f := newBalanceTrailFixture(t)
		ctx := context.Background()

		want := make([]uuid.UUID, 0, 7)
		for i := range 7 {
			id, _ := f.balance(t, fmt.Sprintf("@page-%d", i))
			want = append(want, id)
		}

		sort.Slice(want, func(i, j int) bool { return bytes.Compare(want[i][:], want[j][:]) < 0 })

		var got []uuid.UUID

		cursor := uuid.Nil
		for _, wantLen := range []int{3, 3, 1} {
			pairs, next, err := f.repo.ListBalanceTrailPairs(ctx, cursor, 3)
			require.NoError(t, err)
			require.Len(t, pairs, wantLen)
			assert.Equal(t, pairs[len(pairs)-1].BalanceID, next, "next must be the last id of the page")

			for _, pair := range pairs {
				assert.Equal(t, f.orgID, pair.OrganizationID)
				assert.Equal(t, f.ledgerID, pair.LedgerID)
				assert.Nil(t, pair.Trail)
				got = append(got, pair.BalanceID)
			}

			cursor = next
		}

		assert.Equal(t, want, got, "pages must walk every balance once, in ascending id order")

		pairs, next, err := f.repo.ListBalanceTrailPairs(ctx, cursor, 3)
		require.NoError(t, err)
		assert.Empty(t, pairs, "the page after the last one must be empty")
		assert.Equal(t, cursor, next, "an empty page keeps the cursor")
	})

	t.Run("balance without operations has no trail", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		balanceID, _ := f.balance(t, "@no-trail")

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		assert.Equal(t, balanceID, pairs[0].BalanceID)
		assert.True(t, pairs[0].Available.Equal(decimal.NewFromInt(1000)))
		assert.Nil(t, pairs[0].Trail)
	})

	t.Run("annotation operations are ignored", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		balanceID, accountID := f.balance(t, "@annotated")
		f.operation(t, balanceID, accountID, 2, 200, true, balanceTrailBase)
		f.operation(t, balanceID, accountID, 99, 9900, false, balanceTrailBase.Add(time.Minute))

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		require.NotNil(t, pairs[0].Trail)
		assert.Equal(t, int64(2), pairs[0].Trail.Version)
		assert.True(t, pairs[0].Trail.Available.Equal(decimal.NewFromInt(200)))
		assert.True(t, pairs[0].Trail.OnHold.Equal(decimal.NewFromInt(2)))
		assert.True(t, pairs[0].Trail.RecordedAt.Equal(balanceTrailBase))
	})

	t.Run("version wins when created_at inverts", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		balanceID, accountID := f.balance(t, "@inverted")
		f.operation(t, balanceID, accountID, 5, 500, true, balanceTrailBase)
		f.operation(t, balanceID, accountID, 4, 400, true, balanceTrailBase.Add(time.Minute))

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		require.NotNil(t, pairs[0].Trail)
		assert.Equal(t, int64(5), pairs[0].Trail.Version)
		assert.True(t, pairs[0].Trail.Available.Equal(decimal.NewFromInt(500)))
		assert.True(t, pairs[0].Trail.RecordedAt.Equal(balanceTrailBase))
	})

	t.Run("recorded_at drives the head timestamp when set", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		recordedAt := balanceTrailBase.Add(time.Hour)
		balanceID, accountID := f.balance(t, "@recorded")
		f.operationIn(t, f.txID, "CREDIT", balanceID, accountID, 3, 300, true, balanceTrailBase, &recordedAt)

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		require.NotNil(t, pairs[0].Trail)
		assert.True(t, pairs[0].Trail.RecordedAt.Equal(recordedAt), "got %s", pairs[0].Trail.RecordedAt)
	})

	t.Run("credit head of a cancelled transaction is a validated cancel credit", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		cancelledTx := pgtestutil.CreateTestTransactionWithStatus(t, f.container.DB, f.orgID, f.ledgerID, "CANCELED", decimal.NewFromInt(100), "USD")

		balanceID, accountID := f.balance(t, "@cancelled")
		f.operationIn(t, cancelledTx, "RELEASE", balanceID, accountID, 4, 400, true, balanceTrailBase, nil)
		f.operationIn(t, cancelledTx, "CREDIT", balanceID, accountID, 5, 500, true, balanceTrailBase, nil)

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		require.NotNil(t, pairs[0].Trail)
		assert.Equal(t, int64(5), pairs[0].Trail.Version)
		assert.True(t, pairs[0].Trail.ValidatedCancelCredit)
	})

	t.Run("non-credit head of a cancelled transaction is not a validated cancel credit", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		cancelledTx := pgtestutil.CreateTestTransactionWithStatus(t, f.container.DB, f.orgID, f.ledgerID, "CANCELED", decimal.NewFromInt(100), "USD")

		balanceID, accountID := f.balance(t, "@released")
		f.operationIn(t, cancelledTx, "RELEASE", balanceID, accountID, 4, 400, true, balanceTrailBase, nil)

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		require.NotNil(t, pairs[0].Trail)
		assert.False(t, pairs[0].Trail.ValidatedCancelCredit)
	})

	t.Run("credit head of an approved transaction is not a validated cancel credit", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		balanceID, accountID := f.balance(t, "@approved")
		f.operation(t, balanceID, accountID, 2, 200, true, balanceTrailBase)

		pairs, _, err := f.repo.ListBalanceTrailPairs(context.Background(), uuid.Nil, 10)
		require.NoError(t, err)
		require.Len(t, pairs, 1)
		require.NotNil(t, pairs[0].Trail)
		assert.False(t, pairs[0].Trail.ValidatedCancelCredit)
	})

	t.Run("trail head is read through idx_operation_balance_version", func(t *testing.T) {
		f := newBalanceTrailFixture(t)

		for i := range 3 {
			balanceID, accountID := f.balance(t, fmt.Sprintf("@explain-%d", i))
			f.operation(t, balanceID, accountID, 1, 100, true, balanceTrailBase)
			f.operation(t, balanceID, accountID, 2, 200, true, balanceTrailBase.Add(time.Minute))
		}

		ctx := context.Background()

		rows, err := f.container.DB.QueryContext(ctx, "EXPLAIN (FORMAT TEXT) "+balanceTrailPairsQuery, uuid.Nil, 500)
		require.NoError(t, err)

		defer rows.Close()

		var plan strings.Builder

		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			plan.WriteString(line)
			plan.WriteString("\n")
		}

		require.NoError(t, rows.Err())
		assert.Contains(t, plan.String(), "idx_operation_balance_version", "plan:\n%s", plan.String())
	})
}
