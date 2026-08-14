// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestProperty_ValidateStruct_Consistency verifies that for any valid struct input,
// ValidateStruct consistently returns no error. A struct with all required fields
// populated and valid values must always pass validation.
func TestProperty_ValidateStruct_Consistency(t *testing.T) {
	property := func(name string, age uint8) bool {
		// PROPERTY: A struct with non-empty required name, valid email, and non-negative age
		// must always pass validation.
		if len(name) == 0 || len(name) > 200 {
			return true // skip trivially empty or oversized names
		}

		s := &testStructWithValidation{
			Name:  name,
			Email: "valid@example.com",
			Age:   int(age),
		}

		err := ValidateStruct(s)

		return err == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_ValidateStruct_Determinism verifies that validating the same struct
// twice always produces the same result. Validation must be a pure function with
// no side effects that change outcomes between invocations.
func TestProperty_ValidateStruct_Determinism(t *testing.T) {
	property := func(name, email string, age int) bool {
		// PROPERTY: Validating the same struct twice must produce identical results.
		if len(name) > 200 {
			name = name[:200]
		}

		if len(email) > 200 {
			email = email[:200]
		}

		s := &testStructWithValidation{
			Name:  name,
			Email: email,
			Age:   age,
		}

		err1 := ValidateStruct(s)
		err2 := ValidateStruct(s)

		// Both calls must agree: both nil or both non-nil
		if err1 == nil && err2 == nil {
			return true
		}

		if err1 != nil && err2 != nil {
			return true
		}

		return false
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_ValidateStruct_RequiredFieldInvariant verifies that any struct with
// empty required fields always fails validation. This is a fundamental invariant
// of the validator: required means required, no exceptions.
func TestProperty_ValidateStruct_RequiredFieldInvariant(t *testing.T) {
	property := func(email string, age int) bool {
		// PROPERTY: A struct with an empty required field (Name="") must always fail validation.
		s := &testStructWithValidation{
			Name:  "", // required field left empty
			Email: email,
			Age:   age,
		}

		err := ValidateStruct(s)

		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_ValidateUUID_ValidFormatAlwaysPasses verifies that any properly
// formatted UUID string always passes the custom UUID validator. The UUID validator
// should accept all standard UUID formats without exception.
func TestProperty_ValidateUUID_ValidFormatAlwaysPasses(t *testing.T) {
	// TODO(review): Seed parameter ignored in UUID generation - ring:test-reviewer on 2026-02-21
	property := func(seed uint64) bool {
		// PROPERTY: A valid UUID must always pass validation.
		// Generate a valid UUID deterministically from the seed.
		validUUID := uuid.New().String()

		s := &testStructWithUUID{ID: validUUID}
		err := ValidateStruct(s)

		return err == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_ValidateUUID_InvalidFormatAlwaysFails verifies that strings which
// are not valid UUID format always fail the UUID validator. This ensures the
// validator rejects all malformed inputs consistently.
func TestProperty_ValidateUUID_InvalidFormatAlwaysFails(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: An invalid UUID string must always fail validation.
		if len(input) > 512 {
			input = input[:512]
		}

		// Skip empty strings (UUID validator allows empty = optional)
		if input == "" {
			return true
		}

		// Skip if input happens to be a valid UUID
		if _, parseErr := uuid.Parse(input); parseErr == nil {
			return true
		}

		s := &testStructWithUUID{ID: input}
		err := ValidateStruct(s)

		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_ValidateUUID_EmptyAlwaysPasses verifies that an empty UUID string
// always passes validation, since the UUID field is optional (not tagged "required").
func TestProperty_ValidateUUID_EmptyAlwaysPasses(t *testing.T) {
	property := func(_ uint8) bool {
		// PROPERTY: Empty UUID must always be accepted (optional field).
		s := &testStructWithUUID{ID: ""}
		err := ValidateStruct(s)

		return err == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_MetadataKeyLength_WithinLimitAlwaysPasses verifies that metadata
// keys with rune count at or below the configured maximum always pass the keymax
// validator.
func TestProperty_MetadataKeyLength_WithinLimitAlwaysPasses(t *testing.T) {
	const keyMaxLimit = 10

	property := func(input string) bool {
		// PROPERTY: Keys with rune count <= limit must always pass keymax validation.
		if utf8.RuneCountInString(input) > keyMaxLimit {
			input = string([]rune(input)[:keyMaxLimit])
		}

		s := &testStructWithKeyMax{Key: input}
		err := ValidateStruct(s)

		return err == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_MetadataKeyLength_ExceedingLimitAlwaysFails verifies that metadata
// keys with rune count exceeding the configured maximum always fail the keymax
// validator.
func TestProperty_MetadataKeyLength_ExceedingLimitAlwaysFails(t *testing.T) {
	const keyMaxLimit = 10

	property := func(input string) bool {
		// PROPERTY: Keys with rune count > limit must always fail keymax validation.
		runeCount := utf8.RuneCountInString(input)
		if runeCount <= keyMaxLimit {
			input = input + strings.Repeat("x", keyMaxLimit+1-runeCount)
		}
		if utf8.RuneCountInString(input) > 512 {
			input = string([]rune(input)[:512])
		}

		s := &testStructWithKeyMax{Key: input}
		err := ValidateStruct(s)

		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_MetadataValueLength_WithinLimitAlwaysPasses verifies that metadata
// values with rune count at or below the configured maximum always pass the
// valuemax validator.
func TestProperty_MetadataValueLength_WithinLimitAlwaysPasses(t *testing.T) {
	const valueMaxLimit = 20

	property := func(input string) bool {
		// PROPERTY: Values with rune count <= limit must always pass valuemax validation.
		if utf8.RuneCountInString(input) > valueMaxLimit {
			input = string([]rune(input)[:valueMaxLimit])
		}

		s := &testStructWithValueMax{Value: input}
		err := ValidateStruct(s)

		return err == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_MetadataValueLength_ExceedingLimitAlwaysFails verifies that metadata
// values with rune count exceeding the configured maximum always fail the valuemax
// validator.
func TestProperty_MetadataValueLength_ExceedingLimitAlwaysFails(t *testing.T) {
	const valueMaxLimit = 20

	property := func(input string) bool {
		// PROPERTY: Values with rune count > limit must always fail valuemax validation.
		runeCount := utf8.RuneCountInString(input)
		if runeCount <= valueMaxLimit {
			input = input + strings.Repeat("v", valueMaxLimit+1-runeCount)
		}
		if utf8.RuneCountInString(input) > 512 {
			input = string([]rune(input)[:512])
		}

		s := &testStructWithValueMax{Value: input}
		err := ValidateStruct(s)

		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_SanitizeString_Idempotency verifies that sanitizing a string twice
// produces the same result as sanitizing it once. This is a fundamental property
// of any sanitization function: once sanitized, further sanitization should be a no-op.
func TestProperty_SanitizeString_Idempotency(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: sanitizeString(sanitizeString(x)) == sanitizeString(x)
		if len(input) > 2048 {
			input = input[:2048]
		}

		once := sanitizeString(input)
		twice := sanitizeString(once)

		return once == twice
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_SanitizeString_OutputSubsetOfAllowed verifies that the output of
// sanitizeString only contains characters from the allowed set. No disallowed
// character should ever appear in the output regardless of input.
func TestProperty_SanitizeString_OutputSubsetOfAllowed(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: Every character in sanitizeString output must be in the allowed set.
		if len(input) > 2048 {
			input = input[:2048]
		}

		result := sanitizeString(input)

		for _, r := range result {
			if !isAllowedChar(r) {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_SanitizeString_OutputNeverLongerThanInput verifies that sanitization
// can only remove characters, never add them. The output length must always be
// less than or equal to the input length.
func TestProperty_SanitizeString_OutputNeverLongerThanInput(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: len(sanitizeString(x)) <= len(x)
		if len(input) > 2048 {
			input = input[:2048]
		}

		result := sanitizeString(input)

		return len(result) <= len(input)
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_ValidateStruct_NonStructNeverFails verifies that passing a non-struct
// value to ValidateStruct never returns an error. The validator should gracefully
// handle non-struct inputs by returning nil.
func TestProperty_ValidateStruct_NonStructNeverFails(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: ValidateStruct on a non-struct value must return nil.
		if len(input) > 512 {
			input = input[:512]
		}

		err := ValidateStruct(input)

		return err == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_FormatErrorFieldName_NoDotReturnsSameString verifies that when
// the input contains no dot character, formatErrorFieldName returns the input
// unchanged. This is the base case for namespace extraction.
func TestProperty_FormatErrorFieldName_NoDotReturnsSameString(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: If input has no dot, formatErrorFieldName returns input unchanged.
		if len(input) > 512 {
			input = input[:512]
		}

		// Skip inputs with dots (tested by a different property)
		if strings.Contains(input, ".") {
			return true
		}

		result := formatErrorFieldName(input)

		return result == input
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// TestProperty_FormatErrorFieldName_OutputNeverLongerThanInput verifies that
// formatErrorFieldName output is never longer than its input. The function
// extracts a suffix, which must be shorter or equal to the full string.
func TestProperty_FormatErrorFieldName_OutputNeverLongerThanInput(t *testing.T) {
	property := func(input string) bool {
		// PROPERTY: len(formatErrorFieldName(x)) <= len(x)
		if len(input) > 512 {
			input = input[:512]
		}

		result := formatErrorFieldName(input)

		return len(result) <= len(input)
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}
