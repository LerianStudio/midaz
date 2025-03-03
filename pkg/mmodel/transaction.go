package mmodel

import "time"

// Transaction represents a transaction in the system
type Transaction struct {
	ID                  string         `json:"id"`
	OrganizationID      string         `json:"organizationId"`
	LedgerID            string         `json:"ledgerId"`
	Type                string         `json:"type"`
	Description         string         `json:"description"`
	Status              string         `json:"status"`
	Amount              string         `json:"amount"`
	Currency            string         `json:"currency"`
	SourceAccountID     string         `json:"sourceAccountId"`
	DestinationAccountID string        `json:"destinationAccountId"`
	ParentID            string         `json:"parentId,omitempty"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           *time.Time     `json:"deletedAt,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// Transactions is a collection of Transaction objects
type Transactions struct {
	Items []Transaction `json:"items"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
	Data  []*Transaction `json:"data,omitempty"`
}

// CreateTransactionInput represents input for creating a new transaction
type CreateTransactionInput struct {
	Type                string         `json:"type"`
	Description         string         `json:"description"`
	Status              string         `json:"status,omitempty"`
	Amount              string         `json:"amount,omitempty"`
	Currency            string         `json:"currency,omitempty"`
	SourceAccountID     string         `json:"sourceAccountId"`
	DestinationAccountID string        `json:"destinationAccountId"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// UpdateTransactionInput represents input for updating a transaction
type UpdateTransactionInput struct {
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}