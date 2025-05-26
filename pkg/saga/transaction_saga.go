package saga

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/google/uuid"
)

// TransactionSagaSteps defines the steps for a transaction saga
const (
	StepValidateTransaction = "validate_transaction"
	StepCreateTransaction   = "create_transaction"
	StepCreateOperations    = "create_operations"
	StepUpdateBalances      = "update_balances"
	StepPublishEvents       = "publish_events"
)

// TransactionData holds the data for transaction saga
type TransactionData struct {
	TransactionID  uuid.UUID
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Operations     []interface{}
	Balances       []interface{}
}

// Temporary repository interfaces to fix build issues
// These should be replaced with proper imports when the internal packages are refactored
type TransactionRepository interface {
	// Add methods as needed
}

type OperationRepository interface {
	// Add methods as needed
}

type BalanceRepository interface {
	// Add methods as needed
}

// ValidateTransactionHandler handles transaction validation
type ValidateTransactionHandler struct {
	transactionRepo TransactionRepository
}

// NewValidateTransactionHandler creates a new validation handler
func NewValidateTransactionHandler(repo TransactionRepository) *ValidateTransactionHandler {
	return &ValidateTransactionHandler{transactionRepo: repo}
}

// GetName returns the handler name
func (h *ValidateTransactionHandler) GetName() string {
	return StepValidateTransaction
}

// Execute validates the transaction
func (h *ValidateTransactionHandler) Execute(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Validating transaction", map[string]interface{}{
		"transaction_id": data.TransactionID,
		"organization_id": data.OrganizationID,
		"ledger_id": data.LedgerID,
	})
	
	// Perform validation logic here
	// For example: check if transaction already exists, validate amounts, etc.
	
	return nil
}

// Compensate reverts transaction validation
func (h *ValidateTransactionHandler) Compensate(ctx context.Context, saga *Saga, step *Step) error {
	// No compensation needed for validation
	return nil
}

// CreateTransactionHandler handles transaction creation
type CreateTransactionHandler struct {
	transactionRepo TransactionRepository
}

// NewCreateTransactionHandler creates a new transaction creation handler
func NewCreateTransactionHandler(repo TransactionRepository) *CreateTransactionHandler {
	return &CreateTransactionHandler{transactionRepo: repo}
}

// GetName returns the handler name
func (h *CreateTransactionHandler) GetName() string {
	return StepCreateTransaction
}

// Execute creates the transaction
func (h *CreateTransactionHandler) Execute(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Creating transaction", map[string]interface{}{
		"transaction_id": data.TransactionID,
		"organization_id": data.OrganizationID,
		"ledger_id": data.LedgerID,
	})
	
	// Create transaction in repository
	// transaction := &entity.Transaction{...}
	// err := h.transactionRepo.Create(ctx, transaction)
	
	return nil
}

// Compensate deletes the created transaction
func (h *CreateTransactionHandler) Compensate(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Compensating transaction creation", map[string]interface{}{
		"transaction_id": data.TransactionID,
	})
	
	// Delete the created transaction
	// err := h.transactionRepo.Delete(ctx, data.OrganizationID, data.LedgerID, data.TransactionID)
	
	return nil
}

// CreateOperationsHandler handles operations creation
type CreateOperationsHandler struct {
	operationRepo OperationRepository
}

// NewCreateOperationsHandler creates a new operations creation handler
func NewCreateOperationsHandler(repo OperationRepository) *CreateOperationsHandler {
	return &CreateOperationsHandler{operationRepo: repo}
}

// GetName returns the handler name
func (h *CreateOperationsHandler) GetName() string {
	return StepCreateOperations
}

// Execute creates the operations
func (h *CreateOperationsHandler) Execute(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Creating operations", map[string]interface{}{
		"transaction_id": data.TransactionID,
		"operation_count": len(data.Operations),
	})
	
	// Create operations in repository
	// for _, op := range data.Operations {
	//     err := h.operationRepo.Create(ctx, op)
	// }
	
	return nil
}

