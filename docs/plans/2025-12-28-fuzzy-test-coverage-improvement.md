# Fuzzy Test Coverage Improvement Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Expand fuzz test coverage to critical validation functions that currently lack fuzzing, ensuring financial calculations, datetime parsing, and input validation are resilient to malformed inputs.

**Architecture:** Add new fuzz test files to the existing `/tests/fuzzy/` directory following established patterns. Each test uses `github.com/google/gofuzz` for diverse input generation combined with Go's native fuzzing framework (`testing.F`). Tests focus on panic prevention and contract verification.

**Tech Stack:**
- Go 1.24+ with native fuzzing support
- `github.com/google/gofuzz` for custom seed generation
- `github.com/shopspring/decimal` for precision-safe arithmetic
- `gopkg.in/go-playground/validator.v9` (target validation library)

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.24+
- Tools: Go toolchain with fuzzing support
- Access: Local file system access to `/Users/fredamaral/repos/lerianstudio/midaz`
- State: Working on branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version              # Expected: go1.24+ or higher
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy/... -run=^$ -list='.*' | head -10  # Expected: list of fuzz tests
git status              # Expected: clean working tree or known changes
```

## Historical Precedent

**Query:** "fuzz test coverage validation fuzzy"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach based on existing fuzz test patterns in the codebase.

---

## Gap Analysis

### Already Covered (DO NOT DUPLICATE):
- CPF/CNPJ validation: `crm_holder_fuzz_test.go` (local helper, consider enhancing)
- Balance operations: `balance_operations_fuzz_test.go`
- Alias parsing: `alias_fuzz_test.go`
- Query parameters: `query_params_fuzz_test.go`
- Gold DSL parsing: `gold_dsl_fuzz_test.go`
- Asset rate precision: `assetrate_precision_fuzz_test.go`

### Gaps to Fill (This Plan):
1. **TransactionDate JSON parsing** - 6 format fallbacks at `pkg/transaction/time.go:42-67`
2. **Metadata validation** - Recursive depth + value limits at `pkg/net/http/httputils.go:418-468`
3. **Country code validation** - ISO 3166-1 at `pkg/utils/utils.go:34-57`
4. **Currency code validation** - ISO 4217 at `pkg/utils/utils.go:95-113`
5. **Code validation** - Uppercase letters only at `pkg/utils/utils.go:82-92`
6. **Account/Asset type validation** - Enum validation at `pkg/utils/utils.go:60-79`

---

## Batch 1: DateTime and Metadata Validation

### Task 1.1: Create TransactionDate Fuzz Test File

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/transaction_date_fuzz_test.go`

