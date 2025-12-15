package http

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
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

func TestNewOfTypeWithSimpleStruct(t *testing.T) {
	s := newOfType(new(SimpleStruct))

	if err := json.Unmarshal([]byte("{\"Name\":\"Bruce\", \"Age\": 18}"), s); err != nil {
		t.Error(err)
	}

	sPrt := s.(*SimpleStruct)

	if sPrt.Name != "Bruce" || sPrt.Age != 18 {
		t.Error("Wrong data.")
	}
}

func TestNewOfTypeWithComplexStruct(t *testing.T) {
	s := newOfType(new(ComplexStruct))

	if err := json.Unmarshal([]byte("{\"Simple\": {\"Name\":\"Bruce\", \"Age\": 18}}"), s); err != nil {
		t.Error(err)
	}

	sPrt := s.(*ComplexStruct)

	if sPrt.Simple.Name != "Bruce" || sPrt.Simple.Age != 18 {
		t.Error("Wrong data.")
	}
}

func TestFilterRequiredFields(t *testing.T) {
	myMap := pkg.FieldValidations{
		"legalDocument":        "legalDocument is a required field",
		"legalName":            "legalName is a required field",
		"parentOrganizationId": "parentOrganizationId must be a valid UUID",
	}

	expected := pkg.FieldValidations{
		"legalDocument": "legalDocument is a required field",
		"legalName":     "legalName is a required field",
	}

	result := fieldsRequired(myMap)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Want: %v, got %v", expected, result)
	}
}

func TestFilterRequiredFieldWithNoFields(t *testing.T) {
	myMap := pkg.FieldValidations{
		"parentOrganizationId": "parentOrganizationId must be a valid UUID",
	}

	expected := make(pkg.FieldValidations)
	result := fieldsRequired(myMap)

	if len(result) > 0 {
		t.Errorf("Want %v, got %v", expected, result)
	}
}

func TestParseUUIDPathParameters_ValidUUID(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/organizations/:id", ParseUUIDPathParameters("organization"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK) // Se o middleware passar, responde com 200
	})

	req := httptest.NewRequest("GET", "/v1/organizations/123e4567-e89b-12d3-a456-426614174000", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestParseUUIDPathParameters_MultipleValidUUID(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/organizations/:organization_id/ledgers/:id", ParseUUIDPathParameters("ledger"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(
		"GET",
		"/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/c71ab589-cf46-4f2d-b6ef-b395c9a475da",
		nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestParseUUIDPathParameters_InvalidUUID(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/organizations/:id", ParseUUIDPathParameters("organization"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/v1/organizations/invalid-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestParseUUIDPathParameters_ValidAndInvalidUUID(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/organizations/:organization_id/ledgers/:id", ParseUUIDPathParameters("ledger"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(
		"GET",
		"/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/invalid-uuid",
		nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestFindUnknownFields_BasicComparison(t *testing.T) {
	original := map[string]any{
		"name": "John",
		"age":  30,
		"city": "New York",
	}

	marshaled := map[string]any{
		"name": "John",
		"age":  30,
	}

	diff := FindUnknownFields(original, marshaled)

	expected := map[string]any{
		"city": "New York",
	}

	assert.Equal(t, expected, diff)
}

func TestFindUnknownFields_EmptyMaps(t *testing.T) {
	original := map[string]any{}
	marshaled := map[string]any{}

	diff := FindUnknownFields(original, marshaled)
	assert.Empty(t, diff)
}

func TestFindUnknownFields_IdenticalMaps(t *testing.T) {
	original := map[string]any{
		"name": "John",
		"age":  30,
	}

	marshaled := map[string]any{
		"name": "John",
		"age":  30,
	}

	diff := FindUnknownFields(original, marshaled)
	assert.Empty(t, diff)
}

func TestFindUnknownFields_NestedMaps(t *testing.T) {
	original := map[string]any{
		"person": map[string]any{
			"name":    "John",
			"age":     30,
			"address": "123 Main St",
		},
	}

	marshaled := map[string]any{
		"person": map[string]any{
			"name": "John",
			"age":  30,
		},
	}

	diff := FindUnknownFields(original, marshaled)

	expected := map[string]any{
		"person": map[string]any{
			"address": "123 Main St",
		},
	}

	assert.Equal(t, expected, diff)
}

func TestFindUnknownFields_SliceComparison(t *testing.T) {
	original := map[string]any{
		"tags": []any{"tag1", "tag2", "tag3"},
	}

	marshaled := map[string]any{
		"tags": []any{"tag1", "tag2"},
	}

	diff := FindUnknownFields(original, marshaled)

	expected := map[string]any{
		"tags": []any{"tag3"},
	}

	assert.Equal(t, expected, diff)
}

func TestFindUnknownFields_TypeMismatch(t *testing.T) {
	original := map[string]any{
		"value": map[string]any{"nested": true},
	}

	marshaled := map[string]any{
		"value": "not a map",
	}

	diff := FindUnknownFields(original, marshaled)

	expected := map[string]any{
		"value": map[string]any{"nested": true},
	}

	assert.Equal(t, expected, diff)
}

func TestFindUnknownFields_DecimalValues(t *testing.T) {
	tests := []struct {
		name      string
		original  map[string]any
		marshaled map[string]any
		expected  map[string]any
	}{
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
			diff := FindUnknownFields(tc.original, tc.marshaled)
			assert.Equal(t, tc.expected, diff)
		})
	}
}

func TestIsStringNumeric(t *testing.T) {
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

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := isStringNumeric(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestIsDecimalEqual(t *testing.T) {
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
			result := isDecimalEqual(tc.a, tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestValidateCPF(t *testing.T) {
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
	tests := []struct {
		name     string
		document string
		expected bool
	}{
		{
			name:     "valid CPF",
			document: "52998224725",
			expected: true,
		},
		{
			name:     "valid CNPJ",
			document: "11222333000181",
			expected: true,
		},
		{
			name:     "invalid CPF",
			document: "12345678901",
			expected: false,
		},
		{
			name:     "invalid CNPJ",
			document: "12345678901234",
			expected: false,
		},
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
			result := areDatesEqual(tc.a, tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFindUnknownFields_DateComparison(t *testing.T) {
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
			result := FindUnknownFields(tc.original, tc.marshaled)
			assert.Equal(t, tc.expected, result)
		})
	}
}

