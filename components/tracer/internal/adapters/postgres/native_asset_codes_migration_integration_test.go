//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
)

const (
	checkViolation = "23514"
	stringTooLong  = "22001"
)

// nativeAssetCodesMigrations lists the asset code migrations in apply order.
var nativeAssetCodesMigrations = []string{
	"000032_native_asset_codes",
	"000033_widen_validation_asset_codes",
	"000034_validate_validation_asset_codes",
}

type nativeAssetMigration struct {
	up, down string
}

// nativeAssetCodesDatabase returns a fresh tenant database migrated up to, but
// not including, the first asset code migration, plus the asset code
// migrations' files keyed by name.
func nativeAssetCodesDatabase(t *testing.T) (*sql.DB, map[string]nativeAssetMigration) {
	t.Helper()

	admin := testutil.SetupIntegrationDB(t)
	digest := sha256.Sum256([]byte(t.Name()))
	name := fmt.Sprintf("native_asset_codes_%x", digest[:8])

	_, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		require.NoError(t, err)
	})

	dsn, err := url.Parse(testutil.GetTestDSN())
	require.NoError(t, err)
	dsn.Path = "/" + name
	db, err := sql.Open("pgx", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	require.NoError(t, err)
	sort.Strings(files)

	for _, file := range files {
		if strings.HasPrefix(filepath.Base(file), nativeAssetCodesMigrations[0]) {
			break
		}

		body, err := os.ReadFile(file)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), string(body))
		require.NoError(t, err, filepath.Base(file))
	}

	set := make(map[string]nativeAssetMigration, len(nativeAssetCodesMigrations))
	for _, migration := range nativeAssetCodesMigrations {
		upBody, err := os.ReadFile(filepath.Join(dir, migration+".up.sql"))
		require.NoError(t, err)
		downBody, err := os.ReadFile(filepath.Join(dir, migration+".down.sql"))
		require.NoError(t, err)
		set[migration] = nativeAssetMigration{up: string(upBody), down: string(downBody)}
	}

	return db, set
}

func applyNativeAssetUps(t *testing.T, db *sql.DB, set map[string]nativeAssetMigration, names ...string) {
	t.Helper()

	for _, name := range names {
		_, err := db.ExecContext(t.Context(), set[name].up)
		require.NoError(t, err, name)
	}
}

func requireCheckViolation(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	requirePgCode(t, checkViolation, err, msgAndArgs...)
}

func requirePgCode(t *testing.T, code string, err error, msgAndArgs ...any) {
	t.Helper()

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a PostgreSQL error, got %v", err)
	require.Equal(t, code, pgErr.Code, msgAndArgs...)
}

func insertNativeAssetLimit(t *testing.T, db *sql.DB, id uuid.UUID, asset string) error {
	t.Helper()

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO limits (id, name, limit_type, max_amount, asset, scopes, status)
		 VALUES ($1, $2, 'DAILY', 100, $3, '[]', 'ACTIVE')`,
		id, "native-"+id.String(), asset)

	return err
}

func insertNativeAssetValidation(t *testing.T, db *sql.DB, id uuid.UUID, asset string) error {
	t.Helper()

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO transaction_validations
		 (id, request_id, transaction_type, amount, asset, transaction_timestamp, account, decision, processing_time_ms)
		 VALUES ($1, $1, 'CARD', 10, $2, $3, '{}', 'ALLOW', 1)`,
		id, asset, testutil.FixedTime())

	return err
}

func nativeAssetColumnType(t *testing.T, db *sql.DB, table string) string {
	t.Helper()

	var columnType string
	require.NoError(t, db.QueryRowContext(t.Context(),
		`SELECT format_type(a.atttypid, a.atttypmod)
		 FROM pg_attribute a
		 WHERE a.attrelid = $1::regclass AND a.attname = 'asset'`, table).Scan(&columnType))

	return columnType
}

func nativeAssetConstraintValidated(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var validated bool
	require.NoError(t, db.QueryRowContext(t.Context(),
		`SELECT convalidated FROM pg_constraint WHERE conname = $1`, name).Scan(&validated))

	return validated
}

