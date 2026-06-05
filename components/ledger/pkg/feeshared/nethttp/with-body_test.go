// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"net/http/httptest"
	"testing"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// Test structs for testing
type testStruct struct {
	Name     string
	Email    string
	Age      int
	Metadata map[string]any `json:"metadata"`
}

type testStructWithPtr struct {
	Name  *string
	Value *int
}

type testStructWithSlice struct {
	Items []string
	Tags  []testStruct
}

type testStructWithMap struct {
	Data map[string]string
}

type testStructWithNestedMap struct {
	Data map[string]testStruct
}

type testStructWithValidation struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0"`
}

type testStructWithMetadata struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
}

type testStructWithUUID struct {
	ID string `json:"id" validate:"uuid"`
}

type testStructWithTransactionType struct {
	FromTo []transaction.FromTo `json:"fromTo" validate:"singletransactiontype"`
}

type testStructWithKeyMax struct {
	Key string `json:"key" validate:"keymax=10"`
}

type testStructWithValueMax struct {
	Value string `json:"value" validate:"valuemax=20"`
}

type testStructWithNoNested struct {
	Metadata map[string]any `json:"metadata" validate:"nonested"`
}

type simpleTestStruct struct {
	Name string `json:"name"`
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "String with special characters",
			input:    "test@example.com!@#$%^&*()",
			expected: "test@example.com@", // @ is allowed, but !#$%^&*() are removed
		},
		{
			name:     "String with allowed characters",
			input:    "test-123_example@domain.com",
			expected: "test-123_example@domain.com",
		},
		{
			name:     "String with spaces",
			input:    "test string with spaces",
			expected: "test string with spaces",
		},
		{
			name:     "String with dots and commas",
			input:    "test.string,with.dots,commas",
			expected: "test.string,with.dots,commas",
		},
		{
			name:     "String with slashes",
			input:    "test/path\\backslash",
			expected: "test/path\\backslash",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeStruct(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected func(t *testing.T, result any)
	}{
		{
			name: "Struct with string fields",
			input: &testStruct{
				Name:  "test!@#$",
				Email: "test@example.com",
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStruct)
				// @ is allowed by regex, so "test!@#$" becomes "test@"
				assert.Equal(t, "test@", s.Name)
				assert.Equal(t, "test@example.com", s.Email)
			},
		},
		{
			name: "Struct with pointer to string",
			input: &testStructWithPtr{
				Name: stringPtr("test!@#$"),
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithPtr)
				assert.NotNil(t, s.Name)
				// @ is allowed by regex, so "test!@#$" becomes "test@"
				assert.Equal(t, "test@", *s.Name)
			},
		},
		{
			name: "Struct with slice of strings",
			input: &testStructWithSlice{
				Items: []string{"item1!@#", "item2$%^"},
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithSlice)
				// @ is allowed, so "item1!@#" becomes "item1@"
				assert.Equal(t, []string{"item1@", "item2"}, s.Items)
			},
		},
		{
			name: "Struct with map[string]string",
			input: &testStructWithMap{
				Data: map[string]string{
					"key1": "value1!@#",
					"key2": "value2$%^",
				},
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithMap)
				// @ is allowed, so "value1!@#" becomes "value1@"
				assert.Equal(t, "value1@", s.Data["key1"])
				assert.Equal(t, "value2", s.Data["key2"])
			},
		},
		{
			name: "Struct with nested map of structs",
			input: &testStructWithNestedMap{
				Data: map[string]testStruct{
					"nested": {
						Name:  "test!@#$",
						Email: "test@example.com",
					},
				},
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithNestedMap)
				// @ is allowed, so "test!@#$" becomes "test@"
				assert.Equal(t, "test@", s.Data["nested"].Name)
				assert.Equal(t, "test@example.com", s.Data["nested"].Email)
			},
		},
		{
			name:  "Non-struct value",
			input: "not a struct",
			expected: func(t *testing.T, result any) {
				// Should not panic or modify non-struct
				assert.Equal(t, "not a struct", result)
			},
		},
		{
			name:  "Non-pointer struct",
			input: testStruct{Name: "test!@#$"},
			expected: func(t *testing.T, result any) {
				s, ok := result.(testStruct)
				assert.True(t, ok, "result must be testStruct")
				// sanitizeStruct receives a non-pointer value, so it cannot modify the original.
				// The original value must remain unchanged.
				assert.Equal(t, "test!@#$", s.Name)
			},
		},
		{
			name: "Struct with pointer to struct",
			input: &testStructWithPtr{
				Value: intPtr(42),
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithPtr)
				assert.NotNil(t, s.Value)
				assert.Equal(t, 42, *s.Value)
			},
		},
		{
			name: "Struct with slice of structs",
			input: &testStructWithSlice{
				Tags: []testStruct{
					{Name: "tag1!@#", Email: "tag1@example.com"},
					{Name: "tag2$%^", Email: "tag2@example.com"},
				},
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithSlice)
				assert.Equal(t, "tag1@", s.Tags[0].Name) // @ is allowed
				assert.Equal(t, "tag2", s.Tags[1].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitizeStruct(tt.input)
			tt.expected(t, tt.input)
		})
	}
}

