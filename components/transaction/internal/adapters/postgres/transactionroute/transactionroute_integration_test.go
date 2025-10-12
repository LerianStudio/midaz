//go:build integration

package transactionroute

import (
	"context"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	oprepo "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	nethttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

func envOrSkipTR(t *testing.T, key string) string {
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("missing env %s; set RUN_DB_INTEGRATION=1 and DB_* to run", key)
	}
	return v
}

func newPGConnTR(t *testing.T) *libPostgres.PostgresConnection {
	t.Helper()
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=" + envOrSkipTR(t, "DB_HOST") +
			" user=" + envOrSkipTR(t, "DB_USER") +
			" password=" + envOrSkipTR(t, "DB_PASSWORD") +
			" dbname=" + envOrSkipTR(t, "DB_NAME") +
			" port=" + envOrSkipTR(t, "DB_PORT") +
			" sslmode=disable",
		PrimaryDBName: envOrSkipTR(t, "DB_NAME"),
		Component:     "transaction",
	}
	if _, err := conn.GetDB(); err != nil {
		t.Skipf("database not ready: %v", err)
	}
	return conn
}

func ensureTransactionRouteSchema(t *testing.T, pc *libPostgres.PostgresConnection) {
	db, _ := pc.GetDB()
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS transaction_route (
        id UUID PRIMARY KEY NOT NULL,
        organization_id UUID NOT NULL,
        ledger_id UUID NOT NULL,
        title VARCHAR(255) NOT NULL,
        description VARCHAR(250),
        created_at TIMESTAMPTZ NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        deleted_at TIMESTAMPTZ
    )`)
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
        deleted_at TIMESTAMPTZ,
        code TEXT
    )`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS operation_transaction_route (
        id UUID PRIMARY KEY NOT NULL,
        operation_route_id UUID NOT NULL REFERENCES operation_route(id),
        transaction_route_id UUID NOT NULL REFERENCES transaction_route(id),
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        deleted_at TIMESTAMPTZ
    )`)
}

func TestTransactionRouteRepository_Integration_Flow(t *testing.T) {
	if os.Getenv("RUN_DB_INTEGRATION") == "" {
		t.Skip("set RUN_DB_INTEGRATION=1 to run")
	}

	pc := newPGConnTR(t)
	ensureTransactionRouteSchema(t, pc)

	opRepo := oprepo.NewOperationRoutePostgreSQLRepository(pc)
	trRepo := NewTransactionRoutePostgreSQLRepository(pc)

	orgID := uuid.New()
	ledID := uuid.New()
	now := time.Now().UTC()

	// seed two operation routes
	op1, err := opRepo.Create(context.Background(), orgID, ledID, &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: orgID,
		LedgerID:       ledID,
		Title:          "debit",
		Description:    "op-1",
		OperationType:  "source",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("seed op1 failed: %v", err)
	}
	op2, err := opRepo.Create(context.Background(), orgID, ledID, &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: orgID,
		LedgerID:       ledID,
		Title:          "credit",
		Description:    "op-2",
		OperationType:  "destination",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("seed op2 failed: %v", err)
	}

	// create transaction route with both relations
	trID := uuid.New()
	created, err := trRepo.Create(context.Background(), orgID, ledID, &mmodel.TransactionRoute{
		ID:              trID,
		OrganizationID:  orgID,
		LedgerID:        ledID,
		Title:           "tr-1",
		Description:     "desc",
		OperationRoutes: []mmodel.OperationRoute{*op1, *op2},
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil || created == nil {
		t.Fatalf("create tr failed: %v", err)
	}

	// find by id and verify linked ops
	got, err := trRepo.FindByID(context.Background(), orgID, ledID, trID)
	if err != nil || got == nil || len(got.OperationRoutes) != 2 {
		t.Fatalf("find tr failed: %v got=%#v", err, got)
	}

	// update relations: add a new op, remove one
	op3, err := opRepo.Create(context.Background(), orgID, ledID, &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: orgID,
		LedgerID:       ledID,
		Title:          "credit-2",
		Description:    "op-3",
		OperationType:  "destination",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("seed op3 failed: %v", err)
	}

	_, err = trRepo.Update(context.Background(), orgID, ledID, trID, &mmodel.TransactionRoute{Title: "tr-1-upd"}, []uuid.UUID{op3.ID}, []uuid.UUID{op1.ID})
	if err != nil {
		t.Fatalf("update tr failed: %v", err)
	}

	// list all
	items, _, err := trRepo.FindAll(context.Background(), orgID, ledID, nethttp.Pagination{Limit: 10})
	if err != nil || len(items) == 0 {
		t.Fatalf("find all failed: %v items=%d", err, len(items))
	}

	// delete
	if err := trRepo.Delete(context.Background(), orgID, ledID, trID, []uuid.UUID{op2.ID, op3.ID}); err != nil {
		t.Fatalf("delete tr failed: %v", err)
	}
}
