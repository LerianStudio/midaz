package model

type Balance struct {
	Available int `json:"available"`
	OnHold    int `json:"onHold"`
	Scale     int `json:"scale"`
}

type Responses struct {
	Total        int
	From         map[string]Amount
	To           map[string]Amount
	Sources      []string
	Destinations []string
	Aliases      []string
}

type Metadata struct {
	Key   string `json:"key,omitempty"`
	Value any    `json:"value,omitempty"`
}

type Amount struct {
	Asset string `json:"asset,omitempty" validate:"required"`
	Value int    `json:"value,omitempty" validate:"required"`
	Scale int    `json:"scale,omitempty" validate:"required,gte=0"`
}

type Share struct {
	Percentage             int  `json:"percentage,omitempty" validate:"required"`
	PercentageOfPercentage int  `json:"percentageOfPercentage,omitempty"`
	DescWhatever           bool `json:"descWhatever,omitempty"`
}

type Send struct {
	Asset  string `json:"asset,omitempty" validate:"required"`
	Value  int    `json:"value,omitempty" validate:"required"`
	Scale  int    `json:"scale,omitempty" validate:"required,gte=0"`
	Source Source `json:"source,omitempty" validate:"required"`
}

type Source struct {
	Remaining string   `json:"remaining,omitempty"`
	From      []FromTo `json:"from,omitempty" validate:"singletransactiontype,required,dive"`
}

type FromTo struct {
	Account         string         `json:"account,omitempty"`
	Amount          *Amount        `json:"amount,omitempty"`
	Share           *Share         `json:"share,omitempty"`
	Remaining       string         `json:"remaining,omitempty"`
	Description     string         `json:"description,omitempty"`
	ChartOfAccounts string         `json:"chartOfAccountsG"`
	Metadata        map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	IsFrom          bool           `json:"isFrom,omitempty"`
}

type Distribute struct {
	Remaining string   `json:"remaining,omitempty"`
	To        []FromTo `json:"to,omitempty" validate:"singletransactiontype,required,dive"`
}

type Transaction struct {
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName,omitempty"`
	Description              string         `json:"description,omitempty"`
	Code                     string         `json:"code,omitempty"`
	Pending                  bool           `json:"pending,omitempty"`
	Metadata                 map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	Send                     Send           `json:"send" validate:"required"`
	Distribute               Distribute     `json:"distribute" validate:"required"`
}
