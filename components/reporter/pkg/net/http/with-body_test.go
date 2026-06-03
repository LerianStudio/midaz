// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPayload is a simple struct used in withBody tests.
type testPayload struct {
	Name    string         `json:"name" validate:"required"`
	Age     int            `json:"age"`
	Email   string         `json:"email,omitempty"`
	Active  bool           `json:"active,omitempty"`
	Score   float64        `json:"score,omitempty"`
	Tags    []string       `json:"tags,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// testMetadataPayload has a Metadata field for parseMetadata testing.
type testMetadataPayload struct {
	Name     string         `json:"name" validate:"required"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// testValidatedPayload has validation tags for ValidateStruct testing.
type testValidatedPayload struct {
	Name  string `json:"name" validate:"required"`
	Count int    `json:"count" validate:"gte=1"`
}

// testNoJSONTagPayload has no json tags, used for edge-case testing.
type testNoJSONTagPayload struct {
	Internal string `json:"-"`
	Name     string `json:"name"`
}

func TestWithBody_ValidPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid JSON with required field",
			body:           `{"name":"Alice","age":30}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "Alice",
		},
		{
			name:           "valid JSON with all fields",
			body:           `{"name":"Bob","age":25,"email":"bob@test.com"}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "Bob",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				payload := p.(*testPayload)
				return c.Status(http.StatusOK).SendString(payload.Name)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestWithBody_EmptyAndNullBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "empty body returns 400",
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TPL-0001",
		},
		{
			name:           "whitespace-only body returns 400",
			body:           "   ",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TPL-0001",
		},
		{
			name:           "null literal body returns 400",
			body:           "null",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TPL-0001",
		},
		{
			name:           "null with whitespace returns 400",
			body:           "  null  ",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TPL-0001",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Contains(t, string(body), tt.expectedCode)
		})
	}
}

func TestWithBody_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "malformed JSON missing closing brace",
			body:           `{"name":"Alice"`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "plain text instead of JSON",
			body:           `hello world`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "incomplete JSON array",
			body:           `[{"name":"Alice"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestWithBody_UnmarshalTypeMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "string for int field returns 400",
			body:           `{"name":"Alice","age":"not-a-number"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "array for string field returns 400",
			body:           `{"name":["Alice","Bob"],"age":30}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "object for string field returns 400",
			body:           `{"name":{"first":"Alice"},"age":30}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestWithBody_UnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "extra top-level field returns 400",
			body:           `{"name":"Alice","age":30,"unknown_field":"value"}`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TPL-0015",
		},
		{
			name:           "multiple extra fields returns 400",
			body:           `{"name":"Alice","foo":"bar","baz":123}`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TPL-0015",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Contains(t, string(body), tt.expectedCode)
		})
	}
}

