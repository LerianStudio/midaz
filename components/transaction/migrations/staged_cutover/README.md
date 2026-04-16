# Staged Cutover Migrations (Operator-Run Only)

These migrations are the post-dual-write stages of the partition cutover
(see `components/transaction/migrations/00002[1-2]_partition_migration_state`
and `partitionstate` package). They are intentionally segregated from the
standard `migrations/` directory because:

1. **golang-migrate's multi-statement splitter is naive** (splits on `;`
   with no dollar-quote awareness). The atomic swap migrations use
   `DO $$ ... $$` blocks with `LOCK TABLE ... IN ACCESS EXCLUSIVE MODE` and
   cross-statement state that cannot be executed through the bootstrap
   migration driver. Running these files via the driver would either
   split the DO block on internal `;` (syntax error) or run `LOCK TABLE`
   outside a transaction block (PG error 25P01).

2. **These stages must be coordinated with dual-write uptime, backfill
   completion, and a rolling restart.** Running them automatically on
   service startup would race the running pods mid-request.

## Operator runbook

Each file is designed to run as a single `psql -f` or `db.Exec(entire file)`
invocation (which wraps in a single implicit transaction for multi-statement
input). Sequence:

1. `000022_atomic_swap_operation.up.sql`
   Renames `operation` → `operation_legacy`, `operation_partitioned` →
   `operation`. Asserts row-count parity inside a DO block; RAISEs
   EXCEPTION on mismatch so the whole migration rolls back.

2. `000023_atomic_swap_balance.up.sql`
   Same as 000022 but for `balance`.

3. `000024_drop_legacy_tables.up.sql`
   Explicit cleanup. Irreversible — the down migration RAISES to prevent
   a silent rollback.

4. `000025_restore_operation_fk.up.sql`
   Restores the `fk_operation_transaction` foreign key on PG 15+. Uses
   NOT VALID attach so the swap is fast; operators run `VALIDATE
   CONSTRAINT` separately at a convenient time.

## Testing

Integration coverage lives in `tests/migration_cycle/partition_cutover_test.go`
which applies these files via `db.ExecContext` (bypassing the multi-statement
splitter) and asserts rollback-on-mismatch + successful swap behaviour.
