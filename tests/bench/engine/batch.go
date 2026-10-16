//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/outbox"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

const (
	// batchBudget caps one batch's SQL work across all its passes, on the process clock.
	batchBudget      = 10 * time.Second
	sqlStateDeadlock = "40P01"
)

var (
	errCutoffCrossed = errors.New("a new decision crossed its admission cutoff before COMMIT")
	benchFeeBalance  = balanceID(classFee, 0, constant.DefaultBalanceKey).String()
)

// batcher decides and persists batches on one session, one batch at a time. Events and private rows join each
// pass's SQL transaction through the lib-streaming producer and the lib-commons repository.
type batcher struct {
	sess        *sqlSession
	events      libStreaming.TransactionalBatchEmitter
	rows        outbox.TransactionalBatchWriter
	description string           // the v2 create body's, carried by every transaction and operation
	now         func() time.Time // time.Now in runs, a controlled clock in tests
	owner       *ownerState      // nil: ownerless, locking the money rows in each pass
}

// batchStats counts one batch's passes and, among them, the deadlock retry.
type batchStats struct{ passes, deadlocks int }

func newBatcher(sess *sqlSession, ev libStreaming.TransactionalBatchEmitter, rows outbox.TransactionalBatchWriter, now func() time.Time) (*batcher, error) {
	in, err := decodeCreateV2Body()
	if err != nil {
		return nil, err
	}

	return &batcher{sess: sess, events: ev, rows: rows, description: in.Description, now: now}, nil
}

// run answers every position of reqs, in order. A new decision crossing its cutoff before COMMIT reruns the batch
// from fresh rows, a pre-COMMIT deadlock once; any other error, a COMMIT error included, answers every position
// unknown with its cause and ends an owner, whose later batches answer unknown with that first cause and no SQL.
func (b *batcher) run(ctx context.Context, reqs []request) ([]result, batchStats, error) {
	var stats batchStats

	switch {
	case len(reqs) == 0:
		return nil, stats, nil
	case b.owner != nil && b.owner.err != nil:
		return unknowns(reqs), stats, b.owner.err
	}

	ctx, cancel := context.WithTimeout(ctx, batchBudget)
	defer cancel()

	g := groupBatch(reqs)

	for {
		stats.passes++

		res, sent, err := b.pass(ctx, reqs, g)
		state, _ := libPostgres.SQLState(err)

		switch {
		case err == nil:
			return res, stats, nil
		case errors.Is(err, errCutoffCrossed):
			continue // the crossed request is late at every later check, so passes end
		case !sent && state == sqlStateDeadlock && stats.deadlocks == 0:
			stats.deadlocks++
			continue
		}

		if b.owner != nil {
			b.owner.lose(err)
		}

		return unknowns(reqs), stats, err
	}
}

// batchGroup indexes a batch by identity: first occurrences in arrival order decide, sorted IDs take locks.
type batchGroup struct {
	first  map[uuid.UUID]int // position of each identity's first occurrence
	firsts []int             // those positions in arrival order
	sorted []uuid.UUID       // distinct IDs in PostgreSQL uuid order
}

func groupBatch(reqs []request) batchGroup {
	g := batchGroup{first: make(map[uuid.UUID]int, len(reqs))}

	for i, r := range reqs {
		if _, seen := g.first[r.id]; !seen {
			g.first[r.id], g.firsts, g.sorted = i, append(g.firsts, i), append(g.sorted, r.id)
		}
	}

	slices.SortFunc(g.sorted, compareUUID)

	return g
}

func compareUUID(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }

// passContext bounds a pass by the earliest response deadline still ahead on b.now. A deadline already gone bounds
// nothing: its request only resolves.
func (b *batcher) passContext(ctx context.Context, reqs []request) (context.Context, context.CancelFunc) {
	now, earliest := b.now(), time.Time{}

	for _, r := range reqs {
		if r.deadline.After(now) && (earliest.IsZero() || r.deadline.Before(earliest)) {
			earliest = r.deadline
		}
	}

	if earliest.IsZero() {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, earliest.Sub(now))
}

