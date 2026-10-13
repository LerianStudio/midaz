// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestTenantSchemaMoneyColumnsMigration is the behavioral contract for
// migration 000026_convert_tenant_money_columns_to_decimal.
//
// In multi-tenant schema isolation every tenant owns a schema selected through
// search_path, and 000005 probed only the public schema before converting
// limits.max_amount, usage_counters.current_usage and
// transaction_validations.amount from BIGINT to DECIMAL, so a tenant schema
// kept them BIGINT. 000026 probes current_schema() instead.
//
// Post-conditions enforced:
//  1. After migrating a tenant schema to HEAD the three columns are numeric.
//  2. A reserve and a confirm carrying cents succeed in that schema and the
//     counter holds the exact fractional amount.
//  3. A limit and a transaction validation with fractional amounts persist
//     there.
//  4. BIGINT values stored before 000026 are kept as currency units: the cast
//     never divides by 100.
//  5. On the public schema, already converted by 000005, 000026 is a no-op.
//
// Each sub-test provisions its OWN throwaway Postgres container.
func TestTenantSchemaMoneyColumnsMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	t.Run("tenant_schema_money_columns_are_numeric_at_head", func(t *testing.T) {
		schema, db, mig := newTenantSchemaMigrate(ctx, t)

		require.NoError(t, upToHead(mig), "migrate the tenant schema to HEAD")

		for _, col := range tenantMoneyColumns {
			require.Equal(t, "numeric", schemaColumnType(ctx, t, db, schema, col.table, col.column),
				"%s.%s must be numeric in the tenant schema", col.table, col.column)
		}
	})

	t.Run("reserve_and_confirm_with_cents_succeed_in_tenant_schema", func(t *testing.T) {
		_, db, mig := newTenantSchemaMigrate(ctx, t)

		require.NoError(t, upToHead(mig), "migrate the tenant schema to HEAD")

		createdAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		maxAmount := decimal.RequireFromString("1000")
		amount := decimal.RequireFromString("1.32")
		lmt := createTenantLimit(ctx, t, db, "tenant-cents-limit", 26001, maxAmount, createdAt)

		counterRepo := postgres.NewUsageCounterRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
		reservationRepo := postgres.NewUsageReservationRepositoryWithConnection(counterRepo)

		res, err := model.NewReservation(lmt.ID, testutil.MustDeterministicUUID(26002), "scope-cents", "period-cents",
			amount, createdAt.Add(5*time.Minute), createdAt)
		require.NoError(t, err, "build the reservation")

		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err, "begin reserve transaction")

		replay, err := reservationRepo.ReserveWithTx(ctx, tx, res, maxAmount, nil)
		if err != nil {
			_ = tx.Rollback()
		}

		require.NoError(t, err, "reserve a fractional amount")
		require.False(t, replay, "the first reserve must not be a replay")
		require.NoError(t, tx.Commit(), "commit reserve")

		tx, err = db.BeginTx(ctx, nil)
		require.NoError(t, err, "begin confirm transaction")

		_, err = reservationRepo.ConfirmWithTx(ctx, tx, res.ID)
		if err != nil {
			_ = tx.Rollback()
		}

		require.NoError(t, err, "confirm a fractional reservation")
		require.NoError(t, tx.Commit(), "commit confirm")

		var currentUsage, reservedUsage decimal.Decimal

		require.NoError(t, db.QueryRowContext(
			ctx,
			`SELECT current_usage, reserved_usage FROM usage_counters
			 WHERE limit_id = $1 AND scope_key = 'scope-cents' AND period_key = 'period-cents'`, lmt.ID,
		).Scan(&currentUsage, &reservedUsage), "read the settled counter")
		require.True(t, amount.Equal(currentUsage), "current_usage must hold 1.32 exactly; got %s", currentUsage)
		require.True(t, reservedUsage.IsZero(), "reserved_usage must be released by the confirm; got %s", reservedUsage)
	})

	t.Run("fractional_limit_and_validation_amounts_persist_in_tenant_schema", func(t *testing.T) {
		_, db, mig := newTenantSchemaMigrate(ctx, t)

		require.NoError(t, upToHead(mig), "migrate the tenant schema to HEAD")

		maxAmount := decimal.RequireFromString("1000.50")
		createTenantLimit(ctx, t, db, "tenant-fractional-limit", 26005, maxAmount,
			time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))

		const txRequest = "26003000-0000-0000-0000-000000000000"

		_, err := db.ExecContext(ctx,
			`INSERT INTO transaction_validations
				(request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, decision, processing_time_ms)
			 VALUES ($1, 'PIX', 1.32, 'BRL', '2026-01-02T03:04:05Z'::timestamptz,
				 '{}'::jsonb, 'ALLOW', 10)`, txRequest)
		require.NoError(t, err, "a fractional amount must fit transaction_validations.amount")

		var got decimal.Decimal

		require.NoError(t, db.QueryRowContext(
			ctx,
			`SELECT amount FROM transaction_validations WHERE request_id = $1`, txRequest,
		).Scan(&got), "read the validation amount back")
		require.True(t, decimal.RequireFromString("1.32").Equal(got), "amount must round-trip exactly; got %s", got)
	})

	t.Run("existing_bigint_values_are_kept_as_units", func(t *testing.T) {
		schema, db, mig := newTenantSchemaMigrate(ctx, t)

		require.NoError(t, mig.Migrate(tenantMoneyMigrationVersion-1), "migrate the tenant schema to the version before 000026")
		require.Equal(t, "bigint", schemaColumnType(ctx, t, db, schema, "limits", "max_amount"),
			"before 000026 the tenant schema reproduces the BIGINT left by 000005")

		var limitID uuid.UUID

		require.NoError(t, db.QueryRowContext(
			ctx,
			`INSERT INTO limits (name, limit_type, max_amount, asset)
			 VALUES ('tenant-units-limit', 'PER_TRANSACTION', 1000, 'BRL') RETURNING id`,
		).Scan(&limitID), "seed a limit stored in units")

		_, err := db.ExecContext(ctx,
			`INSERT INTO usage_counters (limit_id, scope_key, period_key, current_usage)
			 VALUES ($1, 'scope-units', 'period-units', 250)`, limitID)
		require.NoError(t, err, "seed a counter stored in units")

		const txRequest = "26004000-0000-0000-0000-000000000000"

		_, err = db.ExecContext(ctx,
			`INSERT INTO transaction_validations
				(request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, decision, processing_time_ms)
			 VALUES ($1, 'PIX', 75, 'BRL', '2026-01-02T03:04:05Z'::timestamptz,
				 '{}'::jsonb, 'ALLOW', 10)`, txRequest)
		require.NoError(t, err, "seed a validation stored in units")

		require.NoError(t, upToHead(mig), "apply 000026 over the seeded rows")

		requireDecimalValue(ctx, t, db, "1000", `SELECT max_amount FROM limits WHERE id = $1`, limitID)
		requireDecimalValue(ctx, t, db, "250",
			`SELECT current_usage FROM usage_counters WHERE limit_id = $1`, limitID)
		requireDecimalValue(ctx, t, db, "75",
			`SELECT amount FROM transaction_validations WHERE request_id = $1`, txRequest)

		var usageDefault string

		require.NoError(t, db.QueryRowContext(
			ctx,
			`SELECT column_default FROM information_schema.columns
			 WHERE table_schema = $1 AND table_name = 'usage_counters' AND column_name = 'current_usage'`, schema,
		).Scan(&usageDefault), "read the current_usage default")
		require.Equal(t, "0", usageDefault, "current_usage must keep DEFAULT 0")
	})

	t.Run("public_schema_is_a_no_op", func(t *testing.T) {
		dsn := startUpgradePathContainer(ctx, t)
		mig, db := newHeadReservationMigrate(ctx, t, dsn)

		require.NoError(t, mig.Migrate(tenantMoneyMigrationVersion-1), "migrate public to the version before 000026")

		_, err := db.ExecContext(ctx,
			`INSERT INTO limits (name, limit_type, max_amount, asset)
			 VALUES ('public-cents-limit', 'PER_TRANSACTION', 1000.50, 'BRL')`)
		require.NoError(t, err, "000005 already made limits.max_amount numeric on public")

		require.NoError(t, upToHead(mig), "apply 000026 on public")

		for _, col := range tenantMoneyColumns {
			require.Equal(t, "numeric", schemaColumnType(ctx, t, db, "public", col.table, col.column),
				"%s.%s must stay numeric on public", col.table, col.column)
		}

		requireDecimalValue(ctx, t, db, "1000.50",
			`SELECT max_amount FROM limits WHERE name = 'public-cents-limit'`)
	})
}

