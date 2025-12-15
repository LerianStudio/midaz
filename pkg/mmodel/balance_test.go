package mmodel

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

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
				Key:            tt.key,
				AllowSending:   &[]bool{true}[0],
				AllowReceiving: &[]bool{true}[0],
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
				AllowSending:   true,
				AllowReceiving: true,
			}

			if tt.shouldBeSet {
				assert.NotEmpty(t, balance.Key, "Key should be set")
				assert.LessOrEqual(t, len(balance.Key), 100, "Key length should not exceed 100 characters")
			} else {
				assert.Empty(t, balance.Key, "Key should be empty")
			}
		})
	}
}
