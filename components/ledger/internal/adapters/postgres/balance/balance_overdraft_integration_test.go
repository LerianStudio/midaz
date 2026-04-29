//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Overdraft Column Integration Tests
// ============================================================================

func TestIntegration_BalanceOverdraft_DirectionPersisted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key,
			asset_code, available, on_hold, version, account_type,
			allow_sending, allow_receiving, created_at, updated_at, direction)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		balanceID, orgID, ledgerID, accountID, "@dir-test", "default",
		"USD", 1000, 0, 1, "deposit",
		true, true, now, now, "debit",
	)
	require.NoError(t, err, "inserting balance with direction=debit")

	// Act — read back via raw SQL to verify column value
	var direction string

	err = container.DB.QueryRow(
		`SELECT direction FROM balance WHERE id = $1`, balanceID,
	).Scan(&direction)

	// Assert
	require.NoError(t, err, "querying direction")
	assert.Equal(t, "debit", direction, "direction should round-trip as debit")
}

func TestIntegration_BalanceOverdraft_SchemaDefaults(t *testing.T) {
	// Arrange — insert a row WITHOUT specifying overdraft columns
	container := pgtestutil.SetupContainer(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key,
			asset_code, available, on_hold, version, account_type,
			allow_sending, allow_receiving, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		balanceID, orgID, ledgerID, accountID, "@defaults-test", "default",
		"USD", 500, 0, 0, "deposit",
		true, true, now, now,
	)
	require.NoError(t, err, "inserting balance without overdraft columns")

	// Act
	var direction string
	var overdraftUsed decimal.Decimal
	var settingsRaw []byte

	err = container.DB.QueryRow(`
		SELECT direction, overdraft_used, settings
		FROM balance WHERE id = $1`, balanceID,
	).Scan(&direction, &overdraftUsed, &settingsRaw)

	// Assert
	require.NoError(t, err, "querying default overdraft columns")
	assert.Equal(t, "credit", direction, "default direction should be credit")
	assert.True(t, overdraftUsed.IsZero(), "default overdraft_used should be 0")
	assert.Nil(t, settingsRaw, "default settings should be NULL")
}

func TestIntegration_BalanceOverdraft_SettingsJSONBRoundTrip(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	limit := "500.00"
	inputSettings := &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          "transactional",
	}

	settingsJSON, err := json.Marshal(inputSettings)
	require.NoError(t, err, "marshalling settings to JSON")

	_, err = container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key,
			asset_code, available, on_hold, version, account_type,
			allow_sending, allow_receiving, created_at, updated_at,
			direction, overdraft_used, settings)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		balanceID, orgID, ledgerID, accountID, "@settings-test", "default",
		"USD", 1000, 0, 1, "deposit",
		true, true, now, now,
		"credit", 0, settingsJSON,
	)
	require.NoError(t, err, "inserting balance with JSONB settings")

	// Act — read settings back
	var raw []byte

	err = container.DB.QueryRow(
		`SELECT settings FROM balance WHERE id = $1`, balanceID,
	).Scan(&raw)
	require.NoError(t, err, "querying settings JSONB")

	var got mmodel.BalanceSettings
	err = json.Unmarshal(raw, &got)

	// Assert
	require.NoError(t, err, "unmarshalling settings JSONB")
	assert.True(t, got.AllowOverdraft, "allowOverdraft should be true")
	assert.True(t, got.OverdraftLimitEnabled, "overdraftLimitEnabled should be true")
	require.NotNil(t, got.OverdraftLimit, "overdraftLimit should not be nil")
	assert.Equal(t, "500.00", *got.OverdraftLimit, "overdraftLimit should round-trip")
	assert.Equal(t, "transactional", got.BalanceScope, "balanceScope should round-trip")
}

