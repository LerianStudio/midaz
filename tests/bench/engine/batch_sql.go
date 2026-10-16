//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// Fixed statement texts: arrays carry the rows, so each text prepares once per session whatever the batch size.
const (
	// claimSQL returns only the identities it inserted; it waits on a claim another transaction has in flight.
	claimSQL = `INSERT INTO engine_request (tenant_id, request_id, payload_fingerprint, admission_cutoff)
		SELECT $1, r.id, r.fingerprint, r.cutoff FROM unnest($2::uuid[], $3::bytea[], $4::timestamptz[]) AS r(id, fingerprint, cutoff)
		ORDER BY r.id
		ON CONFLICT (tenant_id, request_id) DO NOTHING
		RETURNING request_id`

	// touchSQL locks stored identities in order and touches them, so this COMMIT writes WAL that replays wait on.
	touchSQL = `UPDATE engine_request AS r SET durability_touch = r.durability_touch + 1
		FROM (SELECT request_id FROM engine_request WHERE tenant_id = $1 AND request_id = ANY($2::uuid[])
			ORDER BY request_id FOR UPDATE) AS l
		WHERE r.tenant_id = $1 AND r.request_id = l.request_id
		RETURNING r.request_id, r.payload_fingerprint, r.outcome, r.reason, r.transaction_id`

	// The columns scanBalance and scanControl read, in their order.
	lockedBalanceColumns = `id, account_id, alias, key, asset_code, account_type, allow_sending, allow_receiving,
		available, on_hold, version, overdraft_used`
	lockedControlColumns = `organization_id, ledger_id, account_id, blocked, limit_remaining, version`

	lockBalancesSQL = `SELECT ` + lockedBalanceColumns + `
		FROM balance WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL ORDER BY id FOR UPDATE`

	lockControlsSQL = `SELECT ` + lockedControlColumns + ` FROM account_control
		WHERE organization_id = $1 AND ledger_id = $2 AND account_id = ANY($3::uuid[])
		ORDER BY organization_id, ledger_id, account_id FOR UPDATE`

	// Columns a v2 create leaves NULL or at their default stay out: body, parent, group, route, scheme, skips.
	insertTransactionsSQL = `INSERT INTO "transaction" (id, description, status, status_description, amount, asset_code,
		chart_of_accounts_group_name, organization_id, ledger_id, created_at, updated_at)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::numeric[], $6::text[], $7::text[],
			$8::uuid[], $9::uuid[], $10::timestamptz[], $11::timestamptz[])`

	// recorded_at is the decision time, as completion records the engine's applied time.
	insertOperationsSQL = `INSERT INTO operation (id, transaction_id, description, type, asset_code, amount, available_balance,
		on_hold_balance, available_balance_after, on_hold_balance_after, status, account_id, account_alias, balance_id,
		chart_of_accounts, organization_id, ledger_id, created_at, updated_at, balance_affected, balance_key,
		balance_version_before, balance_version_after, direction, snapshot, recorded_at)
		SELECT o.*, o.updated_at FROM unnest($1::uuid[], $2::uuid[], $3::text[], $4::text[], $5::text[], $6::numeric[],
			$7::numeric[], $8::numeric[], $9::numeric[], $10::numeric[], $11::text[], $12::uuid[], $13::text[], $14::uuid[],
			$15::text[], $16::uuid[], $17::uuid[], $18::timestamptz[], $19::timestamptz[], $20::bool[], $21::text[],
			$22::bigint[], $23::bigint[], $24::text[], $25::jsonb[])
		AS o(id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance,
			available_balance_after, on_hold_balance_after, status, account_id, account_alias, balance_id, chart_of_accounts,
			organization_id, ledger_id, created_at, updated_at, balance_affected, balance_key, balance_version_before,
			balance_version_after, direction, snapshot)`

	updateBalancesSQL = `UPDATE balance AS b SET available = v.available, version = v.version_after, updated_at = $1
		FROM unnest($2::uuid[], $3::numeric[], $4::bigint[], $5::bigint[]) AS v(id, available, version_before, version_after)
		WHERE b.id = v.id AND b.version = v.version_before`

	updateControlsSQL = `UPDATE account_control AS c SET limit_remaining = v.limit_remaining, version = v.version_after
		FROM unnest($3::uuid[], $4::numeric[], $5::bigint[], $6::bigint[]) AS v(account_id, limit_remaining, version_before, version_after)
		WHERE c.organization_id = $1 AND c.ledger_id = $2 AND c.account_id = v.account_id AND c.version = v.version_before`

	// decided_at is when the pass decided, not when it committed.
	finalizeSQL = `UPDATE engine_request AS r SET outcome = v.outcome, reason = v.reason, transaction_id = v.transaction_id, decided_at = $2
		FROM unnest($3::uuid[], $4::text[], $5::text[], $6::uuid[]) AS v(request_id, outcome, reason, transaction_id)
		WHERE r.tenant_id = $1 AND r.request_id = v.request_id AND r.outcome IS NULL`
)

