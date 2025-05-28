package unitofwork

import (
	"context"
	"database/sql"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/pkg/errors"
)

// Repository represents a generic repository interface
type Repository interface {
	// GetTx returns the current transaction if any
	GetTx() *sql.Tx
	// SetTx sets the transaction for the repository
	SetTx(tx *sql.Tx)
}

// Work represents a unit of work operation
type Work func(ctx context.Context, uow *UnitOfWork) error

// UnitOfWork manages database transactions across multiple repositories
type UnitOfWork struct {
	db           *sql.DB
	tx           *sql.Tx
	repositories map[string]Repository
	committed    bool
	rolledBack   bool
}

// New creates a new unit of work
func New(db *sql.DB) *UnitOfWork {
	return &UnitOfWork{
		db:           db,
		repositories: make(map[string]Repository),
	}
}

// RegisterRepository registers a repository with the unit of work
func (uow *UnitOfWork) RegisterRepository(name string, repo Repository) {
	uow.repositories[name] = repo
}

// GetRepository retrieves a registered repository
func (uow *UnitOfWork) GetRepository(name string) (Repository, error) {
	repo, exists := uow.repositories[name]
	if !exists {
		return nil, fmt.Errorf("repository %s not registered", name)
	}
	return repo, nil
}

// Begin starts a new transaction
func (uow *UnitOfWork) Begin(ctx context.Context) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "unitofwork.begin")
	defer span.End()

	if uow.tx != nil {
		err := errors.New("transaction already started")
		libOpentelemetry.HandleSpanError(&span, "Transaction already exists", err)
		return err
	}

	tx, err := uow.db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)
		logger.Errorf("Failed to begin transaction: %v", err)
		return err
	}

	uow.tx = tx
	uow.committed = false
	uow.rolledBack = false

	// Set transaction on all registered repositories
	for name, repo := range uow.repositories {
		repo.SetTx(tx)
		logger.Debugf("Set transaction on repository: %s", name)
	}

	logger.Debug("Transaction begun successfully")
	return nil
}

// Commit commits the transaction
func (uow *UnitOfWork) Commit(ctx context.Context) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "unitofwork.commit")
	defer span.End()

	if uow.tx == nil {
		err := errors.New("no transaction to commit")
		libOpentelemetry.HandleSpanError(&span, "No transaction exists", err)
		return err
	}

	if uow.committed {
		err := errors.New("transaction already committed")
		libOpentelemetry.HandleSpanError(&span, "Transaction already committed", err)
		return err
	}

	if uow.rolledBack {
		err := errors.New("transaction already rolled back")
		libOpentelemetry.HandleSpanError(&span, "Transaction already rolled back", err)
		return err
	}

	if err := uow.tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)
		logger.Errorf("Failed to commit transaction: %v", err)
		return err
	}

	uow.committed = true
	uow.clearTransaction()

	logger.Debug("Transaction committed successfully")
	return nil
}

// Rollback rolls back the transaction
func (uow *UnitOfWork) Rollback(ctx context.Context) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "unitofwork.rollback")
	defer span.End()

	if uow.tx == nil {
		logger.Debug("No transaction to rollback")
		return nil
	}

	if uow.committed {
		logger.Debug("Transaction already committed, cannot rollback")
		return nil
	}

	if uow.rolledBack {
		logger.Debug("Transaction already rolled back")
		return nil
	}

	if err := uow.tx.Rollback(); err != nil {
		// Rollback error is not critical if transaction was already completed
		if err != sql.ErrTxDone {
			libOpentelemetry.HandleSpanError(&span, "Failed to rollback transaction", err)
			logger.Errorf("Failed to rollback transaction: %v", err)
			return err
		}
	}

	uow.rolledBack = true
	uow.clearTransaction()

	logger.Debug("Transaction rolled back successfully")
	return nil
}

// Execute executes a unit of work within a transaction
func (uow *UnitOfWork) Execute(ctx context.Context, work Work) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "unitofwork.execute")
	defer span.End()

	// Begin transaction
	if err := uow.Begin(ctx); err != nil {
		return err
	}

	// Ensure rollback on panic or error
	defer func() {
		if r := recover(); r != nil {
			uow.Rollback(ctx)
			panic(r)
		}
	}()

	// Execute work
	if err := work(ctx, uow); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Work execution failed", err)
		logger.Errorf("Work execution failed: %v", err)
		
		// Rollback on error
		if rbErr := uow.Rollback(ctx); rbErr != nil {
			logger.Errorf("Failed to rollback after work error: %v", rbErr)
		}
		
		return err
	}

	// Commit transaction
	if err := uow.Commit(ctx); err != nil {
		// Try to rollback if commit fails
		if rbErr := uow.Rollback(ctx); rbErr != nil {
			logger.Errorf("Failed to rollback after commit error: %v", rbErr)
		}
		return err
	}

	logger.Debug("Unit of work executed successfully")
	return nil
}

// clearTransaction clears the transaction from all repositories
func (uow *UnitOfWork) clearTransaction() {
	for _, repo := range uow.repositories {
		repo.SetTx(nil)
	}
	uow.tx = nil
}

// IsInTransaction returns true if a transaction is active
func (uow *UnitOfWork) IsInTransaction() bool {
	return uow.tx != nil && !uow.committed && !uow.rolledBack
}

// TransactionalRepository provides base functionality for repositories that support transactions
type TransactionalRepository struct {
	db *sql.DB
	tx *sql.Tx
}

// NewTransactionalRepository creates a new transactional repository
func NewTransactionalRepository(db *sql.DB) *TransactionalRepository {
	return &TransactionalRepository{db: db}
}

// GetTx returns the current transaction if any
func (r *TransactionalRepository) GetTx() *sql.Tx {
	return r.tx
}

// SetTx sets the transaction for the repository
func (r *TransactionalRepository) SetTx(tx *sql.Tx) {
	r.tx = tx
}

// GetExecutor returns either the transaction or the database connection
func (r *TransactionalRepository) GetExecutor() interface{} {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

// ExecContext executes a query that doesn't return rows
func (r *TransactionalRepository) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if r.tx != nil {
		return r.tx.ExecContext(ctx, query, args...)
	}
	return r.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows
func (r *TransactionalRepository) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if r.tx != nil {
		return r.tx.QueryContext(ctx, query, args...)
	}
	return r.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns at most one row
func (r *TransactionalRepository) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if r.tx != nil {
		return r.tx.QueryRowContext(ctx, query, args...)
	}
	return r.db.QueryRowContext(ctx, query, args...)
}