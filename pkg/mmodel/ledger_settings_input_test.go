// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerSettingsInput_ToSparseMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   *LedgerSettingsInput
		wantNil bool
		want    map[string]any
	}{
		{
			name:    "nil receiver returns nil map",
			input:   nil,
			wantNil: true,
		},
		{
			name:  "empty input returns non-nil empty map",
			input: &LedgerSettingsInput{},
			want:  map[string]any{},
		},
		{
			name:  "accounting group present but empty yields empty nested map",
			input: &LedgerSettingsInput{Accounting: &AccountingValidationInput{}},
			want:  map[string]any{"accounting": map[string]any{}},
		},
		{
			name:  "tracer group present but empty yields empty nested map",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{}},
			want:  map[string]any{"tracer": map[string]any{}},
		},
		{
			name:  "overrides group present but empty yields empty nested map",
			input: &LedgerSettingsInput{Overrides: &OverridePolicyInput{}},
			want:  map[string]any{"overrides": map[string]any{}},
		},
		{
			name: "accounting with a single field emits only that key",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{ValidateAccountType: testutils.Ptr(true)},
			},
			want: map[string]any{"accounting": map[string]any{"validateAccountType": true}},
		},
		{
			name: "accounting validateRoutes only",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{ValidateRoutes: testutils.Ptr(true)},
			},
			want: map[string]any{"accounting": map[string]any{"validateRoutes": true}},
		},
		{
			name: "accounting requireHolder only",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{RequireHolder: testutils.Ptr(true)},
			},
			want: map[string]any{"accounting": map[string]any{"requireHolder": true}},
		},
		{
			name: "tracer mode only",
			input: &LedgerSettingsInput{
				Tracer: &TracerSettingsInput{Mode: testutils.Ptr(TracerModeEnforce)},
			},
			want: map[string]any{"tracer": map[string]any{"mode": "enforce"}},
		},
		{
			name: "tracer failPosture only",
			input: &LedgerSettingsInput{
				Tracer: &TracerSettingsInput{FailPosture: testutils.Ptr(TracerFailPostureClosed)},
			},
			want: map[string]any{"tracer": map[string]any{"failPosture": "closed"}},
		},
		{
			name: "tracer timeoutMs only",
			input: &LedgerSettingsInput{
				Tracer: &TracerSettingsInput{TimeoutMs: testutils.Ptr(1500)},
			},
			want: map[string]any{"tracer": map[string]any{"timeoutMs": 1500}},
		},
		{
			name: "overrides allowFeeSkip only",
			input: &LedgerSettingsInput{
				Overrides: &OverridePolicyInput{AllowFeeSkip: testutils.Ptr(true)},
			},
			want: map[string]any{"overrides": map[string]any{"allowFeeSkip": true}},
		},
		{
			name: "overrides allowTracerSkip only",
			input: &LedgerSettingsInput{
				Overrides: &OverridePolicyInput{AllowTracerSkip: testutils.Ptr(true)},
			},
			want: map[string]any{"overrides": map[string]any{"allowTracerSkip": true}},
		},
		{
			name: "overrides allowHolderSkip only",
			input: &LedgerSettingsInput{
				Overrides: &OverridePolicyInput{AllowHolderSkip: testutils.Ptr(true)},
			},
			want: map[string]any{"overrides": map[string]any{"allowHolderSkip": true}},
		},
		{
			name: "one field per group emits exactly three keys",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{RequireHolder: testutils.Ptr(true)},
				Tracer:     &TracerSettingsInput{Mode: testutils.Ptr(TracerModeAdvisory)},
				Overrides:  &OverridePolicyInput{AllowTracerSkip: testutils.Ptr(true)},
			},
			want: map[string]any{
				"accounting": map[string]any{"requireHolder": true},
				"tracer":     map[string]any{"mode": "advisory"},
				"overrides":  map[string]any{"allowTracerSkip": true},
			},
		},
		{
			name: "all nine fields set emits nine leaves across three groups",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{
					ValidateAccountType: testutils.Ptr(true),
					ValidateRoutes:      testutils.Ptr(true),
					RequireHolder:       testutils.Ptr(true),
				},
				Tracer: &TracerSettingsInput{
					Mode:        testutils.Ptr(TracerModeEnforce),
					FailPosture: testutils.Ptr(TracerFailPostureClosed),
					TimeoutMs:   testutils.Ptr(999),
				},
				Overrides: &OverridePolicyInput{
					AllowFeeSkip:    testutils.Ptr(true),
					AllowTracerSkip: testutils.Ptr(true),
					AllowHolderSkip: testutils.Ptr(true),
				},
			},
			want: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
					"requireHolder":       true,
				},
				"tracer": map[string]any{
					"mode":        "enforce",
					"failPosture": "closed",
					"timeoutMs":   999,
				},
				"overrides": map[string]any{
					"allowFeeSkip":    true,
					"allowTracerSkip": true,
					"allowHolderSkip": true,
				},
			},
		},
		{
			name: "explicitly sent zero values are kept as present keys",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{
					ValidateAccountType: testutils.Ptr(false),
					ValidateRoutes:      testutils.Ptr(false),
					RequireHolder:       testutils.Ptr(false),
				},
				Tracer: &TracerSettingsInput{
					Mode:        testutils.Ptr(""),
					FailPosture: testutils.Ptr(""),
					TimeoutMs:   testutils.Ptr(0),
				},
				Overrides: &OverridePolicyInput{
					AllowFeeSkip:    testutils.Ptr(false),
					AllowTracerSkip: testutils.Ptr(false),
					AllowHolderSkip: testutils.Ptr(false),
				},
			},
			want: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": false,
					"validateRoutes":      false,
					"requireHolder":       false,
				},
				"tracer": map[string]any{
					"mode":        "",
					"failPosture": "",
					"timeoutMs":   0,
				},
				"overrides": map[string]any{
					"allowFeeSkip":    false,
					"allowTracerSkip": false,
					"allowHolderSkip": false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.input.ToSparseMap()

			if tt.wantNil {
				assert.Nil(t, got, "nil receiver must produce a nil map, not an empty one")
				return
			}

			require.NotNil(t, got, "non-nil receiver must produce a non-nil map")
			assert.Equal(t, tt.want, got, "sparse map must carry exactly the keys the client sent")
			assert.Len(t, got, len(tt.want))
		})
	}
}

