// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringArray_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected StringArray
		wantErr  bool
	}{
		{
			name:     "nil input returns nil slice (NULL column handling, follows lib/pq convention)",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty array",
			input:    "{}",
			expected: StringArray{},
			wantErr:  false,
		},
		{
			name:     "single element",
			input:    "{hello}",
			expected: StringArray{"hello"},
			wantErr:  false,
		},
		{
			name:     "multiple elements",
			input:    "{hello,world,test}",
			expected: StringArray{"hello", "world", "test"},
			wantErr:  false,
		},
		{
			name:     "UUID array",
			input:    "{550e8400-e29b-41d4-a716-446655440000,6ba7b810-9dad-11d1-80b4-00c04fd430c8}",
			expected: StringArray{"550e8400-e29b-41d4-a716-446655440000", "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
			wantErr:  false,
		},
		{
			name:     "quoted element with comma",
			input:    `{hello,"world,test",foo}`,
			expected: StringArray{"hello", "world,test", "foo"},
			wantErr:  false,
		},
		{
			name:     "quoted element with spaces",
			input:    `{"hello world",test}`,
			expected: StringArray{"hello world", "test"},
			wantErr:  false,
		},
		{
			name:     "escaped quotes",
			input:    `{"say \"hello\"",test}`,
			expected: StringArray{`say "hello"`, "test"},
			wantErr:  false,
		},
		{
			name:     "escaped backslash",
			input:    `{"path\\to\\file",test}`,
			expected: StringArray{`path\to\file`, "test"},
			wantErr:  false,
		},
		{
			name:     "byte slice input",
			input:    []byte("{hello,world}"),
			expected: StringArray{"hello", "world"},
			wantErr:  false,
		},
		{
			name:     "invalid format - no braces",
			input:    "hello,world",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid type",
			input:    123,
			expected: nil,
			wantErr:  true,
		},
		// Edge cases for invalid inputs
		{
			name:     "invalid type - float64",
			input:    3.14,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid type - boolean",
			input:    true,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid type - slice of int",
			input:    []int{1, 2, 3},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid type - map",
			input:    map[string]string{"key": "value"},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - missing closing brace",
			input:    "{hello,world",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - missing opening brace",
			input:    "hello,world}",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - single opening brace only",
			input:    "{",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - single closing brace only",
			input:    "}",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - empty string",
			input:    "",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - whitespace only",
			input:    "   ",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - braces swapped",
			input:    "}{",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "nested braces parsed as literal characters",
			input:    "{{a,b},{c,d}}",
			expected: StringArray{"{a", "b}", "{c", "d}"},
			wantErr:  false,
		},
		// Valid edge cases with special content
		{
			name:     "empty byte slice input",
			input:    []byte("{}"),
			expected: StringArray{},
			wantErr:  false,
		},
		{
			name:     "NULL element in array (PostgreSQL literal)",
			input:    "{hello,NULL,world}",
			expected: StringArray{"hello", NullString, "world"},
			wantErr:  false,
		},
		{
			name:     "array with empty quoted element",
			input:    `{hello,"",world}`,
			expected: StringArray{"hello", "", "world"},
			wantErr:  false,
		},
		{
			name:     "array with multiple empty quoted elements",
			input:    `{"","",""}`,
			expected: StringArray{"", "", ""},
			wantErr:  false,
		},
		{
			name:     "element with special characters",
			input:    `{hello,"@#$%^&*()",world}`,
			expected: StringArray{"hello", "@#$%^&*()", "world"},
			wantErr:  false,
		},
		{
			name:     "element with unicode characters",
			input:    `{hello,"世界",test}`,
			expected: StringArray{"hello", "世界", "test"},
			wantErr:  false,
		},
		{
			name:     "element with newline character",
			input:    "{hello,\"line1\nline2\",world}",
			expected: StringArray{"hello", "line1\nline2", "world"},
			wantErr:  false,
		},
		{
			name:     "element with tab character",
			input:    "{hello,\"col1\tcol2\",world}",
			expected: StringArray{"hello", "col1\tcol2", "world"},
			wantErr:  false,
		},
		{
			name:     "element with leading and trailing spaces preserved in quotes",
			input:    `{"  spaced  ",normal}`,
			expected: StringArray{"  spaced  ", "normal"},
			wantErr:  false,
		},
		{
			name:     "mixed escaped characters",
			input:    `{"say \"hello\\world\"",test}`,
			expected: StringArray{`say "hello\world"`, "test"},
			wantErr:  false,
		},
		{
			name:     "consecutive commas treated as elements",
			input:    "{a,,b}",
			expected: StringArray{"a", "", "b"},
			wantErr:  false,
		},
		{
			name:     "single empty quoted element",
			input:    `{""}`,
			expected: StringArray{""},
			wantErr:  false,
		},
		{
			name:     "invalid format - unclosed quote at start",
			input:    `{"hello}`,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - unclosed quote in middle",
			input:    `{hello,"world}`,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - unclosed quote with escaped content",
			input:    `{"hello \"world}`,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var arr StringArray
			err := arr.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, arr)
		})
	}
}

