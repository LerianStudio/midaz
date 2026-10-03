//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee_debt_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

var (
	orgID    = uuid.MustParse("01920000-0000-7000-8000-00000000000a")
	ledgerID = uuid.MustParse("01920000-0000-7000-8000-00000000000b")
	t0       = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
)

// staticRepository builds a single-tenant repository on database dbName.
func staticRepository(t *testing.T, client *mongo.Client, dbName string, tenantDB command.TenantMongoResolver) *fee_debt.Repository {
	t.Helper()

	repo, err := fee_debt.NewRepository(&feesmongo.MongoConnection{Database: dbName, DB: client}, tenantDB)
	require.NoError(t, err)

	return repo
}

func readDebt(t *testing.T, db *mongo.Database, id string) model.FeeDebt {
	t.Helper()

	var debt model.FeeDebt
	require.NoError(t, db.Collection(feeconstant.FeeDebtCollection).FindOne(context.Background(), bson.M{"_id": id}).Decode(&debt))

	return debt
}

func countDebts(t *testing.T, db *mongo.Database) int64 {
	t.Helper()

	n, err := db.Collection(feeconstant.FeeDebtCollection).CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err)

	return n
}

// debtScenario is one debt's life: opened 70 by O, settled 30 by C1 and 40 by C2, then
// O reverted, which refunds the 70 the credits settled.
type debtScenario struct {
	origin, credit1, credit2, revert uuid.UUID
	debtID, opened                   string
}

func newDebtScenario() debtScenario {
	origin := uuid.Must(uuid.NewV7())

	return debtScenario{
		origin:  origin,
		credit1: uuid.Must(uuid.NewV7()),
		credit2: uuid.Must(uuid.NewV7()),
		revert:  uuid.Must(uuid.NewV7()),
		debtID:  origin.String() + ":from:1:debit",
		opened:  "70",
	}
}

func (s debtScenario) change(tx uuid.UUID, postingRef string, kind accounting.FeeDebtChangeKind, amount string) accounting.FeeDebtChange {
	return accounting.FeeDebtChange{
		TransactionID: tx, PostingRef: postingRef, Kind: kind, DebtID: s.debtID,
		DebtorRef: "@payer#default", CreditRef: "@fees#default", OriginTransactionID: s.origin,
		Seq: 3, AssetCode: "BRL", Amount: decimal.RequireFromString(amount), Opened: decimal.RequireFromString(s.opened),
	}
}

func record(appliedAt time.Time, feePackageID string, changes ...accounting.FeeDebtChange) command.FeeDebtRecord {
	return command.FeeDebtRecord{OrganizationID: orgID, LedgerID: ledgerID, FeePackageID: feePackageID, AppliedAt: appliedAt, Changes: changes}
}

// records returns the scenario's four completions in the order they were applied.
func (s debtScenario) records() []command.FeeDebtRecord {
	return []command.FeeDebtRecord{
		record(t0, "pkg-1", s.change(s.origin, "from:1:debit", accounting.FeeDebtOpened, "70")),
		record(t0.Add(time.Minute), "", s.change(s.credit1, "to:0:credit:collect", accounting.FeeDebtSettled, "30")),
		record(t0.Add(2*time.Minute), "", s.change(s.credit2, "to:0:credit:collect", accounting.FeeDebtSettled, "40")),
		record(t0.Add(3*time.Minute), "", s.change(s.revert, "fee-refund:0", accounting.FeeDebtRefunded, "70")),
	}
}