// TestLedgerSettingsInput_ToSparseMap_ValidateSettingsAcceptsEveryLeaf pins ToSparseMap's key
// spelling against settingsSchema: a typo in any emitted key would surface here as
// ErrUnknownSettingsField instead of nil.
func TestLedgerSettingsInput_ToSparseMap_ValidateSettingsAcceptsEveryLeaf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *LedgerSettingsInput
	}{
		{
			name:  "accounting.validateAccountType",
			input: &LedgerSettingsInput{Accounting: &AccountingValidationInput{ValidateAccountType: testutils.Ptr(true)}},
		},
		{
			name:  "accounting.validateRoutes",
			input: &LedgerSettingsInput{Accounting: &AccountingValidationInput{ValidateRoutes: testutils.Ptr(true)}},
		},
		{
			name:  "accounting.requireHolder",
			input: &LedgerSettingsInput{Accounting: &AccountingValidationInput{RequireHolder: testutils.Ptr(true)}},
		},
		{
			name:  "tracer.mode",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{Mode: testutils.Ptr(TracerModeAdvisory)}},
		},
		{
			name:  "tracer.failPosture",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{FailPosture: testutils.Ptr(TracerFailPostureClosed)}},
		},
		{
			name:  "tracer.timeoutMs",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{TimeoutMs: testutils.Ptr(750)}},
		},
		{
			name:  "overrides.allowFeeSkip",
			input: &LedgerSettingsInput{Overrides: &OverridePolicyInput{AllowFeeSkip: testutils.Ptr(true)}},
		},
		{
			name:  "overrides.allowTracerSkip",
			input: &LedgerSettingsInput{Overrides: &OverridePolicyInput{AllowTracerSkip: testutils.Ptr(true)}},
		},
		{
			name:  "overrides.allowHolderSkip",
			input: &LedgerSettingsInput{Overrides: &OverridePolicyInput{AllowHolderSkip: testutils.Ptr(true)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sparse := tt.input.ToSparseMap()
			require.Len(t, sparse, 1, "a single-group input must emit exactly one top-level key")

			require.NoError(t, ValidateSettings(sparse), "leaf key must match settingsSchema")
		})
	}
}