// pass runs one SQL transaction and reports whether COMMIT was sent, after which nothing is retried. Every answer of
// a clean COMMIT carries its acknowledgement instant, taken before an owner publishes the pass's rows to its cache.
func (b *batcher) pass(ctx context.Context, reqs []request, g batchGroup) ([]result, bool, error) {
	ctx, cancel := b.passContext(ctx, reqs)
	defer cancel()

	tx, err := b.sess.begin(ctx)
	if err != nil {
		return nil, false, err
	}

	res, rows, err := b.decide(ctx, tx, reqs, g)
	if err != nil {
		// A context that ended already rolled the transaction back.
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}

		return nil, false, err
	}

	if err := b.sess.commit(tx); err != nil {
		return nil, true, err
	}

	ack := b.now()
	for i := range res {
		res[i].dbAckAt = ack
	}

	if b.owner != nil {
		b.owner.publish(rows)
	}

	return res, true, nil
}

// decide resolves every identity, decides the claimed ones still admissible on their rows, writes them with their
// obligations and finalizes every claim in tx, returning the rows as decided; a new decision late before COMMIT voids
// the pass. An owner first renews its fence, locked through COMMIT, and decides on cached copies; others lock the rows.
func (b *batcher) decide(ctx context.Context, tx *sql.Tx, reqs []request, g batchGroup) ([]result, *lockedRows, error) {
	if b.owner != nil {
		if err := renewFenceTx(ctx, tx, b.owner.tok, ownerLease); err != nil {
			return nil, nil, err
		}
	}

	resolved, err := resolveIdentities(ctx, tx, reqs, g)
	if err != nil {
		return nil, nil, err
	}

	postings := admittedPostings(reqs, g, resolved, b.now())

	var rows *lockedRows
	if b.owner != nil {
		rows, err = b.owner.cached(postings)
	} else {
		rows, err = lockRows(ctx, tx, postings)
	}

	if err != nil {
		return nil, nil, err
	}

	startedAt := b.now()
	at := startedAt.UTC().Truncate(time.Microsecond) // what timestamptz stores; startedAt keeps its monotonic reading

	w, err := b.decideNew(ctx, rows, reqs, g, resolved, startedAt, at)
	if err != nil {
		return nil, nil, err
	}

	if err := b.persist(ctx, tx, w, rows, at); err != nil {
		return nil, nil, err
	}

	if err := finalize(ctx, tx, g, resolved, at); err != nil {
		return nil, nil, err
	}

	late := b.now()
	for _, p := range w.decided {
		if !reqs[p].admissible(late) {
			return nil, nil, errCutoffCrossed
		}
	}

	return answers(reqs, g, resolved, startedAt), rows, nil
}

// resolution answers one identity: the stored row, or this pass's decision when the pass claimed it.
type resolution struct {
	claimed       bool
	fingerprint   []byte
	outcome       outcome
	reason        string
	transactionID *uuid.UUID // approved only
}

// admittedPostings returns the postings of claimed identities admissible at now, the only ones that lock money rows.
func admittedPostings(reqs []request, g batchGroup, resolved map[uuid.UUID]*resolution, now time.Time) []posting {
	var postings []posting

	for _, p := range g.firsts {
		if r := reqs[p]; resolved[r.id].claimed && r.admissible(now) {
			postings = append(postings, r.postings...)
		}
	}

	return postings
}

// passWrites is what one pass writes for its approvals: records and the obligations built from those same records.
type passWrites struct {
	decided []int // positions of new approvals and refusals, checked against their cutoff before COMMIT
	txs     []events.TransactionSource
	ops     []benchOperation
	events  []libStreaming.EmitRequest
	private []*outbox.OutboxEvent
}