func TestApply_ReplayLeavesIdenticalDocument(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	scenario := newDebtScenario()
	ctx := context.Background()

	for _, r := range scenario.records() {
		require.NoError(t, repo.Apply(ctx, r))
	}

	first := readDebt(t, container.Database, scenario.debtID)

	for _, r := range scenario.records() {
		require.NoError(t, repo.Apply(ctx, r), "a replayed change must read as already applied")
	}

	assert.Equal(t, first, readDebt(t, container.Database, scenario.debtID))

	assert.Equal(t, orgID.String(), first.OrganizationID)
	assert.Equal(t, ledgerID.String(), first.LedgerID)
	assert.Equal(t, "@payer#default", first.DebtorBalanceRef)
	assert.Equal(t, "@fees#default", first.CreditBalanceRef)
	assert.Equal(t, scenario.origin.String(), first.OriginTransactionID)
	assert.Equal(t, "pkg-1", first.FeePackageID)
	assert.Equal(t, "BRL", first.AssetCode)
	assert.Equal(t, int64(3), first.Seq)
	assert.Equal(t, "70", first.OpenedAmount.String())
	assert.Equal(t, "0", first.Remaining.String(), "refunded returns money without moving remaining")
	assert.True(t, first.OpenedAt.Equal(t0))
	assert.True(t, first.CreatedAt.Equal(t0))
	assert.True(t, first.UpdatedAt.Equal(t0.Add(3*time.Minute)))
	require.Len(t, first.Entries, 4)
	assert.Equal(t, scenario.origin.String()+":from:1:debit:opened", first.Entries[0].Key)
	assert.Equal(t, []string{"opened", "settled", "settled", "refunded"},
		[]string{first.Entries[0].Kind, first.Entries[1].Kind, first.Entries[2].Kind, first.Entries[3].Kind})
}

func TestApply_SettledBeforeOpenedNeverShowsNegativeRemaining(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	inOrderDB := mongotestutil.CreateOwnedDatabase(t, container)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	inOrder := staticRepository(t, container.Client, inOrderDB.Name(), nil)
	scenario := newDebtScenario()
	ctx := context.Background()
	records := scenario.records()

	for _, r := range records {
		require.NoError(t, inOrder.Apply(ctx, r))
	}

	// Both settlements and the refund land before the opening they depend on.
	for _, step := range []struct {
		record    command.FeeDebtRecord
		remaining string
	}{
		{records[2], "30"}, {records[1], "0"}, {records[3], "0"}, {records[0], "0"},
	} {
		require.NoError(t, repo.Apply(ctx, step.record))

		debt, err := repo.FindByID(ctx, orgID, ledgerID, scenario.debtID)
		require.NoError(t, err)
		assert.Equal(t, step.remaining, debt.Remaining.String())
		assert.Equal(t, "70", debt.OpenedAmount.String(), "every change carries the opened amount")
	}

	assert.Equal(t, readDebt(t, inOrderDB, scenario.debtID), readDebt(t, container.Database, scenario.debtID),
		"changes landing before their opening must converge to the in-order document")
}

func TestApply_RoundsAmountsPastDecimal128Precision(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	scenario := newDebtScenario()
	scenario.opened = "70.000000000000000000000000000000001" // 35 significant digits
	ctx := context.Background()

	records := []command.FeeDebtRecord{
		record(t0, "", scenario.change(scenario.origin, "from:1:debit", accounting.FeeDebtOpened, scenario.opened)),
		record(t0.Add(time.Minute), "", scenario.change(scenario.credit1, "to:0:credit:collect", accounting.FeeDebtSettled, "30.000000000000000000000000000000005")),
	}

	for pass := 0; pass < 2; pass++ {
		for _, r := range records {
			require.NoError(t, repo.Apply(ctx, r), "pass %d: an amount past 34 digits must round, not fail", pass)
		}
	}

	debt := readDebt(t, container.Database, scenario.debtID)
	assert.Equal(t, "70.00000000000000000000000000000000", debt.OpenedAmount.String())
	assert.Equal(t, "39.99999999999999999999999999999999", debt.Remaining.String())
	require.Len(t, debt.Entries, 2)
	assert.Equal(t, "30.000000000000000000000000000000005", debt.Entries[1].Amount, "the entry keeps the exact amount")
}

