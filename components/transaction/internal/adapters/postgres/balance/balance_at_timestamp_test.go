// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestBalanceAtTimestampModel_ToEntity(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 6, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		model BalanceAtTimestampModel
	}{
		{
			name: "all fields populated",
			model: BalanceAtTimestampModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@snap-alias",
				Key:            "snap-key",
				AssetCode:      "USD",
				AccountType:    "deposit",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(50),
				Version:        7,
				CreatedAt:      created,
				UpdatedAt:      updated,
			},
		},
		{
			name: "zero decimal values",
			model: BalanceAtTimestampModel{
				ID:             "00000000-0000-0000-0000-000000000005",
				OrganizationID: "00000000-0000-0000-0000-000000000006",
				LedgerID:       "00000000-0000-0000-0000-000000000007",
				AccountID:      "00000000-0000-0000-0000-000000000008",
				Alias:          "@zero",
				Key:            "default",
				AssetCode:      "EUR",
				AccountType:    "savings",
				Available:      decimal.Zero,
				OnHold:         decimal.Zero,
				Version:        0,
				CreatedAt:      created,
				UpdatedAt:      updated,
			},
		},
		{
			name: "large precision values",
			model: BalanceAtTimestampModel{
				ID:             "00000000-0000-0000-0000-000000000009",
				OrganizationID: "00000000-0000-0000-0000-000000000010",
				LedgerID:       "00000000-0000-0000-0000-000000000011",
				AccountID:      "00000000-0000-0000-0000-000000000012",
				Alias:          "@big",
				Key:            "k1",
				AssetCode:      "BTC",
				AccountType:    "crypto",
				Available:      decimal.RequireFromString("123456789012345678901234567890.123456789012345678901234567890"),
				OnHold:         decimal.RequireFromString("987654321098765432109876543210.987654321098765432109876543210"),
				Version:        99,
				CreatedAt:      created,
				UpdatedAt:      updated,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.model.ToEntity()

			assert.Equal(t, tt.model.ID, result.ID)
			assert.Equal(t, tt.model.OrganizationID, result.OrganizationID)
			assert.Equal(t, tt.model.LedgerID, result.LedgerID)
			assert.Equal(t, tt.model.AccountID, result.AccountID)
			assert.Equal(t, tt.model.Alias, result.Alias)
			assert.Equal(t, tt.model.Key, result.Key)
			assert.Equal(t, tt.model.AssetCode, result.AssetCode)
			assert.Equal(t, tt.model.AccountType, result.AccountType)
			assert.True(t, tt.model.Available.Equal(result.Available),
				"Available mismatch: got %s, want %s", result.Available, tt.model.Available)
			assert.True(t, tt.model.OnHold.Equal(result.OnHold),
				"OnHold mismatch: got %s, want %s", result.OnHold, tt.model.OnHold)
			assert.Equal(t, tt.model.Version, result.Version)
			assert.Equal(t, tt.model.CreatedAt, result.CreatedAt)
			assert.Equal(t, tt.model.UpdatedAt, result.UpdatedAt)

			// ToEntity on the timestamp model does not set AllowSending/AllowReceiving
			// or DeletedAt — document that behavior.
			assert.False(t, result.AllowSending)
			assert.False(t, result.AllowReceiving)
			assert.Nil(t, result.DeletedAt)
		})
	}
}
