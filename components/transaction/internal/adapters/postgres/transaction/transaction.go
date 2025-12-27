// Package transaction provides PostgreSQL adapter implementations for transaction management.
// It contains database models, input/output types, and utilities for storing
// and retrieving financial transaction records and their associated operations.
package transaction

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
)

// Type aliases for backward compatibility with code that imports from this package.
// These aliases point to the canonical types in mmodel package.
type (
	// Transaction is an alias to mmodel.Transaction for backward compatibility
	Transaction = mmodel.Transaction
	// CreateTransactionInput is an alias to mmodel.CreateTransactionInput for backward compatibility
	CreateTransactionInput = mmodel.CreateTransactionInput
	// UpdateTransactionInput is an alias to mmodel.UpdateTransactionInput for backward compatibility
	UpdateTransactionInput = mmodel.UpdateTransactionInput
	// Status is an alias to mmodel.Status for backward compatibility
	Status = mmodel.Status
	// InputDSL is an alias to mmodel.InputDSL for backward compatibility
	InputDSL = mmodel.InputDSL
	// TransactionQueue is an alias to mmodel.TransactionQueue for backward compatibility
	TransactionQueue = mmodel.TransactionQueue
	// CreateTransactionInflowInput is an alias to mmodel.CreateTransactionInflowInput for backward compatibility
	CreateTransactionInflowInput = mmodel.CreateTransactionInflowInput
	// SendInflow is an alias to mmodel.SendInflow for backward compatibility
	SendInflow = mmodel.SendInflow
	// CreateTransactionOutflowInput is an alias to mmodel.CreateTransactionOutflowInput for backward compatibility
	CreateTransactionOutflowInput = mmodel.CreateTransactionOutflowInput
	// SendOutflow is an alias to mmodel.SendOutflow for backward compatibility
	SendOutflow = mmodel.SendOutflow
)

// TransactionPostgreSQLModel represents the entity TransactionPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing transaction information in PostgreSQL
type TransactionPostgreSQLModel struct {
	ID                       string                      // Unique identifier (UUID format)
	ParentTransactionID      *string                     // Parent transaction ID (for reversals or child transactions)
	Description              string                      // Human-readable description
	Status                   string                      // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription        *string                     // Status description
	Amount                   *decimal.Decimal            // Transaction amount value
	AssetCode                string                      // Asset code for the transaction
	ChartOfAccountsGroupName string                      // Chart of accounts group name for accounting
	LedgerID                 string                      // Ledger ID
	OrganizationID           string                      // Organization ID
	Body                     *pkgTransaction.Transaction // Transaction body containing detailed operation data
	CreatedAt                time.Time                   // Creation timestamp
	UpdatedAt                time.Time                   // Last update timestamp
	DeletedAt                sql.NullTime                // Deletion timestamp (if soft-deleted)
	Route                    *string                     // Route
	Metadata                 map[string]any              // Additional custom attributes
}

// ToEntity converts an TransactionPostgreSQLModel to entity mmodel.Transaction
func (t *TransactionPostgreSQLModel) ToEntity() *mmodel.Transaction {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	transaction := &mmodel.Transaction{
		ID:                       t.ID,
		ParentTransactionID:      t.ParentTransactionID,
		Description:              t.Description,
		Status:                   status,
		Amount:                   t.Amount,
		AssetCode:                t.AssetCode,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		LedgerID:                 t.LedgerID,
		OrganizationID:           t.OrganizationID,
		CreatedAt:                t.CreatedAt,
		UpdatedAt:                t.UpdatedAt,
	}

	if t.Body != nil && !t.Body.IsEmpty() {
		transaction.Body = *t.Body
	}

	if t.Route != nil {
		transaction.Route = *t.Route
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		transaction.DeletedAt = &deletedAtCopy
	}

	return transaction
}