func TestWithBody_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "missing required field returns 400",
			body:           `{"age":30}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty required field returns 400",
			body:           `{"name":""}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestWithBody_GteValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "count below gte threshold returns 400",
			body:           `{"name":"test","count":0}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "count at gte threshold succeeds",
			body:           `{"name":"test","count":1}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "count above gte threshold succeeds",
			body:           `{"name":"test","count":5}`,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testValidatedPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestWithBody_MetadataParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		body              string
		expectedStatus    int
		expectMetadataSet bool
	}{
		{
			name:              "with explicit metadata field",
			body:              `{"name":"Alice","metadata":{"key":"value"}}`,
			expectedStatus:    http.StatusOK,
			expectMetadataSet: true,
		},
		{
			name:              "without metadata field defaults to empty map",
			body:              `{"name":"Alice"}`,
			expectedStatus:    http.StatusOK,
			expectMetadataSet: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testMetadataPayload{}, func(p any, c *fiber.Ctx) error {
				payload := p.(*testMetadataPayload)
				if tt.expectMetadataSet {
					assert.NotNil(t, payload.Metadata)
				}

				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestWithBody_TypeMismatchDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "number where string expected returns 400",
			body:           `{"name":123}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "boolean where string expected returns 400",
			body:           `{"name":true}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "string where map expected returns 400",
			body:           `{"name":"Alice","details":"not-a-map"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "string where slice expected returns 400",
			body:           `{"name":"Alice","tags":"not-a-slice"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestNewOfType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		wantType reflect.Type
	}{
		{
			name:     "creates new instance of testPayload pointer",
			input:    &testPayload{},
			wantType: reflect.TypeOf(&testPayload{}),
		},
		{
			name:     "creates new instance of testMetadataPayload pointer",
			input:    &testMetadataPayload{},
			wantType: reflect.TypeOf(&testMetadataPayload{}),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := newOfType(tt.input)
			assert.Equal(t, tt.wantType, reflect.TypeOf(result))
			assert.NotSame(t, tt.input, result)
		})
	}
}

func TestFindUnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		original  map[string]any
		marshaled map[string]any
		wantDiff  map[string]any
	}{
		{
			name:      "no differences returns empty map",
			original:  map[string]any{"name": "Alice", "age": 30.0},
			marshaled: map[string]any{"name": "Alice", "age": 30.0},
			wantDiff:  map[string]any{},
		},
		{
			name:      "extra field in original detected",
			original:  map[string]any{"name": "Alice", "extra": "value"},
			marshaled: map[string]any{"name": "Alice"},
			wantDiff:  map[string]any{"extra": "value"},
		},
		{
			name:      "zero numeric value skipped",
			original:  map[string]any{"name": "Alice", "count": 0.0},
			marshaled: map[string]any{"name": "Alice"},
			wantDiff:  map[string]any{},
		},
		{
			name:      "nested map differences detected",
			original:  map[string]any{"meta": map[string]any{"a": "1", "b": "2"}},
			marshaled: map[string]any{"meta": map[string]any{"a": "1"}},
			wantDiff:  map[string]any{"meta": map[string]any{"b": "2"}},
		},
		{
			name:      "different values detected",
			original:  map[string]any{"name": "Alice"},
			marshaled: map[string]any{"name": "Bob"},
			wantDiff:  map[string]any{"name": "Alice"},
		},
		{
			name:      "type mismatch map vs non-map detected",
			original:  map[string]any{"data": map[string]any{"key": "val"}},
			marshaled: map[string]any{"data": "string-value"},
			wantDiff:  map[string]any{"data": map[string]any{"key": "val"}},
		},
		{
			name:      "type mismatch slice vs non-slice detected",
			original:  map[string]any{"items": []any{"a", "b"}},
			marshaled: map[string]any{"items": "not-a-slice"},
			wantDiff:  map[string]any{"items": []any{"a", "b"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := findUnknownFields(tt.original, tt.marshaled)
			assert.Equal(t, tt.wantDiff, result)
		})
	}
}

func TestCompareSlices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		original  []any
		marshaled []any
		wantDiff  []any
	}{
		{
			name:      "identical slices returns nil",
			original:  []any{"a", "b", "c"},
			marshaled: []any{"a", "b", "c"},
			wantDiff:  nil,
		},
		{
			name:      "original longer than marshaled detects extras",
			original:  []any{"a", "b", "c"},
			marshaled: []any{"a"},
			wantDiff:  []any{"b", "c"},
		},
		{
			name:      "marshaled longer than original detects extras",
			original:  []any{"a"},
			marshaled: []any{"a", "b", "c"},
			wantDiff:  []any{"b", "c"},
		},
		{
			name:      "different values detected",
			original:  []any{"a", "x"},
			marshaled: []any{"a", "y"},
			wantDiff:  []any{"x"},
		},
		{
			name:      "nested map differences in slice detected",
			original:  []any{map[string]any{"k1": "v1", "k2": "v2"}},
			marshaled: []any{map[string]any{"k1": "v1"}},
			wantDiff:  []any{map[string]any{"k2": "v2"}},
		},
		{
			name:      "empty slices returns nil",
			original:  []any{},
			marshaled: []any{},
			wantDiff:  nil,
		},
		{
			name:      "nested maps with no diff returns nil",
			original:  []any{map[string]any{"k": "v"}},
			marshaled: []any{map[string]any{"k": "v"}},
			wantDiff:  nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := compareSlices(tt.original, tt.marshaled)
			assert.Equal(t, tt.wantDiff, result)
		})
	}
}

