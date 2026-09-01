// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package repository

import (
	"context"
	"database/sql"
)

// DBReader is a minimal read interface satisfied by both dbresolver.DB and dbresolver.Tx.
// Read operations depend on this instead of a concrete handle so a caller can be served
// by either a direct connection or a read-only transaction.
type DBReader interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DBReadTx is a read surface that can be finalized (a read-only transaction).
// It is satisfied by database transaction types (e.g., dbresolver.Tx) and lets the
// acquirer release the underlying connection once the read completes.
type DBReadTx interface {
	DBReader
	Commit() error
	Rollback() error
}
