package model

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Balance represents the API model for a balance
type Balance struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organizationId"`
	LedgerID       string                 `json:"ledgerId"`
	AccountID      string                 `json:"accountId"`
	Alias          string                 `json:"alias"`
	AssetCode      string                 `json:"assetCode"`
	Available      int64                  `json:"available"`
	OnHold         int64                  `json:"onHold"`
	Scale          int64                  `json:"scale"`
	Version        int64                  `json:"version"`
	AccountType    string                 `json:"accountType"`
	AllowSending   bool                   `json:"allowSending"`
	AllowReceiving bool                   `json:"allowReceiving"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
	DeletedAt      *string                `json:"deletedAt,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// AsBalance converts a mmodel.Balance to an API Balance
func AsBalance(balance *mmodel.Balance) *Balance {
	if balance == nil {
		return nil
	}

	var deletedAt *string
	if balance.DeletedAt != nil {
		deletedAtStr := balance.DeletedAt.Format("2006-01-02T15:04:05Z")
		deletedAt = &deletedAtStr
	}

	return &Balance{
		ID:             balance.ID,
		OrganizationID: balance.OrganizationID,
		LedgerID:       balance.LedgerID,
		AccountID:      balance.AccountID,
		Alias:          balance.Alias,
		AssetCode:      balance.AssetCode,
		Available:      balance.Available,
		OnHold:         balance.OnHold,
		Scale:          balance.Scale,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending,
		AllowReceiving: balance.AllowReceiving,
		CreatedAt:      balance.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      balance.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		DeletedAt:      deletedAt,
		Metadata:       balance.Metadata,
	}
}