// tenantMoneyMigrationVersion is the version this file is the contract for
// (000026_convert_tenant_money_columns_to_decimal).
const tenantMoneyMigrationVersion = 26

var tenantMoneyColumns = []struct {
	table  string
	column string
}{
	{table: "limits", column: "max_amount"},
	{table: "usage_counters", column: "current_usage"},
	{table: "transaction_validations", column: "amount"},
}

// newTenantSchemaMigrate starts a throwaway Postgres, migrates its public
// schema to HEAD, creates a tenant schema named tracer_<random>, and returns a
// *sql.DB plus a golang-migrate instance whose every connection carries
// options=-csearch_path=<schema>, which is how the tenant manager points a
// schema-isolated tenant at its own schema. The migrate driver leaves
// SchemaName empty so it resolves current_schema(), the same as a runner that
// receives only the tenant DSN.
//
// The public schema is migrated first because a shared database holds it
// already migrated: the guards of 000005 and 000010 read public, so 000005
// finds its columns converted there and skips the tenant's, and 000010
// resolves public.limits.
func newTenantSchemaMigrate(ctx context.Context, t *testing.T) (string, *sql.DB, *migrate.Migrate) {
	t.Helper()

	dsn := startUpgradePathContainer(ctx, t)

	publicMig, _ := newHeadReservationMigrate(ctx, t, dsn)
	require.NoError(t, upToHead(publicMig), "migrate the public schema to HEAD")

	suffix := make([]byte, 6)
	_, err := rand.Read(suffix)
	require.NoError(t, err, "generate tenant schema suffix")

	schema := "tracer_" + hex.EncodeToString(suffix)

	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open admin connection")

	_, err = admin.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err, "create tenant schema")
	require.NoError(t, admin.Close(), "close admin connection")

	tenantURL, err := url.Parse(dsn)
	require.NoError(t, err, "parse container DSN")

	query := tenantURL.Query()
	query.Set("options", "-csearch_path="+schema)
	tenantURL.RawQuery = query.Encode()

	db, err := sql.Open("pgx", tenantURL.String())
	require.NoError(t, err, "open tenant-schema connection")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close tenant-schema db: %v", closeErr)
		}
	})

	var current string

	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&current), "read current_schema")
	require.Equal(t, schema, current, "the tenant connection must resolve to the tenant schema")

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{DatabaseName: "tracer_test"})
	require.NoError(t, err, "build migrate postgres driver for the tenant schema")

	mig, err := migrate.NewWithDatabaseInstance("file://"+resolveHeadMigrationsDir(ctx, t), "tracer_test", driver)
	require.NoError(t, err, "build migrate instance for the tenant schema")

	t.Cleanup(func() {
		if srcErr, _ := mig.Close(); srcErr != nil {
			t.Logf("close tenant-schema migrate source: %v", srcErr)
		}
	})

	return schema, db, mig
}

