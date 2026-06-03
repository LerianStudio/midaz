// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMapNumKinds(t *testing.T) {
	t.Parallel()

	result := GetMapNumKinds()

	// Verify expected numeric kinds are present
	expectedKinds := []reflect.Kind{
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Float32,
		reflect.Float64,
	}

	for _, kind := range expectedKinds {
		assert.True(t, result[kind], "Expected kind %v to be true", kind)
	}

	// Verify non-numeric kinds are not present
	nonNumericKinds := []reflect.Kind{
		reflect.String,
		reflect.Bool,
		reflect.Slice,
		reflect.Map,
		reflect.Struct,
		reflect.Ptr,
	}

	for _, kind := range nonNumericKinds {
		assert.False(t, result[kind], "Expected kind %v to be false or not present", kind)
	}

	// Verify the map has exactly 7 entries
	assert.Equal(t, 7, len(result))
}

func TestIsNilOrEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *string
		expected bool
	}{
		{
			name:     "Nil pointer",
			input:    nil,
			expected: true,
		},
		{
			name:     "Empty string",
			input:    strPtr(""),
			expected: true,
		},
		{
			name:     "Whitespace only",
			input:    strPtr("   "),
			expected: true,
		},
		{
			name:     "Null string literal",
			input:    strPtr("null"),
			expected: true,
		},
		{
			name:     "Nil string literal",
			input:    strPtr("nil"),
			expected: true,
		},
		{
			name:     "Null with whitespace",
			input:    strPtr("  null  "),
			expected: true,
		},
		{
			name:     "Nil with whitespace",
			input:    strPtr("  nil  "),
			expected: true,
		},
		{
			name:     "Valid string",
			input:    strPtr("hello"),
			expected: false,
		},
		{
			name:     "Valid string with spaces",
			input:    strPtr("  hello  "),
			expected: false,
		},
		{
			name:     "Single character",
			input:    strPtr("a"),
			expected: false,
		},
		{
			name:     "Number string",
			input:    strPtr("123"),
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsNilOrEmpty(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsOutputFormatValuesValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "HTML uppercase",
			input:    "HTML",
			expected: true,
		},
		{
			name:     "HTML lowercase",
			input:    "html",
			expected: true,
		},
		{
			name:     "HTML mixed case",
			input:    "Html",
			expected: true,
		},
		{
			name:     "PDF uppercase",
			input:    "PDF",
			expected: true,
		},
		{
			name:     "PDF lowercase",
			input:    "pdf",
			expected: true,
		},
		{
			name:     "CSV uppercase",
			input:    "CSV",
			expected: true,
		},
		{
			name:     "CSV lowercase",
			input:    "csv",
			expected: true,
		},
		{
			name:     "XML uppercase",
			input:    "XML",
			expected: true,
		},
		{
			name:     "XML lowercase",
			input:    "xml",
			expected: true,
		},
		{
			name:     "TXT uppercase",
			input:    "TXT",
			expected: true,
		},
		{
			name:     "TXT lowercase",
			input:    "txt",
			expected: true,
		},
		{
			name:     "Invalid format - JSON",
			input:    "JSON",
			expected: false,
		},
		{
			name:     "Invalid format - XLSX",
			input:    "XLSX",
			expected: false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "Invalid format - DOC",
			input:    "DOC",
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := tt.input
			result := IsOutputFormatValuesValid(&input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateFormDataFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		outFormat   *string
		description *string
		expectError bool
	}{
		{
			name:        "Valid fields",
			outFormat:   strPtr("PDF"),
			description: strPtr("Test description"),
			expectError: false,
		},
		{
			name:        "Nil output format",
			outFormat:   nil,
			description: strPtr("Test description"),
			expectError: true,
		},
		{
			name:        "Empty output format",
			outFormat:   strPtr(""),
			description: strPtr("Test description"),
			expectError: true,
		},
		{
			name:        "Nil description",
			outFormat:   strPtr("PDF"),
			description: nil,
			expectError: true,
		},
		{
			name:        "Empty description",
			outFormat:   strPtr("PDF"),
			description: strPtr(""),
			expectError: true,
		},
		{
			name:        "Invalid output format",
			outFormat:   strPtr("INVALID"),
			description: strPtr("Test description"),
			expectError: true,
		},
		{
			name:        "Both nil",
			outFormat:   nil,
			description: nil,
			expectError: true,
		},
		{
			name:        "Valid HTML format",
			outFormat:   strPtr("HTML"),
			description: strPtr("HTML report"),
			expectError: false,
		},
		{
			name:        "Valid lowercase format",
			outFormat:   strPtr("csv"),
			description: strPtr("CSV export"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateFormDataFields(tt.outFormat, tt.description)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateFileFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		outFormat    string
		templateFile string
		expectError  bool
	}{
		// HTML tests
		{
			name:         "Valid HTML with html tag",
			outFormat:    "HTML",
			templateFile: "<html><body>Hello</body></html>",
			expectError:  false,
		},
		{
			name:         "Valid HTML with DOCTYPE",
			outFormat:    "HTML",
			templateFile: "<!DOCTYPE html><html><body>Hello</body></html>",
			expectError:  false,
		},
		{
			name:         "Invalid HTML - no html tag",
			outFormat:    "HTML",
			templateFile: "Just plain text",
			expectError:  true,
		},
		// PDF tests
		{
			name:         "Valid PDF with html tag",
			outFormat:    "PDF",
			templateFile: "<html><body>PDF Content</body></html>",
			expectError:  false,
		},
		{
			name:         "Valid PDF with DOCTYPE",
			outFormat:    "PDF",
			templateFile: "<!DOCTYPE html><html><body>PDF Content</body></html>",
			expectError:  false,
		},
		{
			name:         "Invalid PDF - no html tag",
			outFormat:    "PDF",
			templateFile: "Plain text for PDF",
			expectError:  true,
		},
		// XML tests
		{
			name:         "Valid XML with declaration",
			outFormat:    "XML",
			templateFile: "<?xml version=\"1.0\"?><root><item>value</item></root>",
			expectError:  false,
		},
		{
			name:         "Valid XML with tag",
			outFormat:    "XML",
			templateFile: "<root><item>value</item></root>",
			expectError:  false,
		},
		{
			name:         "Invalid XML - no tags",
			outFormat:    "XML",
			templateFile: "Just plain text without any XML",
			expectError:  true,
		},
		// CSV tests
		{
			name:         "Valid CSV with comma",
			outFormat:    "CSV",
			templateFile: "name,age,email\nJohn,30,john@example.com",
			expectError:  false,
		},
		{
			name:         "Valid CSV with semicolon",
			outFormat:    "CSV",
			templateFile: "name;age;email\nJohn;30;john@example.com",
			expectError:  false,
		},
		{
			name:         "Invalid CSV - single line",
			outFormat:    "CSV",
			templateFile: "only one line",
			expectError:  true,
		},
		{
			name:         "Invalid CSV - no delimiter",
			outFormat:    "CSV",
			templateFile: "no delimiters here\nsecond line",
			expectError:  true,
		},
		// TXT tests
		{
			name:         "Valid TXT with content",
			outFormat:    "TXT",
			templateFile: "This is plain text content",
			expectError:  false,
		},
		{
			name:         "Invalid TXT - empty",
			outFormat:    "TXT",
			templateFile: "",
			expectError:  true,
		},
		{
			name:         "Invalid TXT - whitespace only",
			outFormat:    "TXT",
			templateFile: "   \n\t\n   ",
			expectError:  true,
		},
		// Case insensitivity
		{
			name:         "Lowercase html format",
			outFormat:    "html",
			templateFile: "<html><body>Content</body></html>",
			expectError:  false,
		},
		{
			name:         "Mixed case PDF format",
			outFormat:    "Pdf",
			templateFile: "<!DOCTYPE html><html><body>PDF</body></html>",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateFileFormat(tt.outFormat, tt.templateFile)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateServerAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid address with port",
			input:    "localhost:8080",
			expected: "localhost:8080",
		},
		{
			name:     "Valid IP with port",
			input:    "192.168.1.1:3000",
			expected: "192.168.1.1:3000",
		},
		{
			name:     "Valid hostname with port",
			input:    "myserver.example.com:443",
			expected: "myserver.example.com:443",
		},
		{
			name:     "Missing port",
			input:    "localhost",
			expected: "",
		},
		{
			name:     "Invalid - no address",
			input:    ":8080",
			expected: "",
		},
		{
			name:     "Invalid - port not numeric",
			input:    "localhost:abc",
			expected: "",
		},
		{
			name:     "Invalid - empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Invalid - multiple colons",
			input:    "host:port:extra",
			expected: "",
		},
		{
			name:     "Valid - high port number",
			input:    "server:65535",
			expected: "server:65535",
		},
		{
			name:     "Valid - port zero",
			input:    "server:0",
			expected: "server:0",
		},
		{
			name:     "Valid - port 1",
			input:    "server:1",
			expected: "server:1",
		},
		{
			name:     "Invalid - port out of range (too high)",
			input:    "server:65536",
			expected: "server:65536", // Note: ValidateServerAddress only validates format, not port range
		},
		{
			name:     "Invalid - negative port",
			input:    "server:-1",
			expected: "", // Regex doesn't match negative numbers
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ValidateServerAddress(tt.input)
			assert.Equal(t, tt.expected, result, "For input %q", tt.input)
		})
	}
}

func TestSafeInt64ToInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    int64
		expected int
	}{
		{
			name:     "Zero",
			input:    0,
			expected: 0,
		},
		{
			name:     "Positive small number",
			input:    100,
			expected: 100,
		},
		{
			name:     "Negative small number",
			input:    -100,
			expected: -100,
		},
		{
			name:     "Max int value",
			input:    int64(math.MaxInt),
			expected: math.MaxInt,
		},
		{
			name:     "Min int value",
			input:    int64(math.MinInt),
			expected: math.MinInt,
		},
		{
			name:     "Large positive within range",
			input:    1000000000,
			expected: 1000000000,
		},
		{
			name:     "Large negative within range",
			input:    -1000000000,
			expected: -1000000000,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := SafeInt64ToInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSafeInt64ToInt_BoundaryBehavior(t *testing.T) {
	t.Parallel()

	// On 64-bit systems, int and int64 have the same range,
	// so the boundary conditions are only relevant on 32-bit systems
	// This test verifies the function works correctly at boundary values

	t.Run("Success - MaxInt boundary", func(t *testing.T) {
		t.Parallel()

		result := SafeInt64ToInt(int64(math.MaxInt))
		assert.Equal(t, math.MaxInt, result)
	})

	t.Run("Success - MinInt boundary", func(t *testing.T) {
		t.Parallel()

		result := SafeInt64ToInt(int64(math.MinInt))
		assert.Equal(t, math.MinInt, result)
	})

	t.Run("Success - Just below MaxInt", func(t *testing.T) {
		t.Parallel()

		result := SafeInt64ToInt(int64(math.MaxInt) - 1)
		assert.Equal(t, math.MaxInt-1, result)
	})

	t.Run("Success - Just above MinInt", func(t *testing.T) {
		t.Parallel()

		result := SafeInt64ToInt(int64(math.MinInt) + 1)
		assert.Equal(t, math.MinInt+1, result)
	})
}

func TestSyscmd_ExecCmd(t *testing.T) {
	t.Parallel()

	syscmd := &Syscmd{}

	t.Run("Success - Execute echo command", func(t *testing.T) {
		t.Parallel()

		output, err := syscmd.ExecCmd("echo", "hello")
		require.NoError(t, err)
		assert.Contains(t, string(output), "hello")
	})

	t.Run("Error - Execute invalid command", func(t *testing.T) {
		t.Parallel()

		_, err := syscmd.ExecCmd("nonexistent_command_xyz")
		require.Error(t, err)
	})
}

func TestDatabaseTypeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "postgresql", PostgreSQLType)
	assert.Equal(t, "mongodb", MongoDBType)
}

// Helper function to create a string pointer
func strPtr(s string) *string {
	return &s
}