// decideNew decides each claimed identity in arrival order: late is expired, the rest approve or refuse over the
// state earlier approvals of this pass left.
func (b *batcher) decideNew(ctx context.Context, rows *lockedRows, reqs []request, g batchGroup, resolved map[uuid.UUID]*resolution,
	startedAt, at time.Time,
) (passWrites, error) {
	var w passWrites

	for _, p := range g.firsts {
		req, r := reqs[p], resolved[reqs[p].id]

		switch {
		case !r.claimed:
			continue
		case !req.admissible(startedAt):
			r.outcome = outcomeExpired
			continue
		}

		w.decided = append(w.decided, p)

		ops, reason := rows.apply(req, b.description, at)
		if reason != "" {
			r.outcome, r.reason = outcomeRefused, reason
			continue
		}

		r.outcome, r.transactionID = outcomeApproved, &req.transactionID

		if err := w.approve(ctx, req, ops, b.description, at); err != nil {
			return passWrites{}, err
		}
	}

	return w, nil
}

// lockedBalance is one balance as locked; its benchBalance is the state decisions read and operations record.
type lockedBalance struct {
	benchBalance
	accountID                      uuid.UUID
	alias, key, asset, accountType string
	allowSending, allowReceiving   bool
}

// lockedControl is one debited account's control.
type lockedControl struct {
	blocked bool
	limit   decimal.Decimal
	version int64
}

// lockedRows keeps the rows as locked, whose versions every UPDATE expects, and the state this pass's approvals left.
type lockedRows struct {
	read, decided       map[uuid.UUID]lockedBalance // by balance ID
	readCtl, decidedCtl map[uuid.UUID]lockedControl // by account ID
}

// apply decides req's postings in order on a private copy over the approved state and merges the copy on approval.
// Debits need sending allowed, an unblocked control, and available and the account's limit not below zero after;
// credits need receiving allowed. It returns the operations recorded at at, or the refusal reason.
func (rows *lockedRows) apply(req request, description string, at time.Time) ([]benchOperation, string) {
	bal, ctl := map[uuid.UUID]lockedBalance{}, map[uuid.UUID]lockedControl{}
	ops := make([]benchOperation, len(req.postings))

	for i, p := range req.postings {
		b := overlay(bal, rows.decided, p.BalanceID)
		amount, before := decimal.NewFromInt(p.Amount), b.benchBalance
		b.Available, b.Version = b.Available.Add(amount), b.Version+1

		if p.Amount < 0 {
			c := overlay(ctl, rows.decidedCtl, b.accountID)
			c.limit, c.version = c.limit.Add(amount), c.version+1

			if reason := debitRefusal(b, c); reason != "" {
				return nil, reason
			}

			ctl[b.accountID] = c
		} else if !b.allowReceiving {
			return nil, "receiving_disabled"
		}

		bal[p.BalanceID] = b
		ops[i] = b.operation(req, i, before, amount, description, at)
	}

	maps.Copy(rows.decided, bal)
	maps.Copy(rows.decidedCtl, ctl)

	return ops, ""
}

// debitRefusal checks a debited balance and its control after the debit.
func debitRefusal(b lockedBalance, c lockedControl) string {
	switch {
	case !b.allowSending:
		return "sending_disabled"
	case c.blocked:
		return "blocked"
	case b.Available.IsNegative():
		return "insufficient_funds"
	case c.limit.IsNegative():
		return "limit_exceeded"
	}

	return ""
}

func overlay[V any](own, base map[uuid.UUID]V, key uuid.UUID) V {
	if v, ok := own[key]; ok {
		return v
	}

	return base[key]
}

