//go:build integration

package postgres

import (
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// OrganizationParams holds parameters for creating a test organization.
type OrganizationParams struct {
	LegalName     string
	LegalDocument string
	Status        string
	DeletedAt     *time.Time
}

// DefaultOrganizationParams returns default parameters for creating a test organization.
func DefaultOrganizationParams() OrganizationParams {
	return OrganizationParams{
		LegalName:     "Test Org",
		LegalDocument: "12345678901234",
		Status:        "ACTIVE",
	}
}

// CreateTestOrganization inserts a test organization and returns its ID.
func CreateTestOrganization(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	return CreateTestOrganizationWithParams(t, db, DefaultOrganizationParams())
}

// CreateTestOrganizationWithParams inserts a test organization with custom params.
func CreateTestOrganizationWithParams(t *testing.T, db *sql.DB, params OrganizationParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO organization (id, legal_name, legal_document, address, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, params.LegalName, params.LegalDocument, `{"city":"Test"}`, params.Status, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test organization")

	return id
}

// LedgerParams holds parameters for creating a test ledger.
type LedgerParams struct {
	Name      string
	Status    string
	DeletedAt *time.Time
}

// DefaultLedgerParams returns default parameters for creating a test ledger.
func DefaultLedgerParams() LedgerParams {
	return LedgerParams{
		Name:   "Test Ledger",
		Status: "ACTIVE",
	}
}

// CreateTestLedger inserts a test ledger and returns its ID.
func CreateTestLedger(t *testing.T, db *sql.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	return CreateTestLedgerWithParams(t, db, orgID, DefaultLedgerParams())
}

// CreateTestLedgerWithParams inserts a test ledger with custom params.
func CreateTestLedgerWithParams(t *testing.T, db *sql.DB, orgID uuid.UUID, params LedgerParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO ledger (id, name, organization_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, params.Name, orgID, params.Status, now, now, params.DeletedAt)
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

// BalanceParams holds parameters for creating a test balance.
type BalanceParams struct {
	Alias          string
	Key            string
	AssetCode      string
	Available      decimal.Decimal
	OnHold         decimal.Decimal
	AccountType    string
	AllowSending   bool
	AllowReceiving bool
	DeletedAt      *time.Time
}

// DefaultBalanceParams returns default parameters for creating a test balance.
func DefaultBalanceParams() BalanceParams {
	return BalanceParams{
		Alias:          "@test-balance",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// CreateTestBalance inserts a balance directly into DB for test setup.
// Uses transaction component's balance table schema.
func CreateTestBalance(t *testing.T, db *sql.DB, orgID, ledgerID, accountID uuid.UUID, params BalanceParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, id, orgID, ledgerID, accountID, params.Alias, params.Key, params.AssetCode,
		params.Available, params.OnHold, 1, params.AccountType,
		params.AllowSending, params.AllowReceiving, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to insert test balance")

	return id
}

// CreateTestBalanceSimple is a convenience wrapper that creates a balance with default params.
func CreateTestBalanceSimple(t *testing.T, db *sql.DB, orgID, ledgerID, accountID uuid.UUID, alias, assetCode string) uuid.UUID {
	t.Helper()

	params := DefaultBalanceParams()
	params.Alias = alias
	params.AssetCode = assetCode

	return CreateTestBalance(t, db, orgID, ledgerID, accountID, params)
}
