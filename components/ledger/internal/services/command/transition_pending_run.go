// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// pendingTransitionRun carries the per-request state the commit/cancel steps read
// and write. It exists so the steps take one argument instead of a dozen; it holds
// no logic and no defaults — every field is set by the step that owns it.
type pendingTransitionRun struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID

	// tran is the PENDING transaction being transitioned. The steps mutate its
	// status, timestamps and operations in place before it is written back.
	tran   *transaction.Transaction
	status string
	action string

	// input is the persisted body the transition replays. It is already
	// fee-inclusive: the create path applied fees and persisted the fee legs.
	input    mtransaction.Transaction
	validate *mtransaction.Responses
	fromTo   []mtransaction.FromTo

	ledgerSettings    mmodel.LedgerSettings
	honoredTracerSkip bool

	balanceOps       []mmodel.BalanceOperation
	companionFromTos []mtransaction.FromTo
	routeCache       *mmodel.TransactionRouteCache

	result *mmodel.BalanceAtomicResult
}

// reservationIdentity is the identity the tracer's by-transaction confirm/release
// is addressed and REPORTED with. The create-time reserve handle does not survive
// into the separate /commit or /cancel request, so the transaction id is the
// address; the amount and asset come off the persisted transaction so a transition
// that cannot be delivered is still legible as a quantity of uncounted spending.
// A transaction persisted without an amount yields the decimal zero rather than
// panicking on the nil pointer — reporting a zero is a worse log line, never a
// dropped transition.
func (run *pendingTransitionRun) reservationIdentity() reservationHandle {
	identity := reservationHandle{Asset: run.tran.AssetCode}

	identity.TransactionID = run.tran.IDtoUUID()

	if run.tran.Amount != nil {
		identity.Amount = *run.tran.Amount
	}

	return identity
}