// operation records posting i, from before to b's state after it, as leg.operation records a fixture leg.
func (b lockedBalance) operation(req request, i int, before benchBalance, amount decimal.Decimal, description string, at time.Time) benchOperation {
	opType, direction := constant.CREDIT, constant.DirectionCredit
	if amount.IsNegative() {
		opType, direction = constant.DEBIT, constant.DirectionDebit
	}

	return benchOperation{
		ID: obligationID(req.id, "operation", i).String(), TransactionID: req.transactionID.String(), Description: description,
		Type: opType, AssetCode: b.asset, Amount: benchAmount{Value: amount.Abs()}, Balance: before, BalanceAfter: b.benchBalance,
		AccountID: b.accountID.String(), AccountAlias: b.alias, BalanceKey: b.key, BalanceID: req.postings[i].BalanceID.String(),
		OrganizationID: benchOrgID.String(), LedgerID: benchLedgerID.String(), BalanceAffected: true, Direction: direction,
		CreatedAt: at, UpdatedAt: at, AccountType: b.accountType,
	}
}

// approve records one approval: its transaction from its operations, the fee marks, and the obligations built from
// those records, so rows and events cannot disagree.
func (w *passWrites) approve(ctx context.Context, req request, ops []benchOperation, description string, at time.Time) error {
	approved, amount := constant.APPROVED, decimal.Zero
	tx := events.TransactionSource{
		ID: req.transactionID.String(), OrganizationID: benchOrgID.String(), LedgerID: benchLedgerID.String(),
		Status: mmodel.Status{Code: approved, Description: &approved}, Amount: &amount, AssetCode: ops[0].AssetCode,
		Description: description, Metadata: feeMarks(ops), CreatedAt: at, UpdatedAt: at,
	}

	for _, op := range ops {
		if op.Type == constant.DEBIT {
			amount, tx.Source = amount.Add(op.Amount.Value), append(tx.Source, op.AccountAlias)
		} else {
			tx.Destination = append(tx.Destination, op.AccountAlias)
		}
	}

	ob, err := buildObligations(ctx, req.id, tx, ops)
	if err != nil {
		return fmt.Errorf("obligations of request %s: %w", req.id, err)
	}

	w.txs, w.ops = append(w.txs, tx), append(w.ops, ops...)
	w.events, w.private = append(w.events, ob.events...), append(w.private, ob.private...)

	return nil
}

// feeMarks stands in for the fee engine: a credit to the seeded fee balance is a fee already decided, marked as
// fixtureLegs marks one, its source the first other credit. It returns the transaction's marks, nil without a fee.
func feeMarks(ops []benchOperation) map[string]any {
	payer, fee := "", -1

	for i, op := range ops {
		switch {
		case op.Type != constant.CREDIT:
		case op.BalanceID == benchFeeBalance:
			fee = i
		case payer == "":
			payer = op.AccountAlias
		}
	}

	if fee < 0 {
		return nil
	}

	ops[fee].Metadata = map[string]any{constant.MetadataKeyFeeLeg: constant.MetadataValueFeeLeg, "source": payer}

	return map[string]any{"packageAppliedID": benchFeePackage.String(), "feeApplied": "true"}
}

// answers gives a claimed identity's first occurrence its decision and every other copy a replay of the identity's
// stored or decided outcome, or a conflict when its payload differs from the one that fixed the identity.
func answers(reqs []request, g batchGroup, resolved map[uuid.UUID]*resolution, startedAt time.Time) []result {
	res := make([]result, len(reqs))

	for i, req := range reqs {
		r := resolved[req.id]
		res[i] = result{requestID: req.id, outcome: r.outcome, reason: r.reason, decisionStartedAt: startedAt}

		switch {
		case r.claimed && g.first[req.id] == i:
		case !bytes.Equal(r.fingerprint, req.fingerprint[:]):
			res[i] = result{requestID: req.id, outcome: outcomeConflict, decisionStartedAt: startedAt}
			continue
		default:
			res[i].outcome, res[i].original = outcomeReplay, r.outcome
		}

		if r.transactionID != nil {
			res[i].transactionID = *r.transactionID
		}
	}

	return res
}

func unknowns(reqs []request) []result {
	res := make([]result, len(reqs))
	for i, r := range reqs {
		res[i] = result{requestID: r.id, outcome: outcomeUnknown}
	}

	return res
}
