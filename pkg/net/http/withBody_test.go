// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/scheme"
)

type SimpleStruct struct {
	Name string
	Age  int
}

type ComplexStruct struct {
	Enable bool
	Simple SimpleStruct
}

func TestNewOfType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        any
		jsonData     string
		validateFunc func(t *testing.T, result any)
	}{
		{
			name:     "simple struct",
			input:    new(SimpleStruct),
			jsonData: `{"Name":"Bruce", "Age": 18}`,
			validateFunc: func(t *testing.T, result any) {
				s := result.(*SimpleStruct)
				assert.Equal(t, "Bruce", s.Name)
				assert.Equal(t, 18, s.Age)
			},
		},
		{
			name:     "complex nested struct",
			input:    new(ComplexStruct),
			jsonData: `{"Simple": {"Name":"Bruce", "Age": 18}}`,
			validateFunc: func(t *testing.T, result any) {
				s := result.(*ComplexStruct)
				assert.Equal(t, "Bruce", s.Simple.Name)
				assert.Equal(t, 18, s.Simple.Age)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newOfType(tc.input)
			err := json.Unmarshal([]byte(tc.jsonData), s)
			require.NoError(t, err)
			tc.validateFunc(t, s)
		})
	}
}

func TestFieldsRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    pkg.FieldValidations
		expected pkg.FieldValidations
	}{
		{
			name: "filters required fields only",
			input: pkg.FieldValidations{
				"legalDocument":        "legalDocument is a required field",
				"legalName":            "legalName is a required field",
				"parentOrganizationId": "parentOrganizationId must be a valid UUID",
			},
			expected: pkg.FieldValidations{
				"legalDocument": "legalDocument is a required field",
				"legalName":     "legalName is a required field",
			},
		},
		{
			name: "returns empty when no required fields",
			input: pkg.FieldValidations{
				"parentOrganizationId": "parentOrganizationId must be a valid UUID",
			},
			expected: pkg.FieldValidations{},
		},
		{
			name:     "handles empty input",
			input:    pkg.FieldValidations{},
			expected: pkg.FieldValidations{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := fieldsRequired(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseUUIDPathParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		route          string
		middleware     string
		requestPath    string
		expectedStatus int
	}{
		{
			name:           "valid single UUID",
			route:          "/v1/organizations/:id",
			middleware:     "organization",
			requestPath:    "/v1/organizations/123e4567-e89b-12d3-a456-426614174000",
			expectedStatus: fiber.StatusOK,
		},
		{
			name:           "valid multiple UUIDs",
			route:          "/v1/organizations/:organization_id/ledgers/:id",
			middleware:     "ledger",
			requestPath:    "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/c71ab589-cf46-4f2d-b6ef-b395c9a475da",
			expectedStatus: fiber.StatusOK,
		},
		{
			name:           "invalid UUID",
			route:          "/v1/organizations/:id",
			middleware:     "organization",
			requestPath:    "/v1/organizations/invalid-uuid",
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "valid first UUID invalid second UUID",
			route:          "/v1/organizations/:organization_id/ledgers/:id",
			middleware:     "ledger",
			requestPath:    "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/invalid-uuid",
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Get(tc.route, ParseUUIDPathParameters(tc.middleware), func(c fiber.Ctx) error {
				return c.SendStatus(fiber.StatusOK)
			})

			req := httptest.NewRequest("GET", tc.requestPath, nil)
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

// TestParseUUIDPathParameters_RouteWithoutParameters records what the middleware does on a
// route that declares NO path parameter: it walks the route's declared parameter names, so an
// empty list leaves it with nothing to parse and the request continues to the next handler.
//
// It is pinned because callers decide whether to keep the middleware on a chain by this
// behaviour, and the three possible answers — pass through, reject, panic — are not
// distinguishable by reading the signature.
func TestParseUUIDPathParameters_RouteWithoutParameters(t *testing.T) {
	t.Parallel()

	app := fiber.New()

	reached := false

	app.Post("/v2/transactions/direct", ParseUUIDPathParameters("transaction"), func(c fiber.Ctx) error {
		reached = true

		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodPost, "/v2/transactions/direct", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.True(t, reached, "a parameterless route must reach the next handler")
	assert.Equal(t, fiber.StatusOK, resp.StatusCode, "a parameterless route must not be rejected")
}

func TestFindUnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		original  map[string]any
		marshaled map[string]any
		expected  map[string]any
	}{
		{
			name: "basic comparison - finds missing field",
			original: map[string]any{
				"name": "John",
				"age":  30,
				"city": "New York",
			},
			marshaled: map[string]any{
				"name": "John",
				"age":  30,
			},
			expected: map[string]any{
				"city": "New York",
			},
		},
		{
			name:      "empty maps",
			original:  map[string]any{},
			marshaled: map[string]any{},
			expected:  map[string]any{},
		},
		{
			name: "identical maps",
			original: map[string]any{
				"name": "John",
				"age":  30,
			},
			marshaled: map[string]any{
				"name": "John",
				"age":  30,
			},
			expected: map[string]any{},
		},
		{
			name: "nested maps - finds nested difference",
			original: map[string]any{
				"person": map[string]any{
					"name":    "John",
					"age":     30,
					"address": "123 Main St",
				},
			},
			marshaled: map[string]any{
				"person": map[string]any{
					"name": "John",
					"age":  30,
				},
			},
			expected: map[string]any{
				"person": map[string]any{
					"address": "123 Main St",
				},
			},
		},
		{
			name: "slice comparison - finds extra elements",
			original: map[string]any{
				"tags": []any{"tag1", "tag2", "tag3"},
			},
			marshaled: map[string]any{
				"tags": []any{"tag1", "tag2"},
			},
			expected: map[string]any{
				"tags": []any{"tag3"},
			},
		},
		{
			name: "type mismatch - reports original value",
			original: map[string]any{
				"value": map[string]any{"nested": true},
			},
			marshaled: map[string]any{
				"value": "not a map",
			},
			expected: map[string]any{
				"value": map[string]any{"nested": true},
			},
		},
		{
			name: "different decimal values",
			original: map[string]any{
				"amount": "200.45",
			},
			marshaled: map[string]any{
				"amount": 200.46,
			},
			expected: map[string]any{
				"amount": "200.45",
			},
		},
		{
			name: "null value in original is not an unknown field",
			original: map[string]any{
				"name":  "John",
				"email": nil,
			},
			marshaled: map[string]any{
				"name": "John",
			},
			expected: map[string]any{},
		},
		{
			name: "nested null value in original is not an unknown field",
			original: map[string]any{
				"accountingEntries": map[string]any{
					"direct": map[string]any{"code": "1001"},
					"hold":   nil,
				},
			},
			marshaled: map[string]any{
				"accountingEntries": map[string]any{
					"direct": map[string]any{"code": "1001"},
				},
			},
			expected: map[string]any{},
		},
		{
			name: "non-null missing field is still unknown",
			original: map[string]any{
				"name":    "John",
				"unknown": "value",
			},
			marshaled: map[string]any{
				"name": "John",
			},
			expected: map[string]any{
				"unknown": "value",
			},
		},
		{
			name: "mixed null and non-null unknown fields",
			original: map[string]any{
				"name":       "John",
				"nullField":  nil,
				"extraField": "not allowed",
			},
			marshaled: map[string]any{
				"name": "John",
			},
			expected: map[string]any{
				"extraField": "not allowed",
			},
		},
		{
			// A bool field set to its zero value is dropped by json omitempty,
			// exactly like numeric zero; it is present-and-default, not unknown.
			name: "boolean false omitted by omitempty is not an unknown field",
			original: map[string]any{
				"name": "John",
				"fees": false,
			},
			marshaled: map[string]any{
				"name": "John",
			},
			expected: map[string]any{},
		},
		{
			// The per-call skip flags live in a nested object; an explicit
			// skip.fees=false must not be flagged as an unexpected field.
			name: "nested boolean false skip flag is not an unknown field",
			original: map[string]any{
				"skip": map[string]any{"fees": false, "tracer": false},
			},
			marshaled: map[string]any{
				"skip": map[string]any{},
			},
			expected: map[string]any{},
		},
		{
			// A bool sent as true is present in the marshaled output and equal,
			// so it produces no diff (sanity that the guard is zero-only).
			name: "boolean true present in both is not a diff",
			original: map[string]any{
				"fees": true,
			},
			marshaled: map[string]any{
				"fees": true,
			},
			expected: map[string]any{},
		},
		{
			// Documented tradeoff: a zero-valued unknown field is tolerated for
			// bools just as it already is for numerics (e.g. {"garbage": 0}).
			// Accepting an unmapped zero is harmless — it binds to no struct
			// field — and keeps the bool and numeric guards consistent.
			name: "unknown bool field at zero value is tolerated like numeric zero",
			original: map[string]any{
				"garbage": false,
			},
			marshaled: map[string]any{},
			expected:  map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diff := FindUnknownFields(tc.original, tc.marshaled)
			assert.Equal(t, tc.expected, diff)
		})
	}
}

func TestIsStringNumeric(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected bool
	}{
		{"200.00", true},
		{"200", true},
		{"-200.45", true},
		{"abc", false},
		{"12abc", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			result := isStringNumeric(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsDecimalEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		a        any
		b        any
		expected bool
	}{
		{
			name:     "string and float equal",
			a:        "100.00",
			b:        100.0,
			expected: false,
		},
		{
			name:     "string and decimal equal",
			a:        "100.00",
			b:        decimal.NewFromFloat(100.0),
			expected: true,
		},
		{
			name:     "float and decimal equal",
			a:        100.0,
			b:        decimal.NewFromFloat(100.0),
			expected: false,
		},
		{
			name:     "different values",
			a:        "100.01",
			b:        100.0,
			expected: false,
		},
		{
			name:     "invalid string",
			a:        "not-a-number",
			b:        100.0,
			expected: false,
		},
		{
			name:     "nil values",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil value",
			a:        "100.00",
			b:        nil,
			expected: false,
		},
		{
			name:     "unsupported types",
			a:        true,
			b:        100.0,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := isDecimalEqual(tc.a, tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMetadataValidation_KeyMaxLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "valid - exactly 100 chars",
			key:      string(make([]byte, 100)),
			expected: true,
		},
		{
			name:     "valid - empty key",
			key:      "",
			expected: true,
		},
		{
			name:     "valid - short key",
			key:      "department",
			expected: true,
		},
		{
			name:     "invalid - 101 chars",
			key:      string(make([]byte, 101)),
			expected: false,
		},
		{
			name:     "invalid - 200 chars",
			key:      string(make([]byte, 200)),
			expected: false,
		},
	}

	v, _, err := newValidator()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				Metadata map[string]any `validate:"dive,keys,keymax=100,endkeys"`
			}
			s := testStruct{Metadata: map[string]any{tc.key: "value"}}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestSchemeValidation_Tag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scheme   string
		expected bool
	}{
		{name: "valid - padded lowercase is normalizable", scheme: " pix ", expected: true},
		{name: "valid - empty is absent under omitempty", scheme: "", expected: true},
		{name: "invalid - punctuation", scheme: "pix!", expected: false},
		{name: "invalid - over max length", scheme: string(bytes.Repeat([]byte("A"), 51)), expected: false},
	}

	v, _, err := newValidator()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				Scheme string `json:"scheme" validate:"omitempty,scheme"`
			}
			s := testStruct{Scheme: tc.scheme}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestSchemeValidation_TranslatedFieldDetail(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Scheme string `json:"scheme" validate:"omitempty,scheme"`
	}

	err := ValidateStruct(&testStruct{Scheme: "pix!"})
	require.Error(t, err)

	var vErr *pkg.ValidationKnownFieldsError
	require.ErrorAs(t, err, &vErr)

	detail, ok := vErr.Fields["scheme"]
	require.True(t, ok, "fields = %v, want a scheme entry", vErr.Fields)
	assert.Equal(t, "scheme "+scheme.FormatHint, detail)
	assert.NotContains(t, detail, "Error:Field validation")
}

func TestMetadataValidation_ValueMaxLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{
			name:     "valid - string exactly 2000 chars",
			value:    string(make([]byte, 2000)),
			expected: true,
		},
		{
			name:     "valid - empty string",
			value:    "",
			expected: true,
		},
		{
			name:     "valid - short string",
			value:    "hello world",
			expected: true,
		},
		{
			name:     "valid - integer",
			value:    12345,
			expected: true,
		},
		{
			name:     "valid - float",
			value:    123.456,
			expected: true,
		},
		{
			name:     "valid - boolean true",
			value:    true,
			expected: true,
		},
		{
			name:     "valid - boolean false",
			value:    false,
			expected: true,
		},
		{
			name:     "invalid - string 2001 chars",
			value:    string(make([]byte, 2001)),
			expected: false,
		},
		{
			name:     "invalid - string 5000 chars",
			value:    string(make([]byte, 5000)),
			expected: false,
		},
	}

	v, _, err := newValidator()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				Metadata map[string]any `validate:"dive,keys,endkeys,valuemax=2000"`
			}
			s := testStruct{Metadata: map[string]any{"key": tc.value}}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestMetadataValidation_NestedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{
			name:     "valid - string value",
			value:    "simple string",
			expected: true,
		},
		{
			name:     "valid - integer value",
			value:    42,
			expected: true,
		},
		{
			name:     "valid - boolean value",
			value:    true,
			expected: true,
		},
		{
			name:     "invalid - nested map",
			value:    map[string]any{"nested": "value"},
			expected: false,
		},
		{
			name:     "invalid - deeply nested map",
			value:    map[string]any{"level1": map[string]any{"level2": "value"}},
			expected: false,
		},
	}

	v, _, err := newValidator()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				Metadata map[string]any `validate:"dive,keys,endkeys,nonested"`
			}
			s := testStruct{Metadata: map[string]any{"key": tc.value}}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestMetadataValidation_Combined(t *testing.T) {
	t.Parallel()

	// Tests combined validation rules - only cases not covered by individual tests
	tests := []struct {
		name     string
		metadata map[string]any
		expected bool
	}{
		{
			name:     "valid - multiple key-value pairs",
			metadata: map[string]any{"department": "finance", "active": true, "count": 42},
			expected: true,
		},
		{
			name:     "valid - empty metadata",
			metadata: map[string]any{},
			expected: true,
		},
		{
			name:     "invalid - multiple violations simultaneously",
			metadata: map[string]any{string(make([]byte, 101)): string(make([]byte, 2001))},
			expected: false,
		},
	}

	v, _, err := newValidator()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				Metadata map[string]any `validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
			}
			s := testStruct{Metadata: tc.metadata}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidateCreateAccountInput_ExternalType exercises the real
// mmodel.CreateAccountInput validation tags through ValidateStruct. It locks in
// that user-supplied type:"external" is accepted (the input contract is open)
// while the canonical @external/ alias prefix stays reserved and invalid alias
// characters are still rejected.
func TestValidateCreateAccountInput_ExternalType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   mmodel.CreateAccountInput
		wantErr bool
	}{
		{
			name: "valid - type external is accepted",
			input: mmodel.CreateAccountInput{
				AssetCode: "BRL",
				Type:      "external",
			},
			wantErr: false,
		},
		{
			name: "valid - type external mixed case is accepted",
			input: mmodel.CreateAccountInput{
				AssetCode: "BRL",
				Type:      "External",
			},
			wantErr: false,
		},
		{
			name: "valid - type deposit still accepted",
			input: mmodel.CreateAccountInput{
				AssetCode: "USD",
				Type:      "deposit",
			},
			wantErr: false,
		},
		{
			name: "valid - external type with normal alias",
			input: mmodel.CreateAccountInput{
				AssetCode: "BRL",
				Type:      "external",
				Alias:     ptr("treasury_external"),
			},
			wantErr: false,
		},
		{
			name: "invalid - reserved @external/ alias prefix rejected",
			input: mmodel.CreateAccountInput{
				AssetCode: "BRL",
				Type:      "external",
				Alias:     ptr("@external/BRL"),
			},
			wantErr: true,
		},
		{
			name: "invalid - reserved @external/ alias prefix rejected for non-external type",
			input: mmodel.CreateAccountInput{
				AssetCode: "USD",
				Type:      "deposit",
				Alias:     ptr("@external/USD"),
			},
			wantErr: true,
		},
		{
			name: "invalid - alias with illegal characters rejected",
			input: mmodel.CreateAccountInput{
				AssetCode: "BRL",
				Type:      "external",
				Alias:     ptr("bad alias!"),
			},
			wantErr: true,
		},
		{
			name: "invalid - missing required type",
			input: mmodel.CreateAccountInput{
				AssetCode: "BRL",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := tc.input
			err := ValidateStruct(&input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAreDatesEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{
			name:     "same RFC3339 dates",
			a:        "2024-01-15T10:30:00Z",
			b:        "2024-01-15T10:30:00Z",
			expected: true,
		},
		{
			name:     "RFC3339 vs RFC3339Nano",
			a:        "2024-01-15T10:30:00Z",
			b:        "2024-01-15T10:30:00.000000000Z",
			expected: true,
		},
		{
			name:     "RFC3339Nano with different precision",
			a:        "2024-01-15T10:30:00.123Z",
			b:        "2024-01-15T10:30:00.123000000Z",
			expected: true,
		},
		{
			name:     "different times",
			a:        "2024-01-15T10:30:00Z",
			b:        "2024-01-15T11:30:00Z",
			expected: false,
		},
		{
			name:     "different dates",
			a:        "2024-01-15T10:30:00Z",
			b:        "2024-01-16T10:30:00Z",
			expected: false,
		},
		{
			name:     "date only format same",
			a:        "2024-01-15",
			b:        "2024-01-15",
			expected: true,
		},
		{
			name:     "date only format different",
			a:        "2024-01-15",
			b:        "2024-01-16",
			expected: false,
		},
		{
			name:     "not a date string",
			a:        "not-a-date",
			b:        "also-not-a-date",
			expected: false,
		},
		{
			name:     "one valid date one invalid",
			a:        "2024-01-15T10:30:00Z",
			b:        "not-a-date",
			expected: false,
		},
		{
			name:     "milliseconds format 3 digits",
			a:        "2024-01-15T10:30:00.000Z",
			b:        "2024-01-15T10:30:00Z",
			expected: true,
		},
		{
			name:     "milliseconds format 2 digits",
			a:        "2024-01-15T10:30:00.00Z",
			b:        "2024-01-15T10:30:00Z",
			expected: true,
		},
		{
			name:     "milliseconds format 1 digit",
			a:        "2024-01-15T10:30:00.0Z",
			b:        "2024-01-15T10:30:00Z",
			expected: true,
		},
		{
			name:     "different milliseconds",
			a:        "2024-01-15T10:30:00.100Z",
			b:        "2024-01-15T10:30:00.200Z",
			expected: false,
		},
		{
			name:     "without timezone same",
			a:        "2024-01-15T10:30:00",
			b:        "2024-01-15T10:30:00",
			expected: true,
		},
		{
			name:     "without timezone different",
			a:        "2024-01-15T10:30:00",
			b:        "2024-01-15T11:30:00",
			expected: false,
		},
		{
			name:     "empty strings",
			a:        "",
			b:        "",
			expected: false,
		},
		{
			name:     "one empty string",
			a:        "2024-01-15T10:30:00Z",
			b:        "",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := areDatesEqual(tc.a, tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFindUnknownFields_DateComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		original  map[string]any
		marshaled map[string]any
		expected  map[string]any
	}{
		{
			name: "same date strings should not be flagged",
			original: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
			marshaled: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
			expected: map[string]any{},
		},
		{
			name: "equivalent dates with different formats should not be flagged",
			original: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
			marshaled: map[string]any{
				"createdAt": "2024-01-15T10:30:00.000000000Z",
			},
			expected: map[string]any{},
		},
		{
			name: "date with milliseconds vs without should not be flagged",
			original: map[string]any{
				"updatedAt": "2024-01-15T10:30:00.000Z",
			},
			marshaled: map[string]any{
				"updatedAt": "2024-01-15T10:30:00Z",
			},
			expected: map[string]any{},
		},
		{
			name: "different dates should be flagged",
			original: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
			marshaled: map[string]any{
				"createdAt": "2024-01-16T10:30:00Z",
			},
			expected: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
		},
		{
			name: "different times should be flagged",
			original: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
			marshaled: map[string]any{
				"createdAt": "2024-01-15T11:30:00Z",
			},
			expected: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
		},
		{
			name: "non-date string different values should be flagged",
			original: map[string]any{
				"name": "original-value",
			},
			marshaled: map[string]any{
				"name": "different-value",
			},
			expected: map[string]any{
				"name": "original-value",
			},
		},
		{
			name: "date only format same should not be flagged",
			original: map[string]any{
				"birthDate": "2024-01-15",
			},
			marshaled: map[string]any{
				"birthDate": "2024-01-15",
			},
			expected: map[string]any{},
		},
		{
			name: "date without timezone same should not be flagged",
			original: map[string]any{
				"scheduledAt": "2024-01-15T10:30:00",
			},
			marshaled: map[string]any{
				"scheduledAt": "2024-01-15T10:30:00",
			},
			expected: map[string]any{},
		},
		{
			name: "multiple date fields with mixed results",
			original: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
				"updatedAt": "2024-01-15T10:30:00.000Z",
				"deletedAt": "2024-01-20T10:30:00Z",
			},
			marshaled: map[string]any{
				"createdAt": "2024-01-15T10:30:00.000000000Z",
				"updatedAt": "2024-01-15T10:30:00Z",
				"deletedAt": "2024-01-21T10:30:00Z",
			},
			expected: map[string]any{
				"deletedAt": "2024-01-20T10:30:00Z",
			},
		},
		{
			name: "date string vs non-string marshaled value",
			original: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
			marshaled: map[string]any{
				"createdAt": 12345,
			},
			expected: map[string]any{
				"createdAt": "2024-01-15T10:30:00Z",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := FindUnknownFields(tc.original, tc.marshaled)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test struct with NullFields for populateNullFields tests
type StructWithNullFields struct {
	Name       string   `json:"name"`
	SegmentID  *string  `json:"segmentId"`
	EntityID   *string  `json:"entityId"`
	NullFields []string `json:"-"`
}

// Test struct without NullFields field
type StructWithoutNullFields struct {
	Name      string  `json:"name"`
	SegmentID *string `json:"segmentId"`
}

func TestPopulateNullFields_SetsNullFieldsFromOriginalMap(t *testing.T) {
	s := &StructWithNullFields{}
	originalMap := map[string]any{
		"name":      "Test Account",
		"segmentId": nil,
		"entityId":  nil,
	}

	populateNullFields(s, originalMap)

	assert.Len(t, s.NullFields, 2)
	assert.Contains(t, s.NullFields, "segmentId")
	assert.Contains(t, s.NullFields, "entityId")
}

func TestPopulateNullFields_IgnoresNonNilValues(t *testing.T) {
	s := &StructWithNullFields{}
	originalMap := map[string]any{
		"name":      "Test Account",
		"segmentId": "some-uuid-value",
		"entityId":  nil,
	}

	populateNullFields(s, originalMap)

	assert.Len(t, s.NullFields, 1)
	assert.Contains(t, s.NullFields, "entityId")
	assert.NotContains(t, s.NullFields, "segmentId")
}

func TestPopulateNullFields_NoOpWithoutNullFieldsField(t *testing.T) {
	s := &StructWithoutNullFields{}
	originalMap := map[string]any{
		"name":      "Test Account",
		"segmentId": nil,
	}

	// Should not panic or error
	populateNullFields(s, originalMap)

	// No assertion needed - just verify no panic
}

func TestPopulateNullFields_EmptyMapProducesEmptySlice(t *testing.T) {
	s := &StructWithNullFields{}
	originalMap := map[string]any{
		"name": "Test Account",
	}

	populateNullFields(s, originalMap)

	assert.Empty(t, s.NullFields)
}

func TestPopulateNullFields_NonPointerInputNoOp(t *testing.T) {
	s := StructWithNullFields{}
	originalMap := map[string]any{
		"segmentId": nil,
	}

	// Should not panic - just no-op for non-pointer
	populateNullFields(s, originalMap)
}

func TestPopulateNullFields_NilInputNoOp(t *testing.T) {
	originalMap := map[string]any{
		"segmentId": nil,
	}

	// Should not panic - just no-op for nil input
	populateNullFields(nil, originalMap)
}

type nullBarrierContact struct {
	PrimaryEmail *string `json:"primaryEmail,omitempty"`
}

type nullBarrierItem struct {
	Name *string `json:"name,omitempty"`
}

type nullBarrierRequest struct {
	Name       *string             `json:"name,omitempty"`
	ExternalID *string             `json:"externalId,omitempty"`
	Contact    *nullBarrierContact `json:"contact,omitempty"`
	Items      []nullBarrierItem   `json:"items,omitempty"`
	Tags       []*string           `json:"tags,omitempty"`
	Metadata   map[string]any      `json:"metadata,omitempty"`
	NullFields []string            `json:"-"`
}

func TestDecodeAndValidate_RejectsUndeclaredNullKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		location string
	}{
		{name: "dotted root key", body: `{"search.document": null}`, location: "search.document"},
		{name: "undeclared root key", body: `{"createdAt": null}`, location: "createdAt"},
		{name: "undeclared nested key", body: `{"contact": {"bogus": null}}`, location: "contact.bogus"},
		{name: "undeclared key inside array item", body: `{"items": [{"name": "a"}, {"bogus": null}]}`, location: "items[1].bogus"},
		{name: "internal control field sent as null", body: `{"NullFields": null}`, location: "NullFields"},
		{name: "internal control field sent with a value", body: `{"NullFields": ["x"]}`, location: "NullFields"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var input nullBarrierRequest
			originalMap, err := DecodeAndValidate([]byte(tc.body), &input)
			require.Error(t, err)
			assert.Nil(t, originalMap)

			var unknownErr pkg.ValidationUnknownFieldsError
			require.ErrorAs(t, err, &unknownErr)
			assert.Equal(t, "0053", unknownErr.Code)
			assert.Contains(t, unknownErr.Fields, tc.location)

			var detailedInput nullBarrierRequest
			_, details, detailedErr := DecodeAndValidateWithDetails([]byte(tc.body), &detailedInput)
			require.Error(t, detailedErr)
			assert.Contains(t, details, pkg.FieldError{Location: tc.location, Message: "unexpected field"})
		})
	}
}

func TestDecodeAndValidate_AcceptsDeclaredNullKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		nilFields []string
	}{
		{name: "declared root null", body: `{"name": "Jane", "externalId": null}`, nilFields: []string{"externalId"}},
		{name: "declared nested null", body: `{"contact": {"primaryEmail": null}}`, nilFields: []string{"contact.primaryEmail"}},
		{name: "declared null inside array item", body: `{"items": [{"name": null}]}`, nilFields: []string{}},
		{name: "null element of a declared slice", body: `{"tags": ["a", null]}`, nilFields: []string{}},
		{name: "open map accepts any null key", body: `{"metadata": {"anything.here": null}}`, nilFields: []string{"metadata.anything.here"}},
		{name: "body without any null", body: `{"name": "Jane", "items": [{"name": "a"}]}`, nilFields: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var input nullBarrierRequest
			originalMap, err := DecodeAndValidate([]byte(tc.body), &input)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.nilFields, FindNilFields(originalMap, ""))
		})
	}
}

func TestCollectNullPaths_SkipsBodiesWithoutNull(t *testing.T) {
	t.Parallel()

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"name": "Jane", "contact": {"primaryEmail": "a"}, "items": [{"name": "a"}]}`), &body))

	assert.Empty(t, collectNullPaths(body))
}

