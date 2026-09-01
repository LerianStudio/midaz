// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package db

//go:generate mockgen -source=interfaces.go -destination=mocks/interfaces_mock.go -package=mocks

import (
	"context"
	"database/sql"
)

// DB defines the minimal database interface required by repositories.
// This interface is satisfied by both *sql.DB and dbresolver.DB.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx defines the interface for database transactions.
// This interface is satisfied by both *sql.Tx and dbresolver.Tx.
// It extends DB with Commit and Rollback capabilities.
type Tx interface {
	DB
	Commit() error
	Rollback() error
}

// TxBeginner defines the interface for starting database transactions.
// Satisfied by dbresolver.DB (from lib-commons PostgresConnection.GetDB()).
// Used by ValidationService to start transactions for atomic operations
// across validation, limits, and audit persistence.
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
}

// Connection defines the interface for database connection providers.
// This allows for easy mocking in tests while maintaining compatibility
// with *libPostgres.Client in production.
type Connection interface {
	// GetDB returns the underlying database connection using the provided context.
	// The context is propagated to the connection resolver, enabling deadline,
	// cancellation, and trace correlation through the connection lifecycle.
	GetDB(ctx context.Context) (DB, error)
}
