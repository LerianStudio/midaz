//go:build integration

package operationroute

import (
	"context"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	nethttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

func envOrSkip(t *testing.T, key string) string {
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("missing env %s; set RUN_DB_INTEGRATION=1 and DB_* to run", key)
	}
	return v
}

func newPGConn(t *testing.T) *libPostgres.PostgresConnection {
	t.Helper()
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=" + envOrSkip(t, "DB_HOST") +
			" user=" + envOrSkip(t, "DB_USER") +
			" password=" + envOrSkip(t, "DB_PASSWORD") +
			" dbname=" + envOrSkip(t, "DB_NAME") +
			" port=" + envOrSkip(t, "DB_PORT") +
			" sslmode=disable",
		PrimaryDBName: envOrSkip(t, "DB_NAME"),
		Component:     "transaction",
	}
	if _, err := conn.GetDB(); err != nil {
		t.Skipf("database not ready: %v", err)
	}
	return conn
}

func ensureOperationRouteSchema(t *testing.T, pc *libPostgres.PostgresConnection) {
	db, _ := pc.GetDB()
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS operation_route (
        id UUID PRIMARY KEY NOT NULL,
        organization_id UUID NOT NULL,
        ledger_id UUID NOT NULL,
        title VARCHAR(255) NOT NULL,
        description VARCHAR(250),
        operation_type VARCHAR(20) NOT NULL CHECK (LOWER(operation_type) IN ('source','destination')),
        account_rule_type VARCHAR(20),
        account_rule_valid_if TEXT,
        created_at TIMESTAMPTZ NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        deleted_at TIMESTAMPTZ
    )`)
}

func TestOperationRouteRepository_Integration_CRUD(t *testing.T) {
	if os.Getenv("RUN_DB_INTEGRATION") == "" {
		t.Skip("set RUN_DB_INTEGRATION=1 to run")
	}

	pc := newPGConn(t)
	ensureOperationRouteSchema(t, pc)
	repo := NewOperationRoutePostgreSQLRepository(pc)

	orgID := uuid.New()
	ledID := uuid.New()
	id := uuid.New()
	now := time.Now().UTC()

	// Create
	created, err := repo.Create(context.Background(), orgID, ledID, &mmodel.OperationRoute{
		ID:             id,
		OrganizationID: orgID,
		LedgerID:       ledID,
		Title:          "route-1",
		Description:    "desc",
		OperationType:  "source",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// FindByID
	got, err := repo.FindByID(context.Background(), orgID, ledID, id)
	if err != nil || got == nil || got.Title != created.Title {
		t.Fatalf("find by id failed: %v got=%#v", err, got)
	}

	// Update
	updTitle := "updated-title"
	upd, err := repo.Update(context.Background(), orgID, ledID, id, &mmodel.OperationRoute{
		Title:       updTitle,
		Description: "d2",
		Code:        "C-1",
		Account:     &mmodel.AccountRule{RuleType: "alias", ValidIf: "@x"},
	})
	if err != nil || upd == nil {
		t.Fatalf("update failed: %v", err)
	}

	// FindByIDs (existing)
	list, err := repo.FindByIDs(context.Background(), orgID, ledID, []uuid.UUID{id})
	if err != nil || len(list) != 1 {
		t.Fatalf("find by ids failed: %v list=%d", err, len(list))
	}

	// FindAll (pagination minimal)
	items, _, err := repo.FindAll(context.Background(), orgID, ledID, nethttp.Pagination{Limit: 10})
	if err != nil || len(items) == 0 {
		t.Fatalf("find all failed: %v items=%d", err, len(items))
	}

	// Delete
	if err := repo.Delete(context.Background(), orgID, ledID, id); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}
