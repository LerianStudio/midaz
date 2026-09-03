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