func TestApply_CancelAndReopenMoveRemaining(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	scenario := newDebtScenario()
	ctx := context.Background()

	require.NoError(t, repo.Apply(ctx, record(t0, "", scenario.change(scenario.origin, "from:1:debit", accounting.FeeDebtOpened, "70"))))
	require.NoError(t, repo.Apply(ctx, record(t0.Add(time.Minute), "", scenario.change(scenario.credit1, "to:0:credit:collect", accounting.FeeDebtSettled, "12.5"))))
	require.NoError(t, repo.Apply(ctx, record(t0.Add(2*time.Minute), "", scenario.change(scenario.credit2, "", accounting.FeeDebtReopened, "12.5"))))
	require.NoError(t, repo.Apply(ctx, record(t0.Add(3*time.Minute), "", scenario.change(scenario.revert, "", accounting.FeeDebtCanceled, "70"))))

	debt := readDebt(t, container.Database, scenario.debtID)
	assert.Equal(t, "0.0", debt.Remaining.String())
	assert.Equal(t, scenario.revert.String()+"::canceled", debt.Entries[3].Key)
	assert.Empty(t, debt.FeePackageID, "an opened change with no package records none")
}

func TestApply_ConcurrentChangesOfANewDebtAllLand(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()

	const debts, settlements = 5, 16

	for d := 0; d < debts; d++ {
		scenario := newDebtScenario()
		scenario.opened = "100"

		var wg sync.WaitGroup

		errs := make(chan error, settlements+1)

		wg.Add(settlements + 1)

		go func() {
			defer wg.Done()

			errs <- repo.Apply(ctx, record(t0, "", scenario.change(scenario.origin, "from:1:debit", accounting.FeeDebtOpened, "100")))
		}()

		for s := 0; s < settlements; s++ {
			go func() {
				defer wg.Done()

				credit := scenario.change(uuid.Must(uuid.NewV7()), "to:0:credit:collect", accounting.FeeDebtSettled, "1")
				errs <- repo.Apply(ctx, record(t0.Add(time.Minute), "", credit))
			}()
		}

		wg.Wait()
		close(errs)

		for err := range errs {
			require.NoError(t, err)
		}

		debt := readDebt(t, container.Database, scenario.debtID)
		assert.Len(t, debt.Entries, settlements+1, "a change that loses the race to seed the debt must still land")
		assert.Equal(t, fmt.Sprint(100-settlements), debt.Remaining.String())
	}
}

// fakeTenantDB resolves every tenant to one database and records who asked.
type fakeTenantDB struct {
	db      *mongo.Database
	mu      sync.Mutex
	tenants []string
}

func (f *fakeTenantDB) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.tenants = append(f.tenants, tenantID)

	return f.db, nil
}

func TestApply_MultiTenantWritesOnlyTheTenantDatabase(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	tenantDB := mongotestutil.CreateOwnedDatabase(t, container)
	otherDB := mongotestutil.CreateOwnedDatabase(t, container)
	resolver := &fakeTenantDB{db: tenantDB}
	repo := staticRepository(t, container.Client, container.DBName, resolver)
	scenario := newDebtScenario()
	opened := record(t0, "", scenario.change(scenario.origin, "from:1:debit", accounting.FeeDebtOpened, "70"))

	err := repo.Apply(context.Background(), opened)
	require.ErrorIs(t, err, tmcore.ErrTenantNotFound, "no tenant on ctx must fail, not fall back to the static database")

	// A completion context carries the transaction store on the generic key.
	ctx := tmcore.ContextWithMB(tmcore.ContextWithTenantID(context.Background(), "tenant-a"), otherDB)
	require.NoError(t, repo.Apply(ctx, opened))

	assert.Equal(t, []string{"tenant-a"}, resolver.tenants)
	assert.Equal(t, int64(1), countDebts(t, tenantDB))
	assert.Zero(t, countDebts(t, container.Database), "the static database must never be written")
	assert.Zero(t, countDebts(t, otherDB), "the store already on ctx must never be written")

	debt, err := repo.FindByID(ctx, orgID, ledgerID, scenario.debtID)
	require.NoError(t, err)
	assert.Equal(t, scenario.debtID, debt.ID)
}

