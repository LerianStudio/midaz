// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"strings"
	"testing"
	"time"

	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalance_IDtoUUID(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			got := tt.balance.IDtoUUID()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateAdditionalBalance_KeyValidation(t *testing.T) {
	t.Parallel()
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
			t.Parallel()

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
	t.Parallel()
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
			t.Parallel()

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

func TestBalance_ToTransactionBalance(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	deletedAt := now.Add(time.Hour)

	tests := []struct {
		name    string
		balance *Balance
		want    *pkgTransaction.Balance
	}{
		{
			name: "all fields populated",
			balance: &Balance{
				ID:             "123e4567-e89b-12d3-a456-426614174000",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				AccountID:      "account-789",
				Alias:          "@person1",
				Key:            "asset-freeze",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1500),
				OnHold:         decimal.NewFromInt(500),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
				CreatedAt:      now,
				UpdatedAt:      now.Add(time.Minute),
				DeletedAt:      &deletedAt,
				Metadata:       map[string]any{"purpose": "savings"},
			},
			want: &pkgTransaction.Balance{
				ID:             "123e4567-e89b-12d3-a456-426614174000",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				AccountID:      "account-789",
				Alias:          "@person1",
				Key:            "asset-freeze",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1500),
				OnHold:         decimal.NewFromInt(500),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
				CreatedAt:      now,
				UpdatedAt:      now.Add(time.Minute),
				DeletedAt:      &deletedAt,
				Metadata:       map[string]any{"purpose": "savings"},
			},
		},
		{
			name: "nil DeletedAt",
			balance: &Balance{
				ID:             "223e4567-e89b-12d3-a456-426614174001",
				OrganizationID: "org-456",
				LedgerID:       "ledger-789",
				AccountID:      "account-012",
				Alias:          "@person2",
				Key:            "default",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(2500.50),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "creditCard",
				AllowSending:   false,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      nil,
				Metadata:       map[string]any{"category": "personal"},
			},
			want: &pkgTransaction.Balance{
				ID:             "223e4567-e89b-12d3-a456-426614174001",
				OrganizationID: "org-456",
				LedgerID:       "ledger-789",
				AccountID:      "account-012",
				Alias:          "@person2",
				Key:            "default",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(2500.50),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "creditCard",
				AllowSending:   false,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      nil,
				Metadata:       map[string]any{"category": "personal"},
			},
		},
		{
			name: "nil metadata",
			balance: &Balance{
				ID:             "323e4567-e89b-12d3-a456-426614174002",
				OrganizationID: "org-789",
				LedgerID:       "ledger-012",
				AccountID:      "account-345",
				Alias:          "",
				Key:            "",
				AssetCode:      "EUR",
				Available:      decimal.NewFromInt(0),
				OnHold:         decimal.NewFromInt(100),
				Version:        10,
				AccountType:    "checking",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      nil,
				Metadata:       nil,
			},
			want: &pkgTransaction.Balance{
				ID:             "323e4567-e89b-12d3-a456-426614174002",
				OrganizationID: "org-789",
				LedgerID:       "ledger-012",
				AccountID:      "account-345",
				Alias:          "",
				Key:            "",
				AssetCode:      "EUR",
				Available:      decimal.NewFromInt(0),
				OnHold:         decimal.NewFromInt(100),
				Version:        10,
				AccountType:    "checking",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      nil,
				Metadata:       nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.balance.ToTransactionBalance()

			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.OrganizationID, got.OrganizationID)
			assert.Equal(t, tt.want.LedgerID, got.LedgerID)
			assert.Equal(t, tt.want.AccountID, got.AccountID)
			assert.Equal(t, tt.want.Alias, got.Alias)
			assert.Equal(t, tt.want.Key, got.Key)
			assert.Equal(t, tt.want.AssetCode, got.AssetCode)
			assert.True(t, tt.want.Available.Equal(got.Available), "Available mismatch: want %s, got %s", tt.want.Available, got.Available)
			assert.True(t, tt.want.OnHold.Equal(got.OnHold), "OnHold mismatch: want %s, got %s", tt.want.OnHold, got.OnHold)
			assert.Equal(t, tt.want.Version, got.Version)
			assert.Equal(t, tt.want.AccountType, got.AccountType)
			assert.Equal(t, tt.want.AllowSending, got.AllowSending)
			assert.Equal(t, tt.want.AllowReceiving, got.AllowReceiving)
			assert.Equal(t, tt.want.CreatedAt, got.CreatedAt)
			assert.Equal(t, tt.want.UpdatedAt, got.UpdatedAt)
			assert.Equal(t, tt.want.DeletedAt, got.DeletedAt)
			assert.Equal(t, tt.want.Metadata, got.Metadata)
		})
	}
}

