package http

import (
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"reflect"
	"testing"
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

	app.Get("/v1/organizations/:id", ParseUUIDPathParameters, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK) // Se o middleware passar, responde com 200
	})

	req := httptest.NewRequest("GET", "/v1/organizations/123e4567-e89b-12d3-a456-426614174000", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestParseUUIDPathParameters_MultipleValidUUID(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/organizations/:organization_id/ledgers/:id", ParseUUIDPathParameters, func(c *fiber.Ctx) error {
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

	app.Get("/v1/organizations/:id", ParseUUIDPathParameters, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/v1/organizations/invalid-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestParseUUIDPathParameters_ValidAndInvalidUUID(t *testing.T) {
	app := fiber.New()

	app.Get("/v1/organizations/:organization_id/ledgers/:id", ParseUUIDPathParameters, func(c *fiber.Ctx) error {
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
