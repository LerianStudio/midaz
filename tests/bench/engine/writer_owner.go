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
	"time"

	"github.com/google/uuid"
)

// The database clock alone decides the fence: a takeover needs a lease it has passed, a renewal its own owner and
// epoch on a lease it has not passed. clock_timestamp() is read when a row is checked, again when a waiter rechecks
// it; now() is the transaction start.
const (
	acquireFenceSQL = `UPDATE engine_writer_fence SET owner_id = $2, epoch = epoch + 1, lease_until = clock_timestamp() + $3::interval
		WHERE tenant_id = $1 AND lease_until <= clock_timestamp() RETURNING epoch`

	renewFenceSQL = `UPDATE engine_writer_fence SET lease_until = clock_timestamp() + $4::interval
		WHERE tenant_id = $1 AND owner_id = $2 AND epoch = $3 AND lease_until > clock_timestamp()`
)

// Keyset pages: each page starts strictly after the last key read, in that key's order; the zero key starts.
const (
	reloadBalancesSQL = `SELECT ` + lockedBalanceColumns + ` FROM balance WHERE id > $1 AND deleted_at IS NULL ORDER BY id LIMIT $2`

	reloadControlsSQL = `SELECT ` + lockedControlColumns + ` FROM account_control
		WHERE (organization_id, ledger_id, account_id) > ($1::uuid, $2::uuid, $3::uuid)
		ORDER BY organization_id, ledger_id, account_id LIMIT $4`
)

// The owner's bench parameters: a lease of 3 s renewed every second, reloads in pages of 50k.
const (
	ownerLease = 3 * time.Second
	ownerTick  = time.Second
	reloadPage = 50_000
)

// ownerState is a fenced batcher's ownership and the money it reloaded. After the constructor, its one worker alone
// reads it and, after each confirmed COMMIT, updates the cache. err is the first cause that ended the ownership: from
// then on nothing runs SQL.
type ownerState struct {
	tok      ownerToken
	ticker   *time.Ticker
	balances map[uuid.UUID]lockedBalance
	controls map[controlKey]lockedControl
	err      error
}

// newOwnerWriter acquires the fence and reloads every balance and control on b's session, then serves b from one
// worker. ctx bounds the startup, then the writer's lifetime; a startup error starts nothing.
func newOwnerWriter(ctx context.Context, b *batcher, owner string, maxBatch int) (*ownerlessWriter, error) {
	tok, err := acquireFence(ctx, b.sess, owner, ownerLease)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(ownerTick)

	balances, controls, err := reload(ctx, b.sess, tok, ownerLease, ticker.C, reloadPage)
	if err != nil {
		ticker.Stop()

		return nil, err
	}

	b.owner = &ownerState{tok: tok, ticker: ticker, balances: balances, controls: controls}

	return newOwnerlessWriter(ctx, []*batcher{b}, maxBatch), nil
}

// renew gives an idle renewal one lease of client time; cancellation may release its SQL lock later.
// Any error ends the ownership.
func (o *ownerState) renew(ctx context.Context, sess *sqlSession) {
	ctx, cancel := context.WithTimeout(ctx, ownerLease)
	defer cancel()

	if err := renewFence(ctx, sess, o.tok, ownerLease); err != nil {
		o.lose(fmt.Errorf("renew on tick: %w", err))
	}
}

// lose ends the ownership on its cause: renewals stop and the cache is dropped. Nothing calls it twice: a lost owner
// runs no SQL, so no later error can follow.
func (o *ownerState) lose(err error) {
	o.ticker.Stop()
	o.err, o.balances, o.controls = err, nil, nil
}

// cached copies the cached rows of the admitted postings as lockRows reads them: balances by id, the debited accounts'
// controls under the bench organization and ledger. A missing row is a missing fixture, never a decision.
func (o *ownerState) cached(postings []posting) (*lockedRows, error) {
	balances, controls := map[uuid.UUID]lockedBalance{}, map[uuid.UUID]lockedControl{}

	for _, p := range postings {
		b, ok := o.balances[p.BalanceID]
		if !ok {
			return nil, fmt.Errorf("cached balances: balance %s not seeded", p.BalanceID)
		}

		balances[p.BalanceID] = b

		if p.Amount < 0 {
			c, ok := o.controls[controlKey{benchOrgID, benchLedgerID, b.accountID}]
			if !ok {
				return nil, fmt.Errorf("cached controls: account %s has no control: not seeded", b.accountID)
			}

			controls[b.accountID] = c
		}
	}

	return &lockedRows{read: balances, decided: maps.Clone(balances), readCtl: controls, decidedCtl: maps.Clone(controls)}, nil
}

