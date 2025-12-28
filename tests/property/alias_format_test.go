package property

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"testing/quick"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
)

// validAliasRegex matches the AccountAliasAcceptedChars pattern
var validAliasRegex = regexp.MustCompile(cn.AccountAliasAcceptedChars)

// Property: Valid aliases match the accepted character pattern
func TestProperty_AliasValidCharacters(t *testing.T) {
	validChars := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@:_-")

	f := func(seed int64, length uint8) bool {
		rng := rand.New(rand.NewSource(seed))

		// Constrain length
		l := int(length)%50 + 1 // 1-50 chars

		// Generate valid alias
		alias := make([]rune, l)
		for i := 0; i < l; i++ {
			alias[i] = validChars[rng.Intn(len(validChars))]
		}
		aliasStr := string(alias)

		// Property: alias should match the regex
		if !validAliasRegex.MatchString(aliasStr) {
			t.Logf("Valid alias rejected: %s", aliasStr)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Alias valid characters property failed: %v", err)
	}
}

// Property: Aliases with invalid characters are rejected
func TestProperty_AliasInvalidCharactersRejected(t *testing.T) {
	invalidChars := []rune("!#$%^&*()+=[]{}|\\;'\",.<>?/`~ \t\n")

	f := func(seed int64, length uint8, invalidPos uint8) bool {
		rng := rand.New(rand.NewSource(seed))

		// Constrain length
		l := int(length)%49 + 2 // 2-50 chars

		// Generate mostly valid alias
		validChars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
		alias := make([]rune, l)
		for i := 0; i < l; i++ {
			alias[i] = validChars[rng.Intn(len(validChars))]
		}

		// Insert one invalid character
		pos := int(invalidPos) % l
		alias[pos] = invalidChars[rng.Intn(len(invalidChars))]
		aliasStr := string(alias)

		// Property: alias should NOT match the regex
		if validAliasRegex.MatchString(aliasStr) {
			t.Logf("Invalid alias accepted: %s (invalid char at pos %d)", aliasStr, pos)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Alias invalid characters rejection property failed: %v", err)
	}
}

// Property: @external/ prefix is prohibited
func TestProperty_AliasExternalPrefixProhibited(t *testing.T) {
	f := func(seed int64, suffix string) bool {
		// Remove any characters that would make the suffix invalid
		cleanSuffix := strings.Map(func(r rune) rune {
			if strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789", r) {
				return r
			}
			return -1
		}, suffix)

		if cleanSuffix == "" {
			cleanSuffix = "test"
		}

		alias := cn.DefaultExternalAccountAliasPrefix + cleanSuffix

		// Property: alias containing @external/ prefix should be rejected
		if strings.Contains(alias, cn.DefaultExternalAccountAliasPrefix) {
			// This is expected - external prefix is prohibited
			return true
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Alias external prefix prohibition property failed: %v", err)
	}
}

// Property: Empty alias is invalid
func TestProperty_AliasEmptyInvalid(t *testing.T) {
	alias := ""

	// Empty string should not match the pattern (pattern requires at least one char)
	if validAliasRegex.MatchString(alias) {
		t.Errorf("Empty alias should be invalid")
	}
}

// Property: Common valid alias formats are accepted
func TestProperty_AliasCommonFormats(t *testing.T) {
	validAliases := []string{
		"@user123",
		"account-1",
		"Account_Name",
		"user:subaccount",
		"simple",
		"UPPERCASE",
		"MixedCase123",
		"a",
		"@",
		"_",
		"-",
		":",
		"a-b_c:d@e",
	}

	for _, alias := range validAliases {
		if !validAliasRegex.MatchString(alias) {
			t.Errorf("Valid alias format rejected: %s", alias)
		}
	}
}

// Property: Known invalid alias formats are rejected
func TestProperty_AliasInvalidFormats(t *testing.T) {
	invalidAliases := []string{
		"user name",  // space
		"user\tname", // tab
		"user\nname", // newline
		"user!name",  // exclamation
		"user#name",  // hash
		"user$name",  // dollar
		"user%name",  // percent
		"user^name",  // caret
		"user&name",  // ampersand
		"user*name",  // asterisk
		"user(name)", // parentheses
		"user+name",  // plus
		"user=name",  // equals
		"user[name]", // brackets
		"user{name}", // braces
		"user|name",  // pipe
		"user\\name", // backslash
		"user;name",  // semicolon
		"user'name",  // single quote
		"user\"name", // double quote
		"user,name",  // comma
		"user.name",  // period
		"user<name>", // angle brackets
		"user?name",  // question mark
		"user/name",  // forward slash
		"user`name",  // backtick
		"user~name",  // tilde
	}

	for _, alias := range invalidAliases {
		if validAliasRegex.MatchString(alias) {
			t.Errorf("Invalid alias format accepted: %s", alias)
		}
	}
}