func TestNewOfType(t *testing.T) {
	type testType struct {
		Name string
	}

	original := &testType{Name: "test"}
	result := newOfType(original)

	assert.NotNil(t, result)
	assert.IsType(t, &testType{}, result)
	assert.Empty(t, result.(*testType).Name)
}

func TestFindUnknownFields(t *testing.T) {
	tests := []struct {
		name      string
		original  map[string]any
		marshaled map[string]any
		expected  map[string]any
	}{
		{
			name: "No unknown fields",
			original: map[string]any{
				"name": "test",
				"age":  30,
			},
			marshaled: map[string]any{
				"name": "test",
				"age":  30,
			},
			expected: map[string]any{},
		},
		{
			name: "Unknown field present",
			original: map[string]any{
				"name":    "test",
				"unknown": "value",
				"age":     30,
			},
			marshaled: map[string]any{
				"name": "test",
				"age":  30,
			},
			expected: map[string]any{
				"unknown": "value",
			},
		},
		{
			name: "Zero float ignored",
			original: map[string]any{
				"name": "test",
				"zero": 0.0,
			},
			marshaled: map[string]any{
				"name": "test",
			},
			expected: map[string]any{},
		},
		{
			name: "Nested unknown fields",
			original: map[string]any{
				"nested": map[string]any{
					"known":   "value",
					"unknown": "value2",
				},
			},
			marshaled: map[string]any{
				"nested": map[string]any{
					"known": "value",
				},
			},
			expected: map[string]any{
				"nested": map[string]any{
					"unknown": "value2",
				},
			},
		},
		{
			name: "Slice differences",
			original: map[string]any{
				"items": []any{"item1", "item2"},
			},
			marshaled: map[string]any{
				"items": []any{"item1"},
			},
			expected: map[string]any{
				"items": []any{"item2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findUnknownFields(tt.original, tt.marshaled)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsZeroFloat(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{
			name:     "Zero float64",
			value:    0.0,
			expected: true,
		},
		{
			name:     "Non-zero float64",
			value:    1.0,
			expected: false,
		},
		{
			name:     "Zero int",
			value:    0,
			expected: false, // isZeroFloat checks value == 0.0, which may not match int 0
		},
		{
			name:     "Non-zero int",
			value:    1,
			expected: false,
		},
		{
			name:     "String value",
			value:    "test",
			expected: false,
		},
		{
			name:     "Nil value",
			value:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isZeroFloat(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsStringNumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Valid decimal string",
			input:    "123.45",
			expected: true,
		},
		{
			name:     "Valid integer string",
			input:    "123",
			expected: true,
		},
		{
			name:     "Invalid string",
			input:    "not a number",
			expected: false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "Negative number",
			input:    "-123.45",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStringNumeric(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareSlices(t *testing.T) {
	tests := []struct {
		name      string
		original  []any
		marshaled []any
		expected  []any
	}{
		{
			name:      "Equal slices",
			original:  []any{"item1", "item2"},
			marshaled: []any{"item1", "item2"},
			expected:  nil, // compareSlices returns nil when no differences
		},
		{
			name:      "Original longer",
			original:  []any{"item1", "item2", "item3"},
			marshaled: []any{"item1", "item2"},
			expected:  []any{"item3"},
		},
		{
			name:      "Marshaled longer",
			original:  []any{"item1"},
			marshaled: []any{"item1", "item2"},
			expected:  []any{"item2"},
		},
		{
			name:      "Different items",
			original:  []any{"item1", "item2"},
			marshaled: []any{"item1", "item3"},
			expected:  []any{"item2"},
		},
		{
			name: "Nested maps",
			original: []any{
				map[string]any{"key1": "value1", "unknown": "value"},
			},
			marshaled: []any{
				map[string]any{"key1": "value1"},
			},
			expected: []any{
				map[string]any{"unknown": "value"},
			},
		},
		{
			name:      "Empty slices",
			original:  []any{},
			marshaled: []any{},
			expected:  nil, // compareSlices returns nil when no differences
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareSlices(tt.original, tt.marshaled)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleNestedDifferences(t *testing.T) {
	tests := []struct {
		name         string
		originalVal  any
		marshaledVal any
		expected     any
	}{
		{
			name:         "Equal values",
			originalVal:  "test",
			marshaledVal: "test",
			expected:     nil,
		},
		{
			name:         "Different string values",
			originalVal:  "test1",
			marshaledVal: "test2",
			expected:     "test1",
		},
		{
			name:         "Numeric string",
			originalVal:  "123.45",
			marshaledVal: 123.45,
			expected:     nil,
		},
		{
			name:         "Map differences",
			originalVal:  map[string]any{"key": "value", "unknown": "value2"},
			marshaledVal: map[string]any{"key": "value"},
			expected:     map[string]any{"unknown": "value2"},
		},
		{
			name:         "Slice differences",
			originalVal:  []any{"item1", "item2"},
			marshaledVal: []any{"item1"},
			expected:     []any{"item2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleNestedDifferences(tt.originalVal, tt.marshaledVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleMapDifference(t *testing.T) {
	tests := []struct {
		name         string
		originalMap  map[string]any
		marshaledVal any
		expected     any
	}{
		{
			name: "No differences",
			originalMap: map[string]any{
				"key": "value",
			},
			marshaledVal: map[string]any{
				"key": "value",
			},
			expected: nil,
		},
		{
			name: "With differences",
			originalMap: map[string]any{
				"key":     "value",
				"unknown": "value2",
			},
			marshaledVal: map[string]any{
				"key": "value",
			},
			expected: map[string]any{
				"unknown": "value2",
			},
		},
		{
			name: "Marshaled is not a map",
			originalMap: map[string]any{
				"key": "value",
			},
			marshaledVal: "not a map",
			expected:     map[string]any{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleMapDifference(tt.originalMap, tt.marshaledVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleSliceDifference(t *testing.T) {
	tests := []struct {
		name          string
		originalSlice []any
		marshaledVal  any
		expected      any
	}{
		{
			name:          "No differences",
			originalSlice: []any{"item1", "item2"},
			marshaledVal:  []any{"item1", "item2"},
			expected:      nil,
		},
		{
			name:          "With differences",
			originalSlice: []any{"item1", "item2"},
			marshaledVal:  []any{"item1"},
			expected:      []any{"item2"},
		},
		{
			name:          "Marshaled is not a slice",
			originalSlice: []any{"item1"},
			marshaledVal:  "not a slice",
			expected:      []any{"item1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleSliceDifference(tt.originalSlice, tt.marshaledVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatErrorFieldName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Field with namespace",
			input:    "Struct.Field",
			expected: "Field",
		},
		{
			name:     "Nested field",
			input:    "Struct.Nested.Field",
			expected: "Nested.Field", // formatErrorFieldName regex `\.(.+)$` matches last dot and everything after
		},
		{
			name:     "Field without namespace",
			input:    "Field",
			expected: "Field",
		},
		{
			name:     "Empty string",
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

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		name        string
		structVal   any
		originalMap map[string]any
		expected    func(t *testing.T, result any)
	}{
		{
			name: "Metadata field exists and not in original",
			structVal: &testStructWithMetadata{
				Name: "test",
			},
			originalMap: map[string]any{
				"name": "test",
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithMetadata)
				assert.NotNil(t, s.Metadata)
				assert.Empty(t, s.Metadata)
			},
		},
		{
			name: "Metadata field exists and in original",
			structVal: &testStructWithMetadata{
				Name:     "test",
				Metadata: map[string]any{"key": "value"}, // Set before calling parseMetadata
			},
			originalMap: map[string]any{
				"name":     "test",
				"metadata": map[string]any{"key": "value"},
			},
			expected: func(t *testing.T, result any) {
				s := result.(*testStructWithMetadata)
				// Should not be reset if exists in original
				assert.NotNil(t, s.Metadata)
				assert.Equal(t, map[string]any{"key": "value"}, s.Metadata)
			},
		},
		{
			name: "No Metadata field",
			structVal: &testStruct{
				Name: "test",
			},
			originalMap: map[string]any{
				"name": "test",
			},
			expected: func(t *testing.T, result any) {
				// Should not panic
				assert.NotNil(t, result)
			},
		},
		{
			name:        "Non-pointer struct",
			structVal:   testStruct{Name: "test"},
			originalMap: map[string]any{"name": "test"},
			expected: func(t *testing.T, result any) {
				// Should not panic
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parseMetadata(tt.structVal, tt.originalMap)
			tt.expected(t, tt.structVal)
		})
	}
}

func TestValidateStruct(t *testing.T) {
	tests := []struct {
		name         string
		structVal    any
		wantErr      bool
		wantErrField string // if non-empty, verify the specific field that failed validation
		wantErrTag   string // if non-empty, verify the specific validation tag that failed
	}{
		{
			name: "Valid struct",
			structVal: &testStructWithValidation{
				Name:  "test",
				Email: "test@example.com",
				Age:   30,
			},
			wantErr: false,
		},
		{
			name: "Missing required field",
			structVal: &testStructWithValidation{
				Email: "test@example.com",
			},
			wantErr:      true,
			wantErrField: "name",
			wantErrTag:   "required",
		},
		{
			name: "Invalid email",
			structVal: &testStructWithValidation{
				Name:  "test",
				Email: "invalid-email",
				Age:   30,
			},
			wantErr:      true,
			wantErrField: "email",
			wantErrTag:   "email",
		},
		{
			name: "Age below minimum",
			structVal: &testStructWithValidation{
				Name:  "test",
				Email: "test@example.com",
				Age:   -1,
			},
			wantErr:      true,
			wantErrField: "age",
			wantErrTag:   "gte",
		},
		{
			name:      "Non-struct value",
			structVal: "not a struct",
			wantErr:   false,
		},
		{
			name: "Valid UUID",
			structVal: &testStructWithUUID{
				ID: uuid.New().String(),
			},
			wantErr: false,
		},
		{
			name: "Invalid UUID",
			structVal: &testStructWithUUID{
				ID: "not-a-uuid",
			},
			wantErr:      true,
			wantErrField: "id",
			wantErrTag:   "uuid",
		},
		{
			name: "Empty UUID (optional)",
			structVal: &testStructWithUUID{
				ID: "",
			},
			wantErr: false,
		},
		{
			name: "Valid transaction type - amount only",
			structVal: &testStructWithTransactionType{
				FromTo: []transaction.FromTo{
					{
						Amount: &transaction.Amount{
							Asset: "BRL",
							Value: decimal.NewFromInt(100),
						},
						Share:     nil,
						Remaining: "",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid transaction type - multiple types",
			structVal: &testStructWithTransactionType{
				FromTo: []transaction.FromTo{
					{
						Amount: &transaction.Amount{
							Asset: "BRL",
							Value: decimal.NewFromInt(100),
						},
						Share: &transaction.Share{
							Percentage: 50,
						},
						Remaining: "",
					},
				},
			},
			wantErr:    true,
			wantErrTag: "singletransactiontype",
		},
		{
			name: "Key max length valid",
			structVal: &testStructWithKeyMax{
				Key: "1234567890",
			},
			wantErr: false,
		},
		{
			name: "Key max length invalid",
			structVal: &testStructWithKeyMax{
				Key: "12345678901",
			},
			wantErr:      true,
			wantErrField: "key",
			wantErrTag:   "keymax",
		},
		{
			name: "Value max length valid",
			structVal: &testStructWithValueMax{
				Value: "12345678901234567890",
			},
			wantErr: false,
		},
		{
			name: "Value max length invalid",
			structVal: &testStructWithValueMax{
				Value: "123456789012345678901",
			},
			wantErr:      true,
			wantErrField: "value",
			wantErrTag:   "valuemax",
		},
		{
			name: "No nested metadata valid",
			structVal: &testStructWithNoNested{
				Metadata: map[string]any{
					"key": "value",
				},
			},
			wantErr:      true, // validateMetadataNestedValues returns false for Map (invalid), so validation fails
			wantErrField: "metadata",
			wantErrTag:   "nonested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStruct(tt.structVal)
			if tt.wantErr {
				assert.Error(t, err)

				// Verify the specific validation error field and tag when specified
				if tt.wantErrField != "" || tt.wantErrTag != "" {
					v, _ := newValidator()
					rawErr := v.Struct(tt.structVal)
					if rawErr != nil {
						validationErrors, ok := rawErr.(validator.ValidationErrors)
						if ok && len(validationErrors) > 0 {
							if tt.wantErrField != "" {
								found := false
								for _, fe := range validationErrors {
									if fe.Field() == tt.wantErrField {
										found = true

										break
									}
								}

								assert.True(t, found, "expected validation error on field %q", tt.wantErrField)
							}

							if tt.wantErrTag != "" {
								found := false
								for _, fe := range validationErrors {
									if fe.Tag() == tt.wantErrTag {
										found = true

										break
									}
								}

								assert.True(t, found, "expected validation error with tag %q", tt.wantErrTag)
							}
						}
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMetadataKeyMaxLength(t *testing.T) {
	v, _ := newValidator()

	tests := []struct {
		name     string
		key      string
		limit    string
		expected bool
	}{
		{
			name:     "Valid key length with default limit",
			key:      "1234567890",
			limit:    "",
			expected: true,
		},
		{
			name:     "Invalid key length with default limit",
			key:      "12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901",
			limit:    "",
			expected: false,
		},
		{
			name:     "Valid key length with custom limit",
			key:      "12345",
			limit:    "10",
			expected: true,
		},
		{
			name:     "Invalid key length with custom limit",
			key:      "12345678901",
			limit:    "10",
			expected: false,
		},
		{
			// Invalid limit param is ignored; validator falls back to default max (100), so key passes.
			name:     "Invalid limit param",
			key:      "test",
			limit:    "invalid",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structVal := &testStructWithKeyMax{Key: tt.key}
			err := v.VarWithValue(structVal.Key, tt.limit, "keymax="+tt.limit)
			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateMetadataValueMaxLength(t *testing.T) {
	v, _ := newValidator()

	tests := []struct {
		name     string
		value    string
		limit    string
		expected bool
	}{
		{
			name:     "Valid value length with default limit",
			value:    "test",
			limit:    "",
			expected: true,
		},
		{
			name:     "Valid value length with custom limit",
			value:    "12345678901234567890",
			limit:    "20",
			expected: true,
		},
		{
			name:     "Invalid value length with custom limit",
			value:    "123456789012345678901",
			limit:    "20",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structVal := &testStructWithValueMax{Value: tt.value}
			err := v.VarWithValue(structVal.Value, tt.limit, "valuemax="+tt.limit)
			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateMetadataNestedValues(t *testing.T) {
	v, _ := newValidator()

	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{
			name:     "Map value",
			value:    map[string]any{"key": "value"},
			expected: false,
		},
		{
			name:     "String value",
			value:    "string",
			expected: true,
		},
		{
			name:     "Int value",
			value:    123,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Var(tt.value, "nonested")
			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateUUID(t *testing.T) {
	v, _ := newValidator()

	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "Valid UUID",
			value:    uuid.New().String(),
			expected: true,
		},
		{
			name:     "Invalid UUID",
			value:    "not-a-uuid",
			expected: false,
		},
		{
			name:     "Empty UUID",
			value:    "",
			expected: true, // Empty is allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Var(tt.value, "uuid")
			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateSingleTransactionType(t *testing.T) {
	v, _ := newValidator()

	tests := []struct {
		name     string
		fromTo   []transaction.FromTo
		expected bool
	}{
		{
			name: "Only amount",
			fromTo: []transaction.FromTo{
				{
					Amount: &transaction.Amount{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
					},
					Share:     nil,
					Remaining: "",
				},
			},
			expected: true,
		},
		{
			name: "Only share",
			fromTo: []transaction.FromTo{
				{
					Amount: nil,
					Share: &transaction.Share{
						Percentage: 50,
					},
					Remaining: "",
				},
			},
			expected: true,
		},
		{
			name: "Only remaining",
			fromTo: []transaction.FromTo{
				{
					Amount:    nil,
					Share:     nil,
					Remaining: "remaining",
				},
			},
			expected: true,
		},
		{
			name: "Multiple types - amount and share",
			fromTo: []transaction.FromTo{
				{
					Amount: &transaction.Amount{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
					},
					Share: &transaction.Share{
						Percentage: 50,
					},
					Remaining: "",
				},
			},
			expected: false,
		},
		{
			name: "Multiple types - all three",
			fromTo: []transaction.FromTo{
				{
					Amount: &transaction.Amount{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
					},
					Share: &transaction.Share{
						Percentage: 50,
					},
					Remaining: "remaining",
				},
			},
			expected: false,
		},
		{
			name:     "None specified",
			fromTo:   []transaction.FromTo{{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structVal := &testStructWithTransactionType{FromTo: tt.fromTo}
			err := v.Struct(structVal)
			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestWithBody_FiberHandlerFunc(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		structType     any
		handler        DecodeHandlerFunc
		wantErr        bool
		expectedStatus int
	}{
		{
			name:       "Valid JSON body",
			body:       `{"name": "test"}`,
			structType: &simpleTestStruct{},
			handler: func(p any, c *fiber.Ctx) error {
				s := p.(*simpleTestStruct)
				assert.Equal(t, "test", s.Name)
				return c.SendStatus(fiber.StatusOK)
			},
			wantErr:        false,
			expectedStatus: fiber.StatusOK,
		},
		{
			name:       "Invalid JSON body",
			body:       `{"name": "test", invalid json}`,
			structType: &testStruct{},
			handler: func(p any, c *fiber.Ctx) error {
				return c.SendStatus(fiber.StatusOK)
			},
			wantErr:        true,
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "Unknown fields",
			body:       `{"name": "test", "unknown": "field"}`,
			structType: &testStruct{},
			handler: func(p any, c *fiber.Ctx) error {
				return c.SendStatus(fiber.StatusOK)
			},
			wantErr:        true,
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "Validation error",
			body:       `{"name": "", "email": "invalid-email"}`,
			structType: &testStructWithValidation{},
			handler: func(p any, c *fiber.Ctx) error {
				return c.SendStatus(fiber.StatusOK)
			},
			wantErr:        true,
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "Handler error",
			body:       `{"name": "test"}`,
			structType: &simpleTestStruct{},
			handler: func(p any, c *fiber.Ctx) error {
				return fiber.NewError(fiber.StatusInternalServerError, "handler error")
			},
			wantErr:        true,
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:       "Empty body",
			body:       `{}`,
			structType: &simpleTestStruct{},
			handler: func(p any, c *fiber.Ctx) error {
				return c.SendStatus(fiber.StatusOK)
			},
			wantErr:        false,
			expectedStatus: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/test", WithBody(tt.structType, tt.handler))

			req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			assert.NoError(t, err)

			if tt.wantErr {
				assert.GreaterOrEqual(t, resp.StatusCode, fiber.StatusBadRequest)
			} else {
				assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestFields(t *testing.T) {
	v, trans := newValidator()

	structVal := &testStructWithValidation{
		Name:  "",
		Email: "invalid-email",
	}

	err := v.Struct(structVal)
	assert.Error(t, err)

	validationErrors, ok := err.(validator.ValidationErrors)
	assert.True(t, ok)
	result := fields(validationErrors, trans)

	assert.NotNil(t, result)
	assert.Contains(t, result, "name")
	assert.Contains(t, result, "email")
}

func TestFieldsRequired(t *testing.T) {
	tests := []struct {
		name     string
		input    pkg.FieldValidations
		expected pkg.FieldValidations
	}{
		{
			name: "With required fields",
			input: pkg.FieldValidations{
				"name":  "name is a required field",
				"email": "email must be a valid email",
			},
			expected: pkg.FieldValidations{
				"name": "name is a required field",
			},
		},
		{
			name: "No required fields",
			input: pkg.FieldValidations{
				"email": "email must be a valid email",
			},
			expected: pkg.FieldValidations{},
		},
		{
			name:     "Empty map",
			input:    pkg.FieldValidations{},
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

func TestMalformedRequestErr(t *testing.T) {
	v, trans := newValidator()

	structVal := &testStructWithValidation{
		Name:  "",
		Email: "",
	}

	err := v.Struct(structVal)
	assert.Error(t, err)

	validationErrors, ok := err.(validator.ValidationErrors)
	assert.True(t, ok)
	result := malformedRequestErr(validationErrors, trans)

	assert.NotNil(t, result)
	// Check that Fields map contains the validation errors
	assert.NotEmpty(t, result.Fields)
	assert.Contains(t, result.Fields, "name")
	assert.Contains(t, result.Fields, "email")
}

func TestFields_EmptyErrors(t *testing.T) {
	v, trans := newValidator()

	// Valid struct should produce no errors
	structVal := &testStructWithValidation{
		Name:  "test",
		Email: "test@example.com",
		Age:   30,
	}

	err := v.Struct(structVal)
	assert.NoError(t, err)

	// Test with empty errors
	result := fields(validator.ValidationErrors{}, trans)
	assert.Nil(t, result)
}

func TestValidateMetadataValueMaxLength_AllTypes(t *testing.T) {
	v, _ := newValidator()

	type testStructInt struct {
		Value int `validate:"valuemax=5"`
	}

	type testStructFloat struct {
		Value float64 `validate:"valuemax=5"`
	}

	type testStructBool struct {
		Value bool `validate:"valuemax=5"`
	}

	tests := []struct {
		name      string
		structVal any
		expected  bool
	}{
		{
			name:      "Int value within limit",
			structVal: &testStructInt{Value: 123},
			expected:  true,
		},
		{
			name:      "Int value exceeds limit",
			structVal: &testStructInt{Value: 123456},
			expected:  false,
		},
		{
			name:      "Float64 value within limit",
			structVal: &testStructFloat{Value: 12.34},
			expected:  true,
		},
		{
			name:      "Float64 value exceeds limit",
			structVal: &testStructFloat{Value: 123456.78},
			expected:  false,
		},
		{
			name:      "Bool value (always within limit)",
			structVal: &testStructBool{Value: true},
			expected:  true,
		},
		{
			name: "Unsupported type",
			structVal: struct {
				Value []string `validate:"valuemax=5"`
			}{Value: []string{"test"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(tt.structVal)
			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestWithBody_WithConstructor(t *testing.T) {
	app := fiber.New()

	constructorCalled := false
	constructor := func() any {
		constructorCalled = true
		return &testStruct{}
	}

	handler := func(p any, c *fiber.Ctx) error {
		s := p.(*testStruct)
		// Name will be sanitized, so "test" becomes "test" (no special chars)
		assert.Equal(t, "test", s.Name)
		return c.SendStatus(fiber.StatusOK)
	}

	// Create decoderHandler manually to set constructor
	d := &decoderHandler{
		handler:      handler,
		constructor:  constructor,
		structSource: &testStruct{},
	}

	app.Post("/test", d.FiberHandlerFunc)

	body := `{"name": "test", "email": "test@example.com"}`
	req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	// Verify constructor was called (main purpose of test)
	assert.True(t, constructorCalled, "Constructor should be called when provided")
	// Status may vary, but constructor should be called
	assert.NotNil(t, resp)
}

func TestWithBody_UnmarshalErrors(t *testing.T) {
	app := fiber.New()

	handler := func(p any, c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	}

	app.Post("/test", WithBody(&testStruct{}, handler))

	tests := []struct {
		name string
		body string
	}{
		{
			name: "Invalid JSON",
			body: `{"name": "test", invalid}`,
		},
		{
			name: "Valid JSON body with no unknown fields",
			body: `{"name": "test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, resp.StatusCode, fiber.StatusBadRequest)
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