**Prerequisites:**
- Directory exists: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/`
- Module: `github.com/LerianStudio/midaz/v3`

**Step 1: Create the transaction date fuzz test file**

```go
package fuzzy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// FuzzTransactionDateUnmarshalJSON fuzzes the TransactionDate JSON unmarshalling
// with various date formats and malformed inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzTransactionDateUnmarshalJSON -run=^$ -fuzztime=60s
func FuzzTransactionDateUnmarshalJSON(f *testing.F) {
	// Seed: valid RFC3339Nano formats
	f.Add(`"2024-01-15T14:30:45.123456789Z"`)
	f.Add(`"2024-12-31T23:59:59.999999999Z"`)
	f.Add(`"2024-01-01T00:00:00.000000001Z"`)

	// Seed: valid RFC3339 formats
	f.Add(`"2024-01-15T14:30:45Z"`)
	f.Add(`"2024-01-15T14:30:45+05:30"`)
	f.Add(`"2024-01-15T14:30:45-08:00"`)

	// Seed: valid milliseconds format
	f.Add(`"2024-01-15T14:30:45.000Z"`)
	f.Add(`"2024-01-15T14:30:45.123Z"`)
	f.Add(`"2024-01-15T14:30:45.999Z"`)

	// Seed: valid ISO 8601 without timezone
	f.Add(`"2024-01-15T14:30:45"`)
	f.Add(`"2024-12-31T00:00:00"`)

	// Seed: valid date-only format
	f.Add(`"2024-01-15"`)
	f.Add(`"2024-12-31"`)
	f.Add(`"2000-01-01"`)
	f.Add(`"1970-01-01"`)

	// Seed: null and empty
	f.Add(`null`)
	f.Add(`""`)
	f.Add(`"null"`)

	// Seed: boundary dates
	f.Add(`"0001-01-01T00:00:00Z"`)
	f.Add(`"9999-12-31T23:59:59Z"`)
	f.Add(`"1970-01-01T00:00:00Z"`)

	// Seed: invalid month/day combinations
	f.Add(`"2024-02-30T00:00:00Z"`) // Feb 30 doesn't exist
	f.Add(`"2024-04-31T00:00:00Z"`) // Apr has 30 days
	f.Add(`"2024-13-01T00:00:00Z"`) // Month 13
	f.Add(`"2024-00-01T00:00:00Z"`) // Month 0
	f.Add(`"2024-01-32T00:00:00Z"`) // Day 32

	// Seed: leap year edge cases
	f.Add(`"2024-02-29T00:00:00Z"`) // Valid leap year
	f.Add(`"2023-02-29T00:00:00Z"`) // Invalid non-leap year
	f.Add(`"2000-02-29T00:00:00Z"`) // Valid century leap year
	f.Add(`"1900-02-29T00:00:00Z"`) // Invalid century non-leap year

	// Seed: malformed JSON
	f.Add(`2024-01-15`)          // Missing quotes
	f.Add(`"2024-01-15`)         // Missing end quote
	f.Add(`2024-01-15"`)         // Missing start quote
	f.Add(`{"date": "2024-01-15"}`) // Object instead of string

	// Seed: wrong separators
	f.Add(`"2024/01/15T14:30:45Z"`)
	f.Add(`"2024.01.15T14:30:45Z"`)
	f.Add(`"2024-01-15 14:30:45Z"`)
	f.Add(`"2024-01-15T14.30.45Z"`)

	// Seed: timezone edge cases
	f.Add(`"2024-01-15T14:30:45+14:00"`)  // Max positive offset
	f.Add(`"2024-01-15T14:30:45-12:00"`)  // Max negative offset
	f.Add(`"2024-01-15T14:30:45+00:00"`)  // UTC explicit
	f.Add(`"2024-01-15T14:30:45+25:00"`)  // Invalid offset

	// Seed: precision edge cases
	f.Add(`"2024-01-15T14:30:45.1Z"`)
	f.Add(`"2024-01-15T14:30:45.12Z"`)
	f.Add(`"2024-01-15T14:30:45.123Z"`)
	f.Add(`"2024-01-15T14:30:45.1234567890Z"`) // Too many digits

	// Seed: injection patterns
	f.Add(`"2024-01-15'; DROP TABLE--"`)
	f.Add(`"2024-01-15<script>alert(1)</script>"`)
	f.Add(`"2024-01-15${env.SECRET}"`)

	// Seed: unicode and control characters
	f.Add(`"2024-01-15T14:30:45Z` + "\x00" + `"`)
	f.Add(`"2024-01-15T14:30:45Z` + "\u200B" + `"`)
	f.Add(`"` + "\u202E" + `2024-01-15"`) // Right-to-left override

	// Seed: extremely long values
	f.Add(`"` + strings.Repeat("2024-01-15", 1000) + `"`)

	f.Fuzz(func(t *testing.T, jsonInput string) {
		// Should NEVER panic regardless of input
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("TransactionDate.UnmarshalJSON panicked on input: %q panic=%v",
					truncateDateStr(jsonInput, 100), r)
			}
		}()

		var td transaction.TransactionDate
		err := json.Unmarshal([]byte(jsonInput), &td)

		// If parsing succeeded, verify the result is usable
		if err == nil {
			// Calling Time() should never panic
			_ = td.Time()

			// Calling IsZero() should never panic
			_ = td.IsZero()

			// Re-marshaling should not panic
			_, marshalErr := json.Marshal(td)
			if marshalErr != nil {
				t.Logf("Marshal failed after successful unmarshal: input=%q err=%v",
					truncateDateStr(jsonInput, 50), marshalErr)
			}
		}
	})
}

