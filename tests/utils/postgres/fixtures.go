// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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

// PortfolioParams holds parameters for creating a test portfolio with full control.
type PortfolioParams struct {
	Name              string
	EntityID          string // TEXT field, not UUID
	Status            string
	StatusDescription *string
	DeletedAt         *time.Time
}

// DefaultPortfolioParams returns default parameters for creating a test portfolio.
func DefaultPortfolioParams() PortfolioParams {
	return PortfolioParams{
		Name:     "Test Portfolio",
		EntityID: "entity-123",
		Status:   "ACTIVE",
	}
}

// CreateTestPortfolio inserts a test portfolio and returns its ID.
func CreateTestPortfolio(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID) uuid.UUID {
	t.Helper()
	return CreateTestPortfolioWithParams(t, db, orgID, ledgerID, DefaultPortfolioParams())
}

// CreateTestPortfolioWithParams inserts a test portfolio with custom params and returns its ID.
func CreateTestPortfolioWithParams(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params PortfolioParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO portfolio (id, name, entity_id, ledger_id, organization_id, status, status_description, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, params.Name, params.EntityID, ledgerID, orgID, params.Status, params.StatusDescription, now, now, params.DeletedAt)
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

// AssetParams holds parameters for creating a test asset with full control.
type AssetParams struct {
	Name              string
	Type              string
	Code              string
	Status            string
	StatusDescription *string
	DeletedAt         *time.Time
}

// DefaultAssetParams returns default parameters for creating a test asset.
func DefaultAssetParams() AssetParams {
	return AssetParams{
		Name:   "Test Asset",
		Type:   "currency",
		Code:   "USD",
		Status: "ACTIVE",
	}
}

// CreateTestAssetWithParams inserts a test asset with custom params and returns its ID.
func CreateTestAssetWithParams(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params AssetParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO asset (id, name, type, code, status, status_description, ledger_id, organization_id, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, params.Name, params.Type, params.Code, params.Status, params.StatusDescription,
		ledgerID, orgID, now, now, params.DeletedAt)
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
		params.Available, params.OnHold, 0, params.AccountType, // version=0 matches schema default
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

// TransactionParams holds parameters for creating a test transaction.
type TransactionParams struct {
	ParentTransactionID      *uuid.UUID
	Description              string
	Status                   string
	StatusDescription        *string
	Amount                   decimal.Decimal
	AssetCode                string
	ChartOfAccountsGroupName string
	Route                    *string
	Body                     *string // JSON body (nullable after migration 000004)
	DeletedAt                *time.Time
}

// DefaultTransactionParams returns default parameters for creating a test transaction.
func DefaultTransactionParams() TransactionParams {
	body := `{"send":{"asset":"USD","value":"100"}}`

	return TransactionParams{
		Description:              "Test transaction",
		Status:                   "PENDING",
		Amount:                   decimal.NewFromInt(100),
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "default",
		Body:                     &body,
	}
}

// CreateTestTransaction inserts a transaction directly into DB for test setup.
func CreateTestTransaction(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params TransactionParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	var parentID any
	if params.ParentTransactionID != nil {
		parentID = *params.ParentTransactionID
	}

	_, err := db.Exec(`
		INSERT INTO transaction (id, parent_transaction_id, description, status, status_description, amount, asset_code, chart_of_accounts_group_name, route, organization_id, ledger_id, body, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, parentID, params.Description, params.Status, params.StatusDescription,
		params.Amount, params.AssetCode, params.ChartOfAccountsGroupName, params.Route,
		orgID, ledgerID, params.Body, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test transaction")

	return id
}

// CreateTestTransactionWithStatus is a convenience wrapper for creating a transaction with a specific status.
func CreateTestTransactionWithStatus(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, status string, amount decimal.Decimal, assetCode string) uuid.UUID {
	t.Helper()

	params := DefaultTransactionParams()
	params.Status = status
	params.Amount = amount
	params.AssetCode = assetCode

	return CreateTestTransaction(t, db, orgID, ledgerID, params)
}

// OperationParams holds parameters for creating a test operation.
type OperationParams struct {
	TransactionID         uuid.UUID
	Description           string
	Type                  string // "debit" or "credit"
	AccountID             uuid.UUID
	AccountAlias          string
	BalanceID             uuid.UUID
	BalanceKey            string
	AssetCode             string
	ChartOfAccounts       string
	Amount                decimal.Decimal
	AvailableBalance      decimal.Decimal // balance before operation
	OnHoldBalance         decimal.Decimal // balance before operation
	AvailableBalanceAfter decimal.Decimal // balance after operation
	OnHoldBalanceAfter    decimal.Decimal // balance after operation
	BalanceVersionBefore  int64
	BalanceVersionAfter   int64
	Status                string
	Route                 *string
	BalanceAffected       bool
	DeletedAt             *time.Time
}

// CreateTestOperation inserts an operation directly into DB for test setup.
func CreateTestOperation(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params OperationParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	// Set defaults for optional fields
	status := params.Status
	if status == "" {
		status = "APPROVED"
	}

	balanceKey := params.BalanceKey
	if balanceKey == "" {
		balanceKey = "default"
	}

	chartOfAccounts := params.ChartOfAccounts
	if chartOfAccounts == "" {
		chartOfAccounts = "default"
	}

	balanceAffected := true
	if !params.BalanceAffected && params.Status != "" {
		// Only respect false if explicitly set (Status is set as indicator)
		balanceAffected = params.BalanceAffected
	}

	_, err := db.Exec(`
		INSERT INTO operation (
			id, transaction_id, description, type, account_id, account_alias, balance_id, balance_key,
			asset_code, chart_of_accounts, amount, available_balance, on_hold_balance,
			available_balance_after, on_hold_balance_after, balance_version_before, balance_version_after,
			status, route, balance_affected, organization_id, ledger_id, created_at, updated_at, deleted_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
	`, id, params.TransactionID, params.Description, params.Type, params.AccountID, params.AccountAlias,
		params.BalanceID, balanceKey, params.AssetCode, chartOfAccounts, params.Amount,
		params.AvailableBalance, params.OnHoldBalance, params.AvailableBalanceAfter, params.OnHoldBalanceAfter,
		params.BalanceVersionBefore, params.BalanceVersionAfter, status, params.Route, balanceAffected,
		orgID, ledgerID, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test operation")

	return id
}

// UpdateTransactionStatus updates the status of a transaction.
func UpdateTransactionStatus(t *testing.T, db *sql.DB, txID uuid.UUID, status string) {
	t.Helper()

	_, err := db.Exec(`UPDATE transaction SET status = $1, updated_at = $2 WHERE id = $3`,
		status, time.Now().Truncate(time.Microsecond), txID)
	require.NoError(t, err, "failed to update transaction status")
}

// GetTransactionStatus retrieves the current status of a transaction.
func GetTransactionStatus(t *testing.T, db *sql.DB, txID uuid.UUID) string {
	t.Helper()

	var status string

	err := db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, txID).Scan(&status)
	require.NoError(t, err, "failed to get transaction status")

	return status
}

// GetTransactionParentID retrieves the parent_transaction_id of a transaction.
func GetTransactionParentID(t *testing.T, db *sql.DB, txID uuid.UUID) *uuid.UUID {
	t.Helper()

	var parentID *string

	err := db.QueryRow(`SELECT parent_transaction_id FROM transaction WHERE id = $1`, txID).Scan(&parentID)
	require.NoError(t, err, "failed to get transaction parent ID")

	if parentID == nil {
		return nil
	}

	id, err := uuid.Parse(*parentID)
	require.NoError(t, err, "failed to parse parent transaction ID")

	return &id
}

// GetBalanceAvailable retrieves the available amount of a balance.
func GetBalanceAvailable(t *testing.T, db *sql.DB, balanceID uuid.UUID) decimal.Decimal {
	t.Helper()

	var available decimal.Decimal

	err := db.QueryRow(`SELECT available FROM balance WHERE id = $1`, balanceID).Scan(&available)
	require.NoError(t, err, "failed to get balance available")

	return available
}

// GetBalanceOnHold retrieves the on_hold amount of a balance.
func GetBalanceOnHold(t *testing.T, db *sql.DB, balanceID uuid.UUID) decimal.Decimal {
	t.Helper()

	var onHold decimal.Decimal

	err := db.QueryRow(`SELECT on_hold FROM balance WHERE id = $1`, balanceID).Scan(&onHold)
	require.NoError(t, err, "failed to get balance on_hold")

	return onHold
}

// CountOperationsByTransactionID counts operations for a given transaction.
func CountOperationsByTransactionID(t *testing.T, db *sql.DB, txID uuid.UUID) int {
	t.Helper()

	var count int

	err := db.QueryRow(`SELECT COUNT(*) FROM operation WHERE transaction_id = $1`, txID).Scan(&count)
	require.NoError(t, err, "failed to count operations")

	return count
}

// GetTransactionByParentID finds a transaction by its parent_transaction_id.
// Returns nil if no transaction found with that parent.
func GetTransactionByParentID(t *testing.T, db *sql.DB, parentID uuid.UUID) *uuid.UUID {
	t.Helper()

	var id string

	err := db.QueryRow(`SELECT id FROM transaction WHERE parent_transaction_id = $1`, parentID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil
	}

	require.NoError(t, err, "failed to get transaction by parent ID")

	txID, err := uuid.Parse(id)
	require.NoError(t, err, "failed to parse transaction ID")

	return &txID
}

// GetBalanceByAlias retrieves the available balance amount for a given alias within an organization and ledger.
func GetBalanceByAlias(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, alias string) decimal.Decimal {
	t.Helper()

	var available decimal.Decimal

	err := db.QueryRow(`
		SELECT available FROM balance
		WHERE organization_id = $1 AND ledger_id = $2 AND alias = $3 AND deleted_at IS NULL
	`, orgID, ledgerID, alias).Scan(&available)
	require.NoError(t, err, "failed to get balance by alias %s", alias)

	return available
}

// SegmentParams holds parameters for creating a test segment.
type SegmentParams struct {
	Name              string
	Status            string
	StatusDescription *string
	DeletedAt         *time.Time
}

// DefaultSegmentParams returns default parameters for creating a test segment.
func DefaultSegmentParams() SegmentParams {
	return SegmentParams{
		Name:   "Test Segment",
		Status: "ACTIVE",
	}
}

// CreateTestSegmentWithParams inserts a test segment with custom params and returns its ID.
func CreateTestSegmentWithParams(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params SegmentParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO segment (id, name, ledger_id, organization_id, status, status_description, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, params.Name, ledgerID, orgID, params.Status, params.StatusDescription, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test segment")

	return id
}

// AccountTypeParams holds parameters for creating a test account type.
type AccountTypeParams struct {
	Name        string
	Description string
	KeyValue    string
	DeletedAt   *time.Time
}

// DefaultAccountTypeParams returns default parameters for creating a test account type.
func DefaultAccountTypeParams() AccountTypeParams {
	return AccountTypeParams{
		Name:        "Test Account Type",
		Description: "Test description",
		KeyValue:    "test-key",
	}
}

// CreateTestAccountType inserts an account type directly into DB for test setup.
func CreateTestAccountType(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params AccountTypeParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO account_type (id, organization_id, ledger_id, name, description, key_value, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, orgID, ledgerID, params.Name, params.Description, params.KeyValue, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test account type")

	return id
}

// AssetRateParams holds parameters for creating a test asset rate.
type AssetRateParams struct {
	ExternalID *uuid.UUID
	From       string
	To         string
	Rate       float64
	RateScale  float64
	Source     *string
	TTL        int
}

// DefaultAssetRateParams returns default parameters for creating a test asset rate.
func DefaultAssetRateParams() AssetRateParams {
	source := "Test Source"

	return AssetRateParams{
		From:      "USD",
		To:        "BRL",
		Rate:      5.25,
		RateScale: 2.0,
		Source:    &source,
		TTL:       3600,
	}
}

// CreateTestAssetRate inserts an asset rate directly into DB for test setup.
func CreateTestAssetRate(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params AssetRateParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	// external_id is NOT NULL in schema, so generate one if not provided
	externalID := libCommons.GenerateUUIDv7()
	if params.ExternalID != nil {
		externalID = *params.ExternalID
	}

	_, err := db.Exec(`
		INSERT INTO asset_rate (id, organization_id, ledger_id, external_id, "from", "to", rate, rate_scale, source, ttl, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, id, orgID, ledgerID, externalID, params.From, params.To, params.Rate, params.RateScale, params.Source, params.TTL, now, now)
	require.NoError(t, err, "failed to create test asset rate")

	return id
}

// CreateTestAssetRateSimple is a convenience wrapper that creates an asset rate with minimal params.
// Note: rate is stored as BIGINT in DB, so values like 1.17 get truncated to 1.
// For proper testing, use integer rates or account for truncation behavior.
func CreateTestAssetRateSimple(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, from, to string, rate float64) uuid.UUID {
	t.Helper()

	params := DefaultAssetRateParams()
	params.From = from
	params.To = to
	params.Rate = rate

	return CreateTestAssetRate(t, db, orgID, ledgerID, params)
}

// GetAssetRateByID retrieves an asset rate by ID for verification.
func GetAssetRateByID(t *testing.T, db *sql.DB, id uuid.UUID) (from, to string, rate float64) {
	t.Helper()

	err := db.QueryRow(`
		SELECT "from", "to", rate FROM asset_rate WHERE id = $1
	`, id).Scan(&from, &to, &rate)
	require.NoError(t, err, "failed to get asset rate by ID")

	return from, to, rate
}

// OperationRouteParams holds parameters for creating a test operation route.
type OperationRouteParams struct {
	Title              string
	Description        string
	Code               *string
	OperationType      string // "source" or "destination"
	AccountRuleType    *string
	AccountRuleValidIf *string
	DeletedAt          *time.Time
}

// DefaultOperationRouteParams returns default parameters for creating a test operation route.
func DefaultOperationRouteParams() OperationRouteParams {
	return OperationRouteParams{
		Title:         "Test Operation Route",
		Description:   "Test description for operation route",
		OperationType: "source",
	}
}

// CreateTestOperationRoute inserts an operation route directly into DB for test setup.
func CreateTestOperationRoute(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params OperationRouteParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO operation_route (id, organization_id, ledger_id, title, description, code, operation_type, account_rule_type, account_rule_valid_if, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, id, orgID, ledgerID, params.Title, params.Description, params.Code, params.OperationType,
		params.AccountRuleType, params.AccountRuleValidIf, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test operation route")

	return id
}

// CreateTestOperationRouteSimple is a convenience wrapper that creates an operation route with minimal params.
// Note: account_rule_type and account_rule_valid_if are set to empty strings instead of NULL
// because the repository model uses string (not sql.NullString) for these fields.
func CreateTestOperationRouteSimple(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, title, operationType string) uuid.UUID {
	t.Helper()

	// Use empty strings for nullable string columns that the repository scans as plain strings
	emptyString := ""
	params := DefaultOperationRouteParams()
	params.Title = title
	params.OperationType = operationType
	params.AccountRuleType = &emptyString
	params.AccountRuleValidIf = &emptyString

	return CreateTestOperationRoute(t, db, orgID, ledgerID, params)
}

// TransactionRouteParams holds parameters for creating a test transaction route.
type TransactionRouteParams struct {
	Title       string
	Description string
	DeletedAt   *time.Time
}

// DefaultTransactionRouteParams returns default parameters for creating a test transaction route.
func DefaultTransactionRouteParams() TransactionRouteParams {
	return TransactionRouteParams{
		Title:       "Test Transaction Route",
		Description: "Test description for transaction route",
	}
}

// CreateTestTransactionRoute inserts a transaction route directly into DB for test setup.
func CreateTestTransactionRoute(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, params TransactionRouteParams) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO transaction_route (id, organization_id, ledger_id, title, description, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, orgID, ledgerID, params.Title, params.Description, now, now, params.DeletedAt)
	require.NoError(t, err, "failed to create test transaction route")

	return id
}

// CreateTestTransactionRouteSimple is a convenience wrapper that creates a transaction route with minimal params.
func CreateTestTransactionRouteSimple(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, title string) uuid.UUID {
	t.Helper()

	params := DefaultTransactionRouteParams()
	params.Title = title

	return CreateTestTransactionRoute(t, db, orgID, ledgerID, params)
}

// CreateTestOperationTransactionRouteLink inserts a link between operation route and transaction route.
func CreateTestOperationTransactionRouteLink(t *testing.T, db *sql.DB, operationRouteID, transactionRouteID uuid.UUID) uuid.UUID {
	t.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at, deleted_at)
		VALUES ($1, $2, $3, $4, NULL)
	`, id, operationRouteID, transactionRouteID, now)
	require.NoError(t, err, "failed to create test operation transaction route link")

	return id
}

// SoftDeleteOperationTransactionRouteLink soft-deletes a link by ID.
func SoftDeleteOperationTransactionRouteLink(t *testing.T, db *sql.DB, linkID uuid.UUID) {
	t.Helper()

	_, err := db.Exec(`
		UPDATE operation_transaction_route SET deleted_at = NOW() WHERE id = $1
	`, linkID)
	require.NoError(t, err, "failed to soft-delete operation transaction route link")
}
