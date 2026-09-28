// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounting

import "github.com/shopspring/decimal"

// PostingType identifies an accounting mutation, independently of transaction status.
type PostingType string

const (
	// PostingDebit decreases available funds for credit or empty direction, and
	// increases them for debit direction. Only credit-direction, non-external
	// accounts with live overdraft permission and DrawAllowed may draw debt.
	PostingDebit PostingType = "debit"
	// PostingCredit reverses debit arithmetic. For credit or empty direction,
	// non-external accounts repay debt before increasing available funds, capped
	// by a positive OverdraftAmount. Debit-direction credits enforce the floor.
	PostingCredit PostingType = "credit"
	// PostingReserve increases on-hold funds without checking available funds
	// again. A preceding debit must forbid overdraft when composing a hold.
	PostingReserve PostingType = "reserve"
	// PostingUnreserve decreases on-hold funds without changing available funds.
	// Insufficient on-hold funds are an integrity failure before any write.
	PostingUnreserve PostingType = "unreserve"
	// PostingHold combines debit arithmetic and reserve in one movement and one
	// version increment. It never draws debt; external accounts retain their
	// arithmetic floor exemption, with targeting restrictions enforced by the
	// transaction's balance requirements.
	PostingHold PostingType = "hold"
	// PostingRelease decreases on-hold funds and restores available funds using
	// the account direction. It does not perform general credit repayment: only
	// a positive OverdraftAmount permits capped repayment on a non-external,
	// credit-direction account. On-hold underflow and the directional floor apply.
	PostingRelease PostingType = "release"
)

// DrawPolicy describes path and route permission, not cached account settings.
// Execution always combines it with live account direction and overdraft settings.
type DrawPolicy string

const (
	// DrawForbidden prevents debt creation, including for pending debits.
	DrawForbidden DrawPolicy = "forbidden"
	// DrawAllowed permits debt creation only when live account settings allow it.
	DrawAllowed DrawPolicy = "allowed"
	// DrawRouteDenied preserves route denial separately from account eligibility.
	DrawRouteDenied DrawPolicy = "route_denied"
)

// Posting addresses a logical alias#key, never a physical storage key.
// Ref must be unique within its transaction. Amount must be positive and
// OverdraftAmount nonnegative; zero means no cap for credit, but no repay for release.
type Posting struct {
	Ref             string          `json:"ref"`
	BalanceRef      string          `json:"balanceRef"`
	Type            PostingType     `json:"type"`
	Amount          decimal.Decimal `json:"amount"`
	DrawPolicy      DrawPolicy      `json:"drawPolicy"`
	OverdraftAmount decimal.Decimal `json:"overdraftAmount"`
}

// BalancePermission identifies the transaction-side permission that must be
// checked against the live cached balance before any posting is applied.
type BalancePermission string

const (
	BalancePermissionSend    BalancePermission = "send"
	BalancePermissionReceive BalancePermission = "receive"
)

// BalanceRequirement carries transaction intent that cannot be inferred from
// posting arithmetic. Implementations evaluate it against live balance state in
// the same atomic execution that applies the transaction.
type BalanceRequirement struct {
	BalanceRef     string            `json:"balanceRef"`
	AssetCode      string            `json:"assetCode"`
	Permission     BalancePermission `json:"permission"`
	ForbidExternal bool              `json:"forbidExternal"`
}
