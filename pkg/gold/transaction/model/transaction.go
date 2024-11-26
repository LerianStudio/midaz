package model

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance is the struct designed to represent the account balance.
type Balance struct {
	Available int `json:"available" example:"1500"`
	OnHold    int `json:"onHold" example:"500"`
	Scale     int `json:"scale" example:"2"`
} // @name Balance

type Responses struct {
	Total        int
	From         map[string]Amount
	To           map[string]Amount
	Sources      []string
	Destinations []string
	Aliases      []string
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
	Asset string `json:"asset,omitempty" validate:"required,eq=BRL" example:"BRL"`
	Value int    `json:"value,omitempty" validate:"required" example:"1000"`
	Scale int    `json:"scale,omitempty" validate:"gte=0" example:"2"`
} // @name Amount

// Share structure for marshaling/unmarshalling JSON.
//
// swagger:model Share
// @Description Share is the struct designed to represent the sharing fields of an operation.
type Share struct {
	Percentage             int  `json:"percentage,omitempty" validate:"required"`
	PercentageOfPercentage int  `json:"percentageOfPercentage,omitempty"`
	DescWhatever           bool `json:"descWhatever,omitempty"`
} // @name Share

// Send structure for marshaling/unmarshalling JSON.
//
// swagger:model Send
// @Description Send is the struct designed to represent the sending fields of an operation.
type Send struct {
	Asset  string `json:"asset,omitempty" validate:"required,eq=BRL" example:"BRL"`
	Value  int    `json:"value,omitempty" validate:"required" example:"1000"`
	Scale  int    `json:"scale,omitempty" validate:"gte=0" example:"2"`
	Source Source `json:"source,omitempty" validate:"required"`
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
	Account         string         `json:"account,omitempty" example:"@person1"`
	Amount          *Amount        `json:"amount,omitempty"`
	Share           *Share         `json:"share,omitempty"`
	Remaining       string         `json:"remaining,omitempty" example:"remaining"`
	Description     string         `json:"description,omitempty" example:"description"`
	ChartOfAccounts string         `json:"chartOfAccountsG" example:"1000"`
	Metadata        map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	IsFrom          bool           `json:"isFrom,omitempty" example:"true"`
} // @name FromTo

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
// swagger:model Transaction
// @Description Transaction is a struct designed to store transaction data.
type Transaction struct {
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName,omitempty" example:"1000"`
	Description              string         `json:"description,omitempty" example:"Description"`
	Code                     string         `json:"code,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Pending                  bool           `json:"pending,omitempty" example:"false"`
	Metadata                 map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	Send                     Send           `json:"send" validate:"required"`
	Distribute               Distribute     `json:"distribute" validate:"required"`
} // @name Transaction
