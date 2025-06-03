package mmodel

import (
	cn "github.com/LerianStudio/midaz/pkg/constant"
	"github.com/shopspring/decimal"
	"strconv"
	"strings"
)

// Transaction structure for marshaling/unmarshalling JSON.
//
// swagger:model Transaction
// @Description Transaction is a struct designed to receive transaction data from apis.
type Transaction struct {
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName,omitempty" example:"1000"`
	Description              string         `json:"description,omitempty" example:"Description"`
	Code                     string         `json:"code,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Pending                  bool           `json:"pending,omitempty" example:"false"`
	Metadata                 map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	Route                    string         `json:"route,omitempty" validate:"omitempty,valuemax=250" example:"00000000-0000-0000-0000-000000000000"`
	Send                     Send           `json:"send" validate:"required"`
} // @name Transaction

// Send structure for marshaling/unmarshalling JSON.
//
// swagger:model Send
// @Description Send is the struct designed to represent the sending fields of an operation.
type Send struct {
	Asset      string     `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value      string     `json:"value,omitempty" validate:"required" example:"1000"`
	Source     Source     `json:"source,omitempty" validate:"required"`
	Distribute Distribute `json:"distribute,omitempty" validate:"required"`
} // @name Send

// Source structure for marshaling/unmarshalling JSON.
//
// swagger:model Source
// @Description Source is the struct designed to represent the source fields of an operation.
type Source struct {
	Remaining string   `json:"remaining,omitempty" example:"remaining"`
	From      []FromTo `json:"from,omitempty" validate:"singletransactiontype,required,dive"`
} // @name Source

// FromTo structure for marshaling/unmarshalling JSON.
//
// swagger:model FromTo
// @Description FromTo is the struct designed to represent the from/to fields of an operation.
type FromTo struct {
	AccountAlias    string         `json:"accountAlias,omitempty" example:"@person1"`
	Amount          *Amount        `json:"amount,omitempty"`
	Share           *Share         `json:"share,omitempty"`
	Remaining       string         `json:"remaining,omitempty" example:"remaining"`
	Rate            *Rate          `json:"rate,omitempty"`
	Description     string         `json:"description,omitempty" example:"description"`
	ChartOfAccounts string         `json:"chartOfAccounts" example:"1000"`
	Metadata        map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	IsFrom          bool           `json:"isFrom,omitempty" example:"true"`
	Route           string         `json:"route,omitempty" validate:"omitempty,valuemax=250" example:"00000000-0000-0000-0000-000000000000"`
} // @name FromTo

// Amount structure for marshaling/unmarshalling JSON.
//
// swagger:model Amount
// @Description Amount is the struct designed to represent the amount of an operation.
type Amount struct {
	Asset     string `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value     string `json:"value,omitempty" validate:"required" example:"1000"`
	Operation string `json:"operation,omitempty"`
} // @name Amount

// Share structure for marshaling/unmarshalling JSON.
//
// swagger:model Share
// @Description Share is the struct designed to represent the sharing fields of an operation.
type Share struct {
	Percentage             int64 `json:"percentage,omitempty" validate:"required"`
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty"`
} // @name Share

// Rate structure for marshaling/unmarshalling JSON.
//
// swagger:model Rate
// @Description Rate is the struct designed to represent the rate fields of an operation.
type Rate struct {
	From       string `json:"from" validate:"required" example:"BRL"`
	To         string `json:"to" validate:"required" example:"USDe"`
	Value      string `json:"value" validate:"required" example:"1000"`
	ExternalID string `json:"externalId" validate:"uuid,required" example:"00000000-0000-0000-0000-000000000000"`
} // @name Rate

// Distribute structure for marshaling/unmarshalling JSON.
//
// swagger:model Distribute
// @Description Distribute is the struct designed to represent the distribution fields of an operation.
type Distribute struct {
	Remaining string   `json:"remaining,omitempty"`
	To        []FromTo `json:"to,omitempty" validate:"singletransactiontype,required,dive"`
} // @name Distribute

