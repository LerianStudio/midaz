package fuzzy

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
)

// FuzzGoldDSLParser fuzzes the Gold DSL parser with malformed and edge-case inputs.
// Uses correct s-expression syntax as defined in pkg/gold/Transaction.g4
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzGoldDSLParser -run=^$ -fuzztime=60s
func FuzzGoldDSLParser(f *testing.F) {
	// Seed corpus: valid DSL examples using correct s-expression syntax
	f.Add(`(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 10000|2 (source (from @source-account :amount USD 10000|2)) (distribute (to @dest-account :amount USD 10000|2))))`)

	// Seed: minimal valid transaction
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100|0 (source (from @a :amount USD 100|0)) (distribute (to @b :amount USD 100|0))))`)

	// Seed: with metadata
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (metadata (key1 value1)) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Seed: with description
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (description "test transaction") (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Seed: with code
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (code tx-001) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Seed: with pending flag
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (pending true) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Seed: share-based distribution
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100|2 (source (from @a :share 100)) (distribute (to @b :share 100))))`)

	// Seed: remaining distribution
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100|2 (source (from @a :remaining)) (distribute (to @b :remaining))))`)

	// Seed: share int of int
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100|2 (source (from @a :share 50 :of 100)) (distribute (to @b :share 50 :of 100))))`)

	// Seed: transaction-template variant
	f.Add(`(transaction-template V1 (chart-of-accounts-group-name @id) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Edge case seeds: potential overflow/underflow
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 9223372036854775807|0 (source (from @a :amount USD 9223372036854775807|0)) (distribute (to @b :amount USD 9223372036854775807|0))))`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 9223372036854775808|0 (source (from @a :amount USD 9223372036854775808|0)) (distribute (to @b :amount USD 9223372036854775808|0))))`)

	// Edge case: huge scale values
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 1|2147483647 (source (from @a :amount USD 1|2147483647)) (distribute (to @b :amount USD 1|2147483647))))`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 1|99 (source (from @a :amount USD 1|99)) (distribute (to @b :amount USD 1|99))))`)

	// Edge case: negative values (should fail gracefully)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD -100|2 (source (from @a :amount USD -100|2)) (distribute (to @b :amount USD -100|2))))`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100|-1 (source (from @a :amount USD 100|-1)) (distribute (to @b :amount USD 100|-1))))`)

	// Edge case: zero values
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 0|0 (source (from @a :amount USD 0|0)) (distribute (to @b :amount USD 0|0))))`)

	// Edge case: escape sequences in strings
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (description "test\nwith\nnewlines") (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (description "test\"with\"quotes") (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Edge case: unicode in identifiers
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id-with-é) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Edge case: variable syntax
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100|2 (source (from $account-var :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Malformed: incomplete structure
	f.Add(`(transaction V1`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (send USD 100`)

	// Malformed: missing required parts
	f.Add(`(transaction V1 (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2)))`)

	// Malformed: empty strings
	f.Add("")
	f.Add("   ")
	f.Add("\t\n")

	// Malformed: binary/null bytes
	f.Add("(transaction\x00V1)")
	f.Add("(transaction V1 (chart-of-accounts-group-name @id\x00))")

	// Malformed: extremely long identifiers
	f.Add(`(transaction V1 (chart-of-accounts-group-name @` + strings.Repeat("a", 10000) + `) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Malformed: deeply nested (if parser has recursion limits)
	f.Add(`(transaction V1 (metadata (` + strings.Repeat("k v ", 1000) + `)) (chart-of-accounts-group-name @id) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Security: Path traversal patterns
	f.Add(`(transaction V1 (chart-of-accounts-group-name @../../../etc/passwd) (send USD 100|2 (source (from @../admin :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Security: SQL injection patterns in metadata
	f.Add(`(transaction V1 (chart-of-accounts-group-name @id) (metadata (key1 "'; DROP TABLE--")) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))`)

	// Security: Unicode zero-width space
	f.Add("(transaction V1 (chart-of-accounts-group-name @id\u200B) (send USD 100|2 (source (from @a :amount USD 100|2)) (distribute (to @b :amount USD 100|2))))")

	f.Fuzz(func(t *testing.T, dsl string) {
		// Skip completely invalid UTF-8 early (reduces noise)
		if !utf8.ValidString(dsl) {
			return
		}

		// The parser should NEVER panic - it should return an error or valid result
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked on input (len=%d): %v\nInput snippet: %q",
					len(dsl), r, truncateStringRune(dsl, 200))
			}
		}()

		// Call the parser - we don't care about the result, only that it doesn't panic
		_ = transaction.Parse(dsl)
	})
}

// FuzzGoldDSLNumericBounds specifically targets numeric parsing edge cases.
// Uses correct s-expression syntax with value|scale format.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzGoldDSLNumericBounds -run=^$ -fuzztime=60s
func FuzzGoldDSLNumericBounds(f *testing.F) {
	// Seed: normal values
	f.Add("100", "2")
	f.Add("1000000", "0")
	f.Add("999999999999", "6")

	// Seed: int64 boundary
	f.Add("9223372036854775807", "0")  // max int64
	f.Add("9223372036854775808", "0")  // max int64 + 1 (overflow)
	f.Add("-9223372036854775808", "0") // min int64
	f.Add("-9223372036854775809", "0") // min int64 - 1 (underflow)

	// Seed: int32 boundary (for scale)
	f.Add("100", "2147483647")  // max int32 scale
	f.Add("100", "2147483648")  // max int32 + 1
	f.Add("100", "-2147483648") // min int32
	f.Add("100", "-2147483649") // min int32 - 1

	// Seed: extreme decimal precision
	f.Add("1", "100")
	f.Add("1", "1000")
	f.Add("1"+strings.Repeat("0", 100), "0") // 100+ digits

	// Seed: scientific notation (if parser supports)
	f.Add("1e18", "0")
	f.Add("1e-18", "0")
	f.Add("1E308", "0")

	// Seed: floating point edge cases
	f.Add("0.1", "0")
	f.Add("0.01", "0")
	f.Add("0.001", "0")

	// Seed: leading zeros
	f.Add("000100", "2")
	f.Add("0", "0")
	f.Add("00000", "0")

	// Seed: whitespace in numbers
	f.Add(" 100", "2")
	f.Add("100 ", "2")
	f.Add("1 00", "2")

	// Seed: special characters
	f.Add("+100", "2")
	f.Add("++100", "2")
	f.Add("--100", "2")

	f.Fuzz(func(t *testing.T, value string, scale string) {
		// Build DSL string with fuzzed numeric values using correct syntax
		dsl := fmt.Sprintf(
			`(transaction V1 (chart-of-accounts-group-name @id) (send USD %s|%s (source (from @a :amount USD %s|%s)) (distribute (to @b :amount USD %s|%s))))`,
			value, scale, value, scale, value, scale,
		)

		// The parser should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked on numeric input: value=%q scale=%q panic=%v",
					truncateStringRune(value, 50), truncateStringRune(scale, 20), r)
			}
		}()

		_ = transaction.Parse(dsl)
	})
}

// truncateStringRune safely truncates a string to maxLen runes (not bytes)
// This properly handles multi-byte UTF-8 characters.
func truncateStringRune(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
