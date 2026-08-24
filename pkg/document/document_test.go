// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package document

import "testing"

// TestClassify pins the shape decision — the half of the rule that decides
// WHETHER a check-digit test applies at all. A value that is neither a CPF nor a
// CNPJ shape is Unrecognized and is deliberately never check-digit tested.
func TestClassify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Shape
	}{
		{"eleven digits is a CPF", "12345678909", ShapeCPF},
		{"fourteen digits is a CNPJ", "11222333000181", ShapeCNPJ},
		{"punctuated CPF is a CPF", "123.456.789-09", ShapeCPF},
		{"punctuated CNPJ is a CNPJ", "11.222.333/0001-81", ShapeCNPJ},
		{"spaces are separators too", "123 456 789 09", ShapeCPF},
		{"ten digits is neither", "1234567890", ShapeUnrecognized},
		{"twelve digits is neither", "123456789012", ShapeUnrecognized},
		{"thirteen digits is neither", "1234567890123", ShapeUnrecognized},
		{"fifteen digits is neither", "123456789012345", ShapeUnrecognized},
		{"a passport number is neither", "AB1234567", ShapeUnrecognized},
		{"eleven chars with a letter is neither", "1234567890A", ShapeUnrecognized},
		{"fourteen chars with a letter is neither", "12ABC34567DE89", ShapeUnrecognized},
		// A letter is NOT a separator: stripping it would turn this into the valid
		// CPF 12345678909 and let an unrecognised value be check-digit tested.
		{"a letter inside an otherwise-valid CPF is neither", "1234567890X9", ShapeUnrecognized},
		{"a letter inside an otherwise-valid CNPJ is neither", "11222333000X181", ShapeUnrecognized},
		{"empty is neither", "", ShapeUnrecognized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.input); got != tt.want {
				t.Fatalf("Classify(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsValidCPF covers the Modulo-11 rule for the 11-digit shape, including the
// negative controls (valid CPFs must keep passing) and the repeated-digit
// sequences, which satisfy the arithmetic but are not issuable numbers.
func TestIsValidCPF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Negative controls: these must NOT be refused.
		{"valid CPF", "12345678909", true},
		{"valid CPF, second vector", "52998224725", true},
		{"valid CPF, third vector", "11144477735", true},
		{"valid CPF, punctuated", "123.456.789-09", true},
		{"valid CPF, spaced", "123 456 789 09", true},

		// Wrong first check digit (the shape the incident produced).
		{"first check digit wrong", "12345678919", false},
		// Wrong second check digit only.
		{"second check digit wrong", "12345678900", false},

		// Repeated-digit sequences: every one of these satisfies the Modulo-11
		// arithmetic, so only the explicit sequence guard refuses them.
		{"all zeros", "00000000000", false},
		{"all ones", "11111111111", false},
		{"all twos", "22222222222", false},
		{"all threes", "33333333333", false},
		{"all fours", "44444444444", false},
		{"all fives", "55555555555", false},
		{"all sixes", "66666666666", false},
		{"all sevens", "77777777777", false},
		{"all eights", "88888888888", false},
		{"all nines", "99999999999", false},

		// Wrong shapes are not CPFs.
		{"too short", "1234567890", false},
		{"too long", "123456789099", false},
		{"contains a letter", "1234567890A", false},
		// Stripping the letter would yield the valid CPF 12345678909; a letter is
		// not a separator, so this value is not a CPF at all.
		{"letter inside an otherwise-valid CPF", "1234567890X9", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidCPF(tt.input); got != tt.want {
				t.Fatalf("IsValidCPF(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsValidCNPJ covers the Modulo-11 rule for the 14-digit shape. Note that,
// unlike CPF, only the all-zeros sequence satisfies the CNPJ arithmetic; the
// other repeated sequences are refused by the check digits themselves. The guard
// is asserted on the one case where it is load-bearing.
func TestIsValidCNPJ(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Negative controls: these must NOT be refused.
		{"valid CNPJ", "11222333000181", true},
		{"valid CNPJ, second vector", "11444777000161", true},
		{"valid CNPJ, punctuated", "11.222.333/0001-81", true},

		{"first check digit wrong", "11222333000101", false},
		{"second check digit wrong", "11222333000180", false},

		// All zeros satisfies the arithmetic; only the sequence guard refuses it.
		{"all zeros", "00000000000000", false},
		{"all ones", "11111111111111", false},
		{"all nines", "99999999999999", false},

		{"too short", "1122233300018", false},
		{"too long", "112223330001811", false},
		{"contains a letter", "1122233300018A", false},
		// Stripping the letter would yield the valid CNPJ 11222333000181.
		{"letter inside an otherwise-valid CNPJ", "11222333000X181", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidCNPJ(tt.input); got != tt.want {
				t.Fatalf("IsValidCNPJ(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestShapeCrossTalk locks the two rules apart: a valid CPF is not a valid CNPJ
// and vice versa, so neither function can be swapped for the other and still pass.
func TestShapeCrossTalk(t *testing.T) {
	if IsValidCNPJ("12345678909") {
		t.Fatal("IsValidCNPJ accepted an 11-digit CPF")
	}

	if IsValidCPF("11222333000181") {
		t.Fatal("IsValidCPF accepted a 14-digit CNPJ")
	}
}