// FuzzTransactionDateMarshalJSON fuzzes the TransactionDate JSON marshalling
// with various time values to ensure consistent output.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzTransactionDateMarshalJSON -run=^$ -fuzztime=30s
func FuzzTransactionDateMarshalJSON(f *testing.F) {
	// Seed: epoch timestamps
	f.Add(int64(0))                    // Unix epoch
	f.Add(int64(1704067200))           // 2024-01-01 00:00:00 UTC
	f.Add(int64(1735689599))           // 2024-12-31 23:59:59 UTC
	f.Add(int64(-62135596800))         // 0001-01-01 00:00:00 UTC
	f.Add(int64(253402300799))         // 9999-12-31 23:59:59 UTC

	// Seed: boundary values
	f.Add(int64(-9223372036854775808)) // min int64
	f.Add(int64(9223372036854775807))  // max int64

	// Seed: nanosecond variations
	f.Add(int64(1704067200) + 1)       // With nanoseconds
	f.Add(int64(1704067200) + 999999999)

	f.Fuzz(func(t *testing.T, unixSeconds int64) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("TransactionDate.MarshalJSON panicked on unix=%d panic=%v",
					unixSeconds, r)
			}
		}()

		// Create TransactionDate from time
		tm := time.Unix(unixSeconds, 0).UTC()
		td := transaction.TransactionDate(tm)

		// Marshal should not panic
		data, err := json.Marshal(td)

		// If marshaling succeeded, the output should be valid JSON
		if err == nil && len(data) > 0 {
			// Verify it's valid JSON string
			var checkStr string
			if jsonErr := json.Unmarshal(data, &checkStr); jsonErr != nil && string(data) != "null" {
				t.Logf("MarshalJSON produced invalid JSON string: unix=%d output=%s",
					unixSeconds, string(data))
			}
		}
	})
}

// truncateDateStr safely truncates a string for logging
func truncateDateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/fuzzy/...`

**Expected output:**
```
(no output - successful compilation)
```

**If you see errors:** Check import paths match module name `github.com/LerianStudio/midaz/v3`

**Step 3: Run the fuzz test briefly to verify it works**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy -fuzz=FuzzTransactionDateUnmarshalJSON -run=^$ -fuzztime=5s`

**Expected output:**
```
=== FUZZ  FuzzTransactionDateUnmarshalJSON
fuzz: elapsed: 0s, gathering baseline coverage: 0/XX completed
fuzz: elapsed: Xs, execs: NNNN (XXX/sec), new interesting: N (total: N)
--- PASS: FuzzTransactionDateUnmarshalJSON (5.XXs)
PASS
```

**Step 4: Commit the new test file**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add tests/fuzzy/transaction_date_fuzz_test.go && git commit -m "test(fuzzy): add TransactionDate JSON parsing fuzz tests

Add comprehensive fuzz tests for TransactionDate unmarshal/marshal:
- 6 date format fallbacks (RFC3339Nano, RFC3339, milliseconds, etc.)
- Boundary dates, leap years, timezone edge cases
- Malformed JSON, injection patterns, unicode handling
- Marshal roundtrip verification"
```

**If Task Fails:**

1. **Import errors:**
   - Check: `go mod tidy` to update dependencies
   - Fix: Verify package path `github.com/LerianStudio/midaz/v3/pkg/transaction`

2. **Test fails immediately:**
   - Run: `go test -v ./tests/fuzzy -run=TestMain` (verify test setup)
   - Check: `tests/fuzzy/main_test.go` for auth requirements

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

### Task 1.2: Create Metadata Validation Fuzz Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/metadata_validation_fuzz_test.go`

**Prerequisites:**
- Task 1.1 completed successfully
- Module compiles without errors

**Step 1: Create the metadata validation fuzz test file**

```go
package fuzzy

import (
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
	f.Add(float64(9223372036854775807))  // max int64 as float
	f.Add(float64(-9223372036854775808)) // min int64 as float
	f.Add(float64(1.7976931348623157e+308)) // max float64
	f.Add(float64(-1.7976931348623157e+308)) // min float64
	f.Add(float64(5e-324)) // smallest positive float64

	// Seed: special float values
	f.Add(float64(0.1))
	f.Add(float64(0.01))
	f.Add(float64(0.001))
	f.Add(float64(0.0000001))

	// Seed: precision edge cases (2^53 boundary)
	f.Add(float64(9007199254740992))
	f.Add(float64(9007199254740993))

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
				if floatResult != value {
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
			t.Logf("ValidateMetadataValue rejected valid nested depth=%d err=%v", depth, err)
		}

		_ = result
	})
}

// truncateMetaStr safely truncates a string for logging
func truncateMetaStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/fuzzy/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run the fuzz tests briefly**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy -fuzz=FuzzValidateMetadataValue -run=^$ -fuzztime=5s`

