// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package repository provides shared types for database repository operations.
package repository

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNilDBExecutor is returned when a nil database executor is provided to bulk operations.
var ErrNilDBExecutor = errors.New("nil database executor provided")

// ErrQueryContextNotSupported is returned when the DBExecutor does not support QueryContext.
var ErrQueryContextNotSupported = errors.New("DBExecutor does not support QueryContext")

// DBExecutor is a minimal interface satisfied by both dbresolver.DB and dbresolver.Tx.
// This allows bulk insert operations to work with either a direct database connection
// or within an external transaction controlled by the caller.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// DBQuerier extends DBExecutor with query capabilities for operations that need
// to retrieve results (e.g., RETURNING clause in bulk inserts).
type DBQuerier interface {
	DBExecutor
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// BulkInsertResult contains the counts from a bulk insert operation.
// It tracks how many rows were attempted, actually inserted, and ignored (duplicates).
type BulkInsertResult struct {
	Attempted   int64    // Rows sent to INSERT
	Inserted    int64    // Rows actually inserted
	Ignored     int64    // Rows skipped (duplicates via ON CONFLICT DO NOTHING)
	InsertedIDs []string // IDs of rows that were actually inserted (for filtering downstream operations)
}

// BulkUpdateResult contains the counts from a bulk update operation.
// It tracks how many rows were attempted and actually updated.
type BulkUpdateResult struct {
	Attempted int64 // Rows sent to UPDATE
	Updated   int64 // Rows actually updated (status changed)
	Unchanged int64 // Rows skipped (status already matches)
}

// DBTransaction extends DBExecutor with commit/rollback capabilities.
// This interface is satisfied by database transaction types (e.g., dbresolver.Tx, sql.Tx)
// and enables atomic multi-table operations controlled by the caller.
type DBTransaction interface {
	DBExecutor
	Commit() error
	Rollback() error
}

// MongoDBBulkInsertResult contains the counts from a MongoDB bulk insert operation.
// It tracks how many documents were attempted, actually inserted, and matched (duplicates).
// This is analogous to BulkInsertResult but designed for MongoDB's BulkWrite semantics.
type MongoDBBulkInsertResult struct {
	Attempted   int64    // Documents sent to BulkWrite
	Inserted    int64    // Documents actually inserted (UpsertedCount)
	Matched     int64    // Documents that matched filter (already existed)
	InsertedIDs []string // EntityIDs of documents that were actually inserted
}

// MongoDBBulkUpdateResult contains the counts from a MongoDB bulk update operation.
// It tracks how many documents were attempted and actually modified.
type MongoDBBulkUpdateResult struct {
	Attempted int64 // Documents sent to BulkWrite
	Modified  int64 // Documents actually modified
	Matched   int64 // Documents that matched filter
}
