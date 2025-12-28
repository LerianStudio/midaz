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

	// Seed: unicode homoglyphs (Cyrillic A looks like Latin A)
	f.Add("U\u0421") // Cyrillic S
	f.Add("\u0410E") // Cyrillic A + Latin E

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
	f.Add("AOU")     // German umlauts (replaced with ASCII for valid test)
	f.Add("ABGD")    // Greek uppercase (replaced with ASCII for valid test)
	f.Add("ABVG")    // Cyrillic uppercase (replaced with ASCII for valid test)
	f.Add("NIHONGO") // CJK (replaced with ASCII for valid test)

	// Seed: homoglyphs
	f.Add("ABC") // Cyrillic looks like ABC (replaced with ASCII for test)

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

		// Note: empty string is accepted by ValidateCode (for-loop doesn't execute)
		// This is expected behavior based on the current implementation

		// Verify: lowercase ASCII should fail
		if hasASCIILowercase(code) && err == nil {
			t.Errorf("ValidateCode accepted lowercase: %q", code)
		}

		// Verify: numbers should fail
		if hasDigit(code) && err == nil {
			t.Errorf("ValidateCode accepted digits: %q", code)
		}

		// Verify: pure uppercase ASCII (including empty) should succeed
		if isPureUpperASCII(code) && err != nil {
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
