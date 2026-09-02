// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionPostgreSQLModel_ToEntity_AppliedExceptionID proves the model's
// nullable applied_exception_id column (a *string, following the
// ParentTransactionID precedent) maps onto the entity's optional pointer
// unchanged: a NULL (nil pointer) stays nil (omitted from JSON), and a populated
// value carries through by pointer.
func TestTransactionPostgreSQLModel_ToEntity_AppliedExceptionID(t *testing.T) {
	t.Parallel()

	populated := "55555555-5555-5555-5555-555555555555"

	tests := []struct {
		name     string
		column   *string
		expected *string
	}{
		{
			name:     "nil column maps to nil pointer",
			column:   nil,
			expected: nil,
		},
		{
			name:     "populated column carries through",
			column:   &populated,
			expected: &populated,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := TransactionPostgreSQLModel{
				ID:                 "33333333-3333-3333-3333-333333333333",
				Status:             "ACTIVE",
				AssetCode:          "BRL",
				AppliedExceptionID: tt.column,
			}

			entity := model.ToEntity()

			require.NotNil(t, entity)
			if tt.expected == nil {
				assert.Nil(t, entity.AppliedExceptionID)
			} else {
				require.NotNil(t, entity.AppliedExceptionID)
				assert.Equal(t, *tt.expected, *entity.AppliedExceptionID)
			}
		})
	}
}

// TestTransactionPostgreSQLModel_FromEntity_AppliedExceptionID proves the
// entity's optional pointer maps onto the model's nullable column: a nil pointer
// leaves the column NULL so historical rows are never forced to a value, and a
// populated pointer carries the same UUID text.
func TestTransactionPostgreSQLModel_FromEntity_AppliedExceptionID(t *testing.T) {
	t.Parallel()

	populated := "66666666-6666-6666-6666-666666666666"

	tests := []struct {
		name  string
		value *string
	}{
		{
			name:  "nil pointer leaves column NULL",
			value: nil,
		},
		{
			name:  "populated pointer sets the column",
			value: &populated,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := &Transaction{
				ID:                 "33333333-3333-3333-3333-333333333333",
				Status:             Status{Code: "ACTIVE"},
				AssetCode:          "BRL",
				AppliedExceptionID: tt.value,
			}

			var model TransactionPostgreSQLModel
			model.FromEntity(entity)

			if tt.value == nil {
				assert.Nil(t, model.AppliedExceptionID)
			} else {
				require.NotNil(t, model.AppliedExceptionID)
				assert.Equal(t, *tt.value, *model.AppliedExceptionID)
			}
		})
	}
}

// TestTransactionPostgreSQLModel_RoundTrip_AppliedExceptionID proves the field
// survives an entity -> model -> entity round trip, both when populated and when
// absent (NULL stays absent).
func TestTransactionPostgreSQLModel_RoundTrip_AppliedExceptionID(t *testing.T) {
	t.Parallel()

	populated := "77777777-7777-7777-7777-777777777777"

	tests := []struct {
		name  string
		value *string
	}{
		{name: "populated survives round trip", value: &populated},
		{name: "absent stays absent", value: nil},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := &Transaction{
				ID:                 "33333333-3333-3333-3333-333333333333",
				Status:             Status{Code: "ACTIVE"},
				AssetCode:          "BRL",
				AppliedExceptionID: tt.value,
			}

			var model TransactionPostgreSQLModel
			model.FromEntity(original)

			result := model.ToEntity()

			require.NotNil(t, result)
			if tt.value == nil {
				assert.Nil(t, result.AppliedExceptionID)
			} else {
				require.NotNil(t, result.AppliedExceptionID)
				assert.Equal(t, *tt.value, *result.AppliedExceptionID)
			}
		})
	}
}
