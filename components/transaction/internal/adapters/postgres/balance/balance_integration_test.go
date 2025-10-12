//go:build integration

package balance

import (
	"context"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func requireEnv(t *testing.T, key string) string {
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("missing env %s; skipping integration test", key)
	}
	return v
}

func newTestPG(t *testing.T) *libPostgres.PostgresConnection {
	t.Helper()
	// expects docker infra up and transaction DB created by init.sql
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=" + requireEnv(t, "DB_HOST") +
			" user=" + requireEnv(t, "DB_USER") +
			" password=" + requireEnv(t, "DB_PASSWORD") +
			" dbname=" + requireEnv(t, "DB_NAME") +
			" port=" + requireEnv(t, "DB_PORT") +
			" sslmode=disable",
		PrimaryDBName: requireEnv(t, "DB_NAME"),
		Component:     "transaction",
	}
	if _, err := conn.GetDB(); err != nil {
		t.Skipf("database not ready: %v", err)
	}
	return conn
}

func TestBalancesUpdate_OptimisticLocking_NoErrorOnZeroRows(t *testing.T) {
	if os.Getenv("RUN_DB_INTEGRATION") == "" {
		t.Skip("set RUN_DB_INTEGRATION=1 to run")
	}

	pc := newTestPG(t)
	repo := NewBalancePostgreSQLRepository(pc)

	orgID := uuid.New()
	ledID := uuid.New()

	// Insert a balance row directly so we can attempt an update
	db, _ := pc.GetDB()
	id := uuid.New()
	now := time.Now()
	_, err := db.Exec(`INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at, key) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'default')`,
		id, orgID, ledID, uuid.New(), "alias-X", "USD", decimal.NewFromInt(0), decimal.NewFromInt(0), 1, "deposit", true, true, now, now,
	)
	if err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}

	// Try to update using lower version (fails optimistic lock → zero rows)
	bal := &mmodel.Balance{
		ID:        id.String(),
		Available: decimal.NewFromInt(10),
		OnHold:    decimal.NewFromInt(0),
		Version:   1, // same version: WHERE version < $N → zero rows
	}
	if err := repo.BalancesUpdate(context.Background(), orgID, ledID, []*mmodel.Balance{bal}); err != nil {
		t.Fatalf("BalancesUpdate returned error (should not error on zero rows): %v", err)
	}
}