// resolveIdentities claims the batch's identities, then locks and touches the stored ones. A stored row answers with
// its own fingerprint; a claimed one with its first occurrence's. A stored NULL outcome fails the scan: claims never
// commit unfinalized.
func resolveIdentities(ctx context.Context, tx *sql.Tx, reqs []request, g batchGroup) (map[uuid.UUID]*resolution, error) {
	fingerprints, cutoffs := make([][]byte, len(g.sorted)), make([]time.Time, len(g.sorted))
	for i, id := range g.sorted {
		first := g.first[id]
		fingerprints[i], cutoffs[i] = reqs[first].fingerprint[:], reqs[first].admissionCutoff
	}

	resolved := make(map[uuid.UUID]*resolution, len(g.sorted))

	err := queryRows(ctx, tx, "claim identities", claimSQL, []any{benchTenantID, g.sorted, fingerprints, cutoffs}, func(rows *sql.Rows) error {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}

		resolved[id] = &resolution{claimed: true, fingerprint: reqs[g.first[id]].fingerprint[:]}

		return nil
	})
	if err != nil {
		return nil, err
	}

	stored := slices.DeleteFunc(slices.Clone(g.sorted), func(id uuid.UUID) bool { return resolved[id] != nil })
	if len(stored) == 0 {
		return resolved, nil
	}

	err = queryRows(ctx, tx, "touch stored identities", touchSQL, []any{benchTenantID, stored}, func(rows *sql.Rows) error {
		var (
			id uuid.UUID
			r  resolution
		)

		if err := rows.Scan(&id, &r.fingerprint, &r.outcome, &r.reason, &r.transactionID); err != nil {
			return err
		}

		resolved[id] = &r

		return nil
	})
	if err == nil && len(resolved) != len(g.sorted) {
		err = fmt.Errorf("touch stored identities: %d of %d resolved", len(resolved), len(g.sorted))
	}

	return resolved, err
}

// lockRows locks the posted balances by id, then the controls of the accounts those balances debit, each in key
// order. The balance row's account owns the control. A missing row is a missing fixture, never a decision.
func lockRows(ctx context.Context, tx *sql.Tx, postings []posting) (*lockedRows, error) {
	ids := make([]uuid.UUID, len(postings))
	for i, p := range postings {
		ids[i] = p.BalanceID
	}

	balances := map[uuid.UUID]lockedBalance{}

	err := queryRows(ctx, tx, "lock balances", lockBalancesSQL, []any{sortedDistinct(ids)}, func(rows *sql.Rows) error {
		id, b, err := scanBalance(rows)
		balances[id] = b

		return err
	})
	if err != nil {
		return nil, err
	}

	var accounts []uuid.UUID

	for _, p := range postings {
		b, ok := balances[p.BalanceID]
		if !ok {
			return nil, fmt.Errorf("lock balances: balance %s not seeded", p.BalanceID)
		}

		if p.Amount < 0 {
			accounts = append(accounts, b.accountID)
		}
	}

	accounts = sortedDistinct(accounts)
	controls := make(map[uuid.UUID]lockedControl, len(accounts))

	err = queryRows(ctx, tx, "lock controls", lockControlsSQL, []any{benchOrgID, benchLedgerID, accounts}, func(rows *sql.Rows) error {
		k, c, err := scanControl(rows)
		controls[k.account] = c

		return err
	})
	if err == nil && len(controls) != len(accounts) {
		err = fmt.Errorf("lock controls: %d of %d debited accounts have a control: not seeded", len(controls), len(accounts))
	}

	return &lockedRows{read: balances, decided: maps.Clone(balances), readCtl: controls, decidedCtl: maps.Clone(controls)}, err
}

