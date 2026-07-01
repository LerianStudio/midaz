// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

// TransactionType represents the type of financial transaction
type TransactionType string

const (
	TransactionTypeCard   TransactionType = "CARD"
	TransactionTypeWire   TransactionType = "WIRE"
	TransactionTypePix    TransactionType = "PIX"
	TransactionTypeCrypto TransactionType = "CRYPTO"
)

// IsValid checks if the transaction type is valid
func (t TransactionType) IsValid() bool {
	switch t {
	case TransactionTypeCard, TransactionTypeWire, TransactionTypePix, TransactionTypeCrypto:
		return true
	default:
		return false
	}
}

// String returns the string representation of the transaction type
func (t TransactionType) String() string {
	return string(t)
}
