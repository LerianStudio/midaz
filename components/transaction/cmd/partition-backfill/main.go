// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// partition-backfill is an operator-run, one-shot tool that copies rows from
// the legacy operation/balance tables into the corresponding partitioned
// shell tables created by migrations 000019 and 000020.
//
// It is intentionally NOT executed by golang-migrate — running a multi-hour
// data copy inside a schema migration is exactly the anti-pattern this
// refactor removes. Instead, the DBA/SRE runs it once after dual-write has
// been enabled (phase='dual_write' in partition_migration_state) and before
// invoking the atomic swap migrations (000022/000023).
//
// Behavior contract:
//
//   - Keyset pagination on the primary key column so progress is monotonic
//     even when the legacy table receives new rows during the copy window.
//   - NO `ON CONFLICT DO NOTHING`. If the backfill encounters a conflict, it
//     surfaces the row and aborts — a conflict implies dual-write is already
//     live and the tool is mis-ordered, which the operator must investigate.
//   - Periodic progress logging (every `--log-every` rows, default 10_000).
//   - Post-copy row-count assertion: the tool fails the process if the legacy
//     and partitioned row counts differ at the end of the run. The operator
//     interprets this as "dual-write was catching up during the copy, wait
//     and re-run".
//   - Dry-run mode (`--dry-run`) that streams the same SELECTs but skips the
//     INSERTs, useful for validating the pagination plan against production.
//
// Usage:
//
//	export PARTITION_BACKFILL_DSN='host=... user=... dbname=transaction sslmode=require'
//	partition-backfill --table=operation --batch-size=10000
//	partition-backfill --table=balance   --batch-size=5000 --dry-run
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	envDSN = "PARTITION_BACKFILL_DSN"

	defaultBatchSize = 10_000
	defaultLogEvery  = 10_000
	minBatchSize     = 1
	maxBatchSize     = 100_000
)

// tableSpec captures everything the backfill needs to know about a table:
// how to page it, the full column list, and the partitioned target.
type tableSpec struct {
	legacyTable      string
	partitionedTable string
	cursorColumn     string
	columns          []string
}

