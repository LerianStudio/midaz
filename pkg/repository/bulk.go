// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package repository provides shared types for database repository operations.
package repository

import (
	"context"
	"database/sql"
)

// DBExecutor is a minimal interface satisfied by both dbresolver.DB and dbresolver.Tx.
// This allows bulk insert operations to work with either a direct database connection
// or within an external transaction controlled by the caller.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// BulkInsertResult contains the counts from a bulk insert operation.
// It tracks how many rows were attempted, actually inserted, and ignored (duplicates).
type BulkInsertResult struct {
	Attempted int64 // Rows sent to INSERT
	Inserted  int64 // Rows actually inserted
	Ignored   int64 // Rows skipped (duplicates via ON CONFLICT DO NOTHING)
}
