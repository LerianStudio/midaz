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
	"time"
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

// ownerToken is one ownership of the bench tenant's fence; every takeover increments the epoch.
type ownerToken struct {
	owner string
	epoch int64
}

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
