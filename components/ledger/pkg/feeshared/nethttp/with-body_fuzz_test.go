// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
)

// FuzzValidateUUID_Value fuzzes the validateUUID custom validator via ValidateStruct.
// It verifies that no input causes a panic and that the function always returns
// either nil (valid) or an error (invalid) without crashing.
func FuzzValidateUUID_Value(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security
	f.Add("")                                      // empty input
	f.Add(uuid.New().String())                     // valid UUID v4
	f.Add("00000000-0000-0000-0000-000000000000")  // nil UUID (valid format)
	f.Add("not-a-uuid")                            // invalid format
	f.Add("00000000-0000-0000-0000-00000000000")   // boundary: one char short
	f.Add("00000000-0000-0000-0000-0000000000000") // boundary: one char long
	f.Add("\x00\x01\x02\x03\x04")                  // binary/null bytes
	f.Add("'; DROP TABLE users; --")               // SQL injection
	f.Add("<script>alert('xss')</script>")         // XSS payload
	f.Add(strings.Repeat("a", 1000))               // long string

	f.Fuzz(func(t *testing.T, input string) {
		// Bound input to prevent resource exhaustion
		if len(input) > 512 {
			input = input[:512]
		}

		s := &testStructWithUUID{ID: input}
		// Property: ValidateStruct must not panic.
		// It should return nil or an error, never crash.
		err := ValidateStruct(s)

		// If input is empty, UUID validator allows it (optional field).
		if input == "" {
			if err != nil {
				t.Errorf("empty UUID should be accepted but got error: %v", err)
			}
			return
		}

		// If input is a valid UUID, validation should pass.
		if _, parseErr := uuid.Parse(input); parseErr == nil {
			if err != nil {
				t.Errorf("valid UUID %q should be accepted but got error: %v", input, err)
			}
		}
		// For invalid UUIDs, we just verify no panic occurred (implicit).
	})
}

// FuzzValidateStruct_KeyMax fuzzes the keymax custom validator with random key strings.
// It verifies that keys within the configured limit (10) pass and keys exceeding it fail,
// without any panics regardless of input content.
func FuzzValidateStruct_KeyMax(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security
	f.Add("")                               // empty string
	f.Add("short")                          // valid: under limit
	f.Add("1234567890")                     // boundary: exactly at limit (10)
	f.Add("12345678901")                    // boundary: one over limit (11)
	f.Add(strings.Repeat("x", 200))         // long string well over limit
	f.Add("\u00e9\u00e8\u00ea\u00eb\u00ef") // unicode accented chars
	f.Add("\U0001f600\U0001f601\U0001f602") // emoji sequence
	f.Add("'; DROP TABLE users; --")        // SQL injection
	f.Add("<img src=x onerror=alert(1)>")   // XSS payload
	f.Add("\x00\xff\xfe\xfd")               // binary bytes

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 512 {
			input = input[:512]
		}

		s := &testStructWithKeyMax{Key: input}
		// Property: ValidateStruct must not panic.
		err := ValidateStruct(s)

		// Property: keys with rune count <= 10 should pass; > 10 should fail (matches keymax validator).
		runeCount := utf8.RuneCountInString(input)
		if runeCount <= 10 {
			if err != nil {
				t.Errorf("key %q (runes=%d) should be valid but got error: %v", input, runeCount, err)
			}
		} else {
			if err == nil {
				t.Errorf("key %q (runes=%d) should be invalid but passed validation", input, runeCount)
			}
		}
	})
}

// FuzzValidateStruct_ValueMax fuzzes the valuemax custom validator with random value strings.
// It verifies that values within the configured limit (20) pass and values exceeding it fail,
// without any panics regardless of input content.
func FuzzValidateStruct_ValueMax(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security
	f.Add("")                                 // empty string
	f.Add("short value")                      // valid: under limit
	f.Add("12345678901234567890")             // boundary: exactly at limit (20)
	f.Add("123456789012345678901")            // boundary: one over limit (21)
	f.Add(strings.Repeat("v", 500))           // long string well over limit
	f.Add("\u4e16\u754c\u4f60\u597d")         // CJK unicode chars
	f.Add("\U0001f4b0\U0001f4b5\U0001f4b6")   // money emoji (multi-byte)
	f.Add("Robert'; DROP TABLE students;--")  // SQL injection (Bobby Tables)
	f.Add("<script>document.cookie</script>") // XSS cookie theft
	f.Add("\t\n\r \x00")                      // whitespace and null bytes

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 512 {
			input = input[:512]
		}

		s := &testStructWithValueMax{Value: input}
		// Property: ValidateStruct must not panic.
		err := ValidateStruct(s)

		// Property: values with rune count <= 20 should pass; > 20 should fail (matches valuemax validator).
		runeCount := utf8.RuneCountInString(input)
		if runeCount <= 20 {
			if err != nil {
				t.Errorf("value %q (runes=%d) should be valid but got error: %v", input, runeCount, err)
			}
		} else {
			if err == nil {
				t.Errorf("value %q (runes=%d) should be invalid but passed validation", input, runeCount)
			}
		}
	})
}

