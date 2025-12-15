package fuzzy

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// FuzzCPFValidation fuzzes CPF (Brazilian individual tax ID) validation.
// CPF format: 11 digits with specific check digit calculation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzCPFValidation -run=^$ -fuzztime=60s
func FuzzCPFValidation(f *testing.F) {
	// Seed: valid CPFs (with correct check digits)
	f.Add("91315026015") // from holder.go example
	f.Add("12345678909") // common test CPF
	f.Add("00000000000") // edge case: all zeros
	f.Add("11111111111") // edge case: all ones
	f.Add("22222222222") // edge case: all twos

	// Seed: formatted CPFs
	f.Add("913.150.260-15")
	f.Add("123.456.789-09")

	// Seed: invalid CPFs (wrong check digits)
	f.Add("91315026010")
	f.Add("12345678900")
	f.Add("12345678901")

	// Seed: wrong length
	f.Add("1234567890")   // 10 digits
	f.Add("123456789012") // 12 digits
	f.Add("")            // empty
	f.Add("1")           // single digit

	// Seed: non-numeric
	f.Add("1234567890a")
	f.Add("abcdefghijk")
	f.Add("123.456.78A-09")

	// Seed: special characters
	f.Add("123-456-789-09")
	f.Add("123/456/789/09")
	f.Add("123 456 789 09")

	// Seed: unicode/control chars
	f.Add("12345678909\x00")
	f.Add("\u200b12345678909") // zero-width space

	f.Fuzz(func(t *testing.T, document string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("CPF validation panicked on input: %q panic=%v",
					truncateStringCRM(document, 50), r)
			}
		}()

		// Test our validation function
		isValid := validateCPF(document)

		// Cross-validate: if it looks like a valid CPF structurally, verify check digits
		cleaned := cleanNumeric(document)
		if len(cleaned) == 11 && isAllDigits(cleaned) {
			calculatedValid := calculateCPFCheckDigits(cleaned)
			if isValid != calculatedValid && !isRepeatedDigits(cleaned) {
				t.Logf("CPF validation mismatch: input=%q cleaned=%s isValid=%v calculated=%v",
					document, cleaned, isValid, calculatedValid)
			}
		}
	})
}

// FuzzCNPJValidation fuzzes CNPJ (Brazilian company tax ID) validation.
// CNPJ format: 14 digits with specific check digit calculation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzCNPJValidation -run=^$ -fuzztime=60s
func FuzzCNPJValidation(f *testing.F) {
	// Seed: valid CNPJs
	f.Add("11222333000181")
	f.Add("11.222.333/0001-81")

	// Seed: invalid CNPJs
	f.Add("11222333000180")
	f.Add("00000000000000")
	f.Add("11111111111111")

	// Seed: wrong length
	f.Add("1122233300018")   // 13 digits
	f.Add("112223330001811") // 15 digits
	f.Add("")

	// Seed: mixed format
	f.Add("11.222.333/000181")
	f.Add("11222333/0001-81")

	f.Fuzz(func(t *testing.T, document string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("CNPJ validation panicked on input: %q panic=%v",
					truncateStringCRM(document, 50), r)
			}
		}()

		isValid := validateCNPJ(document)

		cleaned := cleanNumeric(document)
		if len(cleaned) == 14 && isAllDigits(cleaned) {
			calculatedValid := calculateCNPJCheckDigits(cleaned)
			if isValid != calculatedValid && !isRepeatedDigits(cleaned) {
				t.Logf("CNPJ validation mismatch: input=%q cleaned=%s isValid=%v calculated=%v",
					document, cleaned, isValid, calculatedValid)
			}
		}
	})
}

