package account

import (
	"database/sql"
	"github.com/LerianStudio/midaz/common"
	"time"

	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
)

// AccountPostgreSQLModel represents the entity Account into SQL context in Database
type AccountPostgreSQLModel struct {
	ID                string
	Name              string
	ParentAccountID   *string
	EntityID          *string
	AssetCode         string
	OrganizationID    string
	LedgerID          string
	PortfolioID       *string
	ProductID         *string
	AvailableBalance  *float64
	OnHoldBalance     *float64
	BalanceScale      *float64
	Status            string
	StatusDescription *string
	AllowSending      bool
	AllowReceiving    bool
	Alias             *string
	Type              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// CreateAccountInput is a struct design to encapsulate request create payload data.
type CreateAccountInput struct {
	AssetCode       string         `json:"assetCode" validate:"required,max=100"`
	Name            string         `json:"name" validate:"max=256"`
	Alias           *string        `json:"alias" validate:"max=100"`
	Type            string         `json:"type" validate:"required"`
	ParentAccountID *string        `json:"parentAccountId" validate:"omitempty,uuid"`
	ProductID       *string        `json:"productId" validate:"omitempty,uuid"`
	EntityID        *string        `json:"entityId" validate:"omitempty,max=256"`
	Status          Status         `json:"status"`
	Metadata        map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateAccountInput is a struct design to encapsulate request update payload data.
type UpdateAccountInput struct {
	Name      string         `json:"name" validate:"max=256"`
	Status    Status         `json:"status"`
	Alias     *string        `json:"alias" validate:"max=100"`
	ProductID *string        `json:"productId" validate:"uuid"`
	Metadata  map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// SearchAccountsInput is a struct design to encapsulate request search payload data.
type SearchAccountsInput struct {
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid"`
}

// Account is a struct designed to encapsulate response payload data.
type Account struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ParentAccountID *string        `json:"parentAccountId"`
	EntityID        *string        `json:"entityId"`
	AssetCode       string         `json:"assetCode"`
	OrganizationID  string         `json:"organizationId"`
	LedgerID        string         `json:"ledgerId"`
	PortfolioID     *string        `json:"portfolioId"`
	ProductID       *string        `json:"productId"`
	Balance         Balance        `json:"balance"`
	Status          Status         `json:"status"`
	Alias           *string        `json:"alias"`
	Type            string         `json:"type"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       *time.Time     `json:"deletedAt"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code           string  `json:"code" validate:"max=100"`
	Description    *string `json:"description" validate:"omitempty,max=256"`
	AllowSending   *bool   `json:"allowSending"`
	AllowReceiving *bool   `json:"allowReceiving"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// Balance structure for marshaling/unmarshalling JSON.
type Balance struct {
	Available *float64 `json:"available"`
	OnHold    *float64 `json:"onHold"`
	Scale     *float64 `json:"scale"`
}

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}

// ToEntity converts an AccountPostgreSQLModel to a response entity Account
func (t *AccountPostgreSQLModel) ToEntity() *Account {
	status := Status{
		Code:           t.Status,
		Description:    t.StatusDescription,
		AllowSending:   &t.AllowSending,
		AllowReceiving: &t.AllowReceiving,
	}

	balance := Balance{
		Available: t.AvailableBalance,
		OnHold:    t.OnHoldBalance,
		Scale:     t.BalanceScale,
	}

	acc := &Account{
		ID:              t.ID,
		Name:            t.Name,
		ParentAccountID: t.ParentAccountID,
		EntityID:        t.EntityID,
		AssetCode:       t.AssetCode,
		OrganizationID:  t.OrganizationID,
		LedgerID:        t.LedgerID,
		PortfolioID:     t.PortfolioID,
		ProductID:       t.ProductID,
		Balance:         balance,
		Status:          status,
		Alias:           t.Alias,
		Type:            t.Type,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		DeletedAt:       nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		acc.DeletedAt = &deletedAtCopy
	}

	return acc
}

// FromEntity converts a request entity Account to AccountPostgreSQLModel
func (t *AccountPostgreSQLModel) FromEntity(account *Account) {
	*t = AccountPostgreSQLModel{
		ID:                common.GenerateUUIDv7().String(),
		Name:              account.Name,
		ParentAccountID:   account.ParentAccountID,
		EntityID:          account.EntityID,
		AssetCode:         account.AssetCode,
		OrganizationID:    account.OrganizationID,
		LedgerID:          account.LedgerID,
		ProductID:         account.ProductID,
		AvailableBalance:  account.Balance.Available,
		OnHoldBalance:     account.Balance.OnHold,
		BalanceScale:      account.Balance.Scale,
		Status:            account.Status.Code,
		StatusDescription: account.Status.Description,
		Alias:             account.Alias,
		Type:              account.Type,
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
	}

	if account.Status.AllowSending != nil {
		t.AllowSending = *account.Status.AllowSending
	}

	if account.Status.AllowReceiving != nil {
		t.AllowReceiving = *account.Status.AllowReceiving
	}

	if !common.IsNilOrEmpty(account.PortfolioID) {
		t.PortfolioID = account.PortfolioID
	}

	if account.DeletedAt != nil {
		deletedAtCopy := *account.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// ToProto converts entity Account to a response protobuf proto
func (e *Account) ToProto() *proto.Account {
	status := proto.Status{
		Code:           e.Status.Code,
		Description:    *e.Status.Description,
		AllowSending:   *e.Status.AllowSending,
		AllowReceiving: *e.Status.AllowReceiving,
	}

	balance := proto.Balance{
		Available: *e.Balance.Available,
		OnHold:    *e.Balance.OnHold,
		Scale:     *e.Balance.Scale,
	}

	account := &proto.Account{
		Id:             e.ID,
		Name:           e.Name,
		AssetCode:      e.AssetCode,
		OrganizationId: e.OrganizationID,
		LedgerId:       e.LedgerID,
		Balance:        &balance,
		Status:         &status,
		Type:           e.Type,
	}

	if e.ParentAccountID != nil {
		account.ParentAccountId = *e.ParentAccountID
	}

	if e.DeletedAt != nil {
		account.DeletedAt = e.DeletedAt.String()
	}

	if !e.UpdatedAt.IsZero() {
		account.UpdatedAt = e.UpdatedAt.String()
	}

	if !e.CreatedAt.IsZero() {
		account.CreatedAt = e.CreatedAt.String()
	}

	if e.EntityID != nil {
		account.EntityId = *e.EntityID
	}

	if e.PortfolioID != nil {
		account.PortfolioId = *e.PortfolioID
	}

	if e.ProductID != nil {
		account.ProductId = *e.ProductID
	}

	if e.Alias != nil {
		account.Alias = *e.Alias
	}

	return account
}