// FuzzSanitizeString_Input fuzzes the sanitizeString function which removes special
// characters from input strings. It verifies that the output never contains
// disallowed characters and never panics, regardless of input.
func FuzzSanitizeString_Input(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security
	f.Add("")                                 // empty string
	f.Add("hello world")                      // normal string (all allowed)
	f.Add("test@example.com")                 // allowed special chars (@, .)
	f.Add("path/to\\file-name_here")          // slashes, dash, underscore
	f.Add("!@#$%^&*()+=[]{}|;':\"<>?`~")      // all special chars
	f.Add(strings.Repeat("\U0001f600", 50))   // repeated emoji
	f.Add("\u0000\u0001\u0002\u007f")         // control characters
	f.Add("<script>alert('xss')</script>")    // XSS payload
	f.Add("Robert'); DROP TABLE students;--") // SQL injection
	f.Add(strings.Repeat("a", 10000))         // very long valid string

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 2048 {
			input = input[:2048]
		}

		// Property: sanitizeString must not panic.
		result := sanitizeString(input)

		// Property: output must only contain allowed characters.
		// Allowed: letters, numbers, dash, underscore, space, @, dot, comma, slash, backslash
		for _, r := range result {
			if !isAllowedChar(r) {
				t.Errorf("sanitizeString(%q) produced output containing disallowed char %q (U+%04X)", input, string(r), r)
			}
		}

		// Property: output length must be <= input length (can only remove, not add).
		if len(result) > len(input) {
			t.Errorf("sanitizeString output length (%d) > input length (%d)", len(result), len(input))
		}
	})
}

// isAllowedChar checks if a rune matches the allowed character set from specialCharsRegex.
func isAllowedChar(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '/', '\\', '-', '_', ' ', '@', '.', ',':
		return true
	}
	return false
}

// FuzzFormatErrorFieldName_Input fuzzes the formatErrorFieldName function
// which extracts field names from dotted namespace strings.
// It verifies the function never panics and returns consistent results.
func FuzzFormatErrorFieldName_Input(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security
	f.Add("")                              // empty string
	f.Add("Field")                         // no namespace
	f.Add("Struct.Field")                  // single namespace
	f.Add("Struct.Nested.Field")           // deep namespace
	f.Add(".")                             // just a dot
	f.Add(".Field")                        // leading dot
	f.Add("Field.")                        // trailing dot
	f.Add("...")                           // multiple dots
	f.Add(strings.Repeat("a.", 100) + "z") // very deep namespace
	f.Add("\u00e9.\u00e8")                 // unicode with dots

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1024 {
			input = input[:1024]
		}

		// Property: formatErrorFieldName must not panic.
		result := formatErrorFieldName(input)

		// Property: result must not be longer than input.
		if len(result) > len(input) {
			t.Errorf("formatErrorFieldName(%q) result length (%d) > input length (%d)",
				input, len(result), len(input))
		}

		// Property: if input has no dot, result equals input.
		if !strings.Contains(input, ".") {
			if result != input {
				t.Errorf("formatErrorFieldName(%q) = %q; expected same string when no dot present",
					input, result)
			}
		}
	})
}

// FuzzIsStringNumeric_Input fuzzes the isStringNumeric function which checks
// whether a string can be parsed as a decimal number.
// It verifies the function never panics regardless of input.
func FuzzIsStringNumeric_Input(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security
	f.Add("")                        // empty string
	f.Add("0")                       // zero
	f.Add("123.45")                  // positive decimal
	f.Add("-123.45")                 // negative decimal
	f.Add("999999999999999999.99")   // large number
	f.Add("0.0000000001")            // very small number
	f.Add("not a number")            // invalid
	f.Add("12.34.56")                // multiple dots
	f.Add("1e10")                    // scientific notation
	f.Add(strings.Repeat("9", 1000)) // very long numeric string
	f.Add("\u0660\u0661\u0662")      // Arabic-Indic digits (unicode)
	f.Add("Infinity")                // special float value
	f.Add("NaN")                     // not a number literal

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 512 {
			input = input[:512]
		}

		// Property: isStringNumeric must not panic.
		_ = isStringNumeric(input)
		// No specific assertion on result - just verifying no crash.
	})
}

// FuzzValidateStruct_RequiredFields fuzzes ValidateStruct with random field values
// on a struct that has required and email validation tags.
// It verifies the validator never panics on any combination of inputs.
func FuzzValidateStruct_RequiredFields(f *testing.F) {
	// Seed corpus: combinations of name, email, age
	f.Add("", "", 0)                                                  // all empty/zero
	f.Add("John", "john@example.com", 25)                             // valid
	f.Add("", "test@example.com", 30)                                 // missing required name
	f.Add("Jane", "invalid-email", 20)                                // invalid email
	f.Add("Bob", "bob@test.com", -1)                                  // negative age
	f.Add(strings.Repeat("x", 500), "a@b.c", 999999)                  // long name, edge age
	f.Add("\U0001f600", "\U0001f600@\U0001f600.com", 0)               // emoji in fields
	f.Add("<script>", "xss@evil.com", 0)                              // XSS in name
	f.Add("'; DROP TABLE", "sql@inject.com", 0)                       // SQL injection in name
	f.Add("normal", strings.Repeat("a", 300)+"@test.com", 2147483647) // long email, max int

	f.Fuzz(func(t *testing.T, name, email string, age int) {
		if len(name) > 512 {
			name = name[:512]
		}
		if len(email) > 512 {
			email = email[:512]
		}

		s := &testStructWithValidation{
			Name:  name,
			Email: email,
			Age:   age,
		}

		// Property: ValidateStruct must not panic on any input combination.
		err := ValidateStruct(s)

		// Property: if name is empty, validation must fail (required field).
		if name == "" && err == nil {
			t.Errorf("empty name should fail validation but got nil error")
		}

		// Property: if age is negative, validation must fail (gte=0).
		if age < 0 && err == nil {
			t.Errorf("negative age (%d) should fail validation but got nil error", age)
		}
	})
}