// Metadata structure for marshaling/unmarshalling JSON.
//
// swagger:model Metadata
// @Description Metadata is the struct designed to store metadata.
type Metadata struct {
	Key   string `json:"key,omitempty"`
	Value any    `json:"value,omitempty"`
} // @name Metadata

// IsEmpty is a func that validate if transaction is Empty.
func (t Transaction) IsEmpty() bool {
	return t.Send.Asset == "" && t.Send.Value == ""
}

// IsValid validates if the transaction values are balanced between source and destination amounts
func (t Transaction) IsValid() bool {
	if t.IsEmpty() {
		return false
	}

	sourceTotal := decimal.NewFromInt(0)
	for _, from := range t.Send.Source.From {
		if from.Amount.Value != "" {
			value, err := decimal.NewFromString(from.Amount.Value)
			if err != nil {
				return false
			}
			sourceTotal = sourceTotal.Add(value)
		}
	}

	destinationTotal := decimal.NewFromInt(0)
	for _, to := range t.Send.Distribute.To {
		if to.Amount.Value != "" {
			value, err := decimal.NewFromString(to.Amount.Value)
			if err != nil {
				return false
			}
			destinationTotal = destinationTotal.Add(value)
		}
	}

	sendValue, err := decimal.NewFromString(t.Send.Value)
	if err != nil {
		return false
	}

	return sourceTotal.Equal(sendValue) && destinationTotal.Equal(sendValue) && sourceTotal.Equal(destinationTotal)
}

// SplitAlias function to split alias with index.
func (ft FromTo) SplitAlias() string {
	if strings.Contains(ft.AccountAlias, "#") {
		return strings.Split(ft.AccountAlias, "#")[1]
	}

	return ft.AccountAlias
}

// ConcatAlias function to concat alias with index.
func (ft FromTo) ConcatAlias(i int) string {
	return strconv.Itoa(i) + "#" + ft.AccountAlias
}

// IsEmpty method that set empty or nil in fields
func (r Rate) IsEmpty() bool {
	return r.ExternalID == "" && r.From == "" && r.To == "" && r.Value == ""
}

// Responses structure for marshaling/unmarshalling JSON.
//
// swagger:model Responses
// @Description Responses is the struct designed to represent the responses fields of an operation.
type Responses struct {
	Total        string
	Asset        string
	From         map[string]Amount
	To           map[string]Amount
	Sources      []string
	Destinations []string
	Aliases      []string
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
	// swagger:ignore
	Pending bool `json:"pending,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000" example:"{\"reference\": \"TRANSACTION-001\", \"source\": \"api\"}"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000" maxLength:"250"`

	// Send operation details including source and distribution
	// required: true
	// swagger:type object
	Send Send `json:"send,omitempty" validate:"required"`
} // @name CreateTransactionInput

