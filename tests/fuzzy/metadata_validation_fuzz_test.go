package fuzzy

import (
	"math"
	"strings"
	"testing"

	pkghttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// FuzzValidateMetadataValue fuzzes the metadata value validation function
// that checks for type validity, nesting depth, and value length limits.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateMetadataValue -run=^$ -fuzztime=60s
func FuzzValidateMetadataValue(f *testing.F) {
	// Seed: valid string values
	f.Add("simple string")
	f.Add("string with spaces and punctuation!")
	f.Add("")
	f.Add("a")

	// Seed: length boundary values (limit is 2000)
	f.Add(strings.Repeat("a", 1999))
	f.Add(strings.Repeat("a", 2000))
	f.Add(strings.Repeat("a", 2001))
	f.Add(strings.Repeat("a", 10000))

	// Seed: unicode strings
	f.Add("Hello 世界")
	f.Add("Привет мир")
	f.Add("مرحبا بالعالم")
	f.Add(strings.Repeat("中", 1000))

	// Seed: control characters
	f.Add("string\x00with\x00nulls")
	f.Add("string\nwith\nnewlines")
	f.Add("string\twith\ttabs")
	f.Add("\x01\x02\x03\x04\x05")

	// Seed: unicode edge cases
	f.Add("\u200B") // zero-width space
	f.Add("\u202E") // right-to-left override
	f.Add("\uFEFF") // BOM
	f.Add("\uFFFD") // replacement character

	// Seed: injection patterns
	f.Add("'; DROP TABLE metadata; --")
	f.Add("<script>alert('xss')</script>")
	f.Add("{{template}}")
	f.Add("${env.SECRET}")
	f.Add("$(cat /etc/passwd)")
	f.Add("`cat /etc/passwd`")
	f.Add("../../../etc/passwd")

	// Seed: JSON-like values (should be validated as strings)
	f.Add(`{"nested": "object"}`)
	f.Add(`["array", "value"]`)
	f.Add(`null`)
	f.Add(`true`)
	f.Add(`false`)
	f.Add(`123`)
	f.Add(`12.34`)

	f.Fuzz(func(t *testing.T, value string) {
		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateMetadataValue panicked on string input (len=%d): panic=%v",
					len(value), r)
			}
		}()

		// Call the validation function with a string value
		result, err := pkghttp.ValidateMetadataValue(value)

		// Verify contract: strings over 2000 chars should be rejected
		if len(value) > 2000 && err == nil {
			t.Errorf("ValidateMetadataValue accepted string with len=%d (limit is 2000)", len(value))
		}

		// Verify: if accepted, result should equal input for strings
		if err == nil && result != nil {
			if strResult, ok := result.(string); ok {
				if strResult != value {
					t.Errorf("ValidateMetadataValue modified string: input=%q result=%q",
						truncateMetaStr(value, 50), truncateMetaStr(strResult, 50))
				}
			}
		}
	})
}

// FuzzValidateMetadataValueNumeric fuzzes metadata validation with numeric types.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateMetadataValueNumeric -run=^$ -fuzztime=30s
func FuzzValidateMetadataValueNumeric(f *testing.F) {
	// Seed: integer values
	f.Add(float64(0))
	f.Add(float64(1))
	f.Add(float64(-1))
	f.Add(float64(100))
	f.Add(float64(-100))

	// Seed: float64 boundary values
	f.Add(float64(9223372036854775807))     // max int64 as float
	f.Add(float64(-9223372036854775808))    // min int64 as float
	f.Add(float64(1.7976931348623157e+308)) // max float64
	f.Add(float64(-1.7976931348623157e+308)) // min float64
	f.Add(float64(5e-324))                  // smallest positive float64

	// Seed: special float values
	f.Add(float64(0.1))
	f.Add(float64(0.01))
	f.Add(float64(0.001))
	f.Add(float64(0.0000001))

	// Seed: precision edge cases (2^53 boundary)
	f.Add(float64(9007199254740992))
	f.Add(float64(9007199254740993))

	// Seed: special IEEE 754 values
	f.Add(math.NaN())
	f.Add(math.Inf(1))  // +Inf
	f.Add(math.Inf(-1)) // -Inf

	f.Fuzz(func(t *testing.T, value float64) {
		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateMetadataValue panicked on float64 input=%v: panic=%v",
					value, r)
			}
		}()

		// Numeric values should always be accepted
		result, err := pkghttp.ValidateMetadataValue(value)

		// Verify: numeric values should be accepted (unless NaN/Inf handling is special)
		if err != nil {
			t.Logf("ValidateMetadataValue rejected float64=%v err=%v", value, err)
		}

		// Verify: result should equal input for accepted values
		if err == nil && result != nil {
			if floatResult, ok := result.(float64); ok {
				// Handle NaN specially (NaN != NaN is always true in IEEE 754)
				if math.IsNaN(value) {
					if !math.IsNaN(floatResult) {
						t.Errorf("ValidateMetadataValue changed NaN to %v", floatResult)
					}
				} else if floatResult != value {
					t.Errorf("ValidateMetadataValue modified float64: input=%v result=%v",
						value, floatResult)
				}
			}
		}
	})
}

// FuzzValidateMetadataValueArray fuzzes metadata validation with array inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateMetadataValueArray -run=^$ -fuzztime=30s
func FuzzValidateMetadataValueArray(f *testing.F) {
	// Seed: array sizes
	f.Add(0)
	f.Add(1)
	f.Add(5)
	f.Add(10)
	f.Add(100)
	f.Add(1000)

	f.Fuzz(func(t *testing.T, arraySize int) {
		// Limit array size to prevent OOM
		if arraySize < 0 {
			arraySize = 0
		}
		if arraySize > 10000 {
			arraySize = 10000
		}

		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateMetadataValue panicked on array size=%d: panic=%v",
					arraySize, r)
			}
		}()

		// Build array with string values
		arr := make([]any, arraySize)
		for i := 0; i < arraySize; i++ {
			arr[i] = "value"
		}

		_, _ = pkghttp.ValidateMetadataValue(arr)
	})
}

// FuzzValidateMetadataValueNested tests nested structure rejection.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateMetadataValueNested -run=^$ -fuzztime=30s
func FuzzValidateMetadataValueNested(f *testing.F) {
	// Seed: nesting depths
	f.Add(1)
	f.Add(5)
	f.Add(10)
	f.Add(11)
	f.Add(20)
	f.Add(100)

	f.Fuzz(func(t *testing.T, depth int) {
		// Limit depth to prevent stack overflow
		if depth < 0 {
			depth = 0
		}
		if depth > 100 {
			depth = 100
		}

		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateMetadataValue panicked on nested depth=%d: panic=%v",
					depth, r)
			}
		}()

		// Build nested array structure
		var value any = "leaf"
		for i := 0; i < depth; i++ {
			value = []any{value}
		}

		result, err := pkghttp.ValidateMetadataValue(value)

		// Verify: depth > 10 should be rejected (maxDepth = 10)
		if depth > 10 && err == nil {
			t.Errorf("ValidateMetadataValue accepted nested depth=%d (limit is 10)", depth)
		}

		// Verify: depth <= 10 with valid content should succeed
		if depth <= 10 && err != nil {
			t.Errorf("ValidateMetadataValue rejected valid nested depth=%d err=%v", depth, err)
		}

		_ = result
	})
}

// TODO(review): Consider rune-based truncation for multi-byte UTF-8
// truncateMetaStr safely truncates a string for logging
func truncateMetaStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
