package fuzzy

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v3/pkg/transaction"
	fuzz "github.com/google/gofuzz"
)

// FuzzSplitAliasWithKey tests alias parsing with gofuzz-generated diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzSplitAliasWithKey -run=^$ -fuzztime=30s
func FuzzSplitAliasWithKey(f *testing.F) {
	// Use gofuzz to generate diverse alias strings
	fuzzer := fuzz.New().NilChance(0).Funcs(
		func(s *string, c fuzz.Continue) {
			// Generate alias-like strings
			prefixes := []string{"@", "", "0#", "1#", "123#"}
			names := []string{"person1", "account", "user", "test", ""}
			keys := []string{"#default", "#balance-key", "#freeze", "#", "##", ""}

			prefix := prefixes[c.Intn(len(prefixes))]
			name := names[c.Intn(len(names))]
			key := keys[c.Intn(len(keys))]

			if c.RandBool() {
				*s = prefix + name + key
			} else {
				// Random string
				c.Fuzz(s)
			}
		},
	)

	// Generate 20 diverse seeds using gofuzz
	for i := 0; i < 20; i++ {
		var alias string
		fuzzer.Fuzz(&alias)
		if utf8.ValidString(alias) {
			f.Add(alias)
		}
	}

	// Manual seeds: normal aliases
	f.Add("@person1#default")
	f.Add("@account#balance-key")
	f.Add("0#@person1#default")
	f.Add("1#@account#freeze")

	// Manual seeds: without hash
	f.Add("@person1")
	f.Add("account-alias")

	// Manual seeds: multiple hashes
	f.Add("@person1#key1#key2")
	f.Add("###")
	f.Add("a#b#c#d#e")

	// Manual seeds: empty and whitespace
	f.Add("")
	f.Add("#")
	f.Add("##")
	f.Add(" # ")
	f.Add("\t#\n")

	// Manual seeds: unicode
	f.Add("@user#key")
	f.Add("@user\u200B#key") // zero-width space

	// Manual seeds: special characters
	f.Add("@user#key with spaces")
	f.Add("@user#key\twith\ttabs")
	f.Add("@user#key\nwith\nnewlines")

	// Manual seeds: long inputs
	f.Add("@" + strings.Repeat("a", 1000) + "#" + strings.Repeat("b", 1000))

	// Manual seeds: injection patterns
	f.Add("@user#'; DROP TABLE--")
	f.Add("@user#{{.Exec}}")
	f.Add("@user#../../../etc/passwd")

	f.Fuzz(func(t *testing.T, alias string) {
		// Skip invalid UTF-8
		if !utf8.ValidString(alias) {
			return
		}

		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SplitAliasWithKey panicked on input: %q, panic=%v",
					truncateStringAlias(alias, 100), r)
			}
		}()

		// Call the function
		result := transaction.SplitAliasWithKey(alias)

		// Verify contract: if input contains #, result should be substring after first #
		if idx := strings.Index(alias, "#"); idx != -1 {
			expected := alias[idx+1:]
			if result != expected {
				t.Errorf("SplitAliasWithKey(%q) = %q, want %q", alias, result, expected)
			}
		} else {
			// If no #, result should be the input unchanged
			if result != alias {
				t.Errorf("SplitAliasWithKey(%q) = %q, want %q (unchanged)", alias, result, alias)
			}
		}
	})
}

// FuzzFromToSplitAlias tests the FromTo.SplitAlias method with gofuzz-generated inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzFromToSplitAlias -run=^$ -fuzztime=30s
func FuzzFromToSplitAlias(f *testing.F) {
	// Use gofuzz to generate diverse FromTo struct AccountAlias values
	fuzzer := fuzz.New().NilChance(0).Funcs(
		func(s *string, c fuzz.Continue) {
			// Generate indexed alias patterns
			indices := []string{"0#", "1#", "123#", ""}
			aliases := []string{"@person1", "@account", "user", ""}

			if c.RandBool() {
				idx := indices[c.Intn(len(indices))]
				alias := aliases[c.Intn(len(aliases))]
				*s = idx + alias
			} else {
				c.Fuzz(s)
			}
		},
	)

	// Generate 20 diverse seeds using gofuzz
	for i := 0; i < 20; i++ {
		var alias string
		fuzzer.Fuzz(&alias)
		if utf8.ValidString(alias) {
			f.Add(alias)
		}
	}

	// Manual seeds: indexed aliases (format: index#alias)
	f.Add("0#@person1")
	f.Add("1#@account")
	f.Add("123#@user")

	// Manual seeds: without index prefix
	f.Add("@person1")
	f.Add("account")

	// Manual seeds: edge cases
	f.Add("")
	f.Add("#")
	f.Add("##")
	f.Add("0#")
	f.Add("#@alias")

	f.Fuzz(func(t *testing.T, accountAlias string) {
		// Skip invalid UTF-8
		if !utf8.ValidString(accountAlias) {
			return
		}

		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("FromTo.SplitAlias panicked on AccountAlias=%q, panic=%v",
					truncateStringAlias(accountAlias, 100), r)
			}
		}()

		ft := transaction.FromTo{
			AccountAlias: accountAlias,
		}

		result := ft.SplitAlias()

		// Verify: if contains #, should return element at index 1 from Split
		// Note: actual implementation uses strings.Split()[1], NOT strings.SplitN(..., 2)[1]
		if strings.Contains(accountAlias, "#") {
			parts := strings.Split(accountAlias, "#")
			expected := parts[1]
			if result != expected {
				t.Errorf("FromTo{AccountAlias: %q}.SplitAlias() = %q, want %q",
					accountAlias, result, expected)
			}
		} else {
			if result != accountAlias {
				t.Errorf("FromTo{AccountAlias: %q}.SplitAlias() = %q, want %q (unchanged)",
					accountAlias, result, accountAlias)
			}
		}
	})
}

// truncateStringAlias safely truncates a string to maxLen characters
func truncateStringAlias(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