**Expected output:**
```
=== FUZZ  FuzzValidateMetadataValue
fuzz: elapsed: 0s, gathering baseline coverage...
fuzz: elapsed: Xs, execs: NNNN (XXX/sec)...
--- PASS: FuzzValidateMetadataValue (5.XXs)
PASS
```

**Step 4: Commit the new test file**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add tests/fuzzy/metadata_validation_fuzz_test.go && git commit -m "test(fuzzy): add metadata validation fuzz tests

Add fuzz tests for ValidateMetadataValue covering:
- String length limits (2000 char max)
- Numeric type handling (float64, int)
- Array validation with various sizes
- Nested depth rejection (max depth 10)
- Unicode, injection patterns, control characters"
```

**If Task Fails:**

1. **Import path error for pkghttp:**
   - Check: The package is at `github.com/LerianStudio/midaz/v3/pkg/net/http`
   - Fix: Verify the import alias `pkghttp` doesn't conflict

2. **Function signature mismatch:**
   - Run: `go doc github.com/LerianStudio/midaz/v3/pkg/net/http.ValidateMetadataValue`
   - Adapt: Adjust function call to match actual signature

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

### Code Review Checkpoint 1

**After completing Tasks 1.1-1.2:**

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
   - Wait for all to complete

2. **Handle findings by severity:**
   - **Critical/High/Medium Issues:** Fix immediately, re-run reviewers
   - **Low Issues:** Add `TODO(review):` comments at relevant locations
   - **Cosmetic/Nitpick:** Add `FIXME(nitpick):` comments

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low/Cosmetic issues have appropriate comments added

---

## Batch 2: ISO Code Validation

### Task 2.1: Create Country Code Validation Fuzz Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/iso_codes_fuzz_test.go`

**Prerequisites:**
- Batch 1 completed and reviewed
- All previous tests compile and pass

**Step 1: Create the ISO codes fuzz test file**

