// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package document implements the Brazilian tax-identifier check-digit rules —
// CPF for individuals (11 digits), CNPJ for companies (14 digits) — over the
// free-form document field a holder carries.
//
// It is deliberately shape-driven rather than type-driven. A holder document is
// not necessarily Brazilian: a passport, a national ID or a foreign tax number is
// a legitimate value with no Modulo-11 rule to apply. So Classify decides FIRST
// whether the value is even shaped like a CPF or a CNPJ, and only then is the
// check-digit rule meaningful. Everything else is Unrecognized and no arithmetic
// claim is made about it.
//
// This package is the single home of the weight tables. Adding a second copy of
// the Modulo-11 rule elsewhere in the repo is how the two halves of a platform
// end up disagreeing about the same field.
package document

import "strings"

// Shape is what a document value looks like, decided from its digits alone.
type Shape int

const (
	// ShapeUnrecognized is any value that is not shaped like a CPF or a CNPJ:
	// a different digit count, or a value carrying characters that are neither
	// digits nor the conventional CPF/CNPJ separators. No check-digit rule
	// applies to it.
	ShapeUnrecognized Shape = iota

	// ShapeCPF is the 11-digit individual taxpayer identifier.
	ShapeCPF

	// ShapeCNPJ is the 14-digit company taxpayer identifier.
	ShapeCNPJ
)

// String renders a Shape for diagnostics.
func (s Shape) String() string {
	switch s {
	case ShapeCPF:
		return "CPF"
	case ShapeCNPJ:
		return "CNPJ"
	default:
		return "UNRECOGNIZED"
	}
}

const (
	cpfLength  = 11
	cnpjLength = 14
)

// cnpjWeights1 and cnpjWeights2 are the CNPJ Modulo-11 weight tables. Unlike the
// CPF's, they are not a plain descending run: they restart at 9 after 2.
var (
	cnpjWeights1 = [12]int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	cnpjWeights2 = [13]int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
)

// Classify reports whether s is shaped like a CPF, like a CNPJ, or like neither.
// Only the digit count decides; the conventional separators of a formatted
// document (000.000.000-00 / 00.000.000/0000-00) are ignored, and any other
// character makes the value Unrecognized.
func Classify(s string) Shape {
	digits, ok := digitsOnly(s)
	if !ok {
		return ShapeUnrecognized
	}

	switch len(digits) {
	case cpfLength:
		return ShapeCPF
	case cnpjLength:
		return ShapeCNPJ
	default:
		return ShapeUnrecognized
	}
}

// IsValidCPF reports whether s is a valid CPF: 11 digits, not a repeated-digit
// sequence, and both Modulo-11 check digits agreeing with the number.
//
// A value that is not CPF-shaped returns false — use Classify first when the
// question is "should this value be check-digit tested at all".
func IsValidCPF(s string) bool {
	digits, ok := digitsOnly(s)
	if !ok || len(digits) != cpfLength || allSameDigit(digits) {
		return false
	}

	// The repeated-digit guard above is load-bearing, not decorative: all ten
	// sequences (00000000000 .. 99999999999) satisfy the arithmetic below.
	first := checkDigit(descendingWeightedSum(digits[:9], 10))
	second := checkDigit(descendingWeightedSum(digits[:10], 11))

	return digits[9] == byteDigit(first) && digits[10] == byteDigit(second)
}

// IsValidCNPJ reports whether s is a valid CNPJ: 14 digits, not a repeated-digit
// sequence, and both Modulo-11 check digits agreeing with the number.
//
// A value that is not CNPJ-shaped returns false — use Classify first when the
// question is "should this value be check-digit tested at all".
func IsValidCNPJ(s string) bool {
	digits, ok := digitsOnly(s)
	if !ok || len(digits) != cnpjLength || allSameDigit(digits) {
		return false
	}

	first := checkDigit(tableWeightedSum(digits[:12], cnpjWeights1[:]))
	second := checkDigit(tableWeightedSum(digits[:13], cnpjWeights2[:]))

	return digits[12] == byteDigit(first) && digits[13] == byteDigit(second)
}

// digitsOnly strips the conventional CPF/CNPJ separators and returns the digit
// run. ok is false when s carries any other character, which is what keeps a
// passport number or an alphanumeric registry ID out of the check-digit rule.
func digitsOnly(s string) (string, bool) {
	var b strings.Builder

	b.Grow(len(s))

	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '/' || r == ' ':
			// Conventional separator in a formatted document; carries no value.
		default:
			return "", false
		}
	}

	return b.String(), true
}

// allSameDigit reports whether every digit is the same one. Such sequences are
// not issuable documents even when the arithmetic accepts them.
func allSameDigit(digits string) bool {
	for i := 1; i < len(digits); i++ {
		if digits[i] != digits[0] {
			return false
		}
	}

	return len(digits) > 0
}

// descendingWeightedSum multiplies each digit by a weight counting down from
// firstWeight — the CPF form of the Modulo-11 sum.
func descendingWeightedSum(digits string, firstWeight int) int {
	sum := 0
	for i := 0; i < len(digits); i++ {
		sum += int(digits[i]-'0') * (firstWeight - i)
	}

	return sum
}

// tableWeightedSum multiplies each digit by its positional weight — the CNPJ
// form of the Modulo-11 sum, whose weights are not a plain descending run.
func tableWeightedSum(digits string, weights []int) int {
	sum := 0
	for i := 0; i < len(digits); i++ {
		sum += int(digits[i]-'0') * weights[i]
	}

	return sum
}

// checkDigit turns a Modulo-11 weighted sum into its check digit: the remainder
// complements to 11, and a remainder below 2 yields 0.
func checkDigit(weightedSum int) int {
	remainder := weightedSum % 11
	if remainder < 2 {
		return 0
	}

	return 11 - remainder
}

// byteDigit renders a single check digit as its ASCII character.
func byteDigit(d int) byte {
	return byte('0' + d)
}
