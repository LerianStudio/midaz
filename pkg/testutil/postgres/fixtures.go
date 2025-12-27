//go:build integration

package postgres

import (
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// CreateTestOrganization inserts a test organization and returns its ID.
func CreateTestOrganization(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	_, err := db.Exec(`
		INSERT INTO organization (id, legal_name, legal_document, address, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, "Test Org", "12345678901234", `{"city":"Test"}`, "ACTIVE", time.Now(), time.Now())
	require.NoError(t, err, "failed to create test organization")

	return id
}

// CreateTestLedger inserts a test ledger and returns its ID.
func CreateTestLedger(t *testing.T, db *sql.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	_, err := db.Exec(`
		INSERT INTO ledger (id, name, organization_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, "Test Ledger", orgID, "ACTIVE", time.Now(), time.Now())
	require.NoError(t, err, "failed to create test ledger")

	return id
}

// CreateTestPortfolio inserts a test portfolio and returns its ID.
func CreateTestPortfolio(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	_, err := db.Exec(`
		INSERT INTO portfolio (id, name, entity_id, ledger_id, organization_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, "Test Portfolio", "entity-123", ledgerID, orgID, "ACTIVE", time.Now(), time.Now())
	require.NoError(t, err, "failed to create test portfolio")

	return id
}

// CreateTestAsset inserts a test asset and returns its ID.
func CreateTestAsset(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, code string) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	_, err := db.Exec(`
		INSERT INTO asset (id, name, type, code, organization_id, ledger_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, code+" Asset", "currency", code, orgID, ledgerID, "ACTIVE", time.Now(), time.Now())
	require.NoError(t, err, "failed to create test asset")

	return id
}

// CreateTestAccount inserts an account directly into DB for test setup.
// Parameters: db, orgID, ledgerID, portfolioID (nil for none), name, alias, assetCode, deletedAt (nil for active)
func CreateTestAccount(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, portfolioID *uuid.UUID, name, alias, assetCode string, deletedAt *time.Time) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	var portfolioIDVal any
	if portfolioID != nil {
		portfolioIDVal = *portfolioID
	}

	_, err := db.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, portfolio_id, status, alias, type, blocked, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, id, name, assetCode, orgID, ledgerID, portfolioIDVal, "ACTIVE", alias, "deposit", false, now, now, deletedAt)
	require.NoError(t, err, "failed to insert test account")

	return id
}
