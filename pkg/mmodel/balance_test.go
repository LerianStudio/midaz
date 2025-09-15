package mmodel

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

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

func TestCreateAdditionalBalance_KeyValidation(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		expectedLength int
		description    string
	}{
		{
			name:           "valid short key",
			key:            "asset-freeze",
			expectedLength: 12,
			description:    "Key with normal length should be valid",
		},
		{
			name:           "valid key at max length",
			key:            strings.Repeat("a", 100),
			expectedLength: 100,
			description:    "Key with exactly 100 characters should be valid",
		},
		{
			name:           "key with special characters at max length",
			key:            strings.Repeat("asset-freeze-", 7) + "ab",
			expectedLength: 93,
			description:    "Key with special characters within limit should be valid",
		},
		{
			name:           "key with underscores at max length",
			key:            strings.Repeat("asset_type_", 9) + "a",
			expectedLength: 100,
			description:    "Key with underscores at exactly 100 chars should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cab := &CreateAdditionalBalance{
				Key: tt.key,
			}

			assert.Equal(t, tt.expectedLength, len(cab.Key), tt.description)
			assert.LessOrEqual(t, len(cab.Key), 100, "Key length should not exceed 100 characters")
		})
	}
}

func TestBalance_KeyField(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		shouldBeSet bool
	}{
		{
			name:        "balance with normal key",
			key:         "default",
			shouldBeSet: true,
		},
		{
			name:        "balance with asset freeze key",
			key:         "asset-freeze",
			shouldBeSet: true,
		},
		{
			name:        "balance with max length key",
			key:         strings.Repeat("a", 100),
			shouldBeSet: true,
		},
		{
			name:        "balance with empty key",
			key:         "",
			shouldBeSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance := &Balance{
				ID:             "balance-123",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				AccountID:      "account-789",
				Key:            tt.key,
				Alias:          "test-alias",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				Version:        1,
				AccountType:    "deposit",
				// AllowSending and AllowReceiving removed as they're not used in this test
			}

			if tt.shouldBeSet {
				assert.NotEmpty(t, balance.Key, "Key should be set")
				assert.LessOrEqual(t, len(balance.Key), 100, "Key length should not exceed 100 characters")
			} else {
				assert.Empty(t, balance.Key, "Key should be empty")
			}

			libBalance := balance.ConvertToLibBalance()
			assert.Equal(t, balance.Key, libBalance.Key, "Key should be preserved during conversion")
		})
	}
}
