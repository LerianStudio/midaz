// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionBalance_HasOverdraftFields verifies that the internal
// mtransaction.Balance struct exposes the overdraft fields required for
// the overdraft feature. These fields travel with the balance through
// transaction processing so the engine can evaluate overdraft limits.
func TestTransactionBalance_HasOverdraftFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		fieldType string
	}{
		{name: "Direction field is string", fieldName: "Direction", fieldType: "string"},
		{name: "OverdraftUsed field is decimal.Decimal", fieldName: "OverdraftUsed", fieldType: "decimal.Decimal"},
		{name: "AllowOverdraft field is bool", fieldName: "AllowOverdraft", fieldType: "bool"},
		{name: "OverdraftLimitEnabled field is bool", fieldName: "OverdraftLimitEnabled", fieldType: "bool"},
		{name: "OverdraftLimit field is decimal.Decimal", fieldName: "OverdraftLimit", fieldType: "decimal.Decimal"},
		{name: "BalanceScope field is string", fieldName: "BalanceScope", fieldType: "string"},
	}

	bt := reflect.TypeOf(Balance{})

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := bt.FieldByName(tt.fieldName)
			require.True(t, ok, "mtransaction.Balance must have field %q", tt.fieldName)
			assert.Equal(t, tt.fieldType, field.Type.String(),
				"mtransaction.Balance.%s must be of type %s", tt.fieldName, tt.fieldType)
		})
	}
}
