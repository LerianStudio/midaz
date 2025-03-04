package model

import (
	"encoding/json"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// Account represents a source or destination account in a transaction
type Account struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
}

// Transaction represents the API model for a transaction
type Transaction struct {
	ID                   string                 `json:"id"`
	OrganizationID       string                 `json:"organizationId"`
	LedgerID             string                 `json:"ledgerId"`
	Type                 string                 `json:"type"`
	Description          string                 `json:"description"`
	Status               TransactionStatus      `json:"status"`
	Amount               json.Number            `json:"amount"`
	AmountScale          int64                  `json:"amountScale"`
	Currency             string                 `json:"currency"`
	SourceAccounts       []Account              `json:"sourceAccounts,omitempty"`
	DestinationAccounts  []Account              `json:"destinationAccounts,omitempty"`
	Template             string                 `json:"template,omitempty"`
	ChartOfAccountsGroup string                 `json:"chartOfAccountsGroup,omitempty"`
	ParentID             string                 `json:"parentId,omitempty"`
	IdempotencyKey       string                 `json:"idempotencyKey,omitempty"`
	CreatedAt            time.Time              `json:"createdAt"`
	UpdatedAt            time.Time              `json:"updatedAt"`
	DeletedAt            *time.Time             `json:"deletedAt,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
}

// AsTransaction converts a mmodel.Transaction to an API Transaction
func AsTransaction(tx *mmodel.Transaction) *Transaction {
	if tx == nil {
		return nil
	}

	// Default status code if tx.Status is a string
	statusCode := tx.Status
	var statusDescription *string

	// Create source and destination account arrays
	var sourceAccounts []Account
	if tx.SourceAccountID != "" {
		sourceAccounts = append(sourceAccounts, Account{
			ID: tx.SourceAccountID,
			// Alias would be populated from operations if available
		})
	}

	var destinationAccounts []Account
	if tx.DestinationAccountID != "" {
		destinationAccounts = append(destinationAccounts, Account{
			ID: tx.DestinationAccountID,
			// Alias would be populated from operations if available
		})
	}

	// Default scale
	amountScale := int64(2)

	return &Transaction{
		ID:             tx.ID,
		OrganizationID: tx.OrganizationID,
		LedgerID:       tx.LedgerID,
		Type:           tx.Type,
		Description:    tx.Description,
		Status: TransactionStatus{
			Code:        statusCode.(string),
			Description: statusDescription,
		},
		Amount:              tx.Amount,
		AmountScale:         amountScale,
		Currency:            tx.Currency,
		SourceAccounts:      sourceAccounts,
		DestinationAccounts: destinationAccounts,
		ParentID:            tx.ParentID,
		CreatedAt:           tx.CreatedAt,
		UpdatedAt:           tx.UpdatedAt,
		DeletedAt:           tx.DeletedAt,
		Metadata:            tx.Metadata,
	}
}
