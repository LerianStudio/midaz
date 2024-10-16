package http

import (
	"encoding/json"
	"github.com/LerianStudio/midaz/common"
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
	myMap := common.FieldValidations{
		"legalDocument":        "legalDocument is a required field",
		"legalName":            "legalName is a required field",
		"parentOrganizationId": "parentOrganizationId must be a valid UUID",
	}

	expected := common.FieldValidations{
		"legalDocument": "legalDocument is a required field",
		"legalName":     "legalName is a required field",
	}

	result := fieldsRequired(myMap)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Want: %v, got %v", expected, result)
	}
}

func TestFilterRequiredFieldWithNoFields(t *testing.T) {
	myMap := common.FieldValidations{
		"parentOrganizationId": "parentOrganizationId must be a valid UUID",
	}

	expected := make(common.FieldValidations)
	result := fieldsRequired(myMap)

	if len(result) > 0 {
		t.Errorf("Want %v, got %v", expected, result)
	}
}
