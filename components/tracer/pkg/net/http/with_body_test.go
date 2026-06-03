// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/pkg"
)

func TestFormatErrorFieldName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extracts field name from namespace",
			input:    "CreateRequest.field_name",
			expected: "field_name",
		},
		{
			name:     "extracts after first dot for nested",
			input:    "Request.Parent.child_field",
			expected: "Parent.child_field",
		},
		{
			name:     "returns original if no dot",
			input:    "simple_field",
			expected: "simple_field",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatErrorFieldName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindUnknownFields(t *testing.T) {
	tests := []struct {
		name      string
		original  map[string]any
		marshaled map[string]any
		expected  map[string]any
	}{
		{
			name:      "no differences",
			original:  map[string]any{"field1": "value1", "field2": "value2"},
			marshaled: map[string]any{"field1": "value1", "field2": "value2"},
			expected:  map[string]any{},
		},
		{
			name:      "unknown field in original",
			original:  map[string]any{"field1": "value1", "unknown": "value"},
			marshaled: map[string]any{"field1": "value1"},
			expected:  map[string]any{"unknown": "value"},
		},
		{
			name:      "empty maps",
			original:  map[string]any{},
			marshaled: map[string]any{},
			expected:  map[string]any{},
		},
		{
			name:      "ignores zero float values",
			original:  map[string]any{"field1": "value1", "zero_field": 0.0},
			marshaled: map[string]any{"field1": "value1"},
			expected:  map[string]any{},
		},
		{
			name:      "detects different values",
			original:  map[string]any{"field1": "original"},
			marshaled: map[string]any{"field1": "changed"},
			expected:  map[string]any{"field1": "original"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findUnknownFields(tt.original, tt.marshaled)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindUnknownFields_NestedMaps(t *testing.T) {
	original := map[string]any{
		"parent": map[string]any{
			"known":   "value",
			"unknown": "extra",
		},
	}
	marshaled := map[string]any{
		"parent": map[string]any{
			"known": "value",
		},
	}

	result := findUnknownFields(original, marshaled)

	assert.Contains(t, result, "parent")
	nestedDiff, ok := result["parent"].(map[string]any)
	require.True(t, ok, "expected parent to be map[string]any")
	assert.Contains(t, nestedDiff, "unknown")
}

func TestFindUnknownFields_Arrays(t *testing.T) {
	original := map[string]any{
		"items": []any{"item1", "item2", "item3"},
	}
	marshaled := map[string]any{
		"items": []any{"item1", "item2"},
	}

	result := findUnknownFields(original, marshaled)

	assert.Contains(t, result, "items")
}

func TestCompareSlices(t *testing.T) {
	tests := []struct {
		name      string
		original  []any
		marshaled []any
		hasDiff   bool
	}{
		{
			name:      "identical slices",
			original:  []any{"a", "b", "c"},
			marshaled: []any{"a", "b", "c"},
			hasDiff:   false,
		},
		{
			name:      "original longer",
			original:  []any{"a", "b", "c"},
			marshaled: []any{"a", "b"},
			hasDiff:   true,
		},
		{
			name:      "marshaled longer",
			original:  []any{"a"},
			marshaled: []any{"a", "b", "c"},
			hasDiff:   true,
		},
		{
			name:      "different values",
			original:  []any{"a", "x"},
			marshaled: []any{"a", "b"},
			hasDiff:   true,
		},
		{
			name:      "empty slices",
			original:  []any{},
			marshaled: []any{},
			hasDiff:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareSlices(tt.original, tt.marshaled)
			if tt.hasDiff {
				assert.NotEmpty(t, result)
			} else {
				assert.Empty(t, result)
			}
		})
	}
}

func TestGetValidator(t *testing.T) {
	t.Run("returns validator instance", func(t *testing.T) {
		v, trans, err := getValidator()

		assert.NoError(t, err)
		assert.NotNil(t, v)
		assert.NotNil(t, trans)
	})

	t.Run("returns same instance on multiple calls", func(t *testing.T) {
		v1, _, err1 := getValidator()
		require.NoError(t, err1)

		v2, _, err2 := getValidator()
		require.NoError(t, err2)

		assert.Same(t, v1, v2)
	})
}

func TestValidateStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}

	tests := []struct {
		name        string
		input       any
		expectError bool
	}{
		{
			name: "valid struct",
			input: &TestStruct{
				Name:  "John",
				Email: "john@example.com",
			},
			expectError: false,
		},
		{
			name: "missing required field",
			input: &TestStruct{
				Name:  "",
				Email: "john@example.com",
			},
			expectError: true,
		},
		{
			name: "invalid email",
			input: &TestStruct{
				Name:  "John",
				Email: "invalid-email",
			},
			expectError: true,
		},
		{
			name:        "non-struct input",
			input:       "string value",
			expectError: false,
		},
		{
			name:        "nil pointer",
			input:       (*TestStruct)(nil),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStruct(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWrapJSONError(t *testing.T) {
	t.Run("wraps generic error", func(t *testing.T) {
		err := wrapJSONError(assert.AnError)
		assert.Contains(t, err.Error(), "invalid JSON")
	})
}

func TestNewOfType(t *testing.T) {
	type TestStruct struct {
		Field string
	}

	t.Run("creates new instance from pointer", func(t *testing.T) {
		source := &TestStruct{Field: "original"}
		result, err := newOfType(source)

		require.NoError(t, err)
		require.NotNil(t, result)

		resultStruct, ok := result.(*TestStruct)
		require.True(t, ok, "expected result to be *TestStruct")
		assert.Equal(t, "", resultStruct.Field) // New instance should have zero values
	})

	t.Run("returns error for non-pointer", func(t *testing.T) {
		source := TestStruct{Field: "value"}
		_, err := newOfType(source)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected pointer")
	})
}

func TestValidateMetadataNestedValues(t *testing.T) {
	// Access the validator to test the custom validation function
	v, _, err := getValidator()
	require.NoError(t, err)

	type TestStructWithMetadata struct {
		Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	}

	tests := []struct {
		name        string
		input       TestStructWithMetadata
		expectError bool
	}{
		{
			name: "valid flat metadata",
			input: TestStructWithMetadata{
				Metadata: map[string]any{
					"key1": "value1",
					"key2": 123,
				},
			},
			expectError: false,
		},
		{
			name: "empty metadata",
			input: TestStructWithMetadata{
				Metadata: map[string]any{},
			},
			expectError: false,
		},
		{
			name: "nil metadata",
			input: TestStructWithMetadata{
				Metadata: nil,
			},
			expectError: false,
		},
		{
			name: "nested map should fail",
			input: TestStructWithMetadata{
				Metadata: map[string]any{
					"key1":   "value1",
					"nested": map[string]any{"inner": "value"},
				},
			},
			expectError: true,
		},
		{
			name: "nested slice should fail",
			input: TestStructWithMetadata{
				Metadata: map[string]any{
					"key1":  "value1",
					"array": []string{"item1", "item2"},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMetadataKeyMaxLength(t *testing.T) {
	v, _, err := getValidator()
	require.NoError(t, err)

	type TestStruct struct {
		Key string `json:"key" validate:"keymax=10"`
	}

	tests := []struct {
		name        string
		input       TestStruct
		expectError bool
	}{
		{
			name:        "key within limit",
			input:       TestStruct{Key: "short"},
			expectError: false,
		},
		{
			name:        "key at exact limit",
			input:       TestStruct{Key: "1234567890"},
			expectError: false,
		},
		{
			name:        "key exceeds limit",
			input:       TestStruct{Key: "12345678901"},
			expectError: true,
		},
		{
			name:        "empty key",
			input:       TestStruct{Key: ""},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMetadataValueMaxLength(t *testing.T) {
	v, _, err := getValidator()
	require.NoError(t, err)

	type TestStructString struct {
		Value string `json:"value" validate:"valuemax=10"`
	}

	type TestStructInt struct {
		Value int `json:"value" validate:"valuemax=10"`
	}

	type TestStructFloat struct {
		Value float64 `json:"value" validate:"valuemax=10"`
	}

	type TestStructBool struct {
		Value bool `json:"value" validate:"valuemax=10"`
	}

	t.Run("string value within limit", func(t *testing.T) {
		err := v.Struct(TestStructString{Value: "short"})
		assert.NoError(t, err)
	})

	t.Run("string value exactly at limit (10 chars)", func(t *testing.T) {
		err := v.Struct(TestStructString{Value: "1234567890"})
		assert.NoError(t, err, "10 characters should be valid")
	})

	t.Run("string value exceeds limit (11 chars)", func(t *testing.T) {
		err := v.Struct(TestStructString{Value: "12345678901"})
		assert.Error(t, err, "11 characters should fail")
	})

	t.Run("int value within limit", func(t *testing.T) {
		err := v.Struct(TestStructInt{Value: 123})
		assert.NoError(t, err)
	})

	t.Run("int value at boundary (10 digits)", func(t *testing.T) {
		err := v.Struct(TestStructInt{Value: 1234567890})
		assert.NoError(t, err, "10-digit number should be valid")
	})

	t.Run("int value exceeds boundary (11 digits)", func(t *testing.T) {
		err := v.Struct(TestStructInt{Value: 12345678901})
		assert.Error(t, err, "11-digit number should fail")
	})

	t.Run("float value within limit", func(t *testing.T) {
		err := v.Struct(TestStructFloat{Value: 1.23})
		assert.NoError(t, err)
	})

	t.Run("float value at boundary (10 chars)", func(t *testing.T) {
		err := v.Struct(TestStructFloat{Value: 12345.6789}) // "12345.6789" = 10 chars
		assert.NoError(t, err, "float with 10 char representation should be valid")
	})

	t.Run("float value exceeds boundary", func(t *testing.T) {
		err := v.Struct(TestStructFloat{Value: 1234567890.1}) // Will be formatted as "1.23456789e+09" or similar (>10 chars)
		assert.Error(t, err, "float with 11+ char representation should fail")
	})

	t.Run("bool value within limit", func(t *testing.T) {
		err := v.Struct(TestStructBool{Value: true})
		assert.NoError(t, err)
	})
}

func TestParseMetadata(t *testing.T) {
	type TestStructWithMetadata struct {
		Name     string         `json:"name"`
		Metadata map[string]any `json:"metadata"`
	}

	tests := []struct {
		name             string
		input            *TestStructWithMetadata
		originalMap      map[string]any
		expectedMetadata map[string]any
	}{
		{
			name:  "metadata not in original - creates empty map",
			input: &TestStructWithMetadata{Name: "test"},
			originalMap: map[string]any{
				"name": "test",
			},
			expectedMetadata: map[string]any{},
		},
		{
			name: "metadata in original - keeps as-is",
			input: &TestStructWithMetadata{
				Name:     "test",
				Metadata: map[string]any{"key": "value"},
			},
			originalMap: map[string]any{
				"name":     "test",
				"metadata": map[string]any{"key": "value"},
			},
			expectedMetadata: map[string]any{"key": "value"},
		},
		{
			name: "metadata already present in struct - overwrites with empty map",
			input: &TestStructWithMetadata{
				Name:     "test",
				Metadata: map[string]any{"existing": "data"},
			},
			originalMap: map[string]any{
				"name": "test",
			},
			expectedMetadata: map[string]any{},
		},
		{
			name:  "nil metadata in original - keeps current value",
			input: &TestStructWithMetadata{Name: "test"},
			originalMap: map[string]any{
				"name":     "test",
				"metadata": nil,
			},
			expectedMetadata: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parseMetadata(tt.input, tt.originalMap)

			if tt.expectedMetadata == nil {
				assert.Nil(t, tt.input.Metadata, "Metadata should be nil")
			} else {
				require.NotNil(t, tt.input.Metadata, "Metadata should not be nil")
				assert.Equal(t, tt.expectedMetadata, tt.input.Metadata, "Metadata contents should match expected")
			}
		})
	}
}

func TestParseMetadata_NonStruct(t *testing.T) {
	// Test that parseMetadata handles non-struct input gracefully
	originalMap := map[string]any{"key": "value"}

	// Should not panic
	assert.NotPanics(t, func() {
		parseMetadata("string input", originalMap)
	}, "parseMetadata should not panic for string input")

	assert.NotPanics(t, func() {
		parseMetadata(nil, originalMap)
	}, "parseMetadata should not panic for nil input")

	assert.NotPanics(t, func() {
		parseMetadata(123, originalMap)
	}, "parseMetadata should not panic for integer input")
}

func TestFieldsRequired(t *testing.T) {
	tests := []struct {
		name     string
		input    pkg.FieldValidations
		expected pkg.FieldValidations
	}{
		{
			name: "filters required fields only",
			input: pkg.FieldValidations{
				"name":  "name is a required field",
				"email": "email must be a valid email address",
				"age":   "age is a required field",
			},
			expected: pkg.FieldValidations{
				"name": "name is a required field",
				"age":  "age is a required field",
			},
		},
		{
			name:     "empty input",
			input:    pkg.FieldValidations{},
			expected: pkg.FieldValidations{},
		},
		{
			name: "no required fields",
			input: pkg.FieldValidations{
				"email": "email must be a valid email address",
			},
			expected: pkg.FieldValidations{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fieldsRequired(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareSlices_NestedMaps(t *testing.T) {
	tests := []struct {
		name            string
		original        []any
		marshaled       []any
		expectedDiffLen int
		validateDiff    func(t *testing.T, diff []any)
	}{
		{
			name: "extra field in first item",
			original: []any{
				map[string]any{"id": "1", "extra": "field"},
				map[string]any{"id": "2"},
			},
			marshaled: []any{
				map[string]any{"id": "1"},
				map[string]any{"id": "2"},
			},
			expectedDiffLen: 1,
			validateDiff: func(t *testing.T, diff []any) {
				diffMap, ok := diff[0].(map[string]any)
				require.True(t, ok, "difference should be a map")
				assert.Equal(t, map[string]any{"extra": "field"}, diffMap)
			},
		},
		{
			name: "multiple extra fields",
			original: []any{
				map[string]any{"id": "1", "extra1": "field1", "extra2": "field2"},
			},
			marshaled: []any{
				map[string]any{"id": "1"},
			},
			expectedDiffLen: 1,
			validateDiff: func(t *testing.T, diff []any) {
				diffMap, ok := diff[0].(map[string]any)
				require.True(t, ok, "difference should be a map")
				assert.Contains(t, diffMap, "extra1")
				assert.Contains(t, diffMap, "extra2")
				assert.Equal(t, "field1", diffMap["extra1"])
				assert.Equal(t, "field2", diffMap["extra2"])
			},
		},
		{
			name: "no differences - all items match",
			original: []any{
				map[string]any{"id": "1"},
				map[string]any{"id": "2"},
			},
			marshaled: []any{
				map[string]any{"id": "1"},
				map[string]any{"id": "2"},
			},
			expectedDiffLen: 0,
			validateDiff: func(t *testing.T, diff []any) {
				// No additional validation needed for empty diff
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareSlices(tt.original, tt.marshaled)

			require.Len(t, result, tt.expectedDiffLen)
			if tt.expectedDiffLen > 0 {
				tt.validateDiff(t, result)
			}
		})
	}
}

func TestFindUnknownFields_TypeMismatch(t *testing.T) {
	t.Run("map vs non-map type mismatch", func(t *testing.T) {
		original := map[string]any{
			"field": map[string]any{"nested": "value"},
		}
		marshaled := map[string]any{
			"field": "simple string",
		}

		result := findUnknownFields(original, marshaled)
		assert.Contains(t, result, "field")
	})

	t.Run("slice vs non-slice type mismatch", func(t *testing.T) {
		original := map[string]any{
			"items": []any{"item1", "item2"},
		}
		marshaled := map[string]any{
			"items": "not an array",
		}

		result := findUnknownFields(original, marshaled)
		assert.Contains(t, result, "items")
	})
}