// TestLedgerSettingsInput_ToSparseMap_ParseRoundTrip locks the contract with the consumers the
// create path uses: ToSparseMap -> ValidateSettings -> ParseLedgerSettings. Absent leaves must
// land on their defaults and only the sent leaves must move.
func TestLedgerSettingsInput_ToSparseMap_ParseRoundTrip(t *testing.T) {
	t.Parallel()

	allSet := DefaultLedgerSettings()
	allSet.Accounting = AccountingValidation{ValidateAccountType: true, ValidateRoutes: true, RequireHolder: true}
	allSet.Tracer = TracerSettings{Mode: TracerModeEnforce, FailPosture: TracerFailPostureClosed, TimeoutMs: 999}
	allSet.Overrides = OverridePolicy{AllowFeeSkip: true, AllowTracerSkip: true, AllowHolderSkip: true}

	onlyTracerMode := DefaultLedgerSettings()
	onlyTracerMode.Tracer.Mode = TracerModeAdvisory

	onlyRequireHolder := DefaultLedgerSettings()
	onlyRequireHolder.Accounting.RequireHolder = true

	onlyFeeSkip := DefaultLedgerSettings()
	onlyFeeSkip.Overrides.AllowFeeSkip = true

	explicitZeroTimeout := DefaultLedgerSettings()
	explicitZeroTimeout.Tracer.TimeoutMs = 0

	tests := []struct {
		name  string
		input *LedgerSettingsInput
		want  LedgerSettings
	}{
		{
			name:  "nil input parses to defaults",
			input: nil,
			want:  DefaultLedgerSettings(),
		},
		{
			name:  "empty input parses to defaults",
			input: &LedgerSettingsInput{},
			want:  DefaultLedgerSettings(),
		},
		{
			name:  "empty tracer group parses to defaults",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{}},
			want:  DefaultLedgerSettings(),
		},
		{
			name:  "partial accounting keeps tracer and overrides defaults",
			input: &LedgerSettingsInput{Accounting: &AccountingValidationInput{RequireHolder: testutils.Ptr(true)}},
			want:  onlyRequireHolder,
		},
		{
			name:  "partial tracer keeps failPosture and timeoutMs defaults",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{Mode: testutils.Ptr(TracerModeAdvisory)}},
			want:  onlyTracerMode,
		},
		{
			name:  "partial overrides keeps the other opt-ins false",
			input: &LedgerSettingsInput{Overrides: &OverridePolicyInput{AllowFeeSkip: testutils.Ptr(true)}},
			want:  onlyFeeSkip,
		},
		{
			name:  "explicitly sent zero timeout overrides the default",
			input: &LedgerSettingsInput{Tracer: &TracerSettingsInput{TimeoutMs: testutils.Ptr(0)}},
			want:  explicitZeroTimeout,
		},
		{
			name: "all nine fields set round-trips exactly",
			input: &LedgerSettingsInput{
				Accounting: &AccountingValidationInput{
					ValidateAccountType: testutils.Ptr(true),
					ValidateRoutes:      testutils.Ptr(true),
					RequireHolder:       testutils.Ptr(true),
				},
				Tracer: &TracerSettingsInput{
					Mode:        testutils.Ptr(TracerModeEnforce),
					FailPosture: testutils.Ptr(TracerFailPostureClosed),
					TimeoutMs:   testutils.Ptr(999),
				},
				Overrides: &OverridePolicyInput{
					AllowFeeSkip:    testutils.Ptr(true),
					AllowTracerSkip: testutils.Ptr(true),
					AllowHolderSkip: testutils.Ptr(true),
				},
			},
			want: allSet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sparse := tt.input.ToSparseMap()

			require.NoError(t, ValidateSettings(sparse))
			assert.Equal(t, tt.want, ParseLedgerSettings(sparse))
		})
	}
}

