package pkg

import (
	"testing"

	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/stretchr/testify/assert"
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

func TestRegexIgnoreAccents(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Test single lowercase character",
			input:    "a",
			expected: "[aáàãâ]",
		},
		{
			name:     "Test single uppercase character",
			input:    "A",
			expected: "[AÁÀÃÂ]",
		},
		{
			name:     "Test multiple characters",
			input:    "aAç",
			expected: "[aáàãâ][AÁÀÃÂ][cç]",
		},
		{
			name:     "Test no matching character",
			input:    "z",
			expected: "z",
		},
		{
			name:     "Test string with accented letters",
			input:    "áéíóú",
			expected: "[aáàãâ][eéèê][iíìî][oóòõô][uùúû]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RegexIgnoreAccents(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveChars(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		chars    map[string]bool
		expected string
	}{
		{
			name:     "Remove single character",
			str:      "hello",
			chars:    map[string]bool{"e": true},
			expected: "hllo",
		},
		{
			name:     "Remove characters not in string",
			str:      "hello",
			chars:    map[string]bool{"x": true},
			expected: "hello",
		},
		{
			name:     "Remove all characters",
			str:      "hello",
			chars:    map[string]bool{"h": true, "e": true, "l": true, "o": true},
			expected: "",
		},
		{
			name:     "Empty string",
			str:      "",
			chars:    map[string]bool{"a": true},
			expected: "",
		},
		{
			name:     "Remove no characters",
			str:      "hello",
			chars:    map[string]bool{},
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveChars(tt.str, tt.chars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReplaceUUIDWithPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Replace single UUID",
			path:     "/users/123e4567-e89b-12d3-a456-426614174000/posts",
			expected: "/users/:id/posts",
		},
		{
			name:     "Replace multiple UUIDs",
			path:     "/users/123e4567-e89b-12d3-a456-426614174000/posts/987e6543-a21b-34d5-b678-123456789012",
			expected: "/users/:id/posts/:id",
		},
		{
			name:     "No UUID in path",
			path:     "/users/hello/posts",
			expected: "/users/hello/posts",
		},
		{
			name:     "UUID in middle of path",
			path:     "/items/123e4567-e89b-12d3-a456-426614174000/details",
			expected: "/items/:id/details",
		},
		{
			name:     "Empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "Invalid UUID format",
			path:     "/users/invalid-uuid/posts",
			expected: "/users/invalid-uuid/posts", // Não há UUID válido para substituir
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceUUIDWithPlaceholder(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateServerAddress(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{
			name:     "Valid server address",
			value:    "localhost:8080",
			expected: "localhost:8080",
		},
		{
			name:     "Valid server address with domain",
			value:    "example.com:443",
			expected: "example.com:443",
		},
		{
			name:     "Invalid server address (no port)",
			value:    "localhost",
			expected: "",
		},
		{
			name:     "Invalid server address (no host)",
			value:    ":8080",
			expected: "",
		},
		{
			name:     "Invalid server address (non-numeric port)",
			value:    "localhost:abc",
			expected: "",
		},
		{
			name:     "Empty server address",
			value:    "",
			expected: "",
		},
		{
			name:     "Valid address with IP",
			value:    "192.168.1.1:8080",
			expected: "192.168.1.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateServerAddress(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}
