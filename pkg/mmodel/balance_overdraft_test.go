// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"reflect"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBalance_HasOverdraftFields verifies the Balance struct exposes the new
// overdraft fields (Direction, OverdraftUsed, Settings) used by the overdraft
// feature.
func TestBalance_HasOverdraftFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		fieldType string
	}{
		{name: "Direction field is string", fieldName: "Direction", fieldType: "string"},
		{name: "OverdraftUsed field is decimal.Decimal", fieldName: "OverdraftUsed", fieldType: "decimal.Decimal"},
		{name: "Settings field is *BalanceSettings", fieldName: "Settings", fieldType: "*mmodel.BalanceSettings"},
	}

	bt := reflect.TypeOf(Balance{})

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := bt.FieldByName(tt.fieldName)
			require.True(t, ok, "Balance struct must have field %q", tt.fieldName)
			assert.Equal(t, tt.fieldType, field.Type.String(),
				"Balance.%s must be of type %s", tt.fieldName, tt.fieldType)
		})
	}
}

// TestBalanceHistory_HasOverdraftFields verifies BalanceHistory also carries
// the new fields so historical snapshots preserve overdraft state.
func TestBalanceHistory_HasOverdraftFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		fieldType string
	}{
		{name: "Direction field is string", fieldName: "Direction", fieldType: "string"},
		{name: "OverdraftUsed field is decimal.Decimal", fieldName: "OverdraftUsed", fieldType: "decimal.Decimal"},
		{name: "Settings field is *BalanceSettings", fieldName: "Settings", fieldType: "*mmodel.BalanceSettings"},
	}

	bt := reflect.TypeOf(BalanceHistory{})

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := bt.FieldByName(tt.fieldName)
			require.True(t, ok, "BalanceHistory struct must have field %q", tt.fieldName)
			assert.Equal(t, tt.fieldType, field.Type.String(),
				"BalanceHistory.%s must be of type %s", tt.fieldName, tt.fieldType)
		})
	}
}

// TestCreateAdditionalBalance_HasOverdraftFields verifies the create payload
// accepts Direction (optional) and Settings (optional).
func TestCreateAdditionalBalance_HasOverdraftFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		fieldType string
	}{
		{name: "Direction pointer is *string", fieldName: "Direction", fieldType: "*string"},
		{name: "Settings is *BalanceSettings", fieldName: "Settings", fieldType: "*mmodel.BalanceSettings"},
	}

	bt := reflect.TypeOf(CreateAdditionalBalance{})

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := bt.FieldByName(tt.fieldName)
			require.True(t, ok, "CreateAdditionalBalance must have field %q", tt.fieldName)
			assert.Equal(t, tt.fieldType, field.Type.String(),
				"CreateAdditionalBalance.%s must be of type %s", tt.fieldName, tt.fieldType)
		})
	}
}

// TestUpdateBalance_HasSettingsField verifies the update payload exposes
// Settings (optional). Direction is immutable after creation, so
// UpdateBalance must NOT expose Direction.
func TestUpdateBalance_HasSettingsField(t *testing.T) {
	t.Parallel()

	bt := reflect.TypeOf(UpdateBalance{})

	field, ok := bt.FieldByName("Settings")
	require.True(t, ok, "UpdateBalance must have field Settings")
	assert.Equal(t, "*mmodel.BalanceSettings", field.Type.String(),
		"UpdateBalance.Settings must be *BalanceSettings")

	_, hasDirection := bt.FieldByName("Direction")
	assert.False(t, hasDirection,
		"UpdateBalance must NOT expose Direction - direction is immutable after creation")
}

// TestBalance_ToTransactionBalance_IncludesOverdraftFields ensures that
// converting a Balance to mtransaction.Balance propagates the overdraft
// fields Direction, OverdraftUsed, and the flattened settings fields.
func TestBalance_ToTransactionBalance_IncludesOverdraftFields(t *testing.T) {
	t.Parallel()

	limit := "2500.00"
	settings := &BalanceSettings{
		BalanceScope:          "transactional",
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	tests := []struct {
		name                      string
		direction                 string
		overdraftUsed             decimal.Decimal
		settings                  *BalanceSettings
		wantDirection             string
		wantOverdraftUsed         decimal.Decimal
		wantAllowOverdraft        bool
		wantOverdraftLimitEnabled bool
		wantOverdraftLimit        decimal.Decimal
		wantBalanceScope          string
	}{
		{
			name:                      "credit direction, zero used, full settings",
			direction:                 "credit",
			overdraftUsed:             decimal.Zero,
			settings:                  settings,
			wantDirection:             "credit",
			wantOverdraftUsed:         decimal.Zero,
			wantAllowOverdraft:        true,
			wantOverdraftLimitEnabled: true,
			wantOverdraftLimit:        decimal.RequireFromString("2500.00"),
			wantBalanceScope:          "transactional",
		},
		{
			name:                      "debit direction, non-zero used, nil settings",
			direction:                 "debit",
			overdraftUsed:             decimal.NewFromInt(250),
			settings:                  nil,
			wantDirection:             "debit",
			wantOverdraftUsed:         decimal.NewFromInt(250),
			wantAllowOverdraft:        false,
			wantOverdraftLimitEnabled: false,
			wantOverdraftLimit:        decimal.Zero,
			wantBalanceScope:          "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := &Balance{
				ID:             "123e4567-e89b-12d3-a456-426614174000",
				OrganizationID: "org-1",
				LedgerID:       "ledger-1",
				AccountID:      "account-1",
				Alias:          "@acc",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				Direction:      tt.direction,
				OverdraftUsed:  tt.overdraftUsed,
				Settings:       tt.settings,
			}

			got, err := b.ToTransactionBalance()
			require.NoError(t, err)
			require.NotNil(t, got)

			assert.Equal(t, tt.wantDirection, got.Direction,
				"Direction must be propagated through ToTransactionBalance")
			assert.True(t, tt.wantOverdraftUsed.Equal(got.OverdraftUsed),
				"OverdraftUsed must be propagated: want %s got %s",
				tt.wantOverdraftUsed, got.OverdraftUsed)
			assert.Equal(t, tt.wantAllowOverdraft, got.AllowOverdraft,
				"AllowOverdraft must be propagated")
			assert.Equal(t, tt.wantOverdraftLimitEnabled, got.OverdraftLimitEnabled,
				"OverdraftLimitEnabled must be propagated")
			assert.True(t, tt.wantOverdraftLimit.Equal(got.OverdraftLimit),
				"OverdraftLimit must be propagated: want %s got %s",
				tt.wantOverdraftLimit, got.OverdraftLimit)
			assert.Equal(t, tt.wantBalanceScope, got.BalanceScope,
				"BalanceScope must be propagated")
		})
	}
}
