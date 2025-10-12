//go:build integration

package operation

import (
	"context"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/shopspring/decimal"
)

func envOrSkipOP(t *testing.T, key string) string {
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("missing env %s; set RUN_DB_INTEGRATION=1 and DB_* to run", key)
	}
	return v
}

func newPGConnOP(t *testing.T) *libPostgres.PostgresConnection {
	t.Helper()
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=" + envOrSkipOP(t, "DB_HOST") +
			" user=" + envOrSkipOP(t, "DB_USER") +
			" password=" + envOrSkipOP(t, "DB_PASSWORD") +
			" dbname=" + envOrSkipOP(t, "DB_NAME") +
			" port=" + envOrSkipOP(t, "DB_PORT") +
			" sslmode=disable",
		PrimaryDBName: envOrSkipOP(t, "DB_NAME"),
		Component:     "transaction",
	}
	if _, err := conn.GetDB(); err != nil {
		t.Skipf("database not ready: %v", err)
	}
	return conn
}

func ensureOperationSchema(t *testing.T, pc *libPostgres.PostgresConnection) {
	db, _ := pc.GetDB()
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS operation (
        id UUID PRIMARY KEY NOT NULL,
        transaction_id UUID NOT NULL,
        description TEXT NOT NULL,
        type TEXT NOT NULL,
        asset_code TEXT NOT NULL,
        amount DOUBLE PRECISION,
        available_balance DOUBLE PRECISION,
        on_hold_balance DOUBLE PRECISION,
        available_balance_after DOUBLE PRECISION,
        on_hold_balance_after DOUBLE PRECISION,
        status TEXT NOT NULL,
        status_description TEXT,
        account_id TEXT NOT NULL,
        account_alias TEXT NOT NULL,
        balance_id TEXT NOT NULL,
        chart_of_accounts TEXT NOT NULL,
        organization_id TEXT NOT NULL,
        ledger_id TEXT NOT NULL,
        created_at TIMESTAMPTZ NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL,
        deleted_at TIMESTAMPTZ
    )`)
}

func TestOperationRepository_ModelInsertScan(t *testing.T) {
	if os.Getenv("RUN_DB_INTEGRATION") == "" {
		t.Skip("set RUN_DB_INTEGRATION=1 to run")
	}

	pc := newPGConnOP(t)
	ensureOperationSchema(t, pc)
	db, _ := pc.GetDB()

	amt := decimal.NewFromInt(100)
	bal := decimal.NewFromInt(50)
	now := time.Now().UTC()

	// use model conversion then insert minimal row
	var model OperationPostgreSQLModel
	model.FromEntity(&Operation{
		ID:              "op-1",
		TransactionID:   "tx-1",
		Description:     "desc",
		Type:            "DEBIT",
		AssetCode:       "USD",
		Amount:          Amount{Value: &amt},
		Balance:         Balance{Available: &bal, OnHold: &bal},
		BalanceAfter:    Balance{Available: &bal, OnHold: &bal},
		Status:          Status{Code: "ACTIVE"},
		AccountID:       "acc",
		AccountAlias:    "@a",
		BalanceKey:      "default",
		BalanceID:       "bal",
		OrganizationID:  "org",
		LedgerID:        "led",
		Route:           "R",
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	_, err := db.ExecContext(context.Background(), `INSERT INTO operation (id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance, available_balance_after, on_hold_balance_after, status, status_description, account_id, account_alias, balance_id, chart_of_accounts, organization_id, ledger_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		model.ID, model.TransactionID, model.Description, model.Type, model.AssetCode, model.Amount, model.AvailableBalance, model.OnHoldBalance, model.AvailableBalanceAfter, model.OnHoldBalanceAfter, model.Status, model.StatusDescription, model.AccountID, model.AccountAlias, model.BalanceID, model.ChartOfAccounts, model.OrganizationID, model.LedgerID, model.CreatedAt, model.UpdatedAt)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
}
