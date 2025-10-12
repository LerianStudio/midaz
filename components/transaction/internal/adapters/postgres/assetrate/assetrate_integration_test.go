//go:build integration

package assetrate

import (
	"context"
	"os"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	nethttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

func envOrSkipAR(t *testing.T, key string) string {
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("missing env %s; set RUN_DB_INTEGRATION=1 and DB_* to run", key)
	}
	return v
}

func newPGConnAR(t *testing.T) *libPostgres.PostgresConnection {
	t.Helper()
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=" + envOrSkipAR(t, "DB_HOST") +
			" user=" + envOrSkipAR(t, "DB_USER") +
			" password=" + envOrSkipAR(t, "DB_PASSWORD") +
			" dbname=" + envOrSkipAR(t, "DB_NAME") +
			" port=" + envOrSkipAR(t, "DB_PORT") +
			" sslmode=disable",
		PrimaryDBName: envOrSkipAR(t, "DB_NAME"),
		Component:     "transaction",
	}
	if _, err := conn.GetDB(); err != nil {
		t.Skipf("database not ready: %v", err)
	}
	return conn
}

func ensureAssetRateSchema(t *testing.T, pc *libPostgres.PostgresConnection) {
	db, _ := pc.GetDB()
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS asset_rate (
        id UUID PRIMARY KEY NOT NULL,
        organization_id UUID NOT NULL,
        ledger_id UUID NOT NULL,
        external_id UUID NOT NULL,
        "from" TEXT NOT NULL,
        "to" TEXT NOT NULL,
        rate DOUBLE PRECISION NOT NULL,
        rate_scale DOUBLE PRECISION NOT NULL,
        source TEXT,
        ttl INT NOT NULL,
        created_at TIMESTAMPTZ NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL,
        deleted_at TIMESTAMPTZ
    )`)
}

func TestAssetRateRepository_Integration_CRUD(t *testing.T) {
	if os.Getenv("RUN_DB_INTEGRATION") == "" {
		t.Skip("set RUN_DB_INTEGRATION=1 to run")
	}

	pc := newPGConnAR(t)
	ensureAssetRateSchema(t, pc)
	repo := NewAssetRatePostgreSQLRepository(pc)

	orgID := uuid.New()
	ledID := uuid.New()
	now := time.Now().UTC()
	eid := uuid.New()

	// Create
	created, err := repo.Create(context.Background(), &AssetRate{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledID.String(),
		ExternalID:     eid.String(),
		From:           "USD",
		To:             "BRL",
		Rate:           5.1,
		Scale:          func() *float64 { v := 2.0; return &v }(),
		Source:         nil,
		TTL:            3600,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil || created == nil {
		t.Fatalf("create failed: %v", err)
	}

	// FindByExternalID
	gotByExt, err := repo.FindByExternalID(context.Background(), orgID, ledID, eid)
	if err != nil || gotByExt == nil || gotByExt.ExternalID != created.ExternalID {
		t.Fatalf("find by external id failed: %v %#v", err, gotByExt)
	}

	// FindByCurrencyPair
	gotByPair, err := repo.FindByCurrencyPair(context.Background(), orgID, ledID, "USD", "BRL")
	if err != nil || gotByPair == nil {
		t.Fatalf("find by pair failed: %v %#v", err, gotByPair)
	}

	// Update
	updated, err := repo.Update(context.Background(), orgID, ledID, uuid.MustParse(created.ID), &AssetRate{
		ExternalID: eid.String(),
		Rate:       5.2,
		Scale:      func() *float64 { v := 2.0; return &v }(),
		TTL:        7200,
		CreatedAt:  created.CreatedAt,
		UpdatedAt:  now,
	})
	if err != nil || updated == nil || updated.Rate != 5.2 || updated.TTL != 7200 {
		t.Fatalf("update failed: %v %#v", err, updated)
	}

	// FindAllByAssetCodes
	list, _, err := repo.FindAllByAssetCodes(context.Background(), orgID, ledID, "USD", []string{"BRL"}, nethttp.Pagination{Limit: 10})
	if err != nil || len(list) == 0 {
		t.Fatalf("find all by asset codes failed: %v len=%d", err, len(list))
	}
}
