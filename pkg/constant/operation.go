// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

const (
	DEBIT  = "DEBIT"
	CREDIT = "CREDIT"
	// OVERDRAFT is the public/persisted operation type for system-generated
	// overdraft companion rows. Direction still carries debit/credit semantics.
	OVERDRAFT = "OVERDRAFT"
	ONHOLD    = "ON_HOLD"
	RELEASE   = "RELEASE"
	// BLOCK is the public/persisted operation type for system-generated
	// account-block companion rows. Direction still carries debit/credit semantics.
	BLOCK = "BLOCK"
	// UNBLOCK is the public/persisted operation type for system-generated
	// account-unblock companion rows. Direction still carries debit/credit semantics.
	UNBLOCK = "UNBLOCK"

	DirectionDebit  = "debit"
	DirectionCredit = "credit"
)
