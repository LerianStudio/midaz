package operation

import "time"

type Operation struct {
	ID              string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	TransactionID   string         `json:"transactionId" example:"00000000-0000-0000-0000-000000000000"`
	Description     string         `json:"description" example:"Credit card operation"`
	Type            string         `json:"type" example:"creditCard"`
	AssetCode       string         `json:"assetCode" example:"BRL"`
	ChartOfAccounts string         `json:"chartOfAccounts" example:"1000"`
	Amount          Amount         `json:"amount"`
	Balance         Balance        `json:"balance"`
	BalanceAfter    Balance        `json:"balanceAfter"`
	Status          Status         `json:"status"`
	AccountID       string         `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	AccountAlias    string         `json:"accountAlias" example:"@person1"`
	PortfolioID     *string        `json:"portfolioId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID  string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID        string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt       time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt       time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt       *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata        map[string]any `json:"metadata"`
}

type Amount struct {
	Amount *float64 `json:"amount" example:"1500"`
	Scale  *float64 `json:"scale" example:"2"`
}

type Balance struct {
	Available *float64 `json:"available" example:"1500"`
	OnHold    *float64 `json:"onHold" example:"500"`
	Scale     *float64 `json:"scale" example:"2"`
}

type Status struct {
	Code        string  `json:"code" validate:"max=100" example:"ACTIVE"`
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status"`
}