func TestBalanceRedis_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantAvail  decimal.Decimal
		wantOnHold decimal.Decimal
		wantErr    bool
		errContain string
	}{
		{
			name:       "float64 values",
			input:      `{"id":"bal-1","accountId":"acc-1","assetCode":"USD","available":1500.50,"onHold":250.25,"version":1,"accountType":"deposit","allowSending":1,"allowReceiving":0,"key":"default","alias":"@test"}`,
			wantAvail:  decimal.NewFromFloat(1500.50),
			wantOnHold: decimal.NewFromFloat(250.25),
			wantErr:    false,
		},
		{
			name:       "string values",
			input:      `{"id":"bal-2","accountId":"acc-2","assetCode":"BRL","available":"2500.99","onHold":"100.01","version":2,"accountType":"checking","allowSending":1,"allowReceiving":1,"key":"reserve","alias":"@person"}`,
			wantAvail:  decimal.RequireFromString("2500.99"),
			wantOnHold: decimal.RequireFromString("100.01"),
			wantErr:    false,
		},
		{
			name:       "integer values as float64",
			input:      `{"id":"bal-3","accountId":"acc-3","assetCode":"EUR","available":1000,"onHold":0,"version":1,"accountType":"savings","allowSending":0,"allowReceiving":1,"key":"","alias":""}`,
			wantAvail:  decimal.NewFromInt(1000),
			wantOnHold: decimal.NewFromInt(0),
			wantErr:    false,
		},
		{
			name:       "zero values",
			input:      `{"id":"bal-4","accountId":"acc-4","assetCode":"GBP","available":0,"onHold":0,"version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"key":"default","alias":""}`,
			wantAvail:  decimal.Zero,
			wantOnHold: decimal.Zero,
			wantErr:    false,
		},
		{
			name:       "large decimal string values",
			input:      `{"id":"bal-5","accountId":"acc-5","assetCode":"JPY","available":"999999999999.99999999","onHold":"123456789.12345678","version":5,"accountType":"reserve","allowSending":1,"allowReceiving":1,"key":"large","alias":"@large"}`,
			wantAvail:  decimal.RequireFromString("999999999999.99999999"),
			wantOnHold: decimal.RequireFromString("123456789.12345678"),
			wantErr:    false,
		},
		{
			name:       "invalid available string",
			input:      `{"id":"bal-err","accountId":"acc-err","assetCode":"USD","available":"not-a-number","onHold":"100","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"key":"","alias":""}`,
			wantErr:    true,
			errContain: "available",
		},
		{
			name:       "invalid onHold string",
			input:      `{"id":"bal-err2","accountId":"acc-err2","assetCode":"USD","available":"100","onHold":"invalid","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"key":"","alias":""}`,
			wantErr:    true,
			errContain: "onHold",
		},
		{
			name:       "invalid JSON structure",
			input:      `{"id":"bal-err3","available":}`,
			wantErr:    true,
			errContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var br BalanceRedis
			err := br.UnmarshalJSON([]byte(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)
			assert.True(t, tt.wantAvail.Equal(br.Available), "Available mismatch: want %s, got %s", tt.wantAvail, br.Available)
			assert.True(t, tt.wantOnHold.Equal(br.OnHold), "OnHold mismatch: want %s, got %s", tt.wantOnHold, br.OnHold)
		})
	}
}

func TestBalanceRedis_UnmarshalJSON_OtherFields(t *testing.T) {
	t.Parallel()
	input := `{
		"id": "balance-uuid-123",
		"alias": "@merchant",
		"accountId": "account-uuid-456",
		"assetCode": "USD",
		"available": 5000,
		"onHold": 1000,
		"version": 42,
		"accountType": "merchant",
		"allowSending": 1,
		"allowReceiving": 0,
		"key": "merchant-reserve"
	}`

	var br BalanceRedis
	err := br.UnmarshalJSON([]byte(input))

	require.NoError(t, err)
	assert.Equal(t, "balance-uuid-123", br.ID)
	assert.Equal(t, "@merchant", br.Alias)
	assert.Equal(t, "account-uuid-456", br.AccountID)
	assert.Equal(t, "USD", br.AssetCode)
	assert.True(t, decimal.NewFromInt(5000).Equal(br.Available))
	assert.True(t, decimal.NewFromInt(1000).Equal(br.OnHold))
	assert.Equal(t, int64(42), br.Version)
	assert.Equal(t, "merchant", br.AccountType)
	assert.Equal(t, 1, br.AllowSending)
	assert.Equal(t, 0, br.AllowReceiving)
	assert.Equal(t, "merchant-reserve", br.Key)
}
