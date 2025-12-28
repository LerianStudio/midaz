package property

import (
	"math/rand"
	"testing"
	"testing/quick"
	"unicode"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// Property: Valid asset codes contain only uppercase letters
func TestProperty_AssetCodeUppercaseOnly_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate random uppercase code (valid)
		validCode := generateUppercaseCode(rng, 3)
		err := utils.ValidateCode(validCode)
		if err != nil {
			t.Logf("Valid code rejected: %s err=%v", validCode, err)
			return false
		}

		// Generate code with lowercase (invalid)
		invalidCode := generateMixedCaseCode(rng, 3)
		err = utils.ValidateCode(invalidCode)
		if err == nil {
			// Check if it's actually all uppercase (edge case)
			allUpper := true
			for _, r := range invalidCode {
				if unicode.IsLetter(r) && !unicode.IsUpper(r) {
					allUpper = false
					break
				}
			}
			if !allUpper {
				t.Logf("Mixed case code accepted: %s", invalidCode)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset code uppercase property failed: %v", err)
	}
}

func generateUppercaseCode(rng *rand.Rand, length int) string {
	code := make([]byte, length)
	for i := range code {
		code[i] = byte('A' + rng.Intn(26))
	}
	return string(code)
}

func generateMixedCaseCode(rng *rand.Rand, length int) string {
	code := make([]byte, length)
	for i := range code {
		if rng.Intn(2) == 0 {
			code[i] = byte('A' + rng.Intn(26))
		} else {
			code[i] = byte('a' + rng.Intn(26))
		}
	}
	return string(code)
}

// Property: Asset codes must not contain digits or special characters
func TestProperty_AssetCodeNoDigitsOrSpecial_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate code with digit (invalid)
		codeWithDigit := generateUppercaseCode(rng, 2) + string(byte('0'+rng.Intn(10)))
		err := utils.ValidateCode(codeWithDigit)
		if err == nil {
			t.Logf("Code with digit accepted: %s", codeWithDigit)
			return false
		}

		// Generate code with special char (invalid)
		specialChars := []byte{'@', '#', '$', '-', '_', '.', '!'}
		codeWithSpecial := generateUppercaseCode(rng, 2) + string(specialChars[rng.Intn(len(specialChars))])
		err = utils.ValidateCode(codeWithSpecial)
		if err == nil {
			t.Logf("Code with special char accepted: %s", codeWithSpecial)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset code no digits/special property failed: %v", err)
	}
}

// Property: Only valid asset types are accepted (crypto, currency, commodity, others)
func TestProperty_AssetTypeValidation_Model(t *testing.T) {
	validTypes := []string{"crypto", "currency", "commodity", "others"}
	invalidTypes := []string{"stock", "bond", "CRYPTO", "Currency", "invalid", "", "other"}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Valid types should be accepted
		validType := validTypes[rng.Intn(len(validTypes))]
		err := utils.ValidateType(validType)
		if err != nil {
			t.Logf("Valid type rejected: %s err=%v", validType, err)
			return false
		}

		// Invalid types should be rejected
		invalidType := invalidTypes[rng.Intn(len(invalidTypes))]
		err = utils.ValidateType(invalidType)
		if err == nil {
			t.Logf("Invalid type accepted: %s", invalidType)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset type validation property failed: %v", err)
	}
}

// Property: Currency type requires ISO 4217 compliant code
func TestProperty_CurrencyCodeISO4217_Model(t *testing.T) {
	validCurrencies := []string{"USD", "EUR", "BRL", "JPY", "GBP", "CHF", "CAD", "AUD"}
	invalidCurrencies := []string{"XXX", "ABC", "US", "EURO", "dollar", ""}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Valid currencies should be accepted
		validCurrency := validCurrencies[rng.Intn(len(validCurrencies))]
		err := utils.ValidateCurrency(validCurrency)
		if err != nil {
			t.Logf("Valid currency rejected: %s err=%v", validCurrency, err)
			return false
		}

		// Invalid currencies should be rejected
		invalidCurrency := invalidCurrencies[rng.Intn(len(invalidCurrencies))]
		err = utils.ValidateCurrency(invalidCurrency)
		if err == nil {
			t.Logf("Invalid currency accepted: %s", invalidCurrency)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Currency code ISO 4217 property failed: %v", err)
	}
}
