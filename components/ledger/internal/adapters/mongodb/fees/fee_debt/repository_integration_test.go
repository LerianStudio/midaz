//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee_debt_test

import (
	"context"
	"errors"
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
	debtID                           string
}

func newDebtScenario() debtScenario {
	origin := uuid.Must(uuid.NewV7())

	return debtScenario{
		origin:  origin,
		credit1: uuid.Must(uuid.NewV7()),
		credit2: uuid.Must(uuid.NewV7()),
		revert:  uuid.Must(uuid.NewV7()),
		debtID:  origin.String() + ":from:1:debit",
	}
}

func (s debtScenario) change(tx uuid.UUID, postingRef string, kind accounting.FeeDebtChangeKind, amount string) accounting.FeeDebtChange {
	return accounting.FeeDebtChange{
		TransactionID: tx, PostingRef: postingRef, Kind: kind, DebtID: s.debtID,
		DebtorRef: "@payer#default", CreditRef: "@fees#default", OriginTransactionID: s.origin,
		Seq: 3, AssetCode: "BRL", Amount: decimal.RequireFromString(amount),
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

func TestApply_OutOfOrderConverges(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	reversedDB := mongotestutil.CreateOwnedDatabase(t, container)
	scenario := newDebtScenario()
	ctx := context.Background()

	inOrder := staticRepository(t, container.Client, container.DBName, nil)
	reversed := staticRepository(t, container.Client, reversedDB.Name(), nil)

	records := scenario.records()
	for i := range records {
		require.NoError(t, inOrder.Apply(ctx, records[i]))
		require.NoError(t, reversed.Apply(ctx, records[len(records)-1-i]))
	}

	assert.Equal(t, readDebt(t, container.Database, scenario.debtID), readDebt(t, reversedDB, scenario.debtID),
		"a settled change landing before its opened must converge to the same document")
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
		assert.Len(t, debt.Entries, settlements+1, "a racing first insert must retry, not drop its change")
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
		OriginTransactionID: origin, Seq: seq, AssetCode: "BRL", Amount: decimal.NewFromInt(10),
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

	payer, _, err := repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 10, DebtorBalanceRef: "@payer#default"})
	require.NoError(t, err)
	assert.Equal(t, []string{ids[0], ids[2], ids[4]}, debtIDs(payer))

	_, _, err = repo.FindAll(ctx, orgID, ledgerID, fee_debt.ListQuery{Limit: 2, Cursor: "not-a-cursor"})
	assert.True(t, errors.Is(err, libHTTP.ErrInvalidCursor), "got %v", err)
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
