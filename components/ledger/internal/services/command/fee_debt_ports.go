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

// FeeDebtRecorder is the Fees projection of fee debts, resolving the tenant from ctx.
type FeeDebtRecorder interface {
	// Apply projects applied fee-debt changes. It is idempotent per (DebtID,
	// TransactionID, PostingRef, Kind), so replays and out-of-order completions converge.
	Apply(ctx context.Context, record FeeDebtRecord) error

	// Settled sums, per debt id, the recorded settlements less reopens from their
	// exact amounts; a debt without a record is absent.
	Settled(ctx context.Context, organizationID, ledgerID uuid.UUID, debtIDs []string) (map[string]decimal.Decimal, error)

	// HasOpenCreditor reports whether an open debt of the ledger names one of
	// creditRefs (alias#key) as creditor. The projection lags completion.
	HasOpenCreditor(ctx context.Context, organizationID, ledgerID uuid.UUID, creditRefs []string) (bool, error)
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

	DebitRoute  *accounting.FeeDebtRoute `json:"debitRoute,omitempty"`
	CreditRoute *accounting.FeeDebtRoute `json:"creditRoute,omitempty"`
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

	DebitRoute  *accounting.FeeDebtRoute `json:"debitRoute,omitempty"`
	CreditRoute *accounting.FeeDebtRoute `json:"creditRoute,omitempty"`
}
