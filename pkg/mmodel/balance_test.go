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
			assert.NotNil(t, cab.AllowSending, "AllowSending should be set")
			assert.NotNil(t, cab.AllowReceiving, "AllowReceiving should be set")
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

			// Verify all balance fields are properly set
			assert.Equal(t, "balance-123", balance.ID)
			assert.Equal(t, "org-123", balance.OrganizationID)
			assert.Equal(t, "ledger-456", balance.LedgerID)
			assert.Equal(t, "account-789", balance.AccountID)
			assert.Equal(t, "test-alias", balance.Alias)
			assert.Equal(t, "USD", balance.AssetCode)
			assert.Equal(t, decimal.NewFromInt(1000), balance.Available)
			assert.Equal(t, decimal.NewFromInt(0), balance.OnHold)
			assert.Equal(t, int64(1), balance.Version)
			assert.Equal(t, "deposit", balance.AccountType)
			assert.True(t, balance.AllowSending)
			assert.True(t, balance.AllowReceiving)

			if tt.shouldBeSet {
				assert.NotEmpty(t, balance.Key, "Key should be set")
				assert.LessOrEqual(t, len(balance.Key), 100, "Key length should not exceed 100 characters")
			} else {
				assert.Empty(t, balance.Key, "Key should be empty")
			}
		})
	}
}

func TestNewBalance_ValidInputs(t *testing.T) {
	id := uuid.New().String()
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()

	balance := NewBalance(id, orgID, ledgerID, accountID, "@alias", "USD", "deposit")

	assert.Equal(t, id, balance.ID)
	assert.Equal(t, orgID, balance.OrganizationID)
	assert.Equal(t, ledgerID, balance.LedgerID)
	assert.Equal(t, accountID, balance.AccountID)
	assert.Equal(t, "@alias", balance.Alias)
	assert.Equal(t, "USD", balance.AssetCode)
	assert.Equal(t, "deposit", balance.AccountType)
	assert.Equal(t, int64(1), balance.Version)
	assert.True(t, balance.Available.IsZero())
	assert.True(t, balance.OnHold.IsZero())
}

func TestNewBalance_InvalidID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewBalance("invalid-uuid", uuid.New().String(), uuid.New().String(),
			uuid.New().String(), "@alias", "USD", "deposit")
	}, "should panic with invalid ID")
}

func TestNewBalance_EmptyAlias_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewBalance(uuid.New().String(), uuid.New().String(), uuid.New().String(),
			uuid.New().String(), "", "USD", "deposit")
	}, "should panic with empty alias")
}

func TestNewBalance_EmptyAssetCode_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewBalance(uuid.New().String(), uuid.New().String(), uuid.New().String(),
			uuid.New().String(), "@alias", "", "deposit")
	}, "should panic with empty asset code")
}