//		@example {
//		  "chartOfAccountsGroupName": "FUNDING",
//		  "description": "New Transaction",
//		  "code": "TR12345",
//		  "metadata": {
//		    "reference": "TRANSACTION-001",
//		    "source": "api"
//		  },
//		  "route": "00000000-0000-0000-0000-000000000000",
//		  "send": {
//		    "asset": "USD",
//		    "value": 100,
//		    "scale": 2,
//		    "source": {
//		      "from": [
//		        {
//		          "account": "@external/USD",
//		          "amount": {
//		            "asset": "USD",
//		            "value": "100",
//		          },
//		          "description": "Debit Operation",
//		          "chartOfAccounts": "FUNDING_DEBIT",
//		          "metadata": {
//		            "operation": "funding",
//		            "type": "external"
//		          },
//	         "route": "00000000-0000-0000-0000-000000000000"
//		        }
//		      ]
//		    },
//		    "distribute": {
//		      "to": [
//		        {
//		          "account": "{{accountAlias}}",
//		          "amount": {
//		            "asset": "USD",
//		            "value": "100",
//		          },
//		          "description": "Credit Operation",
//		          "chartOfAccounts": "FUNDING_CREDIT",
//		          "metadata": {
//		            "operation": "funding",
//		            "type": "account"
//		          },
//		          "route": "00000000-0000-0000-0000-000000000000"
//		        }
//		      ]
//		    }
//		  }
//		}
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
	// swagger:ignore
	Pending bool `json:"pending,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty"`

	// Send operation details including source and distribution
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: "100"
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
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Debit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: FUNDING_DEBIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Route
				// example: 00000000-0000-0000-0000-000000000000
				// maxLength: 250
				Route string `json:"route,omitempty"`

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
					// example: "100"
					// required: true
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Credit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: FUNDING_CREDIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Route
				// example: 00000000-0000-0000-0000-000000000000
				// maxLength: 250
				Route string `json:"route,omitempty"`

				// Additional metadata
				// example: {"operation": "funding", "type": "account"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"to"`
		} `json:"distribute"`
	} `json:"send"`
} // @name CreateTransactionSwaggerModel

// CreateTransactionInflowInput is a struct design to encapsulate payload data for inflow transactions.
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

	// Whether the transaction should be created in pending state
	// swagger:ignore
	Pending bool `json:"pending,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000" example:"{\"reference\": \"TRANSACTION-001\", \"source\": \"api\"}"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000" maxLength:"250"`

	// Send operation details including distribution only (no source)
	// required: true
	// swagger:type object
	Send SendInflow `json:"send,omitempty" validate:"required"`
} // @name CreateTransactionInflowInput
//
//	@example {
//	  "chartOfAccountsGroupName": "FUNDING",
//	  "description": "New Inflow Transaction",
//	  "code": "TR12345",
//	  "metadata": {
//	    "reference": "TRANSACTION-001",
//	    "source": "api"
//	  },
//	  "route": "00000000-0000-0000-0000-000000000000",
//	  "send": {
//	    "asset": "USD",
//	    "value": "100",
//	    "distribute": {
//	      "to": [
//	        {
//	          "account": "{{accountAlias}}",
//	          "amount": {
//	            "asset": "USD",
//	            "value": "100,
//	          },
//	          "description": "Credit Operation",
//	          "chartOfAccounts": "FUNDING_CREDIT",
//	          "metadata": {
//	            "operation": "funding",
//	            "type": "account"
//	          },
//	          "route": "00000000-0000-0000-0000-000000000000"
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
	Asset      string     `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value      string     `json:"value,omitempty" validate:"required" example:"1000"`
	Distribute Distribute `json:"distribute,omitempty" validate:"required"`
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

	// Whether the transaction should be created in pending state
	// swagger:ignore
	Pending bool `json:"pending,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty"`

	// Send operation details including distribution only
	// required: true
	Send struct {
		// Asset code for the transaction
		// example: USD
		// required: true
		Asset string `json:"asset"`

		// Transaction amount value in the smallest unit of the asset
		// example: "100"
		// required: true
		Value string `json:"value"`

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
					// example: "100"
					// required: true
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Credit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: FUNDING_CREDIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Route
				// example: 00000000-0000-0000-0000-000000000000
				// maxLength: 250
				Route string `json:"route,omitempty"`

				// Additional metadata
				// example: {"operation": "funding", "type": "account"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"to"`
		} `json:"distribute"`
	} `json:"send"`
} // @name CreateTransactionInflowSwaggerModel

