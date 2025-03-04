package mmodel

import (
	"encoding/json"
	"time"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Transaction represents a transaction in the system
type Transaction struct {
	ID                   string         `json:"id"`
	OrganizationID       string         `json:"organizationId"`
	LedgerID             string         `json:"ledgerId"`
	Type                 string         `json:"type"`
	Description          string         `json:"description"`
	Status               interface{}    `json:"status"`               // Can be string or object
	StatusCode           string         `json:"statusCode,omitempty"` // For client use only
	Amount               json.Number    `json:"amount"`               // Changed from string to json.Number to handle both number and string formats
	Currency             string         `json:"currency"`
	SourceAccountID      string         `json:"sourceAccountId"`
	DestinationAccountID string         `json:"destinationAccountId"`
	ParentID             string         `json:"parentId,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            *time.Time     `json:"deletedAt,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// Transactions is a collection of Transaction objects
type Transactions struct {
	Items []Transaction  `json:"items"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
	Data  []*Transaction `json:"data,omitempty"`
}

// CreateTransactionInput represents input for creating a new transaction
type CreateTransactionInput struct {
	ChartOfAccountsGroupName string           `json:"chartOfAccountsGroupName,omitempty"`
	Description              string           `json:"description"`
	Metadata                 map[string]any   `json:"metadata,omitempty"`
	Send                     *TransactionSend `json:"send,omitempty"`
	Idempotency              string           `json:"-"` // Not sent in JSON payload, only in header
}

// TransactionSend represents the send field in a transaction
type TransactionSend struct {
	Asset      string                 `json:"asset"`
	Value      int64                  `json:"value"` // Must be an integer
	Scale      int                    `json:"scale"`
	Source     *TransactionSource     `json:"source"`
	Distribute *TransactionDistribute `json:"distribute"`
}

// TransactionSource represents the source field in a transaction send
type TransactionSource struct {
	From []*TransactionOperation `json:"from"`
}

// TransactionDistribute represents the distribute field in a transaction send
type TransactionDistribute struct {
	To []*TransactionOperation `json:"to"`
}

// TransactionOperation represents an operation in a transaction
type TransactionOperation struct {
	Account         string             `json:"account"`
	Amount          *TransactionAmount `json:"amount"`
	Description     string             `json:"description"`
	ChartOfAccounts string             `json:"chartOfAccounts"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
}

// TransactionAmount represents an amount in a transaction operation
type TransactionAmount struct {
	Asset string `json:"asset"`
	Value int64  `json:"value"` // Must be an integer
	Scale int    `json:"scale"`
}

// UpdateTransactionInput represents input for updating a transaction
type UpdateTransactionInput struct {
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// CreateTransactionDSLInput represents input for creating a transaction using DSL
type CreateTransactionDSLInput struct {
	DSL                  string         `json:"dsl"`
	Description          string         `json:"description,omitempty"`
	ChartOfAccountsGroup string         `json:"chartOfAccountsGroup,omitempty"`
	IdempotencyKey       string         `json:"idempotencyKey,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}