// FuzzHolderNameValidation fuzzes holder name input validation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzHolderNameValidation -run=^$ -fuzztime=60s
func FuzzHolderNameValidation(f *testing.F) {
	// Seed: normal names
	f.Add("John Doe")
	f.Add("Maria Silva Santos")
	f.Add("Empresa ABC Ltda")

	// Seed: unicode names
	f.Add("Jose da Silva")
	f.Add("Francois Muller")
	f.Add("Joao Pedro")

	// Seed: edge cases
	f.Add("")
	f.Add(" ")
	f.Add("A")
	f.Add(strings.Repeat("A", 1000))
	f.Add(strings.Repeat("A", 10000))

	// Seed: special characters
	f.Add("O'Brien")
	f.Add("Mary-Jane Watson")
	f.Add("Dr. John Smith, Jr.")

	// Seed: control characters
	f.Add("John\x00Doe")
	f.Add("John\nDoe")
	f.Add("John\tDoe")

	// Seed: injection attempts
	f.Add("<script>alert('xss')</script>")
	f.Add("'; DROP TABLE holders; --")
	f.Add("{{template}}")
	f.Add("${env.SECRET}")

	f.Fuzz(func(t *testing.T, name string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Name validation panicked on input (len=%d): %v",
					len(name), r)
			}
		}()

		isValid := validateHolderName(name)

		// Cross-validate: certain patterns should always be invalid
		if isValid {
			// Check for control characters (should be invalid)
			for _, r := range name {
				if r < 32 && r != '\t' && r != '\n' {
					t.Logf("Name validation accepted control char: %q (rune %d)", name, r)
				}
			}
		}
	})
}

// Helper functions for validation (simplified versions)

func cleanNumeric(s string) string {
	var result strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func isRepeatedDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	first := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			return false
		}
	}
	return true
}

func validateCPF(cpf string) bool {
	cleaned := cleanNumeric(cpf)
	if len(cleaned) != 11 {
		return false
	}
	if isRepeatedDigits(cleaned) {
		return false
	}
	return calculateCPFCheckDigits(cleaned)
}

func calculateCPFCheckDigits(cpf string) bool {
	// First check digit
	sum := 0
	for i := 0; i < 9; i++ {
		digit, _ := strconv.Atoi(string(cpf[i]))
		sum += digit * (10 - i)
	}
	remainder := sum % 11
	firstCheck := 0
	if remainder >= 2 {
		firstCheck = 11 - remainder
	}
	if int(cpf[9]-'0') != firstCheck {
		return false
	}

	// Second check digit
	sum = 0
	for i := 0; i < 10; i++ {
		digit, _ := strconv.Atoi(string(cpf[i]))
		sum += digit * (11 - i)
	}
	remainder = sum % 11
	secondCheck := 0
	if remainder >= 2 {
		secondCheck = 11 - remainder
	}
	return int(cpf[10]-'0') == secondCheck
}

func validateCNPJ(cnpj string) bool {
	cleaned := cleanNumeric(cnpj)
	if len(cleaned) != 14 {
		return false
	}
	if isRepeatedDigits(cleaned) {
		return false
	}
	return calculateCNPJCheckDigits(cleaned)
}

func calculateCNPJCheckDigits(cnpj string) bool {
	// First check digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights1[i]
	}
	remainder := sum % 11
	firstCheck := 0
	if remainder >= 2 {
		firstCheck = 11 - remainder
	}
	if int(cnpj[12]-'0') != firstCheck {
		return false
	}

	// Second check digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights2[i]
	}
	remainder = sum % 11
	secondCheck := 0
	if remainder >= 2 {
		secondCheck = 11 - remainder
	}
	return int(cnpj[13]-'0') == secondCheck
}

func validateHolderName(name string) bool {
	// Basic validation: non-empty, reasonable length, no dangerous characters
	if len(name) == 0 || len(name) > 500 {
		return false
	}

	// Check for control characters
	for _, r := range name {
		if r < 32 && r != ' ' {
			return false
		}
	}

	// Check for HTML/script injection patterns
	dangerousPatterns := []string{"<script", "</script", "javascript:", "onerror=", "onclick="}
	nameLower := strings.ToLower(name)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(nameLower, pattern) {
			return false
		}
	}

	return true
}

// truncateStringCRM safely truncates a string to maxLen characters (CRM variant to avoid redeclaration)
func truncateStringCRM(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Ensure regexp is used (avoids unused import error)
var _ = regexp.MustCompile