// TransactionFromInflowInput converts an entity TransactionFromInflowInput to a Transaction
func (c *CreateTransactionInflowInput) TransactionFromInflowInput() *Transaction {
	listFrom := make([]FromTo, 0)

	from := FromTo{
		IsFrom:       true,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	listFrom = append(listFrom, from)

	return &Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Pending:                  c.Pending,
		Metadata:                 c.Metadata,
		Route:                    c.Route,
		Send: Send{
			Asset:      c.Send.Asset,
			Value:      c.Send.Value,
			Distribute: c.Send.Distribute,
			Source: Source{
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
	// swagger:ignore
	Pending bool `json:"pending,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000" example:"{\"reference\": \"TRANSACTION-001\", \"source\": \"api\"}"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,valuemax=250" example:"00000000-0000-0000-0000-000000000000"`

	// Send operation details including source only (no distribution)
	// required: true
	// swagger:type object
	Send SendOutflow `json:"send,omitempty" validate:"required"`
} // @name CreateTransactionOutflowInput

//	@example {
//	  "chartOfAccountsGroupName": "WITHDRAWAL",
//	  "description": "New Outflow Transaction",
//	  "code": "TR12345",
//	  "metadata": {
//	    "reference": "TRANSACTION-001",
//	    "source": "api"
//	  },
//	  "route": "00000000-0000-0000-0000-000000000000",
//	  "send": {
//	    "asset": "USD",
//	    "value": "100",
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
//	          },
//	          "route": "00000000-0000-0000-0000-000000000000"
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
	Asset  string `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value  string `json:"value,omitempty" validate:"required" example:"1000"`
	Source Source `json:"source,omitempty" validate:"required"`
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
	// swagger:ignore
	Pending bool `json:"pending,omitempty"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty"`

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
		Value string `json:"value"`

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
					Value string `json:"value"`
				} `json:"amount"`

				// Operation description
				// example: Debit Operation
				Description string `json:"description,omitempty"`

				// Chart of accounts code
				// example: WITHDRAWAL_DEBIT
				ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

				// Route
				// example: 00000000-0000-0000-0000-000000000000
				// maxLength: 250
				Route string `json:"route,omitempty"`

				// Additional metadata
				// example: {"operation": "withdrawal", "type": "account"}
				Metadata map[string]any `json:"metadata,omitempty"`
			} `json:"from"`
		} `json:"source"`
	} `json:"send"`
} // @name CreateTransactionOutflowSwaggerModel

// TransactionOutflowFromInput converts an entity TransactionOutflowFromInput to a Transaction
func (c *CreateTransactionOutflowInput) TransactionOutflowFromInput() *Transaction {
	listTo := make([]FromTo, 0)

	to := FromTo{
		IsFrom:       false,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	listTo = append(listTo, to)

	dsl := &Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Pending:                  c.Pending,
		Metadata:                 c.Metadata,
		Route:                    c.Route,
		Send: Send{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
			Distribute: Distribute{
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

// TransactionResponse represents a success response containing a single transaction.
//
// swagger:response TransactionResponse
// @Description Successful response containing a single transaction entity.
type TransactionResponse struct {
	// in: body
	Body Transaction
}

// TransactionQueue this is a struct that is responsible to send and receive from queue.
//
// @Description Container for transaction data exchanged via message queues, including validation responses, balances, and transaction details.
type TransactionQueue struct {
	// Validation responses from the transaction processing
	Validate *Responses `json:"validate"`

	// Account balances affected by the transaction
	Balances []*Balance `json:"balances"`

	// The transaction being processed
	Transaction *Transaction `json:"transaction"`

	// Parsed transaction DSL
	ParseDSL *Transaction `json:"parseDSL"`
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

// FromDSL converts an entity FromDSL to goldModel.Transaction
func (cti *CreateTransactionInput) FromDSL() *Transaction {
	dsl := &Transaction{
		ChartOfAccountsGroupName: cti.ChartOfAccountsGroupName,
		Description:              cti.Description,
		Code:                     cti.Code,
		Pending:                  cti.Pending,
		Metadata:                 cti.Metadata,
	}

	dsl.Send = cti.Send

	return dsl
}