func scanBalance(rows *sql.Rows) (uuid.UUID, lockedBalance, error) {
	var (
		id uuid.UUID
		b  lockedBalance
	)

	err := rows.Scan(&id, &b.accountID, &b.alias, &b.key, &b.asset, &b.accountType, &b.allowSending, &b.allowReceiving,
		&b.Available, &b.OnHold, &b.Version, &b.OverdraftUsed)

	return id, b, err
}

func scanControl(rows *sql.Rows) (controlKey, lockedControl, error) {
	var (
		k controlKey
		c lockedControl
	)

	err := rows.Scan(&k.org, &k.ledger, &k.account, &c.blocked, &c.limit, &c.version)

	return k, c, err
}

// persist writes a pass's approvals: transactions before their operations, balances and controls at the versions
// they were locked at, then every event and private row once, all in tx. Each statement must write every row it names.
func (b *batcher) persist(ctx context.Context, tx *sql.Tx, w passWrites, rows *lockedRows, at time.Time) error {
	if len(w.txs) == 0 {
		return nil
	}

	balances, touched := rows.balanceChanges(at)
	controls, debited := rows.controlChanges()

	for _, step := range []struct {
		what, query string
		args        []any
		want        int
	}{
		{"insert transactions", insertTransactionsSQL, transactionArgs(w.txs), len(w.txs)},
		{"insert operations", insertOperationsSQL, operationArgs(w.ops), len(w.ops)},
		{"update balances", updateBalancesSQL, balances, touched},
		{"update controls", updateControlsSQL, controls, debited},
	} {
		if err := execExact(ctx, tx, step.what, step.query, step.want, step.args...); err != nil {
			return err
		}
	}

	if err := b.events.EmitBatch(libStreaming.WithOutboxTx(ctx, tx), w.events); err != nil {
		return fmt.Errorf("emit events: %w", err)
	}

	if _, err := b.rows.CreateManyWithTx(ctx, tx, w.private); err != nil {
		return fmt.Errorf("write private rows: %w", err)
	}

	return nil
}

// balanceChanges returns the UPDATE arguments and count of the balances whose version moved past the one read.
func (rows *lockedRows) balanceChanges(at time.Time) ([]any, int) {
	var (
		ids           []uuid.UUID
		available     []decimal.Decimal
		before, after []int64
	)

	for id, d := range rows.decided {
		if r := rows.read[id]; d.Version != r.Version {
			ids, available, before, after = append(ids, id), append(available, d.Available), append(before, r.Version), append(after, d.Version)
		}
	}

	return []any{at, ids, available, before, after}, len(ids)
}

// controlChanges returns the UPDATE arguments and count of the controls whose version moved past the one read.
func (rows *lockedRows) controlChanges() ([]any, int) {
	var (
		accounts      []uuid.UUID
		limits        []decimal.Decimal
		before, after []int64
	)

	for id, d := range rows.decidedCtl {
		if r := rows.readCtl[id]; d.version != r.version {
			accounts, limits, before, after = append(accounts, id), append(limits, d.limit), append(before, r.version), append(after, d.version)
		}
	}

	return []any{benchOrgID, benchLedgerID, accounts, limits, before, after}, len(accounts)
}