func TestIntegrationNativeAssetCodesMigrationRoundTrip(t *testing.T) {
	db, set := nativeAssetCodesDatabase(t)

	legacyLimit := testutil.MustDeterministicUUID(93001)
	require.NoError(t, insertNativeAssetLimit(t, db, legacyLimit, "USD"))
	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93006), "EUR"))

	applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations...)
	require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "limits"))
	require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "transaction_validations"))
	require.True(t, nativeAssetConstraintValidated(t, db, "transaction_validations_asset_code_format"))

	nativeLimit := testutil.MustDeterministicUUID(93002)
	require.NoError(t, insertNativeAssetLimit(t, db, nativeLimit, "LERIANPOINTS"))
	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93003), "BTC"))

	var stored string
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT asset FROM limits WHERE id = $1`, legacyLimit).Scan(&stored))
	require.Equal(t, "USD", stored, "existing codes are preserved in place")

	for i, code := range []string{"btc", "BT1", "US D", "US_D", "US@", ""} {
		requireCheckViolation(t, insertNativeAssetLimit(t, db, testutil.MustDeterministicUUID(93010+int64(i)), code), code)
		requireCheckViolation(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93040+int64(i)), code), code)
	}

	// The column bound rejects a code over 100 characters before the CHECK runs.
	tooLong := strings.Repeat("A", 101)
	requirePgCode(t, stringTooLong, insertNativeAssetLimit(t, db, testutil.MustDeterministicUUID(93016), tooLong))
	requirePgCode(t, stringTooLong, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93046), tooLong))

	// Non-ASCII uppercase letters are valid ledger codes, and the column bound
	// counts characters, so 100 two-byte letters fit. Validation rows cannot be
	// deleted, so only the three-character code is stored there.
	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93030), "ÉUR"))

	for i, code := range []string{"ÉUR", strings.Repeat("É", 100)} {
		limitID := testutil.MustDeterministicUUID(93020 + int64(i))
		require.NoError(t, insertNativeAssetLimit(t, db, limitID, code), code)
		_, err := db.ExecContext(t.Context(), `DELETE FROM limits WHERE id = $1`, limitID)
		require.NoError(t, err)
	}

	_, err := db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[2]].down)
	require.NoError(t, err, "undoing VALIDATE CONSTRAINT is a no-op")
	_, err = db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[1]].down)
	require.NoError(t, err, "every stored validation code has three characters")
	require.Equal(t, "character(3)", nativeAssetColumnType(t, db, "transaction_validations"))

	_, err = db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[0]].down)
	requireCheckViolation(t, err, "down must refuse while a code longer than three characters is stored")
	require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "limits"))
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT asset FROM limits WHERE id = $1`, nativeLimit).Scan(&stored))
	require.Equal(t, "LERIANPOINTS", stored, "a refused down never truncates")

	_, err = db.ExecContext(t.Context(), `DELETE FROM limits WHERE id = $1`, nativeLimit)
	require.NoError(t, err)

	_, err = db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[0]].down)
	require.NoError(t, err)
	require.Equal(t, "character varying(3)", nativeAssetColumnType(t, db, "limits"))
	require.NoError(t, insertNativeAssetLimit(t, db, testutil.MustDeterministicUUID(93004), "btc"),
		"down removes the limits code format constraint")
	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93007), "btc"),
		"down removes the validation code format constraint")
}

func TestIntegrationNativeAssetCodesMigrationDownRefusesLongValidationAsset(t *testing.T) {
	db, set := nativeAssetCodesDatabase(t)

	applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations...)
	// Validation rows are immutable audit records, so a stored long code blocks
	// the rollback permanently.
	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93005), "LERIANPOINTS"))

	_, err := db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[2]].down)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[1]].down)
	requireCheckViolation(t, err)
	require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "transaction_validations"))
}

func TestIntegrationNativeAssetCodesMigrationDownRefusesNonISOLimitCode(t *testing.T) {
	db, set := nativeAssetCodesDatabase(t)

	applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations[0])
	require.NoError(t, insertNativeAssetLimit(t, db, testutil.MustDeterministicUUID(93070), "USD"))
	nonISO := testutil.MustDeterministicUUID(93071)
	require.NoError(t, insertNativeAssetLimit(t, db, nonISO, "BTC"))

	// The previous schema's application accepted only ISO 4217 codes, so a
	// three-character non-ISO code would be unreadable after the rollback.
	_, err := db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[0]].down)
	requireCheckViolation(t, err, "a three-character non-ISO code blocks the rollback")
	require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "limits"))

	_, err = db.ExecContext(t.Context(), `DELETE FROM limits WHERE id = $1`, nonISO)
	require.NoError(t, err)

	_, err = db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[0]].down)
	require.NoError(t, err, "an ISO code alone does not block the rollback")
	require.Equal(t, "character varying(3)", nativeAssetColumnType(t, db, "limits"))
}

func TestIntegrationNativeAssetCodesMigrationDownRefusesShortCodes(t *testing.T) {
	t.Run("limits", func(t *testing.T) {
		db, set := nativeAssetCodesDatabase(t)

		applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations[0])
		limitID := testutil.MustDeterministicUUID(93050)
		require.NoError(t, insertNativeAssetLimit(t, db, limitID, "X"))

		_, err := db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[0]].down)
		requireCheckViolation(t, err, "a code shorter than three characters cannot return to VARCHAR(3) unchanged")
		require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "limits"))

		var stored string
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT asset FROM limits WHERE id = $1`, limitID).Scan(&stored))
		require.Equal(t, "X", stored)
	})

	t.Run("transaction_validations", func(t *testing.T) {
		db, set := nativeAssetCodesDatabase(t)

		applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations...)
		require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93051), "X"))

		_, err := db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[2]].down)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[1]].down)
		requireCheckViolation(t, err, "CHAR(3) would pad a one-character code")
		require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "transaction_validations"))
	})
}

func TestIntegrationNativeAssetCodesMigrationNotValidConstraintChecksNewRows(t *testing.T) {
	db, set := nativeAssetCodesDatabase(t)

	applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations[0], nativeAssetCodesMigrations[1])
	require.Equal(t, "character varying(100)", nativeAssetColumnType(t, db, "transaction_validations"))
	require.False(t, nativeAssetConstraintValidated(t, db, "transaction_validations_asset_code_format"))

	requireCheckViolation(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93060), "btc"),
		"a NOT VALID constraint still checks new rows")
	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93061), "LERIANPOINTS"))

	applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations[2])
	require.True(t, nativeAssetConstraintValidated(t, db, "transaction_validations_asset_code_format"))
}

func TestIntegrationNativeAssetCodesMigrationValidateRefusesStoredNonConformingCode(t *testing.T) {
	db, set := nativeAssetCodesDatabase(t)

	require.NoError(t, insertNativeAssetValidation(t, db, testutil.MustDeterministicUUID(93070), "usd"))

	applyNativeAssetUps(t, db, set, nativeAssetCodesMigrations[0], nativeAssetCodesMigrations[1])

	_, err := db.ExecContext(t.Context(), set[nativeAssetCodesMigrations[2]].up)
	requireCheckViolation(t, err, "VALIDATE CONSTRAINT reports a stored code outside the rule")
	require.False(t, nativeAssetConstraintValidated(t, db, "transaction_validations_asset_code_format"))
}
