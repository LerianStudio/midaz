package model

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
)

// OperationStatus represents the status of an operation
type OperationStatus struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// Amount represents an amount with a scale
type Amount struct {
	Amount *int64 `json:"amount"`
	Scale  *int64 `json:"scale"`
}

// BalanceOperation represents a balance state for an operation
type BalanceOperation struct {
	Available *int64 `json:"available"`
	OnHold    *int64 `json:"onHold"`
	Scale     *int64 `json:"scale"`
}

// Operation represents the API model for an operation
type Operation struct {
	ID              string                 `json:"id"`
	TransactionID   string                 `json:"transactionId"`
	Description     string                 `json:"description"`
	Type            string                 `json:"type"`
	AssetCode       string                 `json:"assetCode"`
	ChartOfAccounts string                 `json:"chartOfAccounts"`
	Amount          Amount                 `json:"amount"`
	Balance         BalanceOperation       `json:"balance"`
	BalanceAfter    BalanceOperation       `json:"balanceAfter"`
	Status          OperationStatus        `json:"status"`
	AccountID       string                 `json:"accountId"`
	AccountAlias    string                 `json:"accountAlias"`
	BalanceID       string                 `json:"balanceId"`
	OrganizationID  string                 `json:"organizationId"`
	LedgerID        string                 `json:"ledgerId"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
	DeletedAt       *time.Time             `json:"deletedAt,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// AsOperation converts a mmodel.Operation to an API Operation
func AsOperation(operation *mmodel.Operation) *Operation {
	if operation == nil {
		return nil
	}

	return &Operation{
		ID:              operation.ID,
		TransactionID:   operation.TransactionID,
		Description:     operation.Description,
		Type:            operation.Type,
		AssetCode:       operation.AssetCode,
		ChartOfAccounts: operation.ChartOfAccounts,
		Amount: Amount{
			Amount: operation.Amount.Amount,
			Scale:  operation.Amount.Scale,
		},
		Balance: BalanceOperation{
			Available: operation.Balance.Available,
			OnHold:    operation.Balance.OnHold,
			Scale:     operation.Balance.Scale,
		},
		BalanceAfter: BalanceOperation{
			Available: operation.BalanceAfter.Available,
			OnHold:    operation.BalanceAfter.OnHold,
			Scale:     operation.BalanceAfter.Scale,
		},
		Status: OperationStatus{
			Code:        operation.Status.Code,
			Description: operation.Status.Description,
		},
		AccountID:      operation.AccountID,
		AccountAlias:   operation.AccountAlias,
		BalanceID:      operation.BalanceID,
		OrganizationID: operation.OrganizationID,
		LedgerID:       operation.LedgerID,
		CreatedAt:      operation.CreatedAt,
		UpdatedAt:      operation.UpdatedAt,
		DeletedAt:      operation.DeletedAt,
		Metadata:       operation.Metadata,
	}
}