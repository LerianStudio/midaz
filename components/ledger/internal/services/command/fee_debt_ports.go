// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// FeeDebtRecord is the fee-debt projection of one completed transaction. Changes
// keep result order; FeePackageID is the transaction's packageAppliedID, recorded
// only on opened debts, and empty when it has none.
type FeeDebtRecord struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	FeePackageID   string
	AppliedAt      time.Time
	Changes        []accounting.FeeDebtChange
}

// FeeDebtRecorder projects applied fee-debt changes into Fees, resolving the
// tenant from ctx. Apply is idempotent per (DebtID, TransactionID, PostingRef,
// Kind), so replays and out-of-order completions converge.
type FeeDebtRecorder interface {
	Apply(ctx context.Context, record FeeDebtRecord) error
}

// FeeDebtOpening is one element of the feeDebtOpenings transaction metadata, built by
// completion from the result's opened changes. A revert of that transaction turns
// each into an accounting.FeeDebtRefund of the refund posting.
type FeeDebtOpening struct {
	DebtID    string          `json:"debtId"`
	DebtorRef string          `json:"debtorRef"`
	CreditRef string          `json:"creditRef"`
	Opened    decimal.Decimal `json:"opened"`
	Seq       int64           `json:"seq,string"`
}

// FeeDebtSettlement is one element of the feeDebtSettlements transaction metadata, built
// by completion from the result's settled changes. A revert of that transaction turns
// each into an accounting.FeeDebtReopen, unless the debt's origin is already reverted.
type FeeDebtSettlement struct {
	DebtID    string          `json:"debtId"`
	DebtorRef string          `json:"debtorRef"`
	CreditRef string          `json:"creditRef"`
	Amount    decimal.Decimal `json:"amount"`
	Opened    decimal.Decimal `json:"opened"`
	Seq       int64           `json:"seq,string"`
}