func TestIntegration_BalanceOverdraft_UpdateOverdraftUsedViaSQLBatch(t *testing.T) {
	// Arrange — simulates the UpdateMany path (Redis→PG sync) at the SQL level.
	// When the repository is updated to include overdraft_used in UpdateMany,
	// this test verifies the column can be modified via a batch UPDATE.
	container := pgtestutil.SetupContainer(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key,
			asset_code, available, on_hold, version, account_type,
			allow_sending, allow_receiving, created_at, updated_at,
			direction, overdraft_used)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		balanceID, orgID, ledgerID, accountID, "@update-od", "default",
		"USD", 800, 0, 1, "deposit",
		true, true, now, now,
		"credit", 0,
	)
	require.NoError(t, err, "inserting initial balance")

	// Act — update overdraft_used via batch-style UPDATE … FROM (VALUES …)
	_, err = container.DB.Exec(`
		UPDATE balance AS b
		SET overdraft_used = v.overdraft_used,
		    version = v.version,
		    updated_at = $4
		FROM (VALUES ($1::uuid, $2::numeric, $3::bigint)) AS v(id, overdraft_used, version)
		WHERE b.id = v.id
		  AND b.version < v.version
		  AND b.deleted_at IS NULL`,
		balanceID, decimal.NewFromInt(150), int64(2), time.Now(),
	)
	require.NoError(t, err, "updating overdraft_used via batch SQL")

	// Assert
	var overdraftUsed decimal.Decimal

	err = container.DB.QueryRow(
		`SELECT overdraft_used FROM balance WHERE id = $1`, balanceID,
	).Scan(&overdraftUsed)

	require.NoError(t, err, "querying updated overdraft_used")
	assert.True(t, overdraftUsed.Equal(decimal.NewFromInt(150)),
		"overdraft_used should be 150 after update, got %s", overdraftUsed)
}

func TestIntegration_BalanceOverdraft_NilSettingsDoesNotCauseParseError(t *testing.T) {
	// Arrange — insert with NULL settings (legacy/no-overdraft balance)
	container := pgtestutil.SetupContainer(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key,
			asset_code, available, on_hold, version, account_type,
			allow_sending, allow_receiving, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		balanceID, orgID, ledgerID, accountID, "@nil-settings", "default",
		"BRL", 200, 0, 0, "deposit",
		true, true, now, now,
	)
	require.NoError(t, err, "inserting balance without settings")

	// Act — scan settings into a nullable byte slice
	var raw sql.NullString

	err = container.DB.QueryRow(
		`SELECT settings FROM balance WHERE id = $1`, balanceID,
	).Scan(&raw)

	// Assert
	require.NoError(t, err, "querying NULL settings should not error")
	assert.False(t, raw.Valid, "settings should be SQL NULL for legacy balance")
}

func TestIntegration_BalanceOverdraft_ListByAliasesReturnsOverdraftColumns(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)
	alias := "@list-od-test"

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key,
			asset_code, available, on_hold, version, account_type,
			allow_sending, allow_receiving, created_at, updated_at,
			direction, overdraft_used)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		balanceID, orgID, ledgerID, accountID, alias, "default",
		"EUR", 300, 10, 1, "checking",
		true, true, now, now,
		"debit", 75,
	)
	require.NoError(t, err, "inserting balance with overdraft columns for list test")

	// Act — read raw columns back via the alias to verify the data is present
	var direction string
	var overdraftUsed decimal.Decimal

	err = container.DB.QueryRow(`
		SELECT direction, overdraft_used
		FROM balance
		WHERE organization_id = $1
		  AND ledger_id = $2
		  AND alias = $3
		  AND deleted_at IS NULL`,
		orgID, ledgerID, alias,
	).Scan(&direction, &overdraftUsed)

	// Assert
	require.NoError(t, err, "querying overdraft columns by alias")
	assert.Equal(t, "debit", direction, "direction should be debit")
	assert.True(t, overdraftUsed.Equal(decimal.NewFromInt(75)),
		"overdraft_used should be 75, got %s", overdraftUsed)
}