func TestCollectNullPaths_RendersMapAndArrayPaths(t *testing.T) {
	t.Parallel()

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"a": null, "b": {"c": null}, "d": [null, {"e": null}]}`), &body))

	assert.Equal(t, []nullPath{
		{path: "a", objectKey: true},
		{path: "b.c", objectKey: true},
		{path: "d[0]", objectKey: false},
		{path: "d[1].e", objectKey: true},
	}, collectNullPaths(body))
}

func TestUnknownNullFieldLeaf_LocksStdlibFormat(t *testing.T) {
	t.Parallel()

	dec := json.NewDecoder(bytes.NewReader([]byte(`{"contact": {"bo\"gus": null}}`)))
	dec.DisallowUnknownFields()

	err := dec.Decode(&nullBarrierRequest{})
	require.Error(t, err)

	leaf, ok := unknownFieldLeaf(err)
	require.True(t, ok, "encoding/json unknown-field text changed: %q", err.Error())
	assert.Equal(t, `bo"gus`, leaf)
}

func TestUnknownNullFieldLeaf_RefusesOtherErrors(t *testing.T) {
	t.Parallel()

	_, ok := unknownFieldLeaf(errors.New("json: cannot unmarshal number into Go struct field"))
	assert.False(t, ok)

	_, ok = unknownFieldLeaf(errors.New(jsonUnknownFieldPrefix + "unquoted"))
	assert.False(t, ok)
}

