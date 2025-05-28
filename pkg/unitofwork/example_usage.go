package unitofwork

// This file contains example usage that depends on internal transaction repositories
// Temporarily commented out to fix build issues

/*
import (
	"context"
	"database/sql"

	"github.com/LerianStudio/midaz/components/transaction/internal/domain/repository"
	"github.com/google/uuid"
)

// Example usage of Unit of Work pattern with transaction services
type TransactionService struct {
	uow           *UnitOfWork
	transactionRepo repository.TransactionRepository
	operationRepo   repository.OperationRepository
	balanceRepo     repository.BalanceRepository
}

// NewTransactionService creates a new transaction service with unit of work
func NewTransactionService(db *sql.DB, 
	transactionRepo repository.TransactionRepository,
	operationRepo repository.OperationRepository,
	balanceRepo repository.BalanceRepository) *TransactionService {
	
	uow := New(db)
	
	// Register repositories with unit of work
	uow.RegisterRepository("transaction", transactionRepo)
	uow.RegisterRepository("operation", operationRepo)
	uow.RegisterRepository("balance", balanceRepo)
	
	return &TransactionService{
		uow:             uow,
		transactionRepo: transactionRepo,
		operationRepo:   operationRepo,
		balanceRepo:     balanceRepo,
	}
}

// CreateTransactionWithOperations demonstrates using unit of work for complex operations
func (s *TransactionService) CreateTransactionWithOperations(ctx context.Context, 
	organizationID, ledgerID uuid.UUID,
	transaction interface{}, 
	operations []interface{}) error {
	
	// Execute all operations within a single transaction
	return s.uow.Execute(ctx, func(ctx context.Context, uow *UnitOfWork) error {
		// All repository operations here will use the same database transaction
		
		// 1. Create the transaction
		// transactionRepo.Create() will use the transaction from unit of work
		
		// 2. Create all operations
		// for each operation, operationRepo.Create() uses the same transaction
		
		// 3. Update balances
		// balanceRepo.Update() uses the same transaction
		
		// If any operation fails, all changes will be rolled back automatically
		// If all operations succeed, all changes will be committed together
		
		return nil
	})
}

// Example of manual transaction control
func (s *TransactionService) ManualTransactionExample(ctx context.Context) error {
	// Begin transaction
	if err := s.uow.Begin(ctx); err != nil {
		return err
	}
	
	// Ensure cleanup
	defer func() {
		if !s.uow.IsInTransaction() {
			return
		}
		s.uow.Rollback(ctx)
	}()
	
	// Perform operations...
	
	// Commit if all successful
	return s.uow.Commit(ctx)
}
*/