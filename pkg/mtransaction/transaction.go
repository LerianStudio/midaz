// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"

	"github.com/shopspring/decimal"
)

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance is the struct designed to represent the account balance.
type Balance struct {
	ID             string          `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string          `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string          `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	AccountID      string          `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	Alias          string          `json:"alias" example:"@person1"`
	Key            string          `json:"key" example:"asset-freeze"`
	AssetCode      string          `json:"assetCode" example:"BRL"`
	Available      decimal.Decimal `json:"available" example:"1500"`
	OnHold         decimal.Decimal `json:"onHold" example:"500"`
	Version        int64           `json:"version" example:"1"`
	AccountType    string          `json:"accountType" example:"creditCard"`
	AllowSending   bool            `json:"allowSending" example:"true"`
	AllowReceiving bool            `json:"allowReceiving" example:"true"`
	// Direction is the accounting direction of the balance ("credit" or
	// "debit"). Empty string denotes a legacy balance that predates the
	// overdraft feature and is treated as "credit" by the engine.
	Direction string `json:"direction,omitempty" example:"credit"`
	// OverdraftUsed is the amount of overdraft currently consumed by the
	// balance. Always non-negative. Zero when the balance is in the black.
	OverdraftUsed decimal.Decimal `json:"overdraftUsed" example:"0"`
	// AllowOverdraft enables overdraft behavior when true.
	AllowOverdraft bool `json:"allowOverdraft"`
	// OverdraftLimitEnabled gates the OverdraftLimit value. When false,
	// the limit is zero (unlimited if AllowOverdraft is true).
	OverdraftLimitEnabled bool `json:"overdraftLimitEnabled"`
	// OverdraftLimit is the maximum overdraft amount as a decimal.
	// Only meaningful when OverdraftLimitEnabled is true.
	//
	// Type asymmetry note: this field is decimal.Decimal for runtime
	// arithmetic (comparison, subtraction in ValidateOverdraftLimit). The
	// corresponding BalanceSettings.OverdraftLimit is *string to preserve
	// JSON precision across marshal/unmarshal cycles. ToTransactionBalance()
	// in pkg/mmodel/balance.go bridges the two: it parses the *string into
	// a decimal.Decimal, returning an error on malformed values.
	OverdraftLimit decimal.Decimal `json:"overdraftLimit"`
	// BalanceScope is the balance scope ("transactional" or "internal").
	// Empty string is treated as "transactional" for backward compatibility.
	BalanceScope string         `json:"balanceScope"`
	CreatedAt    time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt    time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt    *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata     map[string]any `json:"metadata,omitempty"`
} // @name Balance

type Responses struct {
	Total               decimal.Decimal
	Asset               string
	From                map[string]Amount
	To                  map[string]Amount
	Sources             []string
	Destinations        []string
	Aliases             []string
	Pending             bool
	TransactionRoute    string
	TransactionRouteID  *string
	OperationRoutesFrom map[string]string
	OperationRoutesTo   map[string]string
}

// Metadata structure for marshaling/unmarshalling JSON.
//
// swagger:model Metadata
// @Description Metadata is the struct designed to store metadata.
type Metadata struct {
	Key   string `json:"key,omitempty"`
	Value any    `json:"value,omitempty"`
} // @name Metadata

// Amount structure for marshaling/unmarshalling JSON.
//
// swagger:model Amount
// @Description Amount is the struct designed to represent the amount of an operation.
type Amount struct {
	Asset string          `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value decimal.Decimal `json:"value,omitempty" validate:"required" example:"1000"`
	// Internal fields — populated during transaction processing, not part of the API contract.
	Operation              string `json:"operation,omitempty" swaggerignore:"true"`
	TransactionType        string `json:"transactionType,omitempty" swaggerignore:"true"`
	Direction              string `json:"direction,omitempty" swaggerignore:"true"`
	RouteValidationEnabled bool   `json:"routeValidationEnabled,omitempty" swaggerignore:"true"`
} // @name Amount

// Share structure for marshaling/unmarshalling JSON.
//
// swagger:model Share
// @Description Share is the struct designed to represent the sharing fields of an operation.
type Share struct {
	Percentage             int64 `json:"percentage,omitempty" validate:"required"`
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty"`
} // @name Share

// Send structure for marshaling/unmarshalling JSON.
//
// swagger:model Send
// @Description Send is the struct designed to represent the sending fields of an operation.
type Send struct {
	Asset      string          `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value      decimal.Decimal `json:"value,omitempty" validate:"required" example:"1000"`
	Source     Source          `json:"source,omitempty" validate:"required"`
	Distribute Distribute      `json:"distribute,omitempty" validate:"required"`
} // @name Send

// Source structure for marshaling/unmarshalling JSON.
//
// swagger:model Source
// @Description Source is the struct designed to represent the source fields of an operation.
type Source struct {
	Remaining string   `json:"remaining,omitempty" example:"remaining"`
	From      []FromTo `json:"from,omitempty" validate:"singletransactiontype,required,dive"`
} // @name Source

// Rate structure for marshaling/unmarshalling JSON.
//
// swagger:model Rate
// @Description Rate is the struct designed to represent the rate fields of an operation.
type Rate struct {
	From       string          `json:"from" validate:"required" example:"BRL"`
	To         string          `json:"to" validate:"required" example:"USDe"`
	Value      decimal.Decimal `json:"value" validate:"required" example:"1000"`
	ExternalID string          `json:"externalId" validate:"uuid,required" example:"00000000-0000-0000-0000-000000000000"`
} // @name Rate

// IsEmpty method that set empty or nil in fields
func (r Rate) IsEmpty() bool {
	return r.ExternalID == "" && r.From == "" && r.To == "" && r.Value.IsZero()
}

// FromTo structure for marshaling/unmarshalling JSON.
//
// swagger:model FromTo
// @Description FromTo is the struct designed to represent the from/to fields of an operation.
type FromTo struct {
	AccountAlias    string         `json:"accountAlias,omitempty" example:"@person1"`
	BalanceKey      string         `json:"balanceKey,omitempty" example:"asset-freeze"`
	Amount          *Amount        `json:"amount,omitempty"`
	Share           *Share         `json:"share,omitempty"`
	Remaining       string         `json:"remaining,omitempty" example:"remaining"`
	Rate            *Rate          `json:"rate,omitempty"`
	Description     string         `json:"description,omitempty" example:"description"`
	ChartOfAccounts string         `json:"chartOfAccounts" example:"1000"`
	Metadata        map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	IsFrom          bool           `json:"isFrom,omitempty" example:"true"`
	// Deprecated: passive field kept for backward compatibility. Accepted from client and persisted, but not used in any validation or business logic. Use routeId instead.
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000"`
	// UUID of the operation route. Primary field used for route validation and accounting rules.
	// format: uuid
	RouteID *string `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
} // @name FromTo

// SplitAlias function to split alias with index.
func (ft FromTo) SplitAlias() string {
	if strings.Contains(ft.AccountAlias, "#") {
		return strings.Split(ft.AccountAlias, "#")[1]
	}

	return ft.AccountAlias
}

// SplitAliasWithKey extracts the substring after the '#' character from the provided alias or returns the alias if '#' is not present.
func SplitAliasWithKey(alias string) string {
	if idx := strings.Index(alias, "#"); idx != -1 {
		return alias[idx+1:]
	}

	return alias
}

// ConcatAlias builds a composite key from the entry's index, alias, and balance key.
// If BalanceKey is empty, it defaults to "default" to stay consistent with AliasKey.
func (ft FromTo) ConcatAlias(i int) string {
	balanceKey := ft.BalanceKey
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	return strconv.Itoa(i) + "#" + ft.AccountAlias + "#" + balanceKey
}

// isConcatedAlias returns true if the alias is already in the composite
// "index#alias#balanceKey" format (starts with a digit followed by #).
func isConcatedAlias(alias string) bool {
	if len(alias) < 2 {
		return false
	}

	for i, c := range alias {
		if c == '#' {
			return i > 0
		}

		if c < '0' || c > '9' {
			return false
		}
	}

	return false
}

// MutateConcatAliases rewrites each entry's AccountAlias IN-PLACE to the
// composite "index#alias#balanceKey" format. Entries that are already concat'd
// are left untouched (idempotent).
//
// The in-place mutation is intentional: ValidateSendSourceAndDistribute (called
// after this function) reads the mutated AccountAlias as map keys in
// Responses.From/To, and buildBalanceOperations depends on those keys being
// in concat format for balance resolution.
func MutateConcatAliases(entries []FromTo) []FromTo {
	result := make([]FromTo, 0, len(entries))

	for i := range entries {
		if !isConcatedAlias(entries[i].AccountAlias) {
			entries[i].AccountAlias = entries[i].ConcatAlias(i)
		}

		result = append(result, entries[i])
	}

	return result
}

// MutateSplitAliases restores clean aliases IN-PLACE by stripping the index
// prefix added by MutateConcatAliases. Entries that are not concat'd are left
// untouched (idempotent). Called after ValidateSendSourceAndDistribute has
// consumed the concat'd keys.
func MutateSplitAliases(entries []FromTo) []FromTo {
	result := make([]FromTo, 0, len(entries))

	for i := range entries {
		if isConcatedAlias(entries[i].AccountAlias) {
			entries[i].AccountAlias = entries[i].SplitAlias()
		}

		result = append(result, entries[i])
	}

	return result
}

// ApplyDefaultBalanceKeys sets the balance key to "default" for any entry
// where the caller did not specify one.
func ApplyDefaultBalanceKeys(entries []FromTo) {
	for i := range entries {
		if entries[i].BalanceKey == "" {
			entries[i].BalanceKey = constant.DefaultBalanceKey
		}
	}
}

// Distribute structure for marshaling/unmarshalling JSON.
//
// swagger:model Distribute
// @Description Distribute is the struct designed to represent the distribution fields of an operation.
type Distribute struct {
	Remaining string   `json:"remaining,omitempty"`
	To        []FromTo `json:"to,omitempty" validate:"singletransactiontype,required,dive"`
} // @name Distribute

// Transaction structure for marshaling/unmarshalling JSON.
//
// swagger:model TransactionInput
// @Description TransactionInput is the request payload for creating a transaction.
type Transaction struct {
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName,omitempty" example:"FUNDING"`
	Description              string         `json:"description,omitempty" example:"Description"`
	Code                     string         `json:"code,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Pending                  bool           `json:"pending,omitempty" example:"false"`
	Metadata                 map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	// Deprecated: legacy route identifier, contains the transaction route UUID as a string. Use routeId instead.
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000"`
	// UUID of the transaction route. Primary field replacing the deprecated Route string.
	// format: uuid
	RouteID         *string          `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	TransactionDate *TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z"`
	Send            Send             `json:"send" validate:"required"`
} // @name TransactionInput

// InitialStatus returns the transaction status derived from the Pending flag.
// PENDING when the transaction is held for later commit/cancel, CREATED otherwise.
// Callers may override this for special cases (e.g. NOTED for annotations).
func (t Transaction) InitialStatus() string {
	if t.Pending {
		return constant.PENDING
	}

	return constant.CREATED
}

// IsEmpty is a func that validate if transaction is Empty.
func (t Transaction) IsEmpty() bool {
	return t.Send.Asset == "" && t.Send.Value.IsZero()
}