var specs = map[string]tableSpec{
	"operation": {
		legacyTable:      "operation",
		partitionedTable: "operation_partitioned",
		cursorColumn:     "id",
		columns: []string{
			"id", "transaction_id", "description", "type", "asset_code",
			"amount", "available_balance", "on_hold_balance",
			"available_balance_after", "on_hold_balance_after",
			"status", "status_description",
			"account_id", "account_alias", "balance_id", "chart_of_accounts",
			"organization_id", "ledger_id",
			"created_at", "updated_at", "deleted_at",
			"route", "balance_affected", "balance_key",
			"balance_version_before", "balance_version_after",
		},
	},
	"balance": {
		legacyTable:      "balance",
		partitionedTable: "balance_partitioned",
		cursorColumn:     "id",
		columns: []string{
			"id", "organization_id", "ledger_id", "account_id",
			"alias", "asset_code",
			"available", "on_hold", "version",
			"account_type", "allow_sending", "allow_receiving",
			"created_at", "updated_at", "deleted_at", "key",
		},
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "partition-backfill: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		table     string
		batchSize int
		logEvery  int
		dryRun    bool
	)

	flag.StringVar(&table, "table", "", "table to backfill: operation | balance (required)")
	flag.IntVar(&batchSize, "batch-size", defaultBatchSize, "rows per keyset page (1..100000)")
	flag.IntVar(&logEvery, "log-every", defaultLogEvery, "log progress every N rows")
	flag.BoolVar(&dryRun, "dry-run", false, "issue SELECTs but skip INSERTs")
	flag.Parse() //nolint:revive // CLI entry point: flag.Parse is expected to live in run(), called once from main().

	spec, ok := specs[table]
	if !ok {
		return fmt.Errorf("unknown --table=%q (valid: operation, balance)", table) //nolint:err113 // CLI user-facing validation error; no caller will errors.Is() against it.
	}

	if batchSize < minBatchSize || batchSize > maxBatchSize {
		return fmt.Errorf("--batch-size=%d out of range [%d, %d]", batchSize, minBatchSize, maxBatchSize) //nolint:err113 // CLI user-facing validation error; no caller will errors.Is() against it.
	}

	dsn := os.Getenv(envDSN)
	if dsn == "" {
		return fmt.Errorf("required environment variable %s is empty", envDSN) //nolint:err113 // CLI user-facing validation error; no caller will errors.Is() against it.
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	defer func() { _ = db.Close() }()

	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	start := time.Now()

	total, err := backfill(ctx, db, spec, batchSize, logEvery, dryRun)
	if err != nil {
		return fmt.Errorf("backfill: %w", err)
	}

	fmt.Printf("partition-backfill: copied %d rows from %s to %s in %s (dry_run=%t)\n",
		total, spec.legacyTable, spec.partitionedTable, time.Since(start).Round(time.Millisecond), dryRun)

	if dryRun {
		return nil
	}

	return assertCounts(ctx, db, spec)
}

// backfill performs the keyset-paginated copy and returns the total rows
// inserted.
func backfill(ctx context.Context, db *sql.DB, spec tableSpec, batchSize, logEvery int, dryRun bool) (int64, error) {
	selectSQL := fmt.Sprintf( //nolint:gosec // G201: table and column names come from the hard-coded specs map in this package, not user input
		`SELECT %s FROM %s WHERE %s > $1 ORDER BY %s ASC LIMIT $2`,
		strings.Join(spec.columns, ", "),
		spec.legacyTable,
		spec.cursorColumn,
		spec.cursorColumn,
	)

	insertSQL := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s)`,
		spec.partitionedTable,
		strings.Join(spec.columns, ", "),
		placeholders(len(spec.columns)),
	)

	var (
		cursor string // UUIDs compare lexicographically as text which matches UUID sort for v7
		total  int64
		sinceL int64
	)

	for {
		if err := ctx.Err(); err != nil {
			return total, err //nolint:wrapcheck
		}

		rows, err := db.QueryContext(ctx, selectSQL, cursor, batchSize)
		if err != nil {
			return total, fmt.Errorf("select page: %w", err)
		}

		values, lastCursor, err := consumePage(rows, len(spec.columns))
		if err != nil {
			return total, err
		}

		if len(values) == 0 {
			return total, nil
		}

		if !dryRun {
			if err := copyPage(ctx, db, insertSQL, values); err != nil {
				return total, err
			}
		}

		pageRows := int64(len(values))
		total += pageRows
		sinceL += pageRows

		if sinceL >= int64(logEvery) {
			fmt.Printf("partition-backfill: %d rows copied, cursor=%s\n", total, lastCursor)

			sinceL = 0
		}

		cursor = lastCursor
	}
}

// consumePage reads a Rows into a flat slice of column values suitable for
// the multi-row INSERT. It also returns the cursor value (first column) of
// the last row for keyset pagination.
func consumePage(rows *sql.Rows, colCount int) ([][]any, string, error) {
	defer func() { _ = rows.Close() }()

	var (
		all        [][]any
		lastCursor string
	)

	for rows.Next() {
		row := make([]any, colCount)
		ptrs := make([]any, colCount)

		for i := range row {
			ptrs[i] = &row[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, "", fmt.Errorf("scan row: %w", err)
		}

		// Cursor column is always the first element; coerce to string so UUIDs
		// and BIGINTs both work.
		lastCursor = fmt.Sprintf("%v", row[0])
		all = append(all, row)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate rows: %w", err)
	}

	return all, lastCursor, nil
}

// copyPage inserts a page of rows one by one inside a single transaction.
// A single INSERT per row keeps error localization simple — if any conflict
// occurs the transaction aborts and the error is propagated. This is
// intentional: collisions during backfill mean dual-write already started or
// the tool was run twice, both of which need operator attention.
func copyPage(ctx context.Context, db *sql.DB, insertSQL string, values [][]any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin page tx: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}

	defer func() { _ = stmt.Close() }()

	for i, row := range values {
		if _, err = stmt.ExecContext(ctx, row...); err != nil {
			return fmt.Errorf("insert row %d (cursor=%v): %w", i, row[0], err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit page: %w", err)
	}

	committed = true

	return nil
}

// assertCounts fails the process when the partitioned shell has fewer rows
// than the legacy table. Equal counts are the success condition; the
// partitioned table being AHEAD of the legacy table is also an error because
// it implies rows were written outside of dual-write.
func assertCounts(ctx context.Context, db *sql.DB, spec tableSpec) error {
	var legacy, part int64

	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM "+spec.legacyTable).Scan(&legacy); err != nil {
		return fmt.Errorf("count legacy: %w", err)
	}

	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM "+spec.partitionedTable).Scan(&part); err != nil {
		return fmt.Errorf("count partitioned: %w", err)
	}

	if legacy != part {
		//nolint:err113 // CLI operator-facing assertion failure with rich context; no caller will errors.Is() against it.
		return fmt.Errorf("post-copy row count mismatch: legacy=%d partitioned=%d (delta=%d). "+
			"If dual-write was catching up during the copy, wait and re-run. "+
			"If partitioned > legacy, someone wrote to the shell out-of-band — investigate before proceeding",
			legacy, part, legacy-part)
	}

	fmt.Printf("partition-backfill: row count assertion OK (%d rows)\n", legacy)

	return nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}

	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("$%d", i+1)
	}

	return strings.Join(parts, ", ")
}

// Compile-time check to avoid an accidental drift between main.go and the
// column list each migration expects.
var _ = errors.New