func openedDebt(origin uuid.UUID, debtor string, seq int64) accounting.FeeDebtChange {
	return accounting.FeeDebtChange{
		TransactionID: origin, PostingRef: "from:1:debit", Kind: accounting.FeeDebtOpened,
		DebtID: origin.String() + ":from:1:debit", DebtorRef: debtor, CreditRef: "@fees#default",
		OriginTransactionID: origin, Seq: seq, AssetCode: "BRL", Amount: decimal.NewFromInt(10), Opened: decimal.NewFromInt(10),
	}
}

func debtIDs(debts []*model.FeeDebt) []string {
	ids := make([]string, len(debts))
	for i, debt := range debts {
		ids[i] = debt.ID
	}

	return ids
}

func TestFindAll_OldestFirstWithCursor(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()

	ids := make([]string, 5)

	// Recorded newest first, so the order cannot come from insertion.
	for i := 5; i >= 1; i-- {
		debtor := "@payer#default"
		if i%2 == 0 {
			debtor = "@other#default"
		}

		change := openedDebt(uuid.MustParse(fmt.Sprintf("01920000-0000-7000-8000-%012d", i)), debtor, int64(i))
		require.NoError(t, repo.Apply(ctx, record(t0, "", change)))

		ids[i-1] = change.DebtID
	}

	foreign := record(t0, "", openedDebt(uuid.MustParse("01910000-0000-7000-8000-000000000001"), "@payer#default", 1))
	foreign.LedgerID = uuid.MustParse("01920000-0000-7000-8000-0000000000ff")
	require.NoError(t, repo.Apply(ctx, foreign))

	page1, cursor1, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, ids[0:2], debtIDs(page1), "another ledger's older debt must not appear")
	assert.Empty(t, cursor1.Prev)
	require.NotEmpty(t, cursor1.Next)

	page2, cursor2, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 2, Cursor: cursor1.Next})
	require.NoError(t, err)
	assert.Equal(t, ids[2:4], debtIDs(page2))

	page3, cursor3, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 2, Cursor: cursor2.Next})
	require.NoError(t, err)
	assert.Equal(t, ids[4:5], debtIDs(page3))
	assert.Empty(t, cursor3.Next)

	back, _, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 2, Cursor: cursor3.Prev})
	require.NoError(t, err)
	assert.Equal(t, ids[2:4], debtIDs(back), "a prev page reads oldest first too")
}

func TestFindAll_DebtorListingFollowsSeq(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()

	// Debt ids ascend while seq descends, so the two orders disagree.
	ids := make([]string, 3)

	for i := range ids {
		change := openedDebt(uuid.MustParse(fmt.Sprintf("01920000-0000-7000-8000-%012d", i+1)), "@payer#default", int64(3-i))
		require.NoError(t, repo.Apply(ctx, record(t0, "", change)))

		ids[i] = change.DebtID
	}

	debtor := fee_debt.ListQuery{Limit: 2, DebtorBalanceRef: "@payer#default"}

	page1, cursor1, err := repo.FindAll(ctx, orgID, ledgerID, debtor)
	require.NoError(t, err)
	assert.Equal(t, []string{ids[2], ids[1]}, debtIDs(page1), "one debtor's debts list in settlement order")

	debtor.Cursor = cursor1.Next
	page2, cursor2, err := repo.FindAll(ctx, orgID, ledgerID, debtor)
	require.NoError(t, err)
	assert.Equal(t, []string{ids[0]}, debtIDs(page2))

	debtor.Cursor = cursor2.Prev
	back, _, err := repo.FindAll(ctx, orgID, ledgerID, debtor)
	require.NoError(t, err)
	assert.Equal(t, []string{ids[2], ids[1]}, debtIDs(back))

	all, _, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, ids, debtIDs(all), "the ledger-wide listing stays in debt id order")

	_, ledgerCursor, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 1})
	require.NoError(t, err)

	for _, replay := range []fee_debt.ListQuery{
		{Limit: 2, DebtorBalanceRef: "@payer#default", Cursor: ledgerCursor.Next},
		{Limit: 2, Cursor: cursor1.Next},
	} {
		_, _, err = repo.FindAll(ctx, orgID, ledgerID, replay)
		assert.ErrorIs(t, err, libHTTP.ErrInvalidCursor, "a cursor replayed on the other listing is invalid")
	}
}