// publish writes the rows a confirmed COMMIT left into the cache.
func (o *ownerState) publish(rows *lockedRows) {
	maps.Copy(o.balances, rows.decided)

	for account, c := range rows.decidedCtl {
		o.controls[controlKey{benchOrgID, benchLedgerID, account}] = c
	}
}

// ownerToken is one ownership of the bench tenant's fence; every takeover increments the epoch.
type ownerToken struct {
	owner string
	epoch int64
}

// controlKey is account_control's primary key.
type controlKey struct{ org, ledger, account uuid.UUID }

// acquireFence takes over the fence in a transaction of its own and returns a token only after a clean COMMIT; any
// error returns the zero token. A live lease returns sql.ErrNoRows. Only a COMMIT error can leave the takeover
// committed yet unknown, and then no successor takes over before that lease expires.
func acquireFence(ctx context.Context, sess *sqlSession, owner string, lease time.Duration) (ownerToken, error) {
	tok := ownerToken{owner: owner}

	err := inOwnTx(ctx, sess, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, acquireFenceSQL, benchTenantID, owner, lease).Scan(&tok.epoch)
	})
	if err != nil {
		return ownerToken{}, fmt.Errorf("acquire fence: %w", err)
	}

	return tok, nil
}

// renewFence renews tok in a transaction of its own; only nil keeps the ownership.
func renewFence(ctx context.Context, sess *sqlSession, tok ownerToken, lease time.Duration) error {
	return inOwnTx(ctx, sess, func(tx *sql.Tx) error { return renewFenceTx(ctx, tx, tok, lease) })
}

// renewFenceTx extends tok's lease in tx and keeps the fence row locked until tx ends, its COMMIT's replication
// wait included, so a takeover waiting on that row rechecks the renewal once tx ends.
func renewFenceTx(ctx context.Context, tx *sql.Tx, tok ownerToken, lease time.Duration) error {
	return execExact(ctx, tx, "renew fence", renewFenceSQL, 1, benchTenantID, tok.owner, tok.epoch, lease)
}

// reload reads every live balance, then every control, in pages of autocommit statements on sess, renewing tok
// before a page when a tick is pending. The renewal after the last page is unconditional: only its success, which
// proves no takeover since acquisition, returns the maps. Any error returns neither.
func reload(ctx context.Context, sess *sqlSession, tok ownerToken, lease time.Duration, ticks <-chan time.Time,
	pageSize int,
) (map[uuid.UUID]lockedBalance, map[controlKey]lockedControl, error) {
	renewOnTick := func() error {
		select {
		case <-ticks:
			return renewFence(ctx, sess, tok, lease)
		default:
			return nil
		}
	}

	balances, controls := map[uuid.UUID]lockedBalance{}, map[controlKey]lockedControl{}

	var lastID uuid.UUID

	for n := pageSize; n == pageSize; {
		if err := renewOnTick(); err != nil {
			return nil, nil, fmt.Errorf("reload: %w", err)
		}

		n = 0

		if err := queryRows(ctx, sess.db, "reload balances", reloadBalancesSQL, []any{lastID, pageSize}, func(rows *sql.Rows) error {
			id, b, err := scanBalance(rows)
			balances[id], lastID, n = b, id, n+1

			return err
		}); err != nil {
			return nil, nil, err
		}
	}

	var lastKey controlKey

	for n := pageSize; n == pageSize; {
		if err := renewOnTick(); err != nil {
			return nil, nil, fmt.Errorf("reload: %w", err)
		}

		n = 0

		if err := queryRows(ctx, sess.db, "reload controls", reloadControlsSQL, []any{lastKey.org, lastKey.ledger, lastKey.account, pageSize},
			func(rows *sql.Rows) error {
				k, c, err := scanControl(rows)
				controls[k], lastKey, n = c, k, n+1

				return err
			}); err != nil {
			return nil, nil, err
		}
	}

	if err := renewFence(ctx, sess, tok, lease); err != nil {
		return nil, nil, fmt.Errorf("reload: %w", err)
	}

	return balances, controls, nil
}

// inOwnTx runs write in a transaction of its own; nil only when sess.commit found the COMMIT clean.
func inOwnTx(ctx context.Context, sess *sqlSession, write func(*sql.Tx) error) error {
	tx, err := sess.begin(ctx)
	if err != nil {
		return err
	}

	if err := write(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}

		return err
	}

	return sess.commit(tx)
}