func TestValidateStruct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name:    "valid struct with required field passes",
			input:   &testPayload{Name: "Alice"},
			wantErr: false,
		},
		{
			name:    "missing required field returns error",
			input:   &testPayload{Name: ""},
			wantErr: true,
		},
		{
			name:    "non-struct pointer returns nil",
			input:   strPtr("hello"),
			wantErr: false,
		},
		{
			name:    "non-pointer non-struct returns nil",
			input:   42,
			wantErr: false,
		},
		{
			name:    "valid gte constraint passes",
			input:   &testValidatedPayload{Name: "test", Count: 5},
			wantErr: false,
		},
		{
			name:    "gte constraint violation returns error",
			input:   &testValidatedPayload{Name: "test", Count: 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFields(t *testing.T) {
	t.Parallel()

	t.Run("Success - returns nil for empty errors", func(t *testing.T) {
		t.Parallel()

		// ValidateStruct with a valid struct will have no errors, tested indirectly.
		// Directly test that fields() returns nil for no errors by using ValidateStruct.
		err := ValidateStruct(&testPayload{Name: "valid"})
		require.NoError(t, err)
	})
}

func TestFieldsRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    pkg.FieldValidations
		expected pkg.FieldValidations
	}{
		{
			name:     "filters only required fields",
			input:    pkg.FieldValidations{"Name": "Name is a required field", "Age": "Age must be 1 or greater"},
			expected: pkg.FieldValidations{"Name": "Name is a required field"},
		},
		{
			name:     "empty map returns empty",
			input:    pkg.FieldValidations{},
			expected: pkg.FieldValidations{},
		},
		{
			name:     "no required fields returns empty",
			input:    pkg.FieldValidations{"Count": "must be gte 1"},
			expected: pkg.FieldValidations{},
		},
		{
			name:     "all required fields included",
			input:    pkg.FieldValidations{"A": "A is required", "B": "B is required"},
			expected: pkg.FieldValidations{"A": "A is required", "B": "B is required"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := fieldsRequired(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatErrorFieldName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extracts field name after dot",
			input:    "StructName.FieldName",
			expected: "FieldName",
		},
		{
			name:     "extracts nested field name",
			input:    "Root.Nested.Deep",
			expected: "Nested.Deep",
		},
		{
			name:     "returns original when no dot",
			input:    "SingleField",
			expected: "SingleField",
		},
		{
			name:     "handles dot at start",
			input:    ".FieldName",
			expected: "FieldName",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := formatErrorFieldName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateTypeMismatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		target  any
		wantErr bool
	}{
		{
			name:    "matching types returns nil",
			body:    `{"name":"Alice","age":30}`,
			target:  &testPayload{Name: "Alice", Age: 30},
			wantErr: false,
		},
		{
			name:    "number for string field returns error",
			body:    `{"name":123}`,
			target:  &testPayload{},
			wantErr: true,
		},
		{
			name:    "boolean for string field returns error",
			body:    `{"name":true}`,
			target:  &testPayload{},
			wantErr: true,
		},
		{
			name:    "string for map field returns error",
			body:    `{"name":"Alice","details":"not-map"}`,
			target:  &testPayload{Name: "Alice"},
			wantErr: true,
		},
		{
			name:    "string for slice field returns error",
			body:    `{"name":"Alice","tags":"not-slice"}`,
			target:  &testPayload{Name: "Alice"},
			wantErr: true,
		},
		{
			name:    "non-pointer target returns nil",
			body:    `{"name":"Alice"}`,
			target:  testPayload{Name: "Alice"},
			wantErr: false,
		},
		{
			name:    "invalid JSON body returns error",
			body:    `{invalid}`,
			target:  &testPayload{},
			wantErr: true,
		},
		{
			name:    "object for string field returns error",
			body:    `{"name":{"nested":"value"}}`,
			target:  &testPayload{},
			wantErr: true,
		},
		{
			name:    "array for string field returns error",
			body:    `{"name":["a","b"]}`,
			target:  &testPayload{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateTypeMismatches([]byte(tt.body), tt.target)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetTypeMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     any
		fieldKind reflect.Kind
		wantNil   bool
	}{
		{
			name:      "string to map is a mismatch",
			value:     "hello",
			fieldKind: reflect.Map,
			wantNil:   false,
		},
		{
			name:      "string to slice is a mismatch",
			value:     "hello",
			fieldKind: reflect.Slice,
			wantNil:   false,
		},
		{
			name:      "string to string is not a mismatch",
			value:     "hello",
			fieldKind: reflect.String,
			wantNil:   true,
		},
		{
			name:      "map to simple type is a mismatch",
			value:     map[string]any{"key": "val"},
			fieldKind: reflect.String,
			wantNil:   false,
		},
		{
			name:      "map to map is not a mismatch",
			value:     map[string]any{"key": "val"},
			fieldKind: reflect.Map,
			wantNil:   true,
		},
		{
			name:      "slice to simple type is a mismatch",
			value:     []any{"a"},
			fieldKind: reflect.String,
			wantNil:   false,
		},
		{
			name:      "slice to slice is not a mismatch",
			value:     []any{"a"},
			fieldKind: reflect.Slice,
			wantNil:   true,
		},
		{
			name:      "float64 to string is a mismatch",
			value:     float64(42),
			fieldKind: reflect.String,
			wantNil:   false,
		},
		{
			name:      "float64 to map is a mismatch",
			value:     float64(42),
			fieldKind: reflect.Map,
			wantNil:   false,
		},
		{
			name:      "float64 to float64 is not a mismatch",
			value:     float64(42),
			fieldKind: reflect.Float64,
			wantNil:   true,
		},
		{
			name:      "float64 to int is not a mismatch",
			value:     float64(42),
			fieldKind: reflect.Int,
			wantNil:   true,
		},
		{
			name:      "bool to string is a mismatch",
			value:     true,
			fieldKind: reflect.String,
			wantNil:   false,
		},
		{
			name:      "bool to map is a mismatch",
			value:     true,
			fieldKind: reflect.Map,
			wantNil:   false,
		},
		{
			name:      "bool to bool is not a mismatch",
			value:     true,
			fieldKind: reflect.Bool,
			wantNil:   true,
		},
		{
			name:      "nil value returns nil mismatch",
			value:     nil,
			fieldKind: reflect.String,
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getTypeMismatch(tt.value, tt.fieldKind)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
			}
		})
	}
}

func TestIsSimpleType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     reflect.Kind
		expected bool
	}{
		{name: "string is simple", kind: reflect.String, expected: true},
		{name: "int is simple", kind: reflect.Int, expected: true},
		{name: "float64 is simple", kind: reflect.Float64, expected: true},
		{name: "bool is simple", kind: reflect.Bool, expected: true},
		{name: "map is not simple", kind: reflect.Map, expected: false},
		{name: "slice is not simple", kind: reflect.Slice, expected: false},
		{name: "struct is not simple", kind: reflect.Struct, expected: false},
		{name: "ptr is not simple", kind: reflect.Ptr, expected: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isSimpleType(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFieldNameFromUnmarshalError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		errorMsg string
		expected string
	}{
		{
			name:     "extracts field from standard unmarshal error",
			errorMsg: "json: cannot unmarshal string into Go struct field CreateReportInput.filters of type map[string]map[string]map[string][]string",
			expected: "filters",
		},
		{
			name:     "extracts field from simple struct field error",
			errorMsg: "json: cannot unmarshal number into Go struct field MyStruct.Name of type string",
			expected: "Name",
		},
		{
			name:     "fallback regex extracts field",
			errorMsg: "json: cannot unmarshal string into Go value of type int, field Age of type int",
			expected: "Age",
		},
		{
			name:     "returns empty for unrecognized format",
			errorMsg: "some completely unrelated error message",
			expected: "",
		},
		{
			name:     "returns empty for empty string",
			errorMsg: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractFieldNameFromUnmarshalError(tt.errorMsg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           any
		originalMap     map[string]any
		expectDefaulted bool
	}{
		{
			name:            "sets empty map when metadata key missing from original",
			input:           &testMetadataPayload{Name: "Alice"},
			originalMap:     map[string]any{"name": "Alice"},
			expectDefaulted: true,
		},
		{
			name:            "preserves existing metadata when key present",
			input:           &testMetadataPayload{Name: "Alice", Metadata: map[string]any{"k": "v"}},
			originalMap:     map[string]any{"name": "Alice", "metadata": map[string]any{"k": "v"}},
			expectDefaulted: false,
		},
		{
			name:            "no-op for non-pointer",
			input:           testMetadataPayload{Name: "Alice"},
			originalMap:     map[string]any{"name": "Alice"},
			expectDefaulted: false,
		},
		{
			name:            "no-op for struct without Metadata field",
			input:           &testPayload{Name: "Alice"},
			originalMap:     map[string]any{"name": "Alice"},
			expectDefaulted: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parseMetadata(tt.input, tt.originalMap)

			if mp, ok := tt.input.(*testMetadataPayload); ok && tt.expectDefaulted {
				assert.NotNil(t, mp.Metadata)
				assert.Empty(t, mp.Metadata)
			}
		})
	}
}

func TestNewValidator(t *testing.T) {
	t.Parallel()

	t.Run("Success - creates validator with translator", func(t *testing.T) {
		t.Parallel()

		v, trans, err := newValidator()
		require.NoError(t, err)
		assert.NotNil(t, v)
		assert.NotNil(t, trans)
	})
}

func TestValidateFieldType(t *testing.T) {
	t.Parallel()

	type sampleStruct struct {
		Name string  `json:"name"`
		Age  int     `json:"age"`
		Rate float64 `json:"rate"`
	}

	val := reflect.ValueOf(sampleStruct{})
	typ := val.Type()

	tests := []struct {
		name    string
		value   any
		fieldI  int
		wantErr bool
	}{
		{
			name:    "compatible string value no error",
			value:   "hello",
			fieldI:  0, // Name (string)
			wantErr: false,
		},
		{
			name:    "map for string field returns error",
			value:   map[string]any{"k": "v"},
			fieldI:  0, // Name (string)
			wantErr: true,
		},
		{
			name:    "compatible float64 for int field no error",
			value:   float64(42),
			fieldI:  1, // Age (int)
			wantErr: false,
		},
		{
			name:    "string for int field returns error via unmarshal",
			value:   "not-a-number",
			fieldI:  1,     // Age (int)
			wantErr: false, // string to int is handled by unmarshal, not by type check in getTypeMismatch
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateFieldType(tt.value, val.Field(tt.fieldI), typ.Field(tt.fieldI))

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWithBody_HandlerReceivesDecodedPayload(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
		payload, ok := p.(*testPayload)
		require.True(t, ok, "payload should be *testPayload")
		assert.Equal(t, "Charlie", payload.Name)
		assert.Equal(t, 28, payload.Age)

		return c.SendStatus(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test",
		strings.NewReader(`{"name":"Charlie","age":28}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithBody_ConstructorFunc(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	d := &decoderHandler{
		handler: func(p any, c *fiber.Ctx) error {
			payload, ok := p.(*testPayload)
			require.True(t, ok)
			assert.Equal(t, "Dave", payload.Name)

			return c.SendStatus(http.StatusOK)
		},
		constructor: func() any {
			return &testPayload{}
		},
	}

	app.Post("/test", d.FiberHandlerFunc)

	req := httptest.NewRequest(http.MethodPost, "/test",
		strings.NewReader(`{"name":"Dave","age":35}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithBody_FieldsLocal(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	app.Post("/test", WithBody(&testPayload{}, func(p any, c *fiber.Ctx) error {
		fieldsVal := c.Locals("fields")
		assert.NotNil(t, fieldsVal, "fields local should be set")

		diffFields, ok := fieldsVal.(map[string]any)
		assert.True(t, ok)
		assert.Empty(t, diffFields, "no diff fields expected for valid payload")

		return c.SendStatus(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test",
		strings.NewReader(`{"name":"Eve","age":22}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithBody_IgnoredJSONTagField(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	app.Post("/test", WithBody(&testNoJSONTagPayload{}, func(p any, c *fiber.Ctx) error {
		payload, ok := p.(*testNoJSONTagPayload)
		require.True(t, ok)
		assert.Equal(t, "test", payload.Name)
		assert.Empty(t, payload.Internal, "json:'-' field should not be populated")

		return c.SendStatus(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test",
		strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// testMetadataValidationPayload is a struct with metadata validation tags
// that exercises the keymax, valuemax, and nonested custom validators.
type testMetadataValidationPayload struct {
	Name     string         `json:"name" validate:"required"`
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=10,endkeys,nonested,valuemax=50"`
}

func TestValidateStruct_MetadataKeyMaxLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name: "short key passes keymax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"short": "value"},
			},
			wantErr: false,
		},
		{
			name: "key exceeding max length fails keymax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"this_key_is_definitely_longer_than_ten_chars": "value"},
			},
			wantErr: true,
		},
		{
			name: "key at exactly max length passes",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"exactly_10": "value"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateStruct_MetadataValueMaxLength(t *testing.T) {
	t.Parallel()

	longValue := strings.Repeat("x", 51) // exceeds valuemax=50

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name: "short string value passes valuemax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": "short"},
			},
			wantErr: false,
		},
		{
			name: "string value exceeding max length fails valuemax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": longValue},
			},
			wantErr: true,
		},
		{
			name: "int value passes valuemax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": 42},
			},
			wantErr: false,
		},
		{
			name: "float64 value passes valuemax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": 3.14},
			},
			wantErr: false,
		},
		{
			name: "bool value passes valuemax validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateStruct_MetadataNoNested(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name: "flat string value passes nonested validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "nested map value fails nonested validation",
			input: &testMetadataValidationPayload{
				Name:     "test",
				Metadata: map[string]any{"key": map[string]any{"nested": "value"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWithBody_MetadataValidationViaHTTP(t *testing.T) {
	t.Parallel()

	longKey := strings.Repeat("k", 11) // exceeds keymax=10

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "valid metadata passes",
			body:           `{"name":"Alice","metadata":{"key":"value"}}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "metadata key too long returns 400",
			body:           `{"name":"Alice","metadata":{"` + longKey + `":"value"}}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "nested metadata value returns 400",
			body:           `{"name":"Alice","metadata":{"key":{"nested":"value"}}}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})

			app.Post("/test", WithBody(&testMetadataValidationPayload{}, func(p any, c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

// strPtr is a test helper that returns a pointer to a string.
func strPtr(s string) *string {
	return &s
}
