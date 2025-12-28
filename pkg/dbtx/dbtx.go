// Package dbtx provides database transaction context management.
// It allows passing a database transaction through context to enable
// multiple repository operations to participate in a single atomic transaction.
package dbtx

import (
	"context"
	"database/sql"
	"fmt"
)

// txKey is the context key for database transactions.
// Using a private type prevents collisions with other packages.
type txKey struct{}

// Tx is an interface for transaction operations.
// Both *sql.Tx and dbresolver.Tx implement this interface.
type Tx interface {
	Commit() error
	Rollback() error
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// TxBeginner is an interface for types that can begin a transaction.
// This abstracts both *sql.DB and dbresolver.DB.
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
}

// Executor is an interface for types that can execute queries.
// *sql.DB, *sql.Tx, dbresolver.DB, and dbresolver.Tx all implement this interface.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ContextWithTx returns a new context with the given transaction.
// If tx is nil, the original context is returned unchanged.
func ContextWithTx(ctx context.Context, tx Tx) context.Context {
	if tx == nil {
		return ctx
	}

	return context.WithValue(ctx, txKey{}, tx)
}

// TxFromContext extracts a transaction from the context.
// Returns nil if no transaction is present.
func TxFromContext(ctx context.Context) Tx {
	tx, _ := ctx.Value(txKey{}).(Tx)
	return tx
}

// GetExecutor returns the transaction from context if present,
// otherwise returns the provided database connection.
// This allows repository methods to transparently use either
// a transaction or direct connection.
// The db parameter should implement the Executor interface (e.g., *sql.DB or dbresolver.DB).
func GetExecutor(ctx context.Context, db Executor) Executor {
	if tx := TxFromContext(ctx); tx != nil {
		return tx
	}

	return db
}

// RunInTransaction executes the given function within a database transaction.
// If the function returns an error or panics, the transaction is rolled back.
// Otherwise, the transaction is committed.
func RunInTransaction(ctx context.Context, db TxBeginner, fn func(ctx context.Context) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err) //nolint:wrapcheck // Transaction operations are infrastructure-level, context added via fmt.Errorf
	}

	// Ensure rollback on panic
	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Execute function with transaction in context
	txCtx := ContextWithTx(ctx, tx)
	if err := fn(txCtx); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err) //nolint:wrapcheck // Transaction operations are infrastructure-level, context added via fmt.Errorf
	}

	committed = true

	return nil
}