// createTenantLimit persists a PER_TRANSACTION limit through the limit
// repository and asserts its max_amount reads back exactly.
func createTenantLimit(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	name string,
	accountSeed int64,
	maxAmount decimal.Decimal,
	createdAt time.Time,
) *model.Limit {
	t.Helper()

	accountID := testutil.MustDeterministicUUID(accountSeed)

	lmt, err := model.NewLimit(name, model.LimitTypePerTransaction, maxAmount, "BRL",
		[]model.Scope{{AccountID: &accountID}}, nil, createdAt)
	require.NoError(t, err, "build the limit")

	limitRepo := postgres.NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	require.NoError(t, limitRepo.CreateWithTx(ctx, db, lmt), "persist limit %s", name)

	got, err := limitRepo.GetByID(ctx, lmt.ID)
	require.NoError(t, err, "read limit %s back", name)
	require.True(t, maxAmount.Equal(got.MaxAmount), "max_amount must round-trip exactly; got %s", got.MaxAmount)

	return lmt
}

// upToHead applies every pending migration, treating migrate.ErrNoChange as
// success.
func upToHead(mig *migrate.Migrate) error {
	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// schemaColumnType returns the information_schema data_type of
// schema.table.column.
func schemaColumnType(ctx context.Context, t *testing.T, db *sql.DB, schema, table, column string) string {
	t.Helper()

	var dataType string

	require.NoError(t, db.QueryRowContext(
		ctx,
		`SELECT data_type FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 AND column_name = $3`,
		schema, table, column,
	).Scan(&dataType), "lookup %s.%s.%s data_type", schema, table, column)

	return dataType
}

// requireDecimalValue asserts that the single value selected by query equals
// want exactly.
func requireDecimalValue(ctx context.Context, t *testing.T, db *sql.DB, want, query string, args ...any) {
	t.Helper()

	var got decimal.Decimal

	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&got), "read %q", query)
	require.True(t, decimal.RequireFromString(want).Equal(got), "want %s, got %s for %q", want, got, query)
}
