// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// createTransactionRun carries the per-request state the create steps read and
// write. It exists so the steps take one argument instead of a dozen; it holds
// no logic and no defaults — every field is set by the step that owns it.
type createTransactionRun struct {
	organizationID      uuid.UUID
	ledgerID            uuid.UUID
	parentTransactionID uuid.UUID
	transactionID       uuid.UUID
	transactionDate     time.Time

	// input is the transaction being posted. The fee seam mutates it in place.
	input  mtransaction.Transaction
	status string
	action string

	validate *mtransaction.Responses
	fromTo   []mtransaction.FromTo

	ledgerSettings mmodel.LedgerSettings

	idempotencyKey         string
	idempotencyTTL         time.Duration
	idempotencyHash        string
	idempotencyInternalKey *string

	balances         []*mmodel.Balance
	balanceOps       []mmodel.BalanceOperation
	companionFromTos []mtransaction.FromTo
	routeCache       *mmodel.TransactionRouteCache

	honoredFeeSkip    bool
	honoredTracerSkip bool

	// accountBlockExceptionID is the single-use account-block exception the
	// request presented, before it is looked up. Nil when none was presented — a
	// transport whose contract carries no such field always leaves it nil.
	accountBlockExceptionID *uuid.UUID

	// accountBlockExceptionGrant is that exception resolved from the cache. Only
	// the /v2 pipelines name the resolver, so a /v1 request leaves it nil and
	// every barrier behaves as it did before the field existed.
	accountBlockExceptionGrant *mtransaction.AccountBlockExceptionGrant

	result *mmodel.BalanceAtomicResult
}
