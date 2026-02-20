// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/require"
)

func TestProperty_ValidateStruct_OversizedFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     any
		wantErr   bool
		wantField string
	}{
		{
			name:    "ledger name at limit (256) OK",
			input:   &mmodel.CreateLedgerInput{Name: strings.Repeat("A", 256)},
			wantErr: false,
		},
		{
			name:      "ledger name over limit (257) FAIL",
			input:     &mmodel.CreateLedgerInput{Name: strings.Repeat("A", 257)},
			wantErr:   true,
			wantField: "name",
		},
		{
			name: "account alias at limit (100) OK",
			input: &mmodel.CreateAccountInput{
				AssetCode: "USD",
				Type:      "deposit",
				Alias:     ptrStr(strings.Repeat("a", 100)),
			},
			wantErr: false,
		},
		{
			name: "account alias over limit (101) FAIL",
			input: &mmodel.CreateAccountInput{
				AssetCode: "USD",
				Type:      "deposit",
				Alias:     ptrStr(strings.Repeat("a", 101)),
			},
			wantErr:   true,
			wantField: "alias",
		},
		{
			name: "account name at limit (256) OK",
			input: &mmodel.CreateAccountInput{
				Name:      strings.Repeat("N", 256),
				AssetCode: "USD",
				Type:      "deposit",
			},
			wantErr: false,
		},
		{
			name: "account name over limit (257) FAIL",
			input: &mmodel.CreateAccountInput{
				Name:      strings.Repeat("N", 257),
				AssetCode: "USD",
				Type:      "deposit",
			},
			wantErr:   true,
			wantField: "name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tc.input)

			if tc.wantErr {
				require.Error(t, err, "expected validation error for oversized field")

				var vErr *pkg.ValidationKnownFieldsError
				require.True(t, errors.As(err, &vErr), "expected *ValidationKnownFieldsError type, got %T", err)
				_, hasField := vErr.Fields[tc.wantField]
				require.True(t, hasField, "expected field %q in validation errors, got fields: %v", tc.wantField, vErr.Fields)
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func ptrStr(s string) *string { return &s }

func TestCollectNullByteViolations_MapWithNullBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     any
		wantErr   bool
		wantField string
	}{
		{
			name: "map with null byte in string value",
			input: map[string]any{
				"key": "value\x00with null",
			},
			wantErr:   true,
			wantField: "key",
		},
		{
			name: "map with null byte in nested string",
			input: map[string]any{
				"outer": map[string]any{
					"inner": "nested\x00value",
				},
			},
			wantErr:   true,
			wantField: "outer.inner",
		},
		{
			name: "map with clean values",
			input: map[string]any{
				"key": "clean value",
			},
			wantErr: false,
		},
		{
			name: "map with null byte in array element",
			input: map[string]any{
				"items": []any{"clean", "dirty\x00value"},
			},
			wantErr:   true,
			wantField: "items",
		},
		{
			name: "deeply nested map with null byte",
			input: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "deep\x00null",
					},
				},
			},
			wantErr:   true,
			wantField: "level1.level2.level3",
		},
		{
			name: "map with interface value containing null byte",
			input: map[string]any{
				"data": any("interface\x00value"),
			},
			wantErr:   true,
			wantField: "data",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tc.input)

			if tc.wantErr {
				require.Error(t, err, "expected validation error for null byte")

				var vErr pkg.ValidationKnownFieldsError
				require.True(t, errors.As(err, &vErr), "expected ValidationKnownFieldsError type, got %T", err)
				_, hasField := vErr.Fields[tc.wantField]
				require.True(t, hasField, "expected field %q in validation errors, got fields: %v", tc.wantField, vErr.Fields)
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}
