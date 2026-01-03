// Package operation provides PostgreSQL adapter implementations for operation management.
// It contains database models, input/output types, and utilities for storing
// and retrieving financial operation records that affect account balances.
package operation

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// Type aliases for backward compatibility with code that imports from this package.
// These aliases point to the canonical types in mmodel package.
type (
	// Operation is an alias to mmodel.Operation for backward compatibility
	Operation = mmodel.Operation
	// UpdateOperationInput is an alias to mmodel.UpdateOperationInput for backward compatibility
	UpdateOperationInput = mmodel.UpdateOperationInput
	// Status is an alias to mmodel.Status for backward compatibility
	Status = mmodel.Status
	// Amount is an alias to mmodel.OperationAmount for backward compatibility
	Amount = mmodel.OperationAmount
	// Balance is an alias to mmodel.OperationBalance for backward compatibility
	Balance = mmodel.OperationBalance
	// OperationLog is an alias to mmodel.OperationLog for backward compatibility
	OperationLog = mmodel.OperationLog
)

// OperationPostgreSQLModel represents the entity OperationPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing operation information in PostgreSQL
type OperationPostgreSQLModel struct {
	ID                    string           // Unique identifier (UUID format)
	TransactionID         string           // Parent transaction ID
	Description           string           // Operation description
	Type                  string           // Operation type (e.g., "DEBIT", "CREDIT")
	AssetCode             string           // Asset code for the operation
	Amount                *decimal.Decimal // Operation amount value
	AvailableBalance      *decimal.Decimal // Available balance before operation
	OnHoldBalance         *decimal.Decimal // On-hold balance before operation
	VersionBalance        *int64           // Balance version before operation
	AvailableBalanceAfter *decimal.Decimal // Available balance after operation
	OnHoldBalanceAfter    *decimal.Decimal // On-hold balance after operation
	VersionBalanceAfter   *int64           // Balance version after operation
	Status                string           // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription     *string          // Status description
	AccountID             string           // Account ID associated with operation
	AccountAlias          string           // Account alias
	BalanceKey            string           // Balance key for additional balances
	BalanceID             string           // Balance ID affected by operation
	ChartOfAccounts       string           // Chart of accounts code
	OrganizationID        string           // Organization ID
	LedgerID              string           // Ledger ID
	CreatedAt             time.Time        // Creation timestamp
	UpdatedAt             time.Time        // Last update timestamp
	DeletedAt             sql.NullTime     // Deletion timestamp (if soft-deleted)
	Route                 *string          // Route
	BalanceAffected       bool             // BalanceAffected default true
	Metadata              map[string]any   // Additional custom attributes
}

// ToEntity converts an OperationPostgreSQLModel to entity Operation
func (t *OperationPostgreSQLModel) ToEntity() *mmodel.Operation {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	amount := mmodel.OperationAmount{
		Value: t.Amount,
	}

	balance := mmodel.OperationBalance{
		Available: t.AvailableBalance,
		OnHold:    t.OnHoldBalance,
		Version:   t.VersionBalance,
	}

	balanceAfter := mmodel.OperationBalance{
		Available: t.AvailableBalanceAfter,
		OnHold:    t.OnHoldBalanceAfter,
		Version:   t.VersionBalanceAfter,
	}

	op := &mmodel.Operation{
		ID:              t.ID,
		TransactionID:   t.TransactionID,
		Description:     t.Description,
		Type:            t.Type,
		AssetCode:       t.AssetCode,
		ChartOfAccounts: t.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		Status:          status,
		AccountID:       t.AccountID,
		AccountAlias:    t.AccountAlias,
		BalanceKey:      t.BalanceKey,
		LedgerID:        t.LedgerID,
		OrganizationID:  t.OrganizationID,
		BalanceAffected: t.BalanceAffected,
		BalanceID:       t.BalanceID,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		DeletedAt:       nil,
	}

	if t.Route != nil {
		op.Route = *t.Route
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		op.DeletedAt = &deletedAtCopy
	}

	return op
}

// FromEntity converts an entity Operation to OperationPostgreSQLModel
func (t *OperationPostgreSQLModel) FromEntity(operation *mmodel.Operation) {
	ID := libCommons.GenerateUUIDv7().String()
	if operation.ID != "" {
		ID = operation.ID
	}

	balanceKey := operation.BalanceKey
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	*t = OperationPostgreSQLModel{
		ID:                    ID,
		TransactionID:         operation.TransactionID,
		Description:           operation.Description,
		Type:                  operation.Type,
		AssetCode:             operation.AssetCode,
		ChartOfAccounts:       operation.ChartOfAccounts,
		Amount:                operation.Amount.Value,
		OnHoldBalance:         operation.Balance.OnHold,
		AvailableBalance:      operation.Balance.Available,
		VersionBalance:        operation.Balance.Version,
		AvailableBalanceAfter: operation.BalanceAfter.Available,
		OnHoldBalanceAfter:    operation.BalanceAfter.OnHold,
		VersionBalanceAfter:   operation.BalanceAfter.Version,
		Status:                operation.Status.Code,
		StatusDescription:     operation.Status.Description,
		AccountID:             operation.AccountID,
		AccountAlias:          operation.AccountAlias,
		BalanceKey:            balanceKey,
		BalanceID:             operation.BalanceID,
		LedgerID:              operation.LedgerID,
		OrganizationID:        operation.OrganizationID,
		CreatedAt:             operation.CreatedAt,
		UpdatedAt:             operation.UpdatedAt,
		BalanceAffected:       operation.BalanceAffected,
	}

	if !libCommons.IsNilOrEmpty(&operation.Route) {
		t.Route = &operation.Route
	}

	if operation.DeletedAt != nil {
		deletedAtCopy := *operation.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
