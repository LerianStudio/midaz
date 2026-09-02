// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionPostgreSQLModel_ToEntity_OperationalTypeCode proves the model's
// nullable operational_type_code column maps onto the entity's optional string:
// a NULL (invalid sql.NullString) becomes the empty string (omitted from JSON),
// and a populated value carries through unchanged.
func TestTransactionPostgreSQLModel_ToEntity_OperationalTypeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		column   sql.NullString
		expected string
	}{
		{
			name:     "null column maps to empty string",
			column:   sql.NullString{Valid: false},
			expected: "",
		},
		{
			name:     "empty valid column maps to empty string",
			column:   sql.NullString{String: "", Valid: true},
			expected: "",
		},
		{
			name:     "populated column carries through",
			column:   sql.NullString{String: "PIX_IN", Valid: true},
			expected: "PIX_IN",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := TransactionPostgreSQLModel{
				ID:                  "33333333-3333-3333-3333-333333333333",
				Status:              "ACTIVE",
				AssetCode:           "BRL",
				OperationalTypeCode: tt.column,
			}

			entity := model.ToEntity()

			require.NotNil(t, entity)
			assert.Equal(t, tt.expected, entity.OperationalTypeCode)
		})
	}
}

// TestTransactionPostgreSQLModel_FromEntity_OperationalTypeCode proves the
// entity's optional string maps onto the model's nullable column: an empty
// string leaves the column NULL (Valid=false) so historical rows are never
// forced to a value, and a populated string sets Valid=true with the same text.
func TestTransactionPostgreSQLModel_FromEntity_OperationalTypeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		expectValid bool
	}{
		{
			name:        "empty string leaves column NULL",
			value:       "",
			expectValid: false,
		},
		{
			name:        "populated string sets a valid column",
			value:       "PIX_OUT",
			expectValid: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := &Transaction{
				ID:                  "33333333-3333-3333-3333-333333333333",
				Status:              Status{Code: "ACTIVE"},
				AssetCode:           "BRL",
				OperationalTypeCode: tt.value,
			}

			var model TransactionPostgreSQLModel
			model.FromEntity(entity)

			assert.Equal(t, tt.expectValid, model.OperationalTypeCode.Valid)
			if tt.expectValid {
				assert.Equal(t, tt.value, model.OperationalTypeCode.String)
			}
		})
	}
}

// TestTransactionPostgreSQLModel_RoundTrip_OperationalTypeCode proves the field
// survives an entity -> model -> entity round trip, both when populated and when
// absent (NULL stays absent).
func TestTransactionPostgreSQLModel_RoundTrip_OperationalTypeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "populated survives round trip", value: "PIX_IN"},
		{name: "absent stays absent", value: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := &Transaction{
				ID:                  "33333333-3333-3333-3333-333333333333",
				Status:              Status{Code: "ACTIVE"},
				AssetCode:           "BRL",
				OperationalTypeCode: tt.value,
			}

			var model TransactionPostgreSQLModel
			model.FromEntity(original)

			result := model.ToEntity()

			require.NotNil(t, result)
			assert.Equal(t, tt.value, result.OperationalTypeCode)
		})
	}
}
