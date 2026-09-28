// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegulatoryFields_AccountTypeEnumTagMatchesCanonicalList locks the published
// enum tag of RegulatoryFields.AccountType to InstrumentAccountTypes. The tag is a
// literal (struct tags cannot reference constants), so without this lock the
// OpenAPI contract and the use-case validation could drift apart silently.
func TestRegulatoryFields_AccountTypeEnumTagMatchesCanonicalList(t *testing.T) {
	t.Parallel()

	field, ok := reflect.TypeOf(RegulatoryFields{}).FieldByName("AccountType")
	require.True(t, ok, "RegulatoryFields must declare the AccountType field")

	tag, ok := field.Tag.Lookup("enum")
	require.True(t, ok, "RegulatoryFields.AccountType must carry an enum tag")

	assert.Equal(t, InstrumentAccountTypes(), strings.Split(tag, ","),
		"enum tag of RegulatoryFields.AccountType must equal InstrumentAccountTypes() in content and order")
}

func TestInstrumentAccountTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		index int
		want  string
	}{
		{name: "bacen code 1", index: 0, want: "DEPOSIT"},
		{name: "bacen code 2", index: 1, want: "SAVINGS"},
		{name: "bacen code 3", index: 2, want: "INVESTMENT"},
		{name: "bacen code 4", index: 3, want: "OTHER_FINANCIAL_INVESTMENTS"},
		{name: "bacen code 5", index: 4, want: "NON_RESIDENT"},
		{name: "bacen code 6", index: 5, want: "PAYMENT"},
	}

	got := InstrumentAccountTypes()
	require.Len(t, got, len(tests), "InstrumentAccountTypes must list exactly one value per Bacen code")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, got[tt.index])
		})
	}
}

func TestInstrumentAccountTypes_ReturnsFreshSlice(t *testing.T) {
	t.Parallel()

	first := InstrumentAccountTypes()
	first[0] = "MUTATED"

	assert.Equal(t, InstrumentAccountTypeDeposit, InstrumentAccountTypes()[0],
		"mutating a returned slice must not change the canonical list")
}