// Compensate deletes the created operations
func (h *CreateOperationsHandler) Compensate(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Compensating operations creation", map[string]interface{}{
		"transaction_id": data.TransactionID,
	})
	
	// Delete the created operations
	// err := h.operationRepo.DeleteByTransactionID(ctx, data.TransactionID)
	
	return nil
}

// UpdateBalancesHandler handles balance updates
type UpdateBalancesHandler struct {
	balanceRepo BalanceRepository
}

// NewUpdateBalancesHandler creates a new balance update handler
func NewUpdateBalancesHandler(repo BalanceRepository) *UpdateBalancesHandler {
	return &UpdateBalancesHandler{balanceRepo: repo}
}

// GetName returns the handler name
func (h *UpdateBalancesHandler) GetName() string {
	return StepUpdateBalances
}

// Execute updates the balances
func (h *UpdateBalancesHandler) Execute(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Updating balances", map[string]interface{}{
		"transaction_id": data.TransactionID,
		"balance_count": len(data.Balances),
	})
	
	// Update balances in repository
	// for _, balance := range data.Balances {
	//     err := h.balanceRepo.Update(ctx, balance)
	// }
	
	return nil
}

// Compensate reverts the balance updates
func (h *UpdateBalancesHandler) Compensate(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Compensating balance updates", map[string]interface{}{
		"transaction_id": data.TransactionID,
	})
	
	// Revert balance updates
	// Logic to restore previous balance values
	
	return nil
}

// PublishEventsHandler handles event publishing
type PublishEventsHandler struct {
	// Add event publisher when available
}

// NewPublishEventsHandler creates a new event publishing handler
func NewPublishEventsHandler() *PublishEventsHandler {
	return &PublishEventsHandler{}
}

// GetName returns the handler name
func (h *PublishEventsHandler) GetName() string {
	return StepPublishEvents
}

// Execute publishes transaction events
func (h *PublishEventsHandler) Execute(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	
	// Extract transaction data from saga metadata
	data, ok := saga.Metadata["transaction_data"].(TransactionData)
	if !ok {
		return fmt.Errorf("failed to extract transaction data from saga metadata")
	}
	
	logger.Info("Publishing transaction events", map[string]interface{}{
		"transaction_id": data.TransactionID,
		"organization_id": data.OrganizationID,
	})
	
	// Publish events
	// TODO: Implement event publishing logic when event bus is available
	
	return nil
}

// Compensate for event publishing (typically no compensation needed)
func (h *PublishEventsHandler) Compensate(ctx context.Context, saga *Saga, step *Step) error {
	// No compensation needed for event publishing
	// Events are typically immutable once published
	return nil
}

// BuildTransactionSaga creates a new transaction saga with the coordinator
func BuildTransactionSaga(ctx context.Context, coordinator *Coordinator, data TransactionData) (*Saga, error) {
	// Define the steps for the transaction saga
	steps := []string{
		StepValidateTransaction,
		StepCreateTransaction,
		StepCreateOperations,
		StepUpdateBalances,
		StepPublishEvents,
	}
	
	// Create the saga using the coordinator
	saga, err := coordinator.CreateSaga(ctx, fmt.Sprintf("transaction_%s", data.TransactionID.String()), data.OrganizationID, steps)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction saga: %w", err)
	}
	
	// Store transaction data in saga metadata
	saga.Metadata["transaction_data"] = data
	
	// Save the updated metadata
	if err := coordinator.store.Save(ctx, saga); err != nil {
		return nil, fmt.Errorf("failed to save transaction data to saga: %w", err)
	}
	
	return saga, nil
}

// RegisterTransactionHandlers registers all transaction saga handlers with the coordinator
func RegisterTransactionHandlers(coordinator *Coordinator, transactionRepo TransactionRepository, operationRepo OperationRepository, balanceRepo BalanceRepository) {
	// Register all handlers
	coordinator.RegisterHandler(NewValidateTransactionHandler(transactionRepo))
	coordinator.RegisterHandler(NewCreateTransactionHandler(transactionRepo))
	coordinator.RegisterHandler(NewCreateOperationsHandler(operationRepo))
	coordinator.RegisterHandler(NewUpdateBalancesHandler(balanceRepo))
	coordinator.RegisterHandler(NewPublishEventsHandler())
}