//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// captureWriter records every Submit's request and answer at the writer port while the real writer decides.
type captureWriter struct {
	writer
	mu    sync.Mutex
	calls []call // read only after every outstanding Submit returned
}

type call struct {
	req request
	res result
}

func captureCalls(w writer, n int) *captureWriter {
	return &captureWriter{writer: w, calls: make([]call, 0, n)}
}

func (c *captureWriter) Submit(ctx context.Context, req request) result {
	res := c.writer.Submit(ctx, req)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, call{req: req, res: res})

	return res
}

// snapshotBaseline runs once per connection after a reset and fixtures, before any request or operation is written.
// Analyze the fixed baseline: autovacuum cannot collect statistics for TEMP tables.
func snapshotBaseline(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()

	_, err := conn.Exec(ctx, `CREATE TEMP TABLE base_balance AS
			SELECT id, account_id, available, version, allow_sending, allow_receiving FROM balance;
		CREATE TEMP TABLE base_control AS
			SELECT organization_id, ledger_id, account_id, blocked, limit_remaining, version FROM account_control;
		ANALYZE base_balance, base_control`)
	require.NoError(t, err)
}

// unansweredApproval counts SQL approvals with no new approved answer; a later replay does not remove this count.
const unansweredApproval = "sql-approved/caller-unknown"

// ledgerChecks count violations of the baseline, the ledger and the captured answers; each count must be zero.
var ledgerChecks = [...]struct {
	what, query string
	args        []any
}{
	{"balances whose first operation, or untouched row, differs from the baseline, or whose flags changed", `
		SELECT count(*) FROM base_balance bb JOIN balance b ON b.id = bb.id
		LEFT JOIN (SELECT DISTINCT ON (balance_id) balance_id, available_balance, balance_version_before FROM operation
			ORDER BY balance_id, balance_version_before) f ON f.balance_id = bb.id
		WHERE (b.allow_sending, b.allow_receiving) IS DISTINCT FROM (bb.allow_sending, bb.allow_receiving)
			OR bb.available IS DISTINCT FROM coalesce(f.available_balance, b.available)
			OR bb.version IS DISTINCT FROM coalesce(f.balance_version_before, b.version)`, nil},
	{"controls not at baseline minus their debits, version plus their debit postings, non-negative, same blocking", `
		SELECT count(*) FROM base_control bc JOIN account_control c USING (organization_id, ledger_id, account_id)
		LEFT JOIN (SELECT organization_id, ledger_id, account_id, sum(amount) AS debited, count(*) AS debits FROM operation
			WHERE type = $1 GROUP BY organization_id, ledger_id, account_id) d USING (organization_id, ledger_id, account_id)
		WHERE c.blocked <> bc.blocked OR c.limit_remaining < 0 OR c.limit_remaining <> bc.limit_remaining - coalesce(d.debited, 0)
			OR c.version <> bc.version + coalesce(d.debits, 0)`, []any{constant.DEBIT}},
	{"debits below zero, from a sending-disabled balance or a blocked account, or credits to a receiving-disabled one", `
		SELECT count(*) FROM operation o JOIN base_balance bb ON bb.id = o.balance_id
		LEFT JOIN base_control bc ON (bc.organization_id, bc.ledger_id, bc.account_id) = (o.organization_id, o.ledger_id, o.account_id)
		WHERE CASE WHEN o.type = $1 THEN o.available_balance_after < 0 OR NOT bb.allow_sending OR bc.blocked IS NOT FALSE
			ELSE NOT bb.allow_receiving END`, []any{constant.DEBIT}},
	{"answers that differ from their stored identity: decision or replay outcome, reason, payload or transaction", `
		SELECT count(*) FROM cap_answer a LEFT JOIN engine_request r ON r.tenant_id = $1 AND r.request_id = a.request_id
		WHERE CASE a.outcome WHEN 'unknown' THEN false
			WHEN 'conflict' THEN r.request_id IS NULL OR r.payload_fingerprint = a.fingerprint OR a.answered_tx IS NOT NULL
			ELSE r.request_id IS NULL OR r.payload_fingerprint <> a.fingerprint OR r.reason <> a.reason
				OR r.outcome IS DISTINCT FROM CASE a.outcome WHEN 'replay' THEN a.original ELSE a.outcome END
				OR r.transaction_id IS DISTINCT FROM a.answered_tx
				OR a.answered_tx IS DISTINCT FROM (CASE WHEN r.outcome = 'approved' THEN a.request_tx END) END`, []any{benchTenantID}},
	{"identities answered with more than one new decision", `
		SELECT count(*) FROM (SELECT FROM cap_answer WHERE outcome IN ('approved', 'refused', 'expired')
			GROUP BY request_id HAVING count(*) > 1) d`, nil},
	{"stored identities no captured request carried with their payload", `
		SELECT count(*) FROM engine_request r WHERE r.tenant_id = $1 AND NOT EXISTS
			(SELECT FROM cap_answer a WHERE a.request_id = r.request_id AND a.fingerprint = r.payload_fingerprint)`, []any{benchTenantID}},
	{
		"approved postings of the winning payload missing from operations, or operations no approved payload owes", `
		WITH want AS (SELECT p.request_tx AS transaction_id, p.balance_id, bb.account_id, p.amount::numeric FROM engine_request r
				JOIN cap_payload p ON p.fingerprint = r.payload_fingerprint JOIN base_balance bb ON bb.id = p.balance_id
				WHERE r.tenant_id = $1 AND r.outcome = 'approved'),
			got AS (SELECT transaction_id, balance_id, account_id, CASE WHEN type = $2 THEN -amount ELSE amount END FROM operation)
		SELECT (SELECT count(*) FROM (TABLE want EXCEPT ALL TABLE got) m) + (SELECT count(*) FROM (TABLE got EXCEPT ALL TABLE want) e)`,
		[]any{benchTenantID, constant.DEBIT},
	},
}

// verifyLedger checks a fixture after Close: the ledger once, then money against the baseline and each captured answer
// against its stored identity. It returns the answers by outcome[/original][/reason], with SQL approvals no caller was
// answered under unansweredApproval.
func verifyLedger(ctx context.Context, t *testing.T, conn *pgx.Conn, calls []call) map[string]int {
	t.Helper()

	assertBatchLedger(ctx, t, conn)

	tally, misrouted := map[string]int{}, 0

	for i := range calls {
		res := calls[i].res
		if res.requestID != calls[i].req.id {
			misrouted++
		}

		key := string(outcomeUnknown)
		if res.outcome != outcomeUnknown {
			key = strings.Join(slices.DeleteFunc([]string{string(res.outcome), string(res.original), res.reason},
				func(s string) bool { return s == "" }), "/")
		}

		tally[key]++
	}

	assert.Zero(t, misrouted, "answers carrying another caller's request ID")

	tx, err := conn.Begin(ctx)
	require.NoError(t, err)

	defer func() { assert.NoError(t, tx.Rollback(ctx), "rollback drops the capture tables") }()

	copyCalls(ctx, t, tx, calls)

	for _, c := range ledgerChecks {
		var n int64
		require.NoError(t, tx.QueryRow(ctx, c.query, c.args...).Scan(&n), c.what)
		assert.Zero(t, n, c.what)
	}

	var unanswered int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM engine_request r WHERE r.tenant_id = $1 AND r.outcome = 'approved'
		AND NOT EXISTS (SELECT FROM cap_answer a WHERE a.request_id = r.request_id AND a.outcome = 'approved')`, benchTenantID).Scan(&unanswered))

	if unanswered > 0 {
		tally[unansweredApproval] = unanswered
	}

	return tally
}

// copyCalls loads every answer, and each distinct payload's postings once, into temporary tables of tx.
func copyCalls(ctx context.Context, t *testing.T, tx pgx.Tx, calls []call) {
	t.Helper()

	_, err := tx.Exec(ctx, `CREATE TEMP TABLE cap_answer (request_id uuid, fingerprint bytea, request_tx uuid, outcome text,
		original text, reason text, answered_tx uuid); CREATE TEMP TABLE cap_payload (fingerprint bytea, request_tx uuid, balance_id uuid, amount bigint)`)
	require.NoError(t, err)

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"cap_answer"},
		[]string{"request_id", "fingerprint", "request_tx", "outcome", "original", "reason", "answered_tx"},
		pgx.CopyFromSlice(len(calls), func(i int) ([]any, error) {
			c := &calls[i]

			var answered any // NULL unless the answer carries a transaction
			if c.res.transactionID != uuid.Nil {
				answered = c.res.transactionID
			}

			return []any{
				c.req.id, c.req.fingerprint[:], c.req.transactionID, string(c.res.outcome), string(c.res.original),
				c.res.reason, answered,
			}, nil
		}))
	require.NoError(t, err)

	seen := make(map[[32]byte]bool, len(calls))

	var postings [][]any

	for i := range calls {
		req := &calls[i].req
		if seen[req.fingerprint] {
			continue
		}

		seen[req.fingerprint] = true

		for _, p := range req.postings {
			postings = append(postings, []any{req.fingerprint[:], req.transactionID, p.BalanceID, p.Amount})
		}
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"cap_payload"}, []string{"fingerprint", "request_tx", "balance_id", "amount"}, pgx.CopyFromRows(postings))
	require.NoError(t, err)
}
