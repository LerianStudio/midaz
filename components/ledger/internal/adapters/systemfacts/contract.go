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
var transactionCommittedFactNamespace = uuid.MustParse("ee4ae017-0733-48aa-bf59-b31995b9b223")

// TransactionCommittedFactID derives the fact id of the transaction's
// midaz.transaction_committed fact. APPROVED is terminal and reached once per
// transaction, so the id names the transaction alone; a retried completion
// derives the same id and the kit deduplicates it.
func TransactionCommittedFactID(transactionID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(transactionCommittedFactNamespace, []byte(transactionCommittedFactType+":"+transactionID.String()))
}