func TestUnknownNullFieldDetails_ReportsSingleCandidateWithoutProbing(t *testing.T) {
	t.Parallel()

	probe := func(string) (bool, bool) {
		t.Fatal("a single candidate must not be probed")

		return false, false
	}

	fields, details := undeclaredNullFieldDetails("bogus", []nullPath{
		{path: "contact.bogus", objectKey: true},
		{path: "notbogus", objectKey: true},
		{path: "contact.name", objectKey: true},
	}, probe)

	assert.Equal(t, pkg.UnknownFields{"contact.bogus": nil}, fields)
	assert.Equal(t, []pkg.FieldError{{Location: "contact.bogus", Message: "unexpected field"}}, details)
}

func TestUnknownNullFieldDetails_FallsBackToLeaf(t *testing.T) {
	t.Parallel()

	fields, details := undeclaredNullFieldDetails("bogus", []nullPath{{path: "contact.name", objectKey: true}}, nil)

	assert.Equal(t, pkg.UnknownFields{"bogus": nil}, fields)
	assert.Equal(t, []pkg.FieldError{{Location: "bogus", Message: "unexpected field"}}, details)
}

func TestUnknownNullFieldDetails_SkipsNullArrayElements(t *testing.T) {
	t.Parallel()

	fields, _ := undeclaredNullFieldDetails("bogus", []nullPath{
		{path: "bogus", objectKey: false},
		{path: "items[0].bogus", objectKey: true},
	}, nil)

	assert.Equal(t, pkg.UnknownFields{"items[0].bogus": nil}, fields)
}