// finalize writes the outcome of every identity this pass claimed, approved or not, so no claim commits unfinalized.
func finalize(ctx context.Context, tx *sql.Tx, g batchGroup, resolved map[uuid.UUID]*resolution, at time.Time) error {
	var (
		ids               []uuid.UUID
		outcomes, reasons []string
		transactions      []*uuid.UUID
	)

	for _, id := range g.sorted {
		if r := resolved[id]; r.claimed {
			ids, outcomes = append(ids, id), append(outcomes, string(r.outcome))
			reasons, transactions = append(reasons, r.reason), append(transactions, r.transactionID)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	return execExact(ctx, tx, "finalize claims", finalizeSQL, len(ids), benchTenantID, at, ids, outcomes, reasons, transactions)
}

// transactionArgs maps the transaction records onto insertTransactionsSQL's arrays, in $n order.
func transactionArgs(txs []events.TransactionSource) []any {
	type tx = events.TransactionSource

	return []any{
		column(txs, func(t tx) string { return t.ID }), column(txs, func(t tx) string { return t.Description }),
		column(txs, func(t tx) string { return t.Status.Code }), column(txs, func(t tx) string { return *t.Status.Description }),
		column(txs, func(t tx) decimal.Decimal { return *t.Amount }), column(txs, func(t tx) string { return t.AssetCode }),
		column(txs, func(t tx) string { return t.ChartOfAccountsGroupName }), column(txs, func(t tx) string { return t.OrganizationID }),
		column(txs, func(t tx) string { return t.LedgerID }), column(txs, func(t tx) time.Time { return t.CreatedAt }),
		column(txs, func(t tx) time.Time { return t.UpdatedAt }),
	}
}

// operationArgs maps the operation records onto insertOperationsSQL's arrays, in $n order.
func operationArgs(ops []benchOperation) []any {
	type op = benchOperation

	return []any{
		column(ops, func(o op) string { return o.ID }), column(ops, func(o op) string { return o.TransactionID }),
		column(ops, func(o op) string { return o.Description }), column(ops, func(o op) string { return o.Type }),
		column(ops, func(o op) string { return o.AssetCode }), column(ops, func(o op) decimal.Decimal { return o.Amount.Value }),
		column(ops, func(o op) decimal.Decimal { return o.Balance.Available }), column(ops, func(o op) decimal.Decimal { return o.Balance.OnHold }),
		column(ops, func(o op) decimal.Decimal { return o.BalanceAfter.Available }),
		column(ops, func(o op) decimal.Decimal { return o.BalanceAfter.OnHold }), column(ops, func(o op) string { return o.Status.Code }),
		column(ops, func(o op) string { return o.AccountID }), column(ops, func(o op) string { return o.AccountAlias }),
		column(ops, func(o op) string { return o.BalanceID }), column(ops, func(o op) string { return o.ChartOfAccounts }),
		column(ops, func(o op) string { return o.OrganizationID }), column(ops, func(o op) string { return o.LedgerID }),
		column(ops, func(o op) time.Time { return o.CreatedAt }), column(ops, func(o op) time.Time { return o.UpdatedAt }),
		column(ops, func(o op) bool { return o.BalanceAffected }), column(ops, func(o op) string { return o.BalanceKey }),
		column(ops, func(o op) int64 { return o.Balance.Version }), column(ops, func(o op) int64 { return o.BalanceAfter.Version }),
		column(ops, func(o op) string { return o.Direction }),
		column(ops, func(o op) mmodel.OperationSnapshot {
			return mmodel.OperationSnapshot{OverdraftUsedBefore: o.Balance.OverdraftUsed.String(), OverdraftUsedAfter: o.BalanceAfter.OverdraftUsed.String()}
		}),
	}
}

// column maps every record to one array argument of an unnest statement.
func column[R, V any](records []R, f func(R) V) []V {
	out := make([]V, len(records))
	for i, r := range records {
		out[i] = f(r)
	}

	return out
}

// querier is what queryRows needs from a *sql.Tx or a session's *sql.DB.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// queryRows runs query on q and hands each row to scan.
func queryRows(ctx context.Context, q querier, what, query string, args []any, scan func(*sql.Rows) error) (err error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("%s: scan: %w", what, err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	return nil
}

// execExact runs a statement that must affect exactly want rows.
func execExact(ctx context.Context, tx *sql.Tx, what, query string, want int, args ...any) error {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", what, err)
	}

	if n != int64(want) {
		return fmt.Errorf("%s: %d rows, want %d", what, n, want)
	}

	return nil
}

func sortedDistinct(ids []uuid.UUID) []uuid.UUID {
	slices.SortFunc(ids, compareUUID)

	return slices.Compact(ids)
}