func TestStringArray_Value(t *testing.T) {
	tests := []struct {
		name     string
		input    StringArray
		expected string
		isNil    bool
	}{
		{
			name:     "nil array",
			input:    nil,
			expected: "",
			isNil:    true,
		},
		{
			name:     "empty array",
			input:    StringArray{},
			expected: "{}",
			isNil:    false,
		},
		{
			name:     "single element",
			input:    StringArray{"hello"},
			expected: "{hello}",
			isNil:    false,
		},
		{
			name:     "multiple elements",
			input:    StringArray{"hello", "world", "test"},
			expected: "{hello,world,test}",
			isNil:    false,
		},
		{
			name:     "UUID array",
			input:    StringArray{"550e8400-e29b-41d4-a716-446655440000", "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
			expected: "{550e8400-e29b-41d4-a716-446655440000,6ba7b810-9dad-11d1-80b4-00c04fd430c8}",
			isNil:    false,
		},
		{
			name:     "element with comma needs quoting",
			input:    StringArray{"hello", "world,test", "foo"},
			expected: `{hello,"world,test",foo}`,
			isNil:    false,
		},
		{
			name:     "element with space needs quoting",
			input:    StringArray{"hello world", "test"},
			expected: `{"hello world",test}`,
			isNil:    false,
		},
		{
			name:     "element with quotes needs escaping",
			input:    StringArray{`say "hello"`, "test"},
			expected: `{"say \"hello\"",test}`,
			isNil:    false,
		},
		{
			name:     "element with backslash needs escaping",
			input:    StringArray{`path\to\file`, "test"},
			expected: `{"path\\to\\file",test}`,
			isNil:    false,
		},
		{
			name:     "empty string element needs quoting",
			input:    StringArray{"", "test"},
			expected: `{"",test}`,
			isNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.input.Value()
			require.NoError(t, err)

			if tt.isNil {
				assert.Nil(t, val)
				return
			}

			assert.Equal(t, tt.expected, val)
		})
	}
}

func TestStringArray_RoundTrip(t *testing.T) {
	// Test that Value -> Scan produces the original array
	testCases := []struct {
		name  string
		input StringArray
	}{
		{name: "empty_array", input: StringArray{}},
		{name: "single_element", input: StringArray{"hello"}},
		{name: "multiple_elements", input: StringArray{"hello", "world"}},
		{name: "uuid_element", input: StringArray{"550e8400-e29b-41d4-a716-446655440000"}},
		{name: "element_with_comma", input: StringArray{"element with, comma", "normal"}},
		{name: "element_with_quotes", input: StringArray{`with "quotes"`, "test"}},
		{name: "element_with_backslash", input: StringArray{`with\backslash`, "test"}},
		{name: "element_with_spaces", input: StringArray{"  spaces  ", "test"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to PostgreSQL format
			val, err := tc.input.Value()
			require.NoError(t, err)

			// Parse back from PostgreSQL format
			var parsed StringArray
			err = parsed.Scan(val)
			require.NoError(t, err)

			assert.Equal(t, tc.input, parsed)
		})
	}
}

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "unique violation error",
			err:      &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"},
			expected: true,
		},
		{
			name:     "foreign key violation",
			err:      &pgconn.PgError{Code: "23503", Message: "foreign key violation"},
			expected: false,
		},
		{
			name:     "generic error",
			err:      assert.AnError,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUniqueViolation(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