```go
package fuzzy

import (
	"strings"
	"testing"
	"unicode"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// FuzzValidateCountryAddress fuzzes ISO 3166-1 alpha-2 country code validation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateCountryAddress -run=^$ -fuzztime=60s
func FuzzValidateCountryAddress(f *testing.F) {
	// Seed: valid country codes (sample from 250+)
	validCodes := []string{
		"US", "GB", "DE", "FR", "JP", "CN", "BR", "IN", "RU", "AU",
		"CA", "MX", "AR", "ZA", "EG", "NG", "KE", "AE", "SA", "IL",
		"KR", "TW", "SG", "MY", "TH", "VN", "ID", "PH", "NZ", "CH",
		"AT", "BE", "NL", "SE", "NO", "DK", "FI", "PL", "CZ", "PT",
	}
	for _, code := range validCodes {
		f.Add(code)
	}

	// Seed: lowercase versions (should fail)
	f.Add("us")
	f.Add("gb")
	f.Add("de")

	// Seed: mixed case (should fail)
	f.Add("Us")
	f.Add("uS")
	f.Add("Gb")

	// Seed: invalid codes (not in ISO 3166-1)
	f.Add("XX")
	f.Add("ZZ")
	f.Add("AA")
	f.Add("QQ")

	// Seed: wrong length
	f.Add("")
	f.Add("U")
	f.Add("USA")
	f.Add("USAA")
	f.Add("U S")

	// Seed: numeric codes (ISO 3166-1 numeric are different)
	f.Add("00")
	f.Add("01")
	f.Add("99")
	f.Add("840") // USA numeric code

	// Seed: special characters
	f.Add("U$")
	f.Add("G!")
	f.Add("D.")
	f.Add(" US")
	f.Add("US ")
	f.Add("U\x00S")
	f.Add("U\nS")

	// Seed: unicode homoglyphs (Cyrillic А looks like Latin A)
	f.Add("U\u0421") // Cyrillic С
	f.Add("\u0410E") // Cyrillic А + Latin E

	// Seed: reserved/user-assigned codes
	f.Add("XA") // User-assigned
	f.Add("XZ") // User-assigned
	f.Add("QM") // User-assigned range

	// Seed: exceptionally reserved codes
	f.Add("UK") // Not ISO 3166-1 (use GB)
	f.Add("EU") // Not a country

	// Seed: injection patterns
	f.Add("'; DROP TABLE--")
	f.Add("US<script>")

	// Seed: long strings
	f.Add(strings.Repeat("A", 100))
	f.Add(strings.Repeat("US", 100))

	f.Fuzz(func(t *testing.T, code string) {
		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateCountryAddress panicked on input=%q panic=%v",
					truncateISOStr(code, 50), r)
			}
		}()

		err := utils.ValidateCountryAddress(code)

		// Cross-validate: 2-letter uppercase should match certain codes
		if len(code) == 2 && isUpperAlpha(code) {
			// If it's a valid format, log whether it was accepted
			if err != nil {
				// Not in the list - expected for XX, ZZ, etc.
				_ = err
			}
		}

		// Verify: non-2-letter codes should always fail
		if len(code) != 2 && err == nil {
			t.Errorf("ValidateCountryAddress accepted code with len=%d: %q", len(code), code)
		}

		// Verify: lowercase should always fail
		if len(code) == 2 && hasLowercase(code) && err == nil {
			t.Errorf("ValidateCountryAddress accepted lowercase code: %q", code)
		}
	})
}

// FuzzValidateCurrency fuzzes ISO 4217 currency code validation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateCurrency -run=^$ -fuzztime=60s
func FuzzValidateCurrency(f *testing.F) {
	// Seed: valid currency codes (sample from 170+)
	validCurrencies := []string{
		"USD", "EUR", "GBP", "JPY", "CNY", "BRL", "INR", "RUB", "AUD", "CAD",
		"CHF", "SEK", "NOK", "DKK", "NZD", "SGD", "HKD", "KRW", "MXN", "ZAR",
		"AED", "SAR", "ILS", "TRY", "PLN", "CZK", "HUF", "THB", "MYR", "IDR",
	}
	for _, code := range validCurrencies {
		f.Add(code)
	}

	// Seed: lowercase versions (should fail)
	f.Add("usd")
	f.Add("eur")
	f.Add("gbp")

	// Seed: mixed case (should fail)
	f.Add("Usd")
	f.Add("uSd")
	f.Add("usD")

	// Seed: invalid codes (not in ISO 4217)
	f.Add("XXX") // Used for testing but may not be in list
	f.Add("XYZ")
	f.Add("AAA")
	f.Add("ZZZ")

	// Seed: wrong length
	f.Add("")
	f.Add("US")
	f.Add("USDD")
	f.Add("U")

	// Seed: numeric codes (ISO 4217 has numeric, this validates alpha)
	f.Add("840")
	f.Add("978")
	f.Add("000")

	// Seed: special characters
	f.Add("US$")
	f.Add("US ")
	f.Add(" USD")
	f.Add("USD ")
	f.Add("U$D")

	// Seed: historic/obsolete codes
	f.Add("DEM") // Deutsche Mark (obsolete)
	f.Add("FRF") // French Franc (obsolete)
	f.Add("ITL") // Italian Lira (obsolete)

	// Seed: supranational currencies
	f.Add("XDR") // Special Drawing Rights
	f.Add("XAU") // Gold
	f.Add("XAG") // Silver
	f.Add("XPT") // Platinum

	// Seed: crypto/non-standard (should fail)
	f.Add("BTC")
	f.Add("ETH")
	f.Add("XRP")

	// Seed: injection patterns
	f.Add("US'; DROP--")

	// Seed: long strings
	f.Add(strings.Repeat("USD", 100))

	f.Fuzz(func(t *testing.T, code string) {
		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateCurrency panicked on input=%q panic=%v",
					truncateISOStr(code, 50), r)
			}
		}()

		err := utils.ValidateCurrency(code)

		// Verify: non-3-letter codes should fail
		if len(code) != 3 && err == nil {
			t.Errorf("ValidateCurrency accepted code with len=%d: %q", len(code), code)
		}

		// Verify: lowercase should fail
		if len(code) == 3 && hasLowercase(code) && err == nil {
			t.Errorf("ValidateCurrency accepted lowercase code: %q", code)
		}

		// Verify: non-letter characters should fail
		if hasNonLetter(code) && err == nil {
			t.Errorf("ValidateCurrency accepted code with non-letters: %q", code)
		}
	})
}

// FuzzValidateCode fuzzes the generic code validation (uppercase letters only).
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateCode -run=^$ -fuzztime=30s
func FuzzValidateCode(f *testing.F) {
	// Seed: valid codes
	f.Add("ABC")
	f.Add("USD")
	f.Add("A")
	f.Add("ABCDEFGHIJKLMNOPQRSTUVWXYZ")

	// Seed: lowercase (should fail)
	f.Add("abc")
	f.Add("Abc")
	f.Add("aBc")
	f.Add("abC")

	// Seed: with numbers (should fail)
	f.Add("ABC123")
	f.Add("123ABC")
	f.Add("A1B2C3")
	f.Add("123")

	// Seed: with special characters (should fail)
	f.Add("ABC!")
	f.Add("A-B-C")
	f.Add("A_B_C")
	f.Add("A.B.C")
	f.Add("A B C")

	// Seed: empty and whitespace
	f.Add("")
	f.Add(" ")
	f.Add("   ")
	f.Add("\t")
	f.Add("\n")

	// Seed: unicode letters (non-ASCII)
	f.Add("ÄÖÜ")    // German umlauts
	f.Add("ΑΒΓΔ")   // Greek uppercase
	f.Add("АБВГ")   // Cyrillic uppercase
	f.Add("日本語")  // CJK

	// Seed: homoglyphs
	f.Add("АВС")    // Cyrillic looks like ABC

	// Seed: control characters
	f.Add("ABC\x00")
	f.Add("\x00ABC")
	f.Add("A\x00BC")

	f.Fuzz(func(t *testing.T, code string) {
		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateCode panicked on input=%q panic=%v",
					truncateISOStr(code, 50), r)
			}
		}()

		err := utils.ValidateCode(code)

		// Verify: empty should fail
		if code == "" && err == nil {
			t.Errorf("ValidateCode accepted empty string")
		}

		// Verify: lowercase ASCII should fail
		if hasASCIILowercase(code) && err == nil {
			t.Errorf("ValidateCode accepted lowercase: %q", code)
		}

		// Verify: numbers should fail
		if hasDigit(code) && err == nil {
			t.Errorf("ValidateCode accepted digits: %q", code)
		}

		// Verify: pure uppercase ASCII should succeed
		if isPureUpperASCII(code) && len(code) > 0 && err != nil {
			t.Errorf("ValidateCode rejected valid uppercase ASCII: %q err=%v", code, err)
		}
	})
}

// FuzzValidateAccountType fuzzes account type enum validation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateAccountType -run=^$ -fuzztime=30s
func FuzzValidateAccountType(f *testing.F) {
	// Seed: valid types
	f.Add("deposit")
	f.Add("savings")
	f.Add("loans")
	f.Add("marketplace")
	f.Add("creditCard")

	// Seed: case variations (should fail)
	f.Add("DEPOSIT")
	f.Add("Deposit")
	f.Add("SAVINGS")
	f.Add("Savings")
	f.Add("LOANS")
	f.Add("CREDITCARD")
	f.Add("CreditCard")
	f.Add("credit_card")
	f.Add("credit-card")

	// Seed: similar but invalid
	f.Add("checking")
	f.Add("account")
	f.Add("investment")
	f.Add("retirement")
	f.Add("money_market")

	// Seed: empty and whitespace
	f.Add("")
	f.Add(" ")
	f.Add("deposit ")
	f.Add(" deposit")

	// Seed: injection
	f.Add("deposit'; DROP--")

	f.Fuzz(func(t *testing.T, accountType string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateAccountType panicked on input=%q panic=%v",
					truncateISOStr(accountType, 50), r)
			}
		}()

		err := utils.ValidateAccountType(accountType)

		// Verify: valid types should succeed
		validTypes := []string{"deposit", "savings", "loans", "marketplace", "creditCard"}
		isValid := false
		for _, vt := range validTypes {
			if accountType == vt {
				isValid = true
				break
			}
		}

		if isValid && err != nil {
			t.Errorf("ValidateAccountType rejected valid type=%q err=%v", accountType, err)
		}

		if !isValid && err == nil {
			t.Errorf("ValidateAccountType accepted invalid type=%q", accountType)
		}
	})
}

// FuzzValidateType fuzzes asset type enum validation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidateType -run=^$ -fuzztime=30s
func FuzzValidateType(f *testing.F) {
	// Seed: valid types
	f.Add("crypto")
	f.Add("currency")
	f.Add("commodity")
	f.Add("others")

	// Seed: case variations (should fail)
	f.Add("CRYPTO")
	f.Add("Crypto")
	f.Add("CURRENCY")
	f.Add("Currency")
	f.Add("COMMODITY")
	f.Add("OTHERS")

	// Seed: similar but invalid
	f.Add("stock")
	f.Add("bond")
	f.Add("etf")
	f.Add("fund")
	f.Add("fiat")
	f.Add("token")

	// Seed: empty and whitespace
	f.Add("")
	f.Add(" ")

	f.Fuzz(func(t *testing.T, assetType string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateType panicked on input=%q panic=%v",
					truncateISOStr(assetType, 50), r)
			}
		}()

		err := utils.ValidateType(assetType)

		// Verify: valid types should succeed
		validTypes := []string{"crypto", "currency", "commodity", "others"}
		isValid := false
		for _, vt := range validTypes {
			if assetType == vt {
				isValid = true
				break
			}
		}

		if isValid && err != nil {
			t.Errorf("ValidateType rejected valid type=%q err=%v", assetType, err)
		}

		if !isValid && err == nil {
			t.Errorf("ValidateType accepted invalid type=%q", assetType)
		}
	})
}

// Helper functions for validation checks

func isUpperAlpha(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return len(s) > 0
}

func hasLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

func hasASCIILowercase(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
}

func hasNonLetter(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func isPureUpperASCII(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func truncateISOStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/fuzzy/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run the fuzz tests briefly**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy -fuzz=FuzzValidateCountryAddress -run=^$ -fuzztime=5s`

