// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import "github.com/LerianStudio/midaz/v4/pkg/scheme"

// TransactionType is the payment scheme a transaction rides on: a free-form
// label in its canonical spelling (1 to 50 characters of A-Z, 0-9, _ or -).
// "scheme" is its public name; "transactionType" is the deprecated alias.
type TransactionType string

// Examples of common schemes. The set is open: any canonical value is valid.
const (
	TransactionTypeCard   TransactionType = "CARD"
	TransactionTypeWire   TransactionType = "WIRE"
	TransactionTypePix    TransactionType = "PIX"
	TransactionTypeCrypto TransactionType = "CRYPTO"
)

// NewTransactionType trims and upper-cases raw and reports whether the result
// is a valid, non-empty scheme.
func NewTransactionType(raw string) (TransactionType, bool) {
	value, ok := scheme.Normalize(raw)
	if !ok || value == "" {
		return "", false
	}

	return TransactionType(value), true
}

// Valid reports whether t is a non-empty scheme already in canonical form.
// A value that only becomes valid after normalization (e.g. "pix") is not.
func (t TransactionType) Valid() bool {
	value, ok := scheme.Normalize(string(t))

	return ok && value != "" && value == string(t)
}

// String returns the string representation of the transaction type
func (t TransactionType) String() string {
	return string(t)
}

// normalizeSchemeLenient returns raw trimmed and upper-cased when that yields a
// valid scheme (or empty), and raw unchanged otherwise, so validation still
// sees and reports the caller's original value.
func normalizeSchemeLenient(raw string) string {
	if value, ok := scheme.Normalize(raw); ok {
		return value
	}

	return raw
}

// normalizeTransactionTypePtr returns a fresh pointer to the lenient
// normalization of t, or nil when t is nil. The caller's value is never mutated.
func normalizeTransactionTypePtr(t *TransactionType) *TransactionType {
	if t == nil {
		return nil
	}

	value := TransactionType(normalizeSchemeLenient(string(*t)))

	return &value
}

// canonicalSchemePair leniently normalizes a scheme and its deprecated alias
// and fills an empty side from the other.
func canonicalSchemePair(alias, primary string) (string, string) {
	alias = normalizeSchemeLenient(alias)
	primary = normalizeSchemeLenient(primary)

	if alias == "" {
		alias = primary
	}

	if primary == "" {
		primary = alias
	}

	return alias, primary
}

// schemeAliasConflict reports whether a scheme and its deprecated alias both
// carry a value and those values still differ after normalization.
func schemeAliasConflict(alias, primary string) bool {
	normalizedAlias, _ := scheme.Normalize(alias)
	normalizedPrimary, _ := scheme.Normalize(primary)

	return normalizedAlias != "" && normalizedPrimary != "" && normalizedAlias != normalizedPrimary
}
