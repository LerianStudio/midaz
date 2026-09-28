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

// TransactionStatuses is the closed set of status codes a transaction row can
// carry, in lifecycle order. It is the ONE list every surface that enumerates
// statuses reads: the count filter's allowlist and the dashboard's byStatus
// breakdown both derive from it, so a status added here reaches both without a
// second edit — and cannot be counted by one and silently dropped by the other.
//
// CREATED never survives a write. Every persistence path promotes it to
// APPROVED before the INSERT (services/command/create_transaction_steps.go,
// create_balance_transaction_operations_async.go,
// create_bulk_transaction_operations_async.go), so it is a response-and-
// idempotency status rather than a stored one. It stays in the list because the
// count filter accepts it and a caller may still ask for it — the honest answer
// to that question is zero, not a rejection.
var TransactionStatuses = []string{CREATED, APPROVED, PENDING, CANCELED, NOTED}

// SettledTransactionStatuses is the status set whose transactions MOVED MONEY
// and therefore carry volume.
//
// APPROVED alone. PENDING has only reserved funds and may still be canceled;
// CANCELED moved nothing; NOTED is annotation-only, writes no operations and
// touches no balance (services/command/create_transaction_v2.go:249,
// transaction_fee_application.go:38); CREATED is never stored (see above).
// Counting any of them as volume reports money the ledger did not move.
var SettledTransactionStatuses = []string{APPROVED}

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