**Expected output:**
```
=== FUZZ  FuzzValidateCountryAddress
fuzz: elapsed: Xs, execs: NNNN...
--- PASS: FuzzValidateCountryAddress (5.XXs)
PASS
```

**Step 4: Commit the new test file**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add tests/fuzzy/iso_codes_fuzz_test.go && git commit -m "test(fuzzy): add ISO code validation fuzz tests

Add comprehensive fuzz tests for:
- ValidateCountryAddress: ISO 3166-1 alpha-2 (250+ countries)
- ValidateCurrency: ISO 4217 alpha-3 (170+ currencies)
- ValidateCode: Uppercase letter enforcement
- ValidateAccountType: Enum validation (deposit, savings, etc.)
- ValidateType: Asset type enum (crypto, currency, commodity, others)

Coverage includes:
- Case sensitivity, homoglyphs, unicode
- Injection patterns, control characters
- Boundary values, historic codes"
```

**If Task Fails:**

1. **Import errors:**
   - Check: `go mod tidy`
   - Verify: Package path `github.com/LerianStudio/midaz/v3/pkg/utils`

2. **Function not exported:**
   - Check: Functions in `utils.go` start with uppercase
   - Fix: The functions are `ValidateCountryAddress`, `ValidateCurrency`, etc.

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

### Code Review Checkpoint 2

**After completing Task 2.1:**

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously
   - Wait for all to complete

2. **Handle findings by severity:**
   - **Critical/High/Medium Issues:** Fix immediately, re-run reviewers
   - **Low Issues:** Add `TODO(review):` comments
   - **Cosmetic/Nitpick:** Add `FIXME(nitpick):` comments

3. **Proceed only when:** Zero Critical/High/Medium issues remain

---

## Batch 3: Integration and Verification

### Task 3.1: Run All New Fuzz Tests

**Files:**
- No new files created
- Verify: All tests in `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/`

**Prerequisites:**
- All previous tasks completed
- All code review issues resolved

**Step 1: List all fuzz tests to verify coverage**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy/... -run=^$ -list='Fuzz.*' | sort`

