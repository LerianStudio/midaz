//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

// sqlSession is one writer's only connection, run by one goroutine at a time. A notice therefore belongs to the
// statement this session is running, and a notice during COMMIT to that COMMIT.
type sqlSession struct {
	db     *sql.DB
	notice atomic.Pointer[pgconn.Notice]
}

// openSQLSession dials a copy of a config the bench guard already checked; the session owns the pool.
func openSQLSession(cfg *pgx.ConnConfig) *sqlSession {
	s := &sqlSession{}

	sessionCfg := cfg.Copy()
	// PostgreSQL ignores GUC key case; keep one spelling so startup map order cannot suppress the SyncRep WARNING.
	maps.DeleteFunc(sessionCfg.RuntimeParams, func(key, _ string) bool { return strings.EqualFold(key, "client_min_messages") })
	sessionCfg.RuntimeParams["client_min_messages"] = "warning"
	sessionCfg.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) { s.notice.Store(n) }

	s.db = stdlib.OpenDB(*sessionCfg)
	s.db.SetMaxOpenConns(1)

	return s
}

// begin starts a READ COMMITTED transaction. pgx stdlib also runs COMMIT under this BeginTx context.
func (s *sqlSession) begin(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	return tx, nil
}

// commit returns nil only when COMMIT returned no error and no notice: PostgreSQL reports a cancelled synchronous
// replication wait as a WARNING on a COMMIT that already committed locally. Any error is an unknown outcome,
// committed or not, and nothing here retries.
func (s *sqlSession) commit(tx *sql.Tx) error {
	s.notice.Store(nil)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit outcome unknown: %w", err)
	}

	if n := s.notice.Load(); n != nil {
		return fmt.Errorf("commit outcome unknown: %s %s: %s", n.Severity, n.Code, n.Message)
	}

	return nil
}