// TestLedgerSettingsInput_ToSparseMap_ErrorFieldPathIsDeterministic locks the field path
// reported for an invalid leaf. A sparse map carries only the leaves the client sent, so a
// single invalid leaf leaves ValidateSettings no choice of which path to report — unlike a
// dense map, where Go map iteration order picks a winner among several invalid zero values.
func TestLedgerSettingsInput_ToSparseMap_ErrorFieldPathIsDeterministic(t *testing.T) {
	t.Parallel()

	const repeats = 50

	tests := []struct {
		name          string
		input         *LedgerSettingsInput
		wantCode      string
		wantFieldPath string
	}{
		{
			name:          "misspelled tracer mode reports tracer.mode",
			input:         &LedgerSettingsInput{Tracer: &TracerSettingsInput{Mode: testutils.Ptr("enfroce")}},
			wantCode:      "0176",
			wantFieldPath: "tracer.mode",
		},
		{
			name:          "empty tracer mode reports tracer.mode",
			input:         &LedgerSettingsInput{Tracer: &TracerSettingsInput{Mode: testutils.Ptr("")}},
			wantCode:      "0176",
			wantFieldPath: "tracer.mode",
		},
		{
			name:          "invalid tracer failPosture reports tracer.failPosture",
			input:         &LedgerSettingsInput{Tracer: &TracerSettingsInput{FailPosture: testutils.Ptr("half-open")}},
			wantCode:      "0176",
			wantFieldPath: "tracer.failPosture",
		},
		{
			name:          "empty tracer failPosture reports tracer.failPosture",
			input:         &LedgerSettingsInput{Tracer: &TracerSettingsInput{FailPosture: testutils.Ptr("")}},
			wantCode:      "0176",
			wantFieldPath: "tracer.failPosture",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sparse := tt.input.ToSparseMap()
			require.Len(t, sparse, 1)

			first := ValidateSettings(sparse)
			require.Error(t, first)

			var vErr pkg.ValidationError
			require.True(t, errors.As(first, &vErr), "expected ValidationError, got %T", first)
			assert.Equal(t, tt.wantCode, vErr.Code)
			assert.Contains(t, vErr.Message, tt.wantFieldPath, "reported field path must name the invalid leaf")

			for range repeats {
				again := ValidateSettings(tt.input.ToSparseMap())
				require.Error(t, again)
				assert.Equal(t, first.Error(), again.Error(), "reported field path must not vary between runs")
			}
		})
	}
}

