package utils

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/ptr"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		expected string
	}{
		{
			name:     "Single command",
			commands: []string{"ls"},
			expected: "ls",
		},
		{
			name:     "Multiple commands",
			commands: []string{"ls", "pwd", "cd /"},
			expected: "ls\npwd\ncd /",
		},
		{
			name:     "No commands",
			commands: []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Format(tt.commands...)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFlagFileUnmarshalJSON(t *testing.T) {
	type mockRequest struct {
		Key string `json:"key"`
	}

	tests := []struct {
		name     string
		path     string
		content  string
		expected mockRequest
		hasError bool
	}{
		{
			name:     "Valid JSON from file",
			path:     "testfile.json",
			content:  `{"key": "value"}`,
			expected: mockRequest{Key: "value"},
			hasError: false,
		},
		{
			name:     "Invalid JSON from file",
			path:     "testfile.json",
			content:  `{"key": value}`, // Invalid JSON
			expected: mockRequest{},
			hasError: true,
		},
		{
			name:     "Non-existent file",
			path:     "nonexistent.json",
			content:  "",
			expected: mockRequest{},
			hasError: true,
		},
		{
			name:     "Valid JSON from stdin",
			path:     "-",
			content:  `{"key": "stdin_value"}`,
			expected: mockRequest{Key: "stdin_value"},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request mockRequest

			if tt.path == "-" {
				oldStdin := os.Stdin
				defer func() { os.Stdin = oldStdin }()

				r, w, _ := os.Pipe()
				_, _ = w.Write([]byte(tt.content))
				_ = w.Close()
				os.Stdin = r
			} else if tt.content != "" {
				tmpFile, err := os.CreateTemp("", filepath.Base(tt.path))
				if err != nil {
					t.Fatalf("Failed to create temporary file: %v", err)
				}
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.Write([]byte(tt.content))
				if err != nil {
					t.Fatalf("Failed to write to temporary file: %v", err)
				}
				_ = tmpFile.Close()
				tt.path = tmpFile.Name()
			}

			err := FlagFileUnmarshalJSON(tt.path, &request)
			if tt.hasError {
				if err == nil {
					t.Errorf("expected an error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if request != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, request)
				}
			}
		})
	}
}
func TestUnmarshalJSONFromReader(t *testing.T) {
	type mockObject struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	tests := []struct {
		name     string
		input    string
		expected mockObject
		hasError bool
	}{
		{
			name:     "Valid JSON",
			input:    `{"name": "John", "age": 30}`,
			expected: mockObject{Name: "John", Age: 30},
			hasError: false,
		},
		{
			name:     "Invalid JSON",
			input:    `{"name": "John", "age": }`, // Invalid JSON
			expected: mockObject{},
			hasError: true,
		},
		{
			name:     "Empty JSON",
			input:    `{}`,
			expected: mockObject{},
			hasError: false,
		},
		{
			name:     "Partial JSON",
			input:    `{"name": "Jane"}`,
			expected: mockObject{Name: "Jane"},
			hasError: false,
		},
		{
			name:     "Empty input",
			input:    ``,
			expected: mockObject{},
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result mockObject

			err := UnmarshalJSONFromReader(bytes.NewReader([]byte(tt.input)), &result)

			if tt.hasError {
				if err == nil {
					t.Errorf("expected an error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if !tt.hasError && result != tt.expected {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestWriteDetailsToFile(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		outPath  string
		hasError bool
	}{
		{
			name:     "Write to valid path",
			data:     []byte("Hello, World!"),
			outPath:  "testdata/output1.txt",
			hasError: false,
		},
		{
			name:     "Empty data",
			data:     []byte(""),
			outPath:  "testdata/output2.txt",
			hasError: false,
		},
		{
			name:     "Invalid directory path",
			data:     []byte("Invalid path test"),
			outPath:  "/invalid/path/output3.txt", // Assuming the path is invalid
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if !tt.hasError {
					_ = os.RemoveAll(filepath.Dir(tt.outPath))
				}
			}()

			err := WriteDetailsToFile(tt.data, tt.outPath)

			if tt.hasError {
				if err == nil {
					t.Errorf("expected an error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if _, err := os.Stat(tt.outPath); os.IsNotExist(err) {
					t.Errorf("expected file to be created but it does not exist")
				} else {
					content, _ := os.ReadFile(tt.outPath)
					if string(content) != string(tt.data) {
						t.Errorf("expected file content %q, got %q", string(tt.data), string(content))
					}
				}
			}
		})
	}
}

func TestFormatAskFieldRequired(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected string
	}{
		{
			name:     "Valid field name",
			field:    "email",
			expected: "Enter the email field",
		},
		{
			name:     "Empty field name",
			field:    "",
			expected: "Enter the  field",
		},
		{
			name:     "Field with spaces",
			field:    "first name",
			expected: "Enter the first name field",
		},
		{
			name:     "Field with special characters",
			field:    "phone_number@!",
			expected: "Enter the phone_number@! field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatAskFieldRequired(tt.field)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestAssignStringField(t *testing.T) {
	mockInputFunc := func(expectedInput string, mockResponse string, mockError error) func(string) (string, error) {
		return func(input string) (string, error) {
			if input != expectedInput {
				t.Errorf("expected input %q, got %q", expectedInput, input)
			}
			return mockResponse, mockError
		}
	}

	tests := []struct {
		name        string
		flagValue   string
		fieldName   string
		inputFunc   func(string) (string, error)
		expected    string
		expectError bool
	}{
		{
			name:        "Flag value provided",
			flagValue:   "value_from_flag",
			fieldName:   "exampleField",
			inputFunc:   nil, // inputFunc is not used in this case
			expected:    "value_from_flag",
			expectError: false,
		},
		{
			name:        "Flag value not provided, valid input from function",
			flagValue:   "",
			fieldName:   "exampleField",
			inputFunc:   mockInputFunc("exampleField", "value_from_input", nil),
			expected:    "value_from_input",
			expectError: false,
		},
		{
			name:        "Flag value not provided, error from input function",
			flagValue:   "",
			fieldName:   "exampleField",
			inputFunc:   mockInputFunc("exampleField", "", errors.New("input error")),
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AssignStringField(tt.flagValue, tt.fieldName, tt.inputFunc)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected an error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

func TestParseAndAssign(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		parseFunc   func(string) (int, error)
		expected    *int
		expectError bool
	}{
		{
			name:  "Valid value",
			value: "42",
			parseFunc: func(s string) (int, error) {
				return strconv.Atoi(s)
			},
			expected:    ptr.IntPtr(42),
			expectError: false,
		},
		{
			name:  "Empty value",
			value: "",
			parseFunc: func(s string) (int, error) {
				return strconv.Atoi(s)
			},
			expected:    nil,
			expectError: false,
		},
		{
			name:  "Invalid value",
			value: "invalid",
			parseFunc: func(s string) (int, error) {
				return strconv.Atoi(s)
			},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAndAssign(tt.value, tt.parseFunc)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected an error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.expected == nil {
					if result != nil {
						t.Errorf("expected nil, got %v", result)
					}
				} else {
					if result == nil || *result != *tt.expected {
						t.Errorf("expected %v, got %v", *tt.expected, result)
					}
				}
			}
		})
	}
}

func TestAssignOptionalStringPtr(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		expected  *string
	}{
		{
			name:      "Valid string value",
			flagValue: "hello",
			expected:  ptr.StringPtr("hello"),
		},
		{
			name:      "Empty string value",
			flagValue: "",
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AssignOptionalStringPtr(tt.flagValue)

			// Validate result
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil || *result != *tt.expected {
					t.Errorf("expected %v, got %v", *tt.expected, result)
				}
			}
		})
	}
}

type Parent struct {
	Field *string
}

func TestSafeNestedString(t *testing.T) {
	tests := []struct {
		name      string
		parent    *Parent
		fieldFunc func(*Parent) *string
		expected  string
	}{
		{
			name: "Valid parent and field",
			parent: &Parent{
				Field: ptr.StringPtr("hello"),
			},
			fieldFunc: func(p *Parent) *string { return p.Field },
			expected:  "hello",
		},
		{
			name:      "Parent is nil",
			parent:    nil,
			fieldFunc: func(p *Parent) *string { return nil },
			expected:  "",
		},
		{
			name: "Field is nil",
			parent: &Parent{
				Field: nil,
			},
			fieldFunc: func(p *Parent) *string { return p.Field },
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeNestedString(tt.parent, tt.fieldFunc)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSafeString(t *testing.T) {
	tests := []struct {
		name     string
		value    *string
		expected string
	}{
		{
			name:     "Non-nil string value",
			value:    ptr.StringPtr("hello"),
			expected: "hello",
		},
		{
			name:     "Nil string value",
			value:    nil,
			expected: "",
		},
		{
			name:     "Empty string value",
			value:    ptr.StringPtr(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeString(tt.value)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name          string
		date          string
		expectError   bool
		expectedError string
	}{
		{
			name:          "Valid date",
			date:          "2023-12-01",
			expectError:   false,
			expectedError: "",
		},
		{
			name:          "Invalid date format",
			date:          "01-12-2023",
			expectError:   true,
			expectedError: "invalid date format: expected YYYY-MM-DD",
		},
		{
			name:          "Empty date",
			date:          "",
			expectError:   true,
			expectedError: "invalid date format: expected YYYY-MM-DD",
		},
		{
			name:          "Invalid characters in date",
			date:          "20xx-12-01",
			expectError:   true,
			expectedError: "invalid date format: expected YYYY-MM-DD",
		},
		{
			name:          "Nonexistent date",
			date:          "2023-02-30",
			expectError:   true,
			expectedError: "invalid date format: expected YYYY-MM-DD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDate(tt.date)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected an error but got nil")
				} else if err.Error() != tt.expectedError {
					t.Errorf("expected error %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
