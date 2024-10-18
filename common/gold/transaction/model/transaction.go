package model

type Metadata struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type Amount struct {
	Asset string `json:"asset,omitempty"`
	Value string `json:"value,omitempty"`
	Scale string `json:"scale,omitempty"`
}

type Share struct {
	Percentage             int  `json:"percentage,omitempty"`
	PercentageOfPercentage int  `json:"percentageOfPercentage,omitempty"`
	DescWhatever           bool `json:"descWhatever,omitempty"`
}

type Send struct {
	Asset  string `json:"asset,omitempty"`
	Value  string `json:"value,omitempty"`
	Scale  string `json:"scale,omitempty"`
	Source Source `json:"source,omitempty"`
}

type Source struct {
	Remaining string   `json:"remaining,omitempty"`
	From      []FromTo `json:"from,omitempty"`
}

type FromTo struct {
	Account         string         `json:"account,omitempty"`
	Amount          *Amount        `json:"amount,omitempty"`
	Share           *Share         `json:"share,omitempty"`
	Remaining       string         `json:"remaining,omitempty"`
	Description     string         `json:"description,omitempty"`
	ChartOfAccounts string         `json:"chartOfAccountsG"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	IsFrom          bool           `json:"isFrom,omitempty"`
}

type Distribute struct {
	Remaining string   `json:"remaining,omitempty"`
	To        []FromTo `json:"to,omitempty"`
}

type Transaction struct {
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName"`
	Description              string         `json:"description,omitempty"`
	Code                     string         `json:"code,omitempty"`
	Pending                  bool           `json:"pending,omitempty"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
	Send                     Send           `json:"send,omitempty"`
	Distribute               Distribute     `json:"distribute,omitempty"`
}
