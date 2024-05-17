package http

import (
	"encoding/json"
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