**Expected output:** (should include new tests)
```
FuzzAssetRateFloat64Boundaries
FuzzAssetRatePrecisionLoss
FuzzCNPJValidation
FuzzCPFValidation
FuzzCalculateTotal
FuzzFromToSplitAlias
FuzzOperateBalances
FuzzSplitAliasWithKey
FuzzTransactionDateMarshalJSON
FuzzTransactionDateUnmarshalJSON
FuzzValidateAccountType
FuzzValidateCode
FuzzValidateCountryAddress
FuzzValidateCurrency
FuzzValidateMetadataValue
FuzzValidateMetadataValueArray
FuzzValidateMetadataValueNested
FuzzValidateMetadataValueNumeric
FuzzValidateType
...
```

**Step 2: Run all fuzz tests briefly to verify no panics**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy/... -fuzztime=2s -run='^$' -fuzz='.*' 2>&1 | head -100`

**Expected output:**
```
=== FUZZ  FuzzXxx
fuzz: elapsed: ...
--- PASS: FuzzXxx (2.XXs)
...
```

**If any test panics:** Stop and investigate the panic - it indicates a bug in the target code.

**Step 3: Run extended fuzz session on critical tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy -fuzz=FuzzTransactionDateUnmarshalJSON -run=^$ -fuzztime=30s`

