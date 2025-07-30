package mmodel

import (
	"github.com/shopspring/decimal"
	"testing"
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBalance_IDtoUUID(t *testing.T) {
	tests := []struct {
		name    string
		balance *Balance
		want    uuid.UUID
	}{
		{
			name: "valid UUID",
			balance: &Balance{
				ID: "123e4567-e89b-12d3-a456-426614174000",
			},
			want: uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		},
		{
			name: "valid UUID with different format",
			balance: &Balance{
				ID: "123E4567E89B12D3A456426614174000",
			},
			want: uuid.MustParse("123E4567E89B12D3A456426614174000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.balance.IDtoUUID()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertBalancesToLibBalances(t *testing.T) {
	now := time.Now()
	deletedAt := now.Add(-1 * time.Hour)

	tests := []struct {
		name     string
		balances []*Balance
		want     []*libTransaction.Balance
	}{
		{
			name:     "empty balances",
			balances: []*Balance{},
			want:     []*libTransaction.Balance{},
		},
		{
			name: "single balance",
			balances: []*Balance{
				{
					ID:             "balance-123",
					OrganizationID: "org-123",
					LedgerID:       "ledger-456",
					AccountID:      "account-789",
					Alias:          "alias-1",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(200),
					Version:        1,
					AccountType:    "checking",
					AllowSending:   true,
					AllowReceiving: true,
					CreatedAt:      now,
					UpdatedAt:      now,
					DeletedAt:      nil,
					Metadata:       map[string]any{"key": "value"},
				},
			},
			want: []*libTransaction.Balance{
				{
					ID:             "balance-123",
					OrganizationID: "org-123",
					LedgerID:       "ledger-456",
					AccountID:      "account-789",
					Alias:          "alias-1",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(200),
					Version:        1,
					AccountType:    "checking",
					AllowSending:   true,
					AllowReceiving: true,
					CreatedAt:      now,
					UpdatedAt:      now,
					DeletedAt:      nil,
					Metadata:       map[string]any{"key": "value"},
				},
			},
		},
		{
			name: "multiple balances",
			balances: []*Balance{
				{
					ID:             "balance-123",
					OrganizationID: "org-123",
					LedgerID:       "ledger-456",
					AccountID:      "account-789",
					Alias:          "alias-1",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(200),
					Version:        1,
					AccountType:    "checking",
					AllowSending:   true,
					AllowReceiving: true,
					CreatedAt:      now,
					UpdatedAt:      now,
					DeletedAt:      nil,
					Metadata:       map[string]any{"key": "value"},
				},
				{
					ID:             "balance-456",
					OrganizationID: "org-456",
					LedgerID:       "ledger-789",
					AccountID:      "account-012",
					Alias:          "alias-2",
					AssetCode:      "EUR",
					Available:      decimal.NewFromInt(2000),
					OnHold:         decimal.NewFromInt(300),
					Version:        2,
					AccountType:    "savings",
					AllowSending:   false,
					AllowReceiving: true,
					CreatedAt:      now,
					UpdatedAt:      now,
					DeletedAt:      &deletedAt,
					Metadata:       map[string]any{"key2": "value2"},
				},
			},
			want: []*libTransaction.Balance{
				{
					ID:             "balance-123",
					OrganizationID: "org-123",
					LedgerID:       "ledger-456",
					AccountID:      "account-789",
					Alias:          "alias-1",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(200),
					Version:        1,
					AccountType:    "checking",
					AllowSending:   true,
					AllowReceiving: true,
					CreatedAt:      now,
					UpdatedAt:      now,
					DeletedAt:      nil,
					Metadata:       map[string]any{"key": "value"},
				},
				{
					ID:             "balance-456",
					OrganizationID: "org-456",
					LedgerID:       "ledger-789",
					AccountID:      "account-012",
					Alias:          "alias-2",
					AssetCode:      "EUR",
					Available:      decimal.NewFromInt(2000),
					OnHold:         decimal.NewFromInt(300),
					Version:        2,
					AccountType:    "savings",
					AllowSending:   false,
					AllowReceiving: true,
					CreatedAt:      now,
					UpdatedAt:      now,
					DeletedAt:      &deletedAt,
					Metadata:       map[string]any{"key2": "value2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertBalancesToLibBalances(tt.balances)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBalance_ConvertToLibBalance(t *testing.T) {
	now := time.Now()
	deletedAt := now.Add(-1 * time.Hour)

	tests := []struct {
		name    string
		balance *Balance
		want    *libTransaction.Balance
	}{
		{
			name: "balance with all fields",
			balance: &Balance{
				ID:             "balance-123",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				AccountID:      "account-789",
				Alias:          "alias-1",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(200),
				Version:        1,
				AccountType:    "checking",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      nil,
				Metadata:       map[string]any{"key": "value"},
			},
			want: &libTransaction.Balance{
				ID:             "balance-123",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				AccountID:      "account-789",
				Alias:          "alias-1",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(200),
				Version:        1,
				AccountType:    "checking",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      nil,
				Metadata:       map[string]any{"key": "value"},
			},
		},
		{
			name: "balance with deleted at",
			balance: &Balance{
				ID:             "balance-456",
				OrganizationID: "org-456",
				LedgerID:       "ledger-789",
				AccountID:      "account-012",
				Alias:          "alias-2",
				AssetCode:      "EUR",
				Available:      decimal.NewFromInt(2000),
				OnHold:         decimal.NewFromInt(300),
				Version:        2,
				AccountType:    "savings",
				AllowSending:   false,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      &deletedAt,
				Metadata:       map[string]any{"key2": "value2"},
			},
			want: &libTransaction.Balance{
				ID:             "balance-456",
				OrganizationID: "org-456",
				LedgerID:       "ledger-789",
				AccountID:      "account-012",
				Alias:          "alias-2",
				AssetCode:      "EUR",
				Available:      decimal.NewFromInt(2000),
				OnHold:         decimal.NewFromInt(300),
				Version:        2,
				AccountType:    "savings",
				AllowSending:   false,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      &deletedAt,
				Metadata:       map[string]any{"key2": "value2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.balance.ConvertToLibBalance()
			assert.Equal(t, tt.want, got)
		})
	}
}
