package mmodel

import (
	"encoding/json"
	"fmt"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/google/uuid"
	"strconv"
	"time"
)

// Balance is a struct designed to encapsulate response payload data.
//
// swagger:model Balance
// @Description Balance is a struct designed to store balance data.
type Balance struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	AccountID      string         `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	Alias          string         `json:"alias" example:"@person1"`
	AssetCode      string         `json:"assetCode" example:"BRL"`
	Available      int64          `json:"available" example:"1500"`
	OnHold         int64          `json:"onHold" example:"500"`
	Scale          int64          `json:"scale" example:"2"`
	Version        int64          `json:"version" example:"1"`
	AccountType    string         `json:"accountType" example:"creditCard"`
	AllowSending   bool           `json:"allowSending" example:"true"`
	AllowReceiving bool           `json:"allowReceiving" example:"true"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type UpdateBalance struct {
	AllowSending   *bool `json:"allowSending" example:"true"`
	AllowReceiving *bool `json:"allowReceiving" example:"true"`
}

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (b *Balance) IDtoUUID() uuid.UUID {
	return uuid.MustParse(b.ID)
}

// Balances struct to return get all.
//
// swagger:model Balances
// @Description Balances is the struct designed to return a list of balances with pagination.
type Balances struct {
	Items []Balance `json:"items"`
	Page  int       `json:"page" example:"1"`
	Limit int       `json:"limit" example:"10"`
} // @name Balances

type BalanceRedis struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	AssetCode      string `json:"assetCode"`
	Available      int64  `json:"available"`
	OnHold         int64  `json:"onHold"`
	Scale          int64  `json:"scale"`
	Version        int64  `json:"version"`
	AccountType    string `json:"accountType"`
	AllowSending   int    `json:"allowSending"`
	AllowReceiving int    `json:"allowReceiving"`
}

// ConvertBalancesToLibBalances is a func that convert []*Balance to []*libTransaction.Balance
func ConvertBalancesToLibBalances(balances []*Balance) []*libTransaction.Balance {
	result := make([]*libTransaction.Balance, 0)
	for _, balance := range balances {
		result = append(result, &libTransaction.Balance{
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
			CreatedAt:      balance.CreatedAt,
			UpdatedAt:      balance.UpdatedAt,
			DeletedAt:      balance.DeletedAt,
			Metadata:       balance.Metadata,
		})
	}

	return result
}

// ConvertToLibBalance is a func that convert Balance to libTransaction.Balance
func (b *Balance) ConvertToLibBalance() *libTransaction.Balance {
	return &libTransaction.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Scale:          b.Scale,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
		DeletedAt:      b.DeletedAt,
		Metadata:       b.Metadata,
	}
}

// UnmarshalJSON is a custom unmarshal function for BalanceRedis
func (b *BalanceRedis) UnmarshalJSON(data []byte) error {
	type Alias BalanceRedis

	aux := struct {
		Available interface{} `json:"available"`
		OnHold    interface{} `json:"onHold"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch v := aux.Available.(type) {
	case float64:
		b.Available = int64(v)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("err to converter available field from string to float64: %v", err)
		}

		b.Available = int64(f)
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			f, err := v.Float64()
			if err != nil {
				return fmt.Errorf("err to converter available field from json.Number: %v", err)
			}

			b.Available = int64(f)
		} else {
			b.Available = i
		}
	default:
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("type unsuported to available: %T", v)
		}

		b.Available = int64(f)
	}

	switch v := aux.OnHold.(type) {
	case float64:
		b.OnHold = int64(v)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("err to converter onHold field from string to float64: %v", err)
		}

		b.OnHold = int64(f)
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			f, err := v.Float64()
			if err != nil {
				return fmt.Errorf("err to converter onHold field from json.Number: %v", err)
			}

			b.OnHold = int64(f)
		} else {
			b.OnHold = i
		}
	default:
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("type unsuported to  onHold: %T", v)
		}

		b.OnHold = int64(f)
	}

	return nil
}
