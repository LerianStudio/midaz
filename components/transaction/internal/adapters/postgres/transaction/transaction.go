package transaction

import (
	"database/sql"
	"time"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
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
	Body                     *libTransaction.Transaction // Transaction body containing detailed operation data
	CreatedAt                time.Time                   // Creation timestamp
	UpdatedAt                time.Time                   // Last update timestamp
	DeletedAt                sql.NullTime                // Deletion timestamp (if soft-deleted)
	Route                    *string                     // Route
	Metadata                 map[string]any              // Additional custom attributes
}

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Status is the struct designed to represent the status of a transaction. Contains code and optional description for transaction states.
type Status struct {
	// Status code identifying the state of the transaction
	// example: ACTIVE
	// maxLength: 100
	Code string `json:"code" validate:"max=100" example:"ACTIVE" maxLength:"100"`

	// Optional descriptive text explaining the status
	// example: Active status
	// maxLength: 256
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status" maxLength:"256"`
} // @name Status

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// CreateTransactionInput is a struct design to encapsulate payload data.
//
// swagger:model CreateTransactionInput
// @Description CreateTransactionInput is the input payload to create a transaction. Contains all necessary fields to create a financial transaction, including source and destination information.
type CreateTransactionInput struct {
	// Chart of accounts group name for accounting purposes
	// example: FUNDING
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256" example:"FUNDING"`

	// Human-readable description of the transaction
	// example: New Transaction
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"New Transaction" maxLength:"256"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"TR12345" maxLength:"100"`

	// Whether the transaction should be created in pending state
	// example: true
	// swagger: type boolean
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000" example:"{\"reference\": \"TRANSACTION-001\", \"source\": \"api\"}"`

	//Route
	// example: "00000000-0000-0000-0000-000000000000"
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,valuemax=250" example:"00000000-0000-0000-0000-000000000000"`

	// Send operation details including source and distribution
	// required: true
	// swagger:type object
	Send *libTransaction.Send `json:"send,omitempty" validate:"required,dive"`
} // @name CreateTransactionInput

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
//	    "value": 100,
//	    "scale": 2,
//	    "source": {
//	      "from": [
//	        {
//	          "account": "@external/USD",
//	          "amount": {
//	            "asset": "USD",
//	            "value": 100,
//	            "scale": 2
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
//	            "value": 100,
//	            "scale": 2
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
// CreateTransactionSwagger is a struct that mirrors CreateTransactionInput but with explicit types for Swagger
// This is only used for Swagger documentation generation
//
// swagger:model CreateTransactionSwaggerModel
// @Description Schema for creating transaction with the complete Send operation structure defined inline
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

	// Send operation details including source and distribution
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: 100
		// required: true
		Value decimal.Decimal `json:"value"`

		// Source accounts and amounts for the transaction
		// required: true
		Source struct {
			// List of source operations
			// required: true
			From []struct {
				// Account identifier or alias
				// example: @external/USD
				// required: true
				Account string `json:"account"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: 100
					// required: true
					Value decimal.Decimal `json:"value"`
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
				// example: {{accountAlias}}
				// required: true
				Account string `json:"account"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: 100
					// required: true
					Value decimal.Decimal `json:"value"`
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

// InputDSL is a struct design to encapsulate payload data.
//
// swagger:model InputDSL
// @Description Template-based transaction input for creating transactions from predefined templates with variable substitution.
type InputDSL struct {
	// Transaction type identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	TransactionType uuid.UUID `json:"transactionType" format:"uuid"`

	// Transaction type code for reference
	// example: PAYMENT
	// maxLength: 50
	TransactionTypeCode string `json:"transactionTypeCode" maxLength:"50"`

	// Variables to substitute in the transaction template
	// example: {"amount": 1000, "recipient": "@person2"}
	Variables map[string]any `json:"variables,omitempty"`
}

// UpdateTransactionInput is a struct design to encapsulate payload data.
//
// swagger:model UpdateTransactionInput
// @Description UpdateTransactionInput is the input payload to update a transaction. Contains fields that can be modified after a transaction is created.
type UpdateTransactionInput struct {
	// Human-readable description of the transaction
	// example: Transaction description
	// maxLength: 256
	Description string `json:"description" validate:"max=256" example:"Transaction description" maxLength:"256"`

	// Additional custom attributes
	// example: {"purpose": "Monthly payment", "category": "Utility"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateTransactionInput

// Transaction is a struct designed to encapsulate response payload data.
//
// swagger:model Transaction
// @Description Transaction is a struct designed to store transaction data. Represents a financial transaction that consists of multiple operations affecting account balances, including details about the transaction's status, amounts, and related operations.
type Transaction struct {
	// Unique identifier for the transaction
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Parent transaction identifier (for reversals or child transactions)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ParentTransactionID *string `json:"parentTransactionId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable description of the transaction
	// example: Transaction description
	// maxLength: 256
	Description string `json:"description" example:"Transaction description" maxLength:"256"`

	// Transaction status information
	Status Status `json:"status"`

	// Transaction amount value in the smallest unit of the asset
	// example: 1500
	// minimum: 0
	Amount *decimal.Decimal `json:"amount" example:"1500" minimum:"0"`

	// Asset code for the transaction
	// example: BRL
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"BRL" minLength:"2" maxLength:"10"`

	// Chart of accounts group name for accounting purposes
	// example: Chart of accounts group name
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName" example:"Chart of accounts group name" maxLength:"256"`

	// List of source account aliases or identifiers
	// example: ["@person1"]
	Source []string `json:"source" example:"@person1"`

	// List of destination account aliases or identifiers
	// example: ["@person2"]
	Destination []string `json:"destination" example:"@person2"`

	// Ledger identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Organization identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Transaction body containing detailed operation data (not exposed in JSON)
	Body libTransaction.Transaction `json:"-"`

	//Route
	// example: 00000000-0000-0000-0000-000000000000
	// format: string
	Route string `json:"route" example:"00000000-0000-0000-0000-000000000000" format:"string"`

	// Timestamp when the transaction was created
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the transaction was last updated
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the transaction was deleted (if soft-deleted)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Additional custom attributes
	// example: {"purpose": "Monthly payment", "category": "Utility"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// List of operations associated with this transaction
	Operations []*operation.Operation `json:"operations"`
} // @name Transaction

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (t Transaction) IDtoUUID() uuid.UUID {
	return uuid.MustParse(t.ID)
}

// ToEntity converts an TransactionPostgreSQLModel to entity Transaction
func (t *TransactionPostgreSQLModel) ToEntity() *Transaction {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	transaction := &Transaction{
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

// FromEntity converts an entity Transaction to TransactionPostgreSQLModel
func (t *TransactionPostgreSQLModel) FromEntity(transaction *Transaction) {
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

// FromDSL converts an entity FromDSL to goldModel.Transaction
func (cti *CreateTransactionInput) FromDSL() *libTransaction.Transaction {
	dsl := &libTransaction.Transaction{
		ChartOfAccountsGroupName: cti.ChartOfAccountsGroupName,
		Description:              cti.Description,
		Code:                     cti.Code,
		Pending:                  cti.Pending,
		Metadata:                 cti.Metadata,
		Route:                    cti.Route,
	}

	if cti.Send != nil {
		for i := range cti.Send.Source.From {
			cti.Send.Source.From[i].IsFrom = true
		}

		dsl.Send = *cti.Send
	}

	return dsl
}

// TransactionRevert is a func that revert transaction
func (t Transaction) TransactionRevert() libTransaction.Transaction {
	froms := make([]libTransaction.FromTo, 0)
	tos := make([]libTransaction.FromTo, 0)

	for _, op := range t.Operations {
		switch op.Type {
		case constant.CREDIT:
			from := libTransaction.FromTo{
				IsFrom:       true,
				AccountAlias: op.AccountAlias,
				Amount: &libTransaction.Amount{
					Asset: op.AssetCode,
					Value: *op.Amount.Value,
				},
				Description:     op.Description,
				ChartOfAccounts: op.ChartOfAccounts,
				Metadata:        op.Metadata,
				Route:           op.Route,
			}

			froms = append(froms, from)
		case constant.DEBIT:
			to := libTransaction.FromTo{
				IsFrom:       false,
				AccountAlias: op.AccountAlias,
				Amount: &libTransaction.Amount{
					Asset: op.AssetCode,
					Value: *op.Amount.Value,
				},
				Description:     op.Description,
				ChartOfAccounts: op.ChartOfAccounts,
				Metadata:        op.Metadata,
				Route:           op.Route,
			}

			tos = append(tos, to)
		}
	}

	send := libTransaction.Send{
		Asset: t.AssetCode,
		Value: *t.Amount,
		Source: libTransaction.Source{
			From: froms,
		},
		Distribute: libTransaction.Distribute{
			To: tos,
		},
	}

	transaction := libTransaction.Transaction{
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Description:              t.Description,
		Pending:                  false,
		Metadata:                 t.Metadata,
		Route:                    t.Route,
		Send:                     send,
	}

	return transaction
}

// TransactionQueue this is a struct that is responsible to send and receive from queue.
//
// @Description Container for transaction data exchanged via message queues, including validation responses, balances, and transaction details.
type TransactionQueue struct {
	// Validation responses from the transaction processing
	Validate *libTransaction.Responses `json:"validate"`

	// Account balances affected by the transaction
	Balances []*mmodel.Balance `json:"balances"`

	// The transaction being processed
	Transaction *Transaction `json:"transaction"`

	// Parsed transaction DSL
	ParseDSL *libTransaction.Transaction `json:"parseDSL"`
}

// TransactionResponse represents a success response containing a single transaction.
//
// swagger:response TransactionResponse
// @Description Successful response containing a single transaction entity.
type TransactionResponse struct {
	// in: body
	Body Transaction
}

// TransactionsResponse represents a success response containing a paginated list of transactions.
//
// swagger:response TransactionsResponse
// @Description Successful response containing a paginated list of transactions.
type TransactionsResponse struct {
	// in: body
	Body struct {
		Items      []Transaction `json:"items"`
		Pagination struct {
			Limit      int     `json:"limit"`
			NextCursor *string `json:"next_cursor,omitempty"`
			PrevCursor *string `json:"prev_cursor,omitempty"`
		} `json:"pagination"`
	}
}

// CreateTransactionInflowInput is a struct designed to encapsulate payload data for inflow transactions.
//
// swagger:model CreateTransactionInflowInput
// @Description CreateTransactionInflowInput is the input payload to create an inflow transaction. Contains all necessary fields to create a financial transaction without source information, only destination.
type CreateTransactionInflowInput struct {
	// Chart of accounts group name for accounting purposes
	// example: FUNDING
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256" example:"FUNDING"`

	// Human-readable description of the transaction
	// example: New Transaction
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"New Transaction" maxLength:"256"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"TR12345" maxLength:"100"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000" example:"{\"reference\": \"TRANSACTION-001\", \"source\": \"api\"}"`

	// Transaction route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,valuemax=250" example:"00000000-0000-0000-0000-000000000000"`

	// Send operation details including distribution only (no source)
	// required: true
	// swagger:type object
	Send *SendInflow `json:"send,omitempty" validate:"required,dive"`
} // @name CreateTransactionInflowInput

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
//	    "value": 100,
//	    "scale": 2,
//	    "distribute": {
//	      "to": [
//	        {
//	          "account": "{{accountAlias}}",
//	          "amount": {
//	            "asset": "USD",
//	            "value": 100,
//	            "scale": 2
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

// SendInflow structure for marshaling/unmarshalling JSON for inflow transactions.
//
// swagger:model SendInflow
// @Description SendInflow is the struct designed to represent the sending fields of an inflow operation without source information.
type SendInflow struct {
	Asset      string                    `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value      decimal.Decimal           `json:"value,omitempty" validate:"required" example:"1000"`
	Distribute libTransaction.Distribute `json:"distribute,omitempty" validate:"required"`
} // @name SendInflow

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

	// Send operation details including distribution only
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: 100
		// required: true
		Value int64 `json:"value"`

		// Decimal places for the transaction amount
		// example: 2
		// required: true
		Scale int64 `json:"scale"`

		// Destination accounts and amounts for the transaction
		// required: true
		Distribute struct {
			// List of destination operations
			// required: true
			To []struct {
				// Account identifier or alias
				// example: {{accountAlias}}
				// required: true
				Account string `json:"account"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: 100
					// required: true
					Value int64 `json:"value"`

					// Decimal places
					// example: 2
					// required: true
					Scale int64 `json:"scale"`
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

// InflowFromDSL converts an entity InflowFromDSL to a libTransaction.Transaction
func (c *CreateTransactionInflowInput) InflowFromDSL() *libTransaction.Transaction {
	listFrom := make([]libTransaction.FromTo, 0)

	from := libTransaction.FromTo{
		IsFrom:       true,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &libTransaction.Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	listFrom = append(listFrom, from)

	return &libTransaction.Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Metadata:                 c.Metadata,
		Route:                    c.Route,
		Send: libTransaction.Send{
			Asset:      c.Send.Asset,
			Value:      c.Send.Value,
			Distribute: c.Send.Distribute,
			Source: libTransaction.Source{
				From: listFrom,
			},
		},
	}
}

// CreateTransactionOutflowInput is a struct design to encapsulate payload data for outflow transactions.
//
// swagger:model CreateTransactionOutflowInput
// @Description CreateTransactionOutflowInput is the input payload to create an outflow transaction. Contains all necessary fields to create a financial transaction with source information only, without destination.
type CreateTransactionOutflowInput struct {
	// Chart of accounts group name for accounting purposes
	// example: WITHDRAWAL
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256" example:"WITHDRAWAL"`

	// Human-readable description of the transaction
	// example: New Outflow Transaction
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"New Outflow Transaction" maxLength:"256"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"TR12345" maxLength:"100"`

	// Whether the transaction should be created in pending state
	// example: true
	// swagger: type boolean
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000" example:"{\"reference\": \"TRANSACTION-001\", \"source\": \"api\"}"`

	// Transaction route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,valuemax=250" example:"00000000-0000-0000-0000-000000000000"`

	// Send operation details including source only (no distribution)
	// required: true
	// swagger:type object
	Send *SendOutflow `json:"send,omitempty" validate:"required,dive"`
} // @name CreateTransactionOutflowInput

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
//	    "value": 100,
//	    "scale": 2,
//	    "source": {
//	      "from": [
//	        {
//	          "account": "{{accountAlias}}",
//	          "amount": {
//	            "asset": "USD",
//	            "value": "100",
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

// SendOutflow structure for marshaling/unmarshalling JSON for outflow transactions.
//
// swagger:model SendOutflow
// @Description SendOutflow is the struct designed to represent the sending fields of an outflow operation without distribution information.
type SendOutflow struct {
	Asset  string                `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value  decimal.Decimal       `json:"value,omitempty" validate:"required" example:"1000"`
	Source libTransaction.Source `json:"source,omitempty" validate:"required"`
} // @name SendOutflow

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

	// Send operation details including source only
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: "100"
		// required: true
		Value decimal.Decimal `json:"value"`

		// Source accounts and amounts for the transaction
		// required: true
		Source struct {
			// List of source operations
			// required: true
			From []struct {
				// Account identifier or alias
				// example: {{accountAlias}}
				// required: true
				Account string `json:"account"`

				// Amount details for the operation
				// required: true
				Amount struct {
					// Asset code
					// example: USD
					// required: true
					Asset string `json:"asset"`

					// Amount value in smallest unit
					// example: "100"
					// required: true
					Value decimal.Decimal `json:"value"`
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

// OutflowFromDSL converts an entity OutflowFromDSL to a libTransaction.Transaction
func (c *CreateTransactionOutflowInput) OutflowFromDSL() *libTransaction.Transaction {
	listTo := make([]libTransaction.FromTo, 0)

	to := libTransaction.FromTo{
		IsFrom:       false,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &libTransaction.Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	listTo = append(listTo, to)

	dsl := &libTransaction.Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Pending:                  c.Pending,
		Metadata:                 c.Metadata,
		Route:                    c.Route,
		Send: libTransaction.Send{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
			Distribute: libTransaction.Distribute{
				To: listTo,
			},
		},
	}

	for i := range c.Send.Source.From {
		c.Send.Source.From[i].IsFrom = true
	}

	dsl.Send.Source = c.Send.Source

	return dsl
}