func TestUnknownNullFieldDetails_ReportsEveryCandidateWhenProbeCannotDecide(t *testing.T) {
	t.Parallel()

	probe := func(path string) (bool, bool) {
		return path == "bogus", path == "bogus"
	}

	fields, details := undeclaredNullFieldDetails("bogus", []nullPath{
		{path: "bogus", objectKey: true},
		{path: "contact.bogus", objectKey: true},
	}, probe)

	assert.Equal(t, pkg.UnknownFields{"bogus": nil, "contact.bogus": nil}, fields)
	assert.Equal(t, []pkg.FieldError{
		{Location: "bogus", Message: "unexpected field"},
		{Location: "contact.bogus", Message: "unexpected field"},
	}, details)
}

func TestNullKeyRefused_CannotDecideOnOtherDecodeFailures(t *testing.T) {
	t.Parallel()

	originalMap := map[string]any{"name": 1.0, "zbogus": nil}

	refused, ok := nullKeyRefused(originalMap, "zbogus", &nullBarrierRequest{}, "zbogus")
	assert.False(t, refused)
	assert.False(t, ok)
}

func TestDecodeAndValidate_DisambiguatesNullKeysSharingALeaf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		fields pkg.UnknownFields
	}{
		{
			name:   "declared root key shares the leaf of an undeclared nested key",
			body:   `{"name": null, "contact": {"name": null}}`,
			fields: pkg.UnknownFields{"contact.name": nil},
		},
		{
			name:   "every candidate undeclared",
			body:   `{"bogus": null, "contact": {"bogus": null}}`,
			fields: pkg.UnknownFields{"bogus": nil, "contact.bogus": nil},
		},
		{
			name:   "two of three candidates undeclared",
			body:   `{"primaryEmail": null, "contact": {"primaryEmail": null}, "items": [{"primaryEmail": null}]}`,
			fields: pkg.UnknownFields{"primaryEmail": nil, "items[0].primaryEmail": nil},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var input nullBarrierRequest
			_, details, err := DecodeAndValidateWithDetails([]byte(tc.body), &input)
			require.Error(t, err)

			var unknownErr pkg.ValidationUnknownFieldsError
			require.ErrorAs(t, err, &unknownErr)
			assert.Equal(t, "0053", unknownErr.Code)
			assert.Equal(t, tc.fields, unknownErr.Fields)
			assert.Equal(t, unknownFieldDetailsFallback(tc.fields), details)
		})
	}
}

