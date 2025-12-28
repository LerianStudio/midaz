package property

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
)

const (
	cpfLength  = 11
	cnpjLength = 14
)

// generateValidCPF generates a valid CPF with correct check digits
func generateValidCPF(rng *rand.Rand) string {
	// Generate first 9 digits
	digits := make([]int, 11)
	for i := 0; i < 9; i++ {
		digits[i] = rng.Intn(10)
	}

	// Avoid all equal digits (invalid CPFs)
	allEqual := true
	for i := 1; i < 9; i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		digits[8] = (digits[8] + 1) % 10
	}

	// Calculate first check digit
	sum := 0
	for i := 0; i < 9; i++ {
		sum += digits[i] * (10 - i)
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	digits[9] = remainder

	// Calculate second check digit
	sum = 0
	for i := 0; i < 10; i++ {
		sum += digits[i] * (11 - i)
	}
	remainder = (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	digits[10] = remainder

	result := ""
	for _, d := range digits {
		result += fmt.Sprintf("%d", d)
	}
	return result
}

// generateValidCNPJ generates a valid CNPJ with correct check digits
func generateValidCNPJ(rng *rand.Rand) string {
	// Generate first 12 digits
	digits := make([]int, 14)
	for i := 0; i < 12; i++ {
		digits[i] = rng.Intn(10)
	}

	// Avoid all equal digits
	allEqual := true
	for i := 1; i < 12; i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		digits[11] = (digits[11] + 1) % 10
	}

	// Calculate first check digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += digits[i] * weights1[i]
	}
	remainder := sum % 11
	if remainder < 2 {
		digits[12] = 0
	} else {
		digits[12] = 11 - remainder
	}

	// Calculate second check digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		sum += digits[i] * weights2[i]
	}
	remainder = sum % 11
	if remainder < 2 {
		digits[13] = 0
	} else {
		digits[13] = 11 - remainder
	}

	result := ""
	for _, d := range digits {
		result += fmt.Sprintf("%d", d)
	}
	return result
}

// validateCPF checks if a CPF is valid (implements the same logic as the validator)
func validateCPF(cpf string) bool {
	if len(cpf) != cpfLength {
		return false
	}

	// Check for all equal digits
	allEqual := true
	for i := 1; i < len(cpf); i++ {
		if cpf[i] != cpf[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return false
	}

	// Check all characters are digits
	for _, c := range cpf {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Validate first check digit
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(cpf[i]-'0') * (10 - i)
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	if remainder != int(cpf[9]-'0') {
		return false
	}

	// Validate second check digit
	sum = 0
	for i := 0; i < 10; i++ {
		sum += int(cpf[i]-'0') * (11 - i)
	}
	remainder = (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	return remainder == int(cpf[10]-'0')
}

// validateCNPJ checks if a CNPJ is valid (implements the same logic as the validator)
func validateCNPJ(cnpj string) bool {
	if len(cnpj) != cnpjLength {
		return false
	}

	// Check for all equal digits
	allEqual := true
	for i := 1; i < len(cnpj); i++ {
		if cnpj[i] != cnpj[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return false
	}

	// Check all characters are digits
	for _, c := range cnpj {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Validate first check digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(cnpj[i]-'0') * weights1[i]
	}
	remainder := sum % 11
	expectedDigit := 0
	if remainder >= 2 {
		expectedDigit = 11 - remainder
	}
	if expectedDigit != int(cnpj[12]-'0') {
		return false
	}

	// Validate second check digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		sum += int(cnpj[i]-'0') * weights2[i]
	}
	remainder = sum % 11
	expectedDigit = 0
	if remainder >= 2 {
		expectedDigit = 11 - remainder
	}
	return expectedDigit == int(cnpj[13]-'0')
}

// Property: Generated valid CPFs pass validation
func TestProperty_ValidCPFPasses(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cpf := generateValidCPF(rng)

		if !validateCPF(cpf) {
			t.Logf("Generated valid CPF failed validation: %s", cpf)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Valid CPF property failed: %v", err)
	}
}

// Property: Generated valid CNPJs pass validation
func TestProperty_ValidCNPJPasses(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cnpj := generateValidCNPJ(rng)

		if !validateCNPJ(cnpj) {
			t.Logf("Generated valid CNPJ failed validation: %s", cnpj)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Valid CNPJ property failed: %v", err)
	}
}

// Property: CPFs with incorrect check digit fail validation
func TestProperty_InvalidCPFCheckDigitFails(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cpf := generateValidCPF(rng)

		// Corrupt the last check digit
		digits := []byte(cpf)
		original := digits[10]
		digits[10] = byte('0' + (int(original-'0')+1)%10)
		corruptedCPF := string(digits)

		if validateCPF(corruptedCPF) {
			t.Logf("Corrupted CPF passed validation: %s (original: %s)", corruptedCPF, cpf)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Invalid CPF check digit property failed: %v", err)
	}
}

// Property: CNPJs with incorrect check digit fail validation
func TestProperty_InvalidCNPJCheckDigitFails(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cnpj := generateValidCNPJ(rng)

		// Corrupt the last check digit
		digits := []byte(cnpj)
		original := digits[13]
		digits[13] = byte('0' + (int(original-'0')+1)%10)
		corruptedCNPJ := string(digits)

		if validateCNPJ(corruptedCNPJ) {
			t.Logf("Corrupted CNPJ passed validation: %s (original: %s)", corruptedCNPJ, cnpj)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Invalid CNPJ check digit property failed: %v", err)
	}
}

// Property: All-equal-digit documents are invalid
func TestProperty_AllEqualDigitsInvalid(t *testing.T) {
	for digit := '0'; digit <= '9'; digit++ {
		cpf := ""
		for i := 0; i < cpfLength; i++ {
			cpf += string(digit)
		}

		if validateCPF(cpf) {
			t.Errorf("All-equal CPF should be invalid: %s", cpf)
		}

		cnpj := ""
		for i := 0; i < cnpjLength; i++ {
			cnpj += string(digit)
		}

		if validateCNPJ(cnpj) {
			t.Errorf("All-equal CNPJ should be invalid: %s", cnpj)
		}
	}
}

// Property: Wrong length documents are invalid
func TestProperty_WrongLengthInvalid(t *testing.T) {
	wrongLengths := []int{0, 1, 5, 10, 12, 13, 15, 20}

	for _, length := range wrongLengths {
		doc := ""
		for i := 0; i < length; i++ {
			doc += "1"
		}

		if validateCPF(doc) && length != cpfLength {
			t.Errorf("Wrong length CPF should be invalid: length=%d", length)
		}

		if validateCNPJ(doc) && length != cnpjLength {
			t.Errorf("Wrong length CNPJ should be invalid: length=%d", length)
		}
	}
}

// Property: Non-digit characters make document invalid
func TestProperty_NonDigitCharactersInvalid(t *testing.T) {
	invalidChars := []rune{'a', 'Z', '-', '.', ' ', '/', '#'}

	for _, char := range invalidChars {
		// Create CPF with invalid char
		cpf := "1234567890" + string(char)
		if len(cpf) == cpfLength && validateCPF(cpf) {
			t.Errorf("CPF with non-digit should be invalid: %s", cpf)
		}

		// Create CNPJ with invalid char
		cnpj := "1234567890123" + string(char)
		if len(cnpj) == cnpjLength && validateCNPJ(cnpj) {
			t.Errorf("CNPJ with non-digit should be invalid: %s", cnpj)
		}
	}
}
