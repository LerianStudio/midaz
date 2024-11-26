package http

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/pkg"

	"github.com/gofiber/fiber/v2"
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
