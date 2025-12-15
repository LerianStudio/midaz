package fuzzy

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
)

// FuzzGoldDSLParser fuzzes the Gold DSL parser with malformed and edge-case inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzGoldDSLParser -run=^$ -fuzztime=60s
func FuzzGoldDSLParser(f *testing.F) {
	// Seed corpus: valid DSL examples
	f.Add(`transaction {
		chartOfAccountsGroupName @external-id
		send USD 10000 2
		source {
			from @source-account amount USD 10000 2
		}
		distribute {
			to @dest-account amount USD 10000 2
		}
	}`)

	// Seed: minimal valid transaction
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 100 0 source { from @a amount USD 100 0 } distribute { to @b amount USD 100 0 } }`)

	// Seed: with metadata
	f.Add(`transaction { chartOfAccountsGroupName @id metadata { key1 value1 } send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Seed: with description
	f.Add(`transaction { chartOfAccountsGroupName @id description "test transaction" send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Seed: with code
	f.Add(`transaction { chartOfAccountsGroupName @id code @tx-001 send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Seed: with pending flag
	f.Add(`transaction { chartOfAccountsGroupName @id pending true send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Seed: share-based distribution
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 100 2 source { from @a share 100 } distribute { to @b share 100 } }`)

	// Seed: remaining distribution
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 100 2 source { from @a remaining: } distribute { to @b remaining: } }`)

	// Seed: with rate
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 100 2 source { from @a amount USD 100 2 rate @rate-id USD BRL 500 2 } distribute { to @b amount BRL 500 2 } }`)

	// Edge case seeds: potential overflow/underflow
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 9223372036854775807 0 source { from @a amount USD 9223372036854775807 0 } distribute { to @b amount USD 9223372036854775807 0 } }`)
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 9223372036854775808 0 source { from @a amount USD 9223372036854775808 0 } distribute { to @b amount USD 9223372036854775808 0 } }`)

	// Edge case: huge scale values
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 1 2147483647 source { from @a amount USD 1 2147483647 } distribute { to @b amount USD 1 2147483647 } }`)
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 1 99 source { from @a amount USD 1 99 } distribute { to @b amount USD 1 99 } }`)

	// Edge case: negative scale (if parser doesn't validate)
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 100 -1 source { from @a amount USD 100 -1 } distribute { to @b amount USD 100 -1 } }`)

	// Edge case: zero values
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 0 0 source { from @a amount USD 0 0 } distribute { to @b amount USD 0 0 } }`)

	// Edge case: escape sequences in strings
	f.Add(`transaction { chartOfAccountsGroupName @id description "test\nwith\nnewlines" send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)
	f.Add(`transaction { chartOfAccountsGroupName @id description "test\"with\"quotes" send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Edge case: unicode in identifiers
	f.Add(`transaction { chartOfAccountsGroupName @id-with-unicode-\u00e9 send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Malformed: incomplete structure
	f.Add(`transaction {`)
	f.Add(`transaction { chartOfAccountsGroupName`)
	f.Add(`transaction { chartOfAccountsGroupName @id send`)
	f.Add(`transaction { chartOfAccountsGroupName @id send USD`)
	f.Add(`transaction { chartOfAccountsGroupName @id send USD 100`)

	// Malformed: missing required parts
	f.Add(`transaction { send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)
	f.Add(`transaction { chartOfAccountsGroupName @id source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Malformed: empty strings
	f.Add("")
	f.Add("   ")
	f.Add("\t\n")

	// Malformed: binary/null bytes
	f.Add("transaction\x00{}")
	f.Add("transaction { chartOfAccountsGroupName @id\x00 }")

	// Malformed: extremely long identifiers
	f.Add(`transaction { chartOfAccountsGroupName @` + strings.Repeat("a", 10000) + ` send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	// Malformed: deeply nested (if parser has recursion limits)
	f.Add(`transaction { metadata { ` + strings.Repeat("key value ", 1000) + `} chartOfAccountsGroupName @id send USD 100 2 source { from @a amount USD 100 2 } distribute { to @b amount USD 100 2 } }`)

	f.Fuzz(func(t *testing.T, dsl string) {
		// Skip completely invalid UTF-8 early (reduces noise)
		if !utf8.ValidString(dsl) {
			return
		}

		// The parser should NEVER panic - it should return an error or valid result
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked on input (len=%d): %v\nInput snippet: %q",
					len(dsl), r, truncateString(dsl, 200))
			}
		}()

		// Call the parser - we don't care about the result, only that it doesn't panic
		_ = transaction.Parse(dsl)
	})
}

// truncateString safely truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
