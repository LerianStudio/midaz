package common

import (
	"testing"

	"github.com/LerianStudio/midaz/common/mpointers"
)

func Test_RemoveAccents(t *testing.T) {
	want := "aaaaeeeiiioooouuu"
	got, err := RemoveAccents("àáãâèéêìíîòóôõùúû")
	if err != nil {
		t.Error(err)
		return
	}
	if got != want {
		t.Errorf("Want: %s, got: %s", want, got)
	}
}

func Test_RemoveSpaces(t *testing.T) {
	want := "foobar"
	got := RemoveSpaces("foo bar")
	if got != want {
		t.Errorf("Want: %s, got: %s", want, got)
	}
}

func Test_IsEmpty(t *testing.T) {
	m := map[*string]bool{
		mpointers.String("foo"):     false,
		mpointers.String(""):        true,
		mpointers.String(" "):       true,
		mpointers.String("       "): true,
		mpointers.String(" bar "):   false,
		nil:                         true,
	}
	for str, want := range m {
		got := IsNilOrEmpty(str)
		if want != got {
			value := "nil"
			if str != nil {
				value = *str
			}
			t.Errorf("Want: %v, got: %v to value \"%v\"", want, IsNilOrEmpty(str), value)
		}
	}
}

func TestCamelToSnakeCase(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "AllLowerCase",
			input:    "goland",
			expected: "goland",
		},
		{
			name:     "AllUpperCase",
			input:    "GOLAND",
			expected: "g_o_l_a_n_d",
		},
		{
			name:     "LeadingUpperCase",
			input:    "GoLand",
			expected: "go_land",
		},
		{
			name:     "MixedUpperLowerCase",
			input:    "GoLand2023",
			expected: "go_land2023",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := CamelToSnakeCase(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s but got %s", tc.expected, result)
			}
		})
	}
}
