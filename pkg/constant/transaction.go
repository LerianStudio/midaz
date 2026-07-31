// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

const (
	CREATED             = "CREATED"
	APPROVED            = "APPROVED"
	PENDING             = "PENDING"
	CANCELED            = "CANCELED"
	NOTED               = "NOTED"
	UniqueViolationCode = "23505"
)

// Option sets named by the invalid-transaction-type rejection (ErrInvalidTransactionType).
// The sentinel is shared by every surface that enforces "exactly one value expression per
// entry", and the surfaces do not accept the same expressions — so each one passes the set IT
// accepts. A surface that names an expression it does not accept sends a rejected caller to
// resubmit with that expression, which is answered with a different rejection.
const (
	// TransactionTypeOptionsDetailed is the set the detailed transaction body accepts.
	TransactionTypeOptionsDetailed = "'amount', 'share', or 'remaining'"

	// TransactionTypeOptionsLeg is the set one leg of a transaction side accepts. It has no
	// `remaining`: that expression resolves during validation but contributes no operation
	// row, so a transaction carrying it commits unbalanced.
	TransactionTypeOptionsLeg = "'amount' or 'share'"
)