// TestLedgerSettingsInput_JSONTagsOmitEmpty enforces omitempty on every field of the request
// tree. Huma marks a field required unless its json tag carries omitempty, and
// pkgHTTP.DecodeAndValidate diffs the client payload against a re-marshal of the struct — a
// missing omitempty would both re-add the nine leaves to the spec's required list and put nil
// pointers into that re-marshal.
func TestLedgerSettingsInput_JSONTagsOmitEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		typ      reflect.Type
		wantKeys []string
	}{
		{
			name:     "LedgerSettingsInput",
			typ:      reflect.TypeOf(LedgerSettingsInput{}),
			wantKeys: []string{"accounting", "tracer", "overrides"},
		},
		{
			name:     "AccountingValidationInput",
			typ:      reflect.TypeOf(AccountingValidationInput{}),
			wantKeys: []string{"validateAccountType", "validateRoutes", "requireHolder"},
		},
		{
			name:     "TracerSettingsInput",
			typ:      reflect.TypeOf(TracerSettingsInput{}),
			wantKeys: []string{"mode", "failPosture", "timeoutMs"},
		},
		{
			name:     "OverridePolicyInput",
			typ:      reflect.TypeOf(OverridePolicyInput{}),
			wantKeys: []string{"allowFeeSkip", "allowTracerSkip", "allowHolderSkip"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, len(tt.wantKeys), tt.typ.NumField(), "field count is part of the wire contract")

			gotKeys := make([]string, 0, tt.typ.NumField())

			for i := range tt.typ.NumField() {
				field := tt.typ.Field(i)
				tag := field.Tag.Get("json")

				require.NotEmpty(t, tag, "%s.%s must carry a json tag", tt.name, field.Name)

				parts := strings.Split(tag, ",")
				gotKeys = append(gotKeys, parts[0])

				assert.Contains(t, parts[1:], "omitempty", "%s.%s json tag must carry omitempty", tt.name, field.Name)
				assert.Equal(t, reflect.Ptr, field.Type.Kind(), "%s.%s must be a pointer so absent stays distinguishable from zero", tt.name, field.Name)
				assert.Empty(t, field.Tag.Get("validate"), "%s.%s must not carry a validate tag; ValidateSettings owns the value space", tt.name, field.Name)
			}

			assert.Equal(t, tt.wantKeys, gotKeys)
		})
	}
}

// TestLedgerSettingsInput_UnmarshalJSON_AbsentVsExplicitZero proves the pointer shape survives
// real JSON decoding: this is the absent-vs-zero distinction the dense LedgerSettings destroys.
func TestLedgerSettingsInput_UnmarshalJSON_AbsentVsExplicitZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		payload    string
		wantSparse map[string]any
	}{
		{
			name:       "empty object leaves every group nil",
			payload:    `{}`,
			wantSparse: map[string]any{},
		},
		{
			name:       "empty tracer object yields a present but empty group",
			payload:    `{"tracer":{}}`,
			wantSparse: map[string]any{"tracer": map[string]any{}},
		},
		{
			name:       "explicit false is preserved as a present key",
			payload:    `{"accounting":{"validateRoutes":false}}`,
			wantSparse: map[string]any{"accounting": map[string]any{"validateRoutes": false}},
		},
		{
			name:       "explicit empty string is preserved as a present key",
			payload:    `{"tracer":{"mode":""}}`,
			wantSparse: map[string]any{"tracer": map[string]any{"mode": ""}},
		},
		{
			name:       "explicit zero number is preserved as a present key",
			payload:    `{"tracer":{"timeoutMs":0}}`,
			wantSparse: map[string]any{"tracer": map[string]any{"timeoutMs": 0}},
		},
		{
			name:    "single leaf leaves the sibling leaves absent",
			payload: `{"tracer":{"mode":"enforce"}}`,
			wantSparse: map[string]any{
				"tracer": map[string]any{"mode": "enforce"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var input LedgerSettingsInput
			require.NoError(t, json.Unmarshal([]byte(tt.payload), &input))

			assert.Equal(t, tt.wantSparse, input.ToSparseMap())
		})
	}
}

// TestLedgerSettingsInput_ToSparseMap_DoesNotAliasInput guards the caller against a shared
// backing map: two calls must hand back independent maps so mutating one cannot corrupt the
// other.
func TestLedgerSettingsInput_ToSparseMap_DoesNotAliasInput(t *testing.T) {
	t.Parallel()

	input := &LedgerSettingsInput{Tracer: &TracerSettingsInput{Mode: testutils.Ptr(TracerModeEnforce)}}

	first := input.ToSparseMap()
	second := input.ToSparseMap()

	require.Equal(t, first, second)

	firstTracer, ok := first["tracer"].(map[string]any)
	require.True(t, ok)
	firstTracer["mode"] = TracerModeOff

	secondTracer, ok := second["tracer"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, TracerModeEnforce, secondTracer["mode"], "each call must return an independent map")
	assert.Equal(t, TracerModeEnforce, *input.Tracer.Mode, "ToSparseMap must not mutate the receiver")
}
