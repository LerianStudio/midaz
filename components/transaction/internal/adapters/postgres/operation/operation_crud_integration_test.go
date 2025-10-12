//go:build integration

package operation

import (
	"context"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	nethttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func envOrSkipOPCRUD(t *testing.T, key string) string {
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("missing env %s; set RUN_DB_INTEGRATION=1 and DB_* to run", key)
	}
	return v
}

func newPGConnOPCRUD(t *testing.T) *libPostgres.PostgresConnection {
	t.Helper()
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=" + envOrSkipOPCRUD(t, "DB_HOST") +
			" user=" + envOrSkipOPCRUD(t, "DB_USER") +
			" password=" + envOrSkipOPCRUD(t, "DB_PASSWORD") +
			" dbname=" + envOrSkipOPCRUD(t, "DB_NAME") +
			" port=" + envOrSkipOPCRUD(t, "DB_PORT") +
			" sslmode=disable",
		PrimaryDBName: envOrSkipOPCRUD(t, "DB_NAME"),
		Component:     "transaction",
	}
	if _, err := conn.GetDB(); err != nil {
		t.Skipf("database not ready: %v", err)
	}
	return conn
}

func ensureOperationRepoSchema(t *testing.T, pc *libPostgres.PostgresConnection) {
	db, _ := pc.GetDB()
	// Minimal schema aligned with repository scans/queries
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS operation (
        id UUID PRIMARY KEY NOT NULL,
        transaction_id UUID NOT NULL,
        description TEXT NOT NULL,
        type TEXT NOT NULL,
        asset_code TEXT NOT NULL,
        amount NUMERIC,
        available_balance NUMERIC,
        on_hold_balance NUMERIC,
        available_balance_after NUMERIC,
        on_hold_balance_after NUMERIC,
        status TEXT NOT NULL,
        status_description TEXT,
        account_id UUID NOT NULL,
        account_alias TEXT NOT NULL,
        balance_id UUID NOT NULL,
        chart_of_accounts TEXT NOT NULL,
        organization_id UUID NOT NULL,
        ledger_id UUID NOT NULL,
        created_at TIMESTAMPTZ NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL,
        deleted_at TIMESTAMPTZ,
        route TEXT,
        balance_affected BOOLEAN DEFAULT TRUE,
        balance_key TEXT
    )`)
}

func TestOperationRepository_Integration_CRUD(t *testing.T) {
	if os.Getenv("RUN_DB_INTEGRATION") == "" {
		t.Skip("set RUN_DB_INTEGRATION=1 to run")
	}

	pc := newPGConnOPCRUD(t)
	ensureOperationRepoSchema(t, pc)

	repo := NewOperationPostgreSQLRepository(pc)

	orgID := uuid.New()
	ledID := uuid.New()
	txID := uuid.New()
	accID := uuid.New()
	balID := uuid.New()
	now := time.Now().UTC()

	amt := decimal.NewFromInt(100)
	bal := decimal.NewFromInt(50)

	// Create
	created, err := repo.Create(context.Background(), &Operation{
		TransactionID:   txID.String(),
		Description:     "initial",
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          Amount{Value: &amt},
		Balance:         Balance{Available: &bal, OnHold: &bal},
		BalanceAfter:    Balance{Available: &bal, OnHold: &bal},
		Status:          Status{Code: "ACTIVE"},
		AccountID:       accID.String(),
		AccountAlias:    "@alias",
		BalanceKey:      "default",
		BalanceID:       balID.String(),
		OrganizationID:  orgID.String(),
		LedgerID:        ledID.String(),
		Route:           "R",
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil || created == nil || created.ID == "" {
		t.Fatalf("create failed: %v created=%#v", err, created)
	}

	opID := uuid.MustParse(created.ID)

	// Find by ID
	got, err := repo.Find(context.Background(), orgID, ledID, txID, opID)
	if err != nil || got == nil || got.ID != created.ID {
		t.Fatalf("find failed: %v got=%#v", err, got)
	}

	// List by IDs
	list, err := repo.ListByIDs(context.Background(), orgID, ledID, []uuid.UUID{opID})
	if err != nil || len(list) != 1 {
		t.Fatalf("list by ids failed: %v size=%d", err, len(list))
	}

	// Find by account
	gotAcc, err := repo.FindByAccount(context.Background(), orgID, ledID, accID, opID)
	if err != nil || gotAcc == nil || gotAcc.ID != created.ID {
		t.Fatalf("find by account failed: %v got=%#v", err, gotAcc)
	}

	// Find all by account
	itemsAcc, _, err := repo.FindAllByAccount(context.Background(), orgID, ledID, accID, nil, nethttp.Pagination{Limit: 10})
	if err != nil || len(itemsAcc) == 0 {
		t.Fatalf("find all by account failed: %v size=%d", err, len(itemsAcc))
	}

	// Find all by transaction
	items, _, err := repo.FindAll(context.Background(), orgID, ledID, txID, nethttp.Pagination{Limit: 10})
	if err != nil || len(items) == 0 {
		t.Fatalf("find all failed: %v size=%d", err, len(items))
	}

	// Update
	upd, err := repo.Update(context.Background(), orgID, ledID, txID, opID, &Operation{Description: "updated"})
	if err != nil || upd == nil || upd.Description == "" {
		t.Fatalf("update failed: %v upd=%#v", err, upd)
	}

	// Delete (soft)
	if err := repo.Delete(context.Background(), orgID, ledID, opID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}