func TestFindAll_StatusAndOpenTotal(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()

	debts := make([]accounting.FeeDebtChange, 5)
	for i := range debts {
		debtor := "@payer#default"
		if i == 4 {
			debtor = "@other#default"
		}

		debts[i] = openedDebt(uuid.MustParse(fmt.Sprintf("01920000-0000-7000-8000-%012d", i+1)), debtor, int64(i+1))
		require.NoError(t, repo.Apply(ctx, record(t0, "", debts[i])))
	}

	settle := func(debt accounting.FeeDebtChange, amount int64) {
		change := debt
		change.TransactionID, change.PostingRef, change.Kind = uuid.Must(uuid.NewV7()), "to:0:credit:collect", accounting.FeeDebtSettled
		change.Amount = decimal.NewFromInt(amount)
		require.NoError(t, repo.Apply(ctx, record(t0.Add(time.Minute), "", change)))
	}

	settle(debts[1], 10)
	settle(debts[2], 4)
	settle(debts[3], 10)
	settle(debts[3], 10) // a settlement recorded before the reopen it depends on

	reopen := debts[0] // a reopen recorded before the settlement it undoes: remaining 20
	reopen.TransactionID, reopen.PostingRef, reopen.Kind = uuid.Must(uuid.NewV7()), "fee-reopen:0", accounting.FeeDebtReopened
	require.NoError(t, repo.Apply(ctx, record(t0.Add(time.Minute), "", reopen)))

	foreign := record(t0, "", openedDebt(uuid.MustParse("01910000-0000-7000-8000-000000000001"), "@payer#default", 1))
	foreign.LedgerID = uuid.MustParse("01920000-0000-7000-8000-0000000000ff")
	require.NoError(t, repo.Apply(ctx, foreign))

	list := func(query fee_debt.ListQuery) ([]string, libHTTP.CursorPagination) {
		page, cursor, err := repo.FindAll(ctx, orgID, ledgerID, query)
		require.NoError(t, err)

		return debtIDs(page), cursor
	}

	open, _ := list(fee_debt.ListQuery{Limit: 10, Status: fee_debt.StatusOpen})
	assert.Equal(t, []string{debts[0].DebtID, debts[2].DebtID, debts[4].DebtID}, open)

	settled, _ := list(fee_debt.ListQuery{Limit: 10, Status: fee_debt.StatusSettled})
	assert.Equal(t, []string{debts[1].DebtID, debts[3].DebtID}, settled, "remaining below zero lists as settled")

	payerOpen := fee_debt.ListQuery{Limit: 1, DebtorBalanceRef: "@payer#default", Status: fee_debt.StatusOpen}
	page1, cursor := list(payerOpen)
	payerOpen.Cursor = cursor.Next
	page2, _ := list(payerOpen)
	assert.Equal(t, []string{debts[0].DebtID, debts[2].DebtID}, append(page1, page2...))

	total, err := repo.OpenTotal(ctx, orgID, ledgerID, "@payer#default")
	require.NoError(t, err)
	assert.Equal(t, "16", total.String(), "this debtor's open debts in this ledger, each at most its opened amount")

	total, err = repo.OpenTotal(ctx, orgID, ledgerID, "@nobody#default")
	require.NoError(t, err)
	assert.True(t, total.IsZero())
}

