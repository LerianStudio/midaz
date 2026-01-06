package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			app.Get(tc.route, ParseUUIDPathParameters(tc.middleware), func(c *fiber.Ctx) error {
				return c.SendStatus(fiber.StatusOK)
			})

			req := httptest.NewRequest("GET", tc.requestPath, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
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

	v, _ := newValidator()

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

	v, _ := newValidator()

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

	v, _ := newValidator()

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

	v, _ := newValidator()

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

func TestValidateCPF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cpf      string
		expected bool
	}{
		{
			name:     "valid CPF",
			cpf:      "52998224725",
			expected: true,
		},
		{
			name:     "valid CPF 2",
			cpf:      "11144477735",
			expected: true,
		},
		{
			name:     "valid CPF 3",
			cpf:      "91315026015",
			expected: true,
		},
		{
			name:     "invalid CPF - wrong check digits",
			cpf:      "52998224700",
			expected: false,
		},
		{
			name:     "invalid CPF - all same digits 1",
			cpf:      "11111111111",
			expected: false,
		},
		{
			name:     "invalid CPF - all same digits 0",
			cpf:      "00000000000",
			expected: false,
		},
		{
			name:     "invalid CPF - all same digits 9",
			cpf:      "99999999999",
			expected: false,
		},
		{
			name:     "invalid CPF - wrong length short",
			cpf:      "1234567890",
			expected: false,
		},
		{
			name:     "invalid CPF - wrong length long",
			cpf:      "123456789012",
			expected: false,
		},
		{
			name:     "invalid CPF - contains letters",
			cpf:      "5299822472a",
			expected: false,
		},
		{
			name:     "empty CPF",
			cpf:      "",
			expected: true,
		},
	}

	v, _ := newValidator()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				CPF string `validate:"cpf"`
			}
			s := testStruct{CPF: tc.cpf}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateCNPJ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cnpj     string
		expected bool
	}{
		{
			name:     "valid CNPJ",
			cnpj:     "11222333000181",
			expected: true,
		},
		{
			name:     "valid CNPJ 2",
			cnpj:     "11444777000161",
			expected: true,
		},
		{
			name:     "valid CNPJ 3",
			cnpj:     "45997418000153",
			expected: true,
		},
		{
			name:     "invalid CNPJ - wrong check digits",
			cnpj:     "11222333000100",
			expected: false,
		},
		{
			name:     "invalid CNPJ - all same digits 1",
			cnpj:     "11111111111111",
			expected: false,
		},
		{
			name:     "invalid CNPJ - all same digits 0",
			cnpj:     "00000000000000",
			expected: false,
		},
		{
			name:     "invalid CNPJ - all same digits 9",
			cnpj:     "99999999999999",
			expected: false,
		},
		{
			name:     "invalid CNPJ - wrong length short",
			cnpj:     "1122233300018",
			expected: false,
		},
		{
			name:     "invalid CNPJ - wrong length long",
			cnpj:     "112223330001811",
			expected: false,
		},
		{
			name:     "invalid CNPJ - contains letters",
			cnpj:     "1122233300018a",
			expected: false,
		},
		{
			name:     "empty CNPJ",
			cnpj:     "",
			expected: true,
		},
	}

	v, _ := newValidator()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				CNPJ string `validate:"cnpj"`
			}
			s := testStruct{CNPJ: tc.cnpj}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateCPFCNPJ(t *testing.T) {
	t.Parallel()

	// Tests cpfcnpj combined validator - only length edge cases not covered by individual validators
	tests := []struct {
		name     string
		document string
		expected bool
	}{
		{
			name:     "invalid - wrong length 10 digits",
			document: "1234567890",
			expected: false,
		},
		{
			name:     "invalid - wrong length 12 digits",
			document: "123456789012",
			expected: false,
		},
		{
			name:     "invalid - wrong length 13 digits",
			document: "1234567890123",
			expected: false,
		},
		{
			name:     "invalid - wrong length 15 digits",
			document: "123456789012345",
			expected: false,
		},
		{
			name:     "empty document",
			document: "",
			expected: true,
		},
	}

	v, _ := newValidator()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type testStruct struct {
				Document string `validate:"cpfcnpj"`
			}
			s := testStruct{Document: tc.document}
			err := v.Struct(s)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
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