// FromEntity converts an entity mmodel.Transaction to TransactionPostgreSQLModel
func (t *TransactionPostgreSQLModel) FromEntity(transaction *mmodel.Transaction) {
	ID := libCommons.GenerateUUIDv7().String()
	if transaction.ID != "" {
		ID = transaction.ID
	}

	*t = TransactionPostgreSQLModel{
		ID:                       ID,
		ParentTransactionID:      transaction.ParentTransactionID,
		Description:              transaction.Description,
		Status:                   transaction.Status.Code,
		StatusDescription:        transaction.Status.Description,
		Amount:                   transaction.Amount,
		AssetCode:                transaction.AssetCode,
		ChartOfAccountsGroupName: transaction.ChartOfAccountsGroupName,
		LedgerID:                 transaction.LedgerID,
		OrganizationID:           transaction.OrganizationID,
		CreatedAt:                transaction.CreatedAt,
		UpdatedAt:                transaction.UpdatedAt,
	}

	if !transaction.Body.IsEmpty() {
		t.Body = &transaction.Body
	}

	if !libCommons.IsNilOrEmpty(&transaction.Route) {
		t.Route = &transaction.Route
	}

	if transaction.DeletedAt != nil {
		deletedAtCopy := *transaction.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// CreateTransactionSwaggerModel is a struct that mirrors CreateTransactionInput but with explicit types for Swagger.
// This is only used for Swagger documentation generation.
//
// swagger:model CreateTransactionSwaggerModel
// @Description Schema for creating transaction with the complete Send operation structure defined inline
//
//	@example {
//	  "chartOfAccountsGroupName": "FUNDING",
//	  "description": "New Transaction",
//	  "code": "TR12345",
//	  "metadata": {
//	    "reference": "TRANSACTION-001",
//	    "source": "api"
//	  },
//	  "route": "00000000-0000-0000-0000-000000000000",
//	  "send": {
//	    "asset": "USD",
//	    "value": "100.00",
//	    "source": {
//	      "from": [
//	        {
//	          "account": "@external/USD",
//	          "amount": {
//	            "asset": "USD",
//	            "value": "100.00"
//	          },
//	          "description": "Debit Operation",
//	          "chartOfAccounts": "FUNDING_DEBIT",
//	          "metadata": {
//	            "operation": "funding",
//	            "type": "external"
//	          }
//	        }
//	      ]
//	    },
//	    "distribute": {
//	      "to": [
//	        {
//	          "account": "{{accountAlias}}",
//	          "amount": {
//	            "asset": "USD",
//	            "value": "100.00"
//	          },
//	          "description": "Credit Operation",
//	          "chartOfAccounts": "FUNDING_CREDIT",
//	          "metadata": {
//	            "operation": "funding",
//	            "type": "account"
//	          }
//	        }
//	      ]
//	    }
//	  }
//	}
type CreateTransactionSwaggerModel struct {
	// Chart of accounts group name for accounting purposes
	// example: FUNDING
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty"`

	// Human-readable description of the transaction
	// example: New Transaction
	// maxLength: 256
	Description string `json:"description,omitempty"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty"`

	// Whether the transaction should be created in pending state
	// example: true
	// swagger: type boolean
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// TransactionDate Period from transaction creation date until now
	// Example "2021-01-01T00:00:00Z"
	// swagger: type string
	// required: false
	TransactionDate *pkgTransaction.TransactionDate `json:"transactionDate,omitempty"`

	// Send operation details including source and distribution
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: "100.00"
		// required: true
		Value string `json:"value"`

		// Source accounts and amounts for the transaction
		// required: true
		Source struct {
			// List of source operations
			// required: true
			From []struct {
				// Account identifier or alias
				// example: @external/USD
				// required: true
				Account string `json:"accountAlias"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: "100.00"
					// required: true
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Debit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: FUNDING_DEBIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Additional metadata
				// example: {"operation": "funding", "type": "external"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"from"`
		} `json:"source"`

		// Destination accounts and amounts for the transaction
		// required: true
		Distribute struct {
			// List of destination operations
			// required: true
			To []struct {
				// Account identifier or alias
				// example: @myAccount
				// required: true
				Account string `json:"accountAlias"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: "100.00"
					// required: true
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Credit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: FUNDING_CREDIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Additional metadata
				// example: {"operation": "funding", "type": "account"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"to"`
		} `json:"distribute"`
	} `json:"send"`
}

// TransactionResponse represents a success response containing a single transaction.
//
// swagger:response TransactionResponse
// @Description Successful response containing a single transaction entity.
type TransactionResponse struct {
	// in: body
	Body mmodel.Transaction
}

// TransactionsResponse represents a success response containing a paginated list of transactions.
//
// swagger:response TransactionsResponse
// @Description Successful response containing a paginated list of transactions.
type TransactionsResponse struct {
	// in: body
	Body struct {
		Items      []mmodel.Transaction `json:"items"`
		Pagination struct {
			Limit      int     `json:"limit"`
			NextCursor *string `json:"next_cursor,omitempty"`
			PrevCursor *string `json:"prev_cursor,omitempty"`
		} `json:"pagination"`
	}
}

//	@example {
//	  "chartOfAccountsGroupName": "FUNDING",
//	  "description": "New Inflow Transaction",
//	  "code": "TR12345",
//	  "metadata": {
//	    "reference": "TRANSACTION-001",
//	    "source": "api"
//	  },
//	  "send": {
//	    "asset": "USD",
//	    "value": "100.00",
//	    "distribute": {
//	      "to": [
//	        {
//	          "account": "{{accountAlias}}",
//	          "amount": {
//	            "asset": "USD",
//	            "value": "100.00",
//	          },
//	          "description": "Credit Operation",
//	          "chartOfAccounts": "FUNDING_CREDIT",
//	          "metadata": {
//	            "operation": "funding",
//	            "type": "account"
//	          }
//	        }
//	      ]
//	    }
//	  }
//	}
//

// CreateTransactionInflowSwaggerModel is a struct that mirrors CreateTransactionInflowInput but with explicit types for Swagger
// This is only used for Swagger documentation generation
//
// swagger:model CreateTransactionInflowSwaggerModel
// @Description Schema for creating inflow transaction with the complete SendInflow operation structure defined inline
type CreateTransactionInflowSwaggerModel struct {
	// Chart of accounts group name for accounting purposes
	// example: FUNDING
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty"`

	// Human-readable description of the transaction
	// example: New Inflow Transaction
	// maxLength: 256
	Description string `json:"description,omitempty"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// TransactionDate Period from transaction creation date until now
	// Example "2021-01-01T00:00:00Z"
	// swagger: type string
	// required: false
	TransactionDate *pkgTransaction.TransactionDate `json:"transactionDate,omitempty"`

	// Send operation details including distribution only
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: "100.00"
		// required: true
		Value string `json:"value"`

		// Destination accounts and amounts for the transaction
		// required: true
		Distribute struct {
			// List of destination operations
			// required: true
			To []struct {
				// Account identifier or alias
				// example: @myAccount
				// required: true
				Account string `json:"accountAlias"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: "100.00"
					// required: true
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Credit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: FUNDING_CREDIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Additional metadata
				// example: {"operation": "funding", "type": "account"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"to"`
		} `json:"distribute"`
	} `json:"send"`
} // @name CreateTransactionInflowSwaggerModel

//	@example {
//	  "chartOfAccountsGroupName": "WITHDRAWAL",
//	  "description": "New Outflow Transaction",
//	  "code": "TR12345",
//	  "metadata": {
//	    "reference": "TRANSACTION-001",
//	    "source": "api"
//	  },
//	  "send": {
//	    "asset": "USD",
//	    "value": "100.00",
//	    "source": {
//	      "from": [
//	        {
//	          "account": "{{accountAlias}}",
//	          "amount": {
//	            "asset": "USD",
//	            "value": "100.00",
//	          },
//	          "description": "Debit Operation",
//	          "chartOfAccounts": "WITHDRAWAL_DEBIT",
//	          "metadata": {
//	            "operation": "withdrawal",
//	            "type": "account"
//	          }
//	        }
//	      ]
//	    }
//	  }
//	}
//

// CreateTransactionOutflowSwaggerModel is a struct that mirrors CreateTransactionOutflowInput but with explicit types for Swagger
// This is only used for Swagger documentation generation
//
// swagger:model CreateTransactionOutflowSwaggerModel
// @Description Schema for creating outflow transaction with the complete SendOutflow operation structure defined inline
type CreateTransactionOutflowSwaggerModel struct {
	// Chart of accounts group name for accounting purposes
	// example: WITHDRAWAL
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty"`

	// Human-readable description of the transaction
	// example: New Outflow Transaction
	// maxLength: 256
	Description string `json:"description,omitempty"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty"`

	// Whether the transaction should be created in pending state
	// example: true
	// swagger: type boolean
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// TransactionDate Period from transaction creation date until now
	// Example "2021-01-01T00:00:00Z"
	// swagger: type string
	// required: false
	TransactionDate *pkgTransaction.TransactionDate `json:"transactionDate,omitempty"`

	// Send operation details including source only
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: "100.00"
		// required: true
		Value string `json:"value"`

		// Source accounts and amounts for the transaction
		// required: true
		Source struct {
			// List of source operations
			// required: true
			From []struct {
				// Account identifier or alias
				// example: @myAccount
				// required: true
				Account string `json:"accountAlias"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: "100.00"
					// required: true
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Debit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: WITHDRAWAL_DEBIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Additional metadata
				// example: {"operation": "withdrawal", "type": "account"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"from"`
		} `json:"source"`
	} `json:"send"`
} // @name CreateTransactionOutflowSwaggerModel