func TestFindByID_ScopedToTheLedger(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()
	change := openedDebt(uuid.Must(uuid.NewV7()), "@payer#default", 1)

	require.NoError(t, repo.Apply(ctx, record(t0, "", change)))

	debt, err := repo.FindByID(ctx, orgID, ledgerID, change.DebtID)
	require.NoError(t, err)
	assert.Equal(t, "10", debt.Remaining.String())

	_, err = repo.FindByID(ctx, orgID, uuid.Must(uuid.NewV7()), change.DebtID)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestSettled_SumsTheExactEntriesOfTheLedgerDebts(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()
	paid, reopened, untouched := newDebtScenario(), newDebtScenario(), newDebtScenario()
	reopened.opened = "70.000000000000000000000000000000001" // past Decimal128 precision

	for _, r := range []command.FeeDebtRecord{
		record(t0, "", paid.change(paid.origin, "from:1:debit", accounting.FeeDebtOpened, "70")),
		record(t0, "", paid.change(paid.credit1, "to:0:credit:collect", accounting.FeeDebtSettled, "30")),
		record(t0, "", paid.change(paid.credit2, "to:0:credit:collect", accounting.FeeDebtSettled, "40")),
		record(t0, "", reopened.change(reopened.origin, "from:1:debit", accounting.FeeDebtOpened, reopened.opened)),
		record(t0, "", reopened.change(reopened.credit1, "to:0:credit:collect", accounting.FeeDebtSettled, "30.000000000000000000000000000000005")),
		record(t0, "", reopened.change(reopened.credit2, "", accounting.FeeDebtReopened, "12.000000000000000000000000000000002")),
		record(t0, "", untouched.change(untouched.origin, "from:1:debit", accounting.FeeDebtOpened, "70")),
	} {
		require.NoError(t, repo.Apply(ctx, r))
	}

	settled, err := repo.Settled(ctx, orgID, ledgerID, []string{paid.debtID, reopened.debtID, untouched.debtID, "missing"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{paid.debtID: "70", reopened.debtID: "18.000000000000000000000000000000003", untouched.debtID: "0"}, decimalStrings(settled),
		"settled minus reopened, exact past Decimal128, and nothing for a debt the ledger does not hold")

	other, err := repo.Settled(ctx, orgID, uuid.Must(uuid.NewV7()), []string{paid.debtID})
	require.NoError(t, err)
	assert.Empty(t, other, "another ledger's debts are not read")
}

func decimalStrings(values map[string]decimal.Decimal) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value.String()
	}

	return out
}

func TestFindAll_ConfinedToTheDebtorAliases(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := staticRepository(t, container.Client, container.DBName, nil)
	ctx := context.Background()

	debtIDByRef := map[string]string{}

	for i, ref := range []string{"@alice#default", "@alice#savings", "@bob#default", "@alice2#default", "@al.ce#default"} {
		change := openedDebt(uuid.MustParse(fmt.Sprintf("01920000-0000-7000-8000-%012d", 100+i)), ref, int64(i+1))
		require.NoError(t, repo.Apply(ctx, record(t0, "", change)))

		debtIDByRef[ref] = change.DebtID
	}

	tests := []struct {
		name  string
		query fee_debt.ListQuery
		want  []string
	}{
		{
			name:  "every balance of an allowed debtor, and only of it",
			query: fee_debt.ListQuery{Limit: 10, ConfineDebtors: true, DebtorAliases: []string{"@alice"}},
			want:  []string{debtIDByRef["@alice#default"], debtIDByRef["@alice#savings"]},
		},
		{
			name:  "an alias is matched literally",
			query: fee_debt.ListQuery{Limit: 10, ConfineDebtors: true, DebtorAliases: []string{"@al.ce", "@bob"}},
			want:  []string{debtIDByRef["@al.ce#default"], debtIDByRef["@bob#default"]},
		},
		{
			name:  "confined to no debtor lists nothing",
			query: fee_debt.ListQuery{Limit: 10, ConfineDebtors: true},
			want:  []string{},
		},
		{
			name:  "a debtor filter inside the confinement",
			query: fee_debt.ListQuery{Limit: 10, ConfineDebtors: true, DebtorAliases: []string{"@bob"}, DebtorBalanceRef: "@alice#default"},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			debts, _, err := repo.FindAll(ctx, orgID, ledgerID, tt.query)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, debtIDs(debts))
		})
	}
}
