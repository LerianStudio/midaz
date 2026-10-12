// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package systemfacts

import "github.com/google/uuid"

// The names the embedded contract (contracts/system-facts) declares for the
// transaction fact. They are the only place the adapter spells them.
const (
	transactionCommittedFactType      = "midaz.transaction_committed"
	transactionCommittedSchemaVersion = "1.0.0"
	transactionSubjectType            = "midaz.transaction"
	transactionIDReferenceType        = "lerian.midaz.transaction_id"
	scopeKeyOrganizationID            = "organizationId"
	scopeKeyLedgerID                  = "ledgerId"
)

// transactionCommittedFactNamespace is the UUIDv5 namespace of every
// midaz.transaction_committed fact id. It was generated once and must never
// change: a different namespace would make a retry record a second fact.
var transactionCommittedFactNamespace = uuid.UUID{ // ee4ae017-0733-48aa-bf59-b31995b9b223
	0xee, 0x4a, 0xe0, 0x17, 0x07, 0x33, 0x48, 0xaa,
	0xbf, 0x59, 0xb3, 0x19, 0x95, 0xb9, 0xb2, 0x23,
}

// TransactionCommittedFactID derives the fact id of the transaction's
// midaz.transaction_committed fact. APPROVED is terminal and reached once per
// transaction, so the id names the transaction alone; a retried completion
// derives the same id and the kit deduplicates it.
func TransactionCommittedFactID(transactionID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(transactionCommittedFactNamespace, []byte(transactionCommittedFactType+":"+transactionID.String()))
}