**Expected output:**
```
=== FUZZ  FuzzTransactionDateUnmarshalJSON
fuzz: elapsed: 30s, execs: NNNNN (XXX/sec), new interesting: N (total: N)
--- PASS: FuzzTransactionDateUnmarshalJSON (30.XXs)
PASS
```

**Step 4: Generate coverage report for new tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/fuzzy/... -coverprofile=coverage_fuzzy.out && go tool cover -func=coverage_fuzzy.out | grep -E '(transaction/time|pkg/net/http|pkg/utils)'`

**Expected output:**
```
github.com/LerianStudio/midaz/v3/pkg/transaction/time.go:XX.X%
github.com/LerianStudio/midaz/v3/pkg/net/http/httputils.go:XX.X%
github.com/LerianStudio/midaz/v3/pkg/utils/utils.go:XX.X%
```

**If Task Fails:**

1. **Tests don't run:**
   - Check: `go test -v ./tests/fuzzy/...` (without fuzz flags)
   - Fix: Ensure TestMain is present and auth is configured

2. **Coverage report empty:**
   - Run: `go test -v ./tests/fuzzy/... -coverprofile=coverage.out` (simpler)
   - Check: Coverage output file exists

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

### Task 3.2: Final Commit with All New Tests

**Prerequisites:**
- All tests pass
- All code reviews complete

**Step 1: Check git status**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git status`

**Expected output:**
```
On branch fix/fred-several-ones-dec-13-2025
nothing to commit, working tree clean
```

(If there are uncommitted changes, they should be the test files we created)

**Step 2: View git log to see commits**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git log --oneline -5`

**Expected output:**
```
XXXXXXX test(fuzzy): add ISO code validation fuzz tests
XXXXXXX test(fuzzy): add metadata validation fuzz tests
XXXXXXX test(fuzzy): add TransactionDate JSON parsing fuzz tests
...
```

---

## Failure Recovery

### General Recovery Steps

1. **Test won't compile:**
   - Check: `go mod tidy`
   - Check: Import paths match module `github.com/LerianStudio/midaz/v3`
   - Run: `go build ./tests/fuzzy/...` for detailed errors

2. **Test panics:**
   - This is a SUCCESS - you found a bug!
   - Document: Exact input that caused panic
   - Report: Create issue with reproduction steps

3. **Function signature changed:**
   - Run: `go doc <package>.<Function>` to see current signature
   - Update: Test to match new signature

4. **Coverage too low:**
   - Add: More seed values for uncovered paths
   - Check: `go test -v ./tests/fuzzy/... -coverprofile=c.out && go tool cover -html=c.out`

5. **CI/CD failures:**
   - Check: Tests run in isolation
   - Fix: Remove any external dependencies (network, file system)

---

## Summary

This plan adds **8 new fuzz tests** across 3 files:

1. **transaction_date_fuzz_test.go** (2 tests)
   - `FuzzTransactionDateUnmarshalJSON` - 6 format fallbacks
   - `FuzzTransactionDateMarshalJSON` - roundtrip verification

2. **metadata_validation_fuzz_test.go** (4 tests)
   - `FuzzValidateMetadataValue` - string validation
   - `FuzzValidateMetadataValueNumeric` - numeric handling
   - `FuzzValidateMetadataValueArray` - array sizes
   - `FuzzValidateMetadataValueNested` - depth limits

3. **iso_codes_fuzz_test.go** (5 tests)
   - `FuzzValidateCountryAddress` - ISO 3166-1 alpha-2
   - `FuzzValidateCurrency` - ISO 4217
   - `FuzzValidateCode` - uppercase letters
   - `FuzzValidateAccountType` - enum validation
   - `FuzzValidateType` - asset type enum

**Total: 11 new fuzz test functions covering TIER 1 and TIER 2 priorities.**

---

## Execution Options

After reviewing this plan, choose one:

1. **Subagent-Driven (this session)** - Fresh subagent per task, review between tasks, fast iteration

2. **Parallel Session (separate)** - Open new session in worktree, batch execution with checkpoints
