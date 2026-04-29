// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBalancePostgreSQLModel_HasOverdraftFields verifies via reflection that
// BalancePostgreSQLModel exposes the overdraft fields with the expected types
// and db struct tags required for repository persistence.
func TestBalancePostgreSQLModel_HasOverdraftFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fieldName    string
		expectedType reflect.Type
		expectedTag  string
	}{
		{
			name:         "direction field",
			fieldName:    "Direction",
			expectedType: reflect.TypeOf(""),
			expectedTag:  "direction",
		},
		{
			name:         "overdraft_used field",
			fieldName:    "OverdraftUsed",
			expectedType: reflect.TypeOf(decimal.Decimal{}),
			expectedTag:  "overdraft_used",
		},
		{
			name:         "settings field",
			fieldName:    "Settings",
			expectedType: reflect.TypeOf([]byte(nil)),
			expectedTag:  "settings",
		},
	}

	modelType := reflect.TypeOf(BalancePostgreSQLModel{})

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := modelType.FieldByName(tt.fieldName)
			require.True(t, ok, "expected field %q on BalancePostgreSQLModel", tt.fieldName)
			assert.Equal(t, tt.expectedType, field.Type, "field %q type mismatch", tt.fieldName)
			assert.Equal(t, tt.expectedTag, field.Tag.Get("db"), "field %q db tag mismatch", tt.fieldName)
		})
	}
}

// TestBalancePostgreSQLModel_FromEntity_MapsOverdraftFields verifies that
// FromEntity copies the scalar overdraft fields and marshals Settings as
// valid JSON suitable for the JSONB column.
func TestBalancePostgreSQLModel_FromEntity_MapsOverdraftFields(t *testing.T) {
	t.Parallel()

	limit := "500.00"
	entity := &mmodel.Balance{
		ID:            "00000000-0000-0000-0000-000000000001",
		Direction:     "debit",
		OverdraftUsed: decimal.NewFromFloat(50.00),
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
			BalanceScope:          mmodel.BalanceScopeTransactional,
		},
	}

	var model BalancePostgreSQLModel
	model.FromEntity(entity)

	assert.Equal(t, "debit", model.Direction)
	assert.True(t, model.OverdraftUsed.Equal(decimal.NewFromFloat(50.00)),
		"expected overdraft_used=50.00, got %s", model.OverdraftUsed.String())

	require.NotNil(t, model.Settings, "expected Settings to be marshaled to non-nil bytes")
	assert.True(t, json.Valid(model.Settings), "expected Settings to be valid JSON")

	var decoded mmodel.BalanceSettings
	require.NoError(t, json.Unmarshal(model.Settings, &decoded))
	assert.True(t, decoded.AllowOverdraft)
	assert.True(t, decoded.OverdraftLimitEnabled)
	require.NotNil(t, decoded.OverdraftLimit)
	assert.Equal(t, "500.00", *decoded.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, decoded.BalanceScope)
}

// TestBalancePostgreSQLModel_ToEntity_MapsOverdraftFields verifies that
// ToEntity round-trips the overdraft fields and unmarshals the Settings
// JSON bytes back into a typed BalanceSettings on the entity.
func TestBalancePostgreSQLModel_ToEntity_MapsOverdraftFields(t *testing.T) {
	t.Parallel()

	limit := "500.00"
	settings := mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)

	model := BalancePostgreSQLModel{
		ID:            "00000000-0000-0000-0000-000000000001",
		Key:           "default",
		Direction:     "debit",
		OverdraftUsed: decimal.NewFromInt(50),
		Settings:      settingsBytes,
	}

	entity := model.ToEntity()

	assert.Equal(t, "debit", entity.Direction)
	assert.True(t, entity.OverdraftUsed.Equal(decimal.NewFromInt(50)),
		"expected overdraft_used=50, got %s", entity.OverdraftUsed.String())

	require.NotNil(t, entity.Settings, "expected Settings unmarshaled onto entity")
	assert.True(t, entity.Settings.AllowOverdraft)
	assert.True(t, entity.Settings.OverdraftLimitEnabled)
	require.NotNil(t, entity.Settings.OverdraftLimit)
	assert.Equal(t, "500.00", *entity.Settings.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, entity.Settings.BalanceScope)
}

// TestBalancePostgreSQLModel_ToEntity_NilSettings ensures a model with nil
// Settings bytes produces an entity with nil Settings (legacy-row
// backwards compatibility).
func TestBalancePostgreSQLModel_ToEntity_NilSettings(t *testing.T) {
	t.Parallel()

	model := BalancePostgreSQLModel{
		ID:       "00000000-0000-0000-0000-000000000001",
		Key:      "default",
		Settings: nil,
	}

	entity := model.ToEntity()

	assert.Nil(t, entity.Settings, "expected nil Settings on entity when model Settings is nil")
}

// TestBalancePostgreSQLModel_FromEntity_NilSettings ensures an entity with
// nil Settings produces a model with nil Settings bytes, so the repository
// writes SQL NULL rather than a JSON null literal.
func TestBalancePostgreSQLModel_FromEntity_NilSettings(t *testing.T) {
	t.Parallel()

	entity := &mmodel.Balance{
		ID:       "00000000-0000-0000-0000-000000000001",
		Settings: nil,
	}

	var model BalancePostgreSQLModel
	model.FromEntity(entity)

	assert.Nil(t, model.Settings, "expected nil Settings bytes when entity Settings is nil")
}

// TestBalanceColumnList_IncludesOverdraftColumns ensures the column list used
// by SELECT/INSERT/UPDATE statements references the overdraft columns, so
// reads and writes stay in sync with the physical schema.
func TestBalanceColumnList_IncludesOverdraftColumns(t *testing.T) {
	t.Parallel()

	expected := []string{"direction", "overdraft_used", "settings"}

	for _, col := range expected {
		col := col
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, balanceColumnList, col,
				"balanceColumnList must contain %q for repository round-trip", col)
		})
	}
}