func TestUnknownNullFields_ReturnsUnmarshallingErrorForOtherDecodeFailures(t *testing.T) {
	t.Parallel()

	body := []byte(`{"name": 1, "bogus": null}`)

	var originalMap map[string]any
	require.NoError(t, json.Unmarshal(body, &originalMap))

	fields, details, err := findUndeclaredNullFields(body, &nullBarrierRequest{}, originalMap)
	require.Error(t, err)
	assert.Nil(t, fields)
	assert.Nil(t, details)

	var responseErr pkg.ResponseError
	assert.ErrorAs(t, err, &responseErr)
}

func TestDecodeAndValidate_NullFieldsReceiveOnlyDeclaredKeys(t *testing.T) {
	t.Parallel()

	var rejected StructWithNullFields
	_, err := DecodeAndValidate([]byte(`{"segmentId": null, "bogus": null}`), &rejected)
	require.Error(t, err)

	var unknownErr pkg.ValidationUnknownFieldsError
	require.ErrorAs(t, err, &unknownErr)
	assert.Equal(t, "0053", unknownErr.Code)
	assert.Equal(t, pkg.UnknownFields{"bogus": nil}, unknownErr.Fields)
	assert.Empty(t, rejected.NullFields)

	var accepted StructWithNullFields
	_, err = DecodeAndValidate([]byte(`{"segmentId": null}`), &accepted)
	require.NoError(t, err)
	assert.Equal(t, []string{"segmentId"}, accepted.NullFields)
}
