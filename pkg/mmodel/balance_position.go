// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

// Position is a derived view computed from the balance state at response
// time. It is response-only — never persisted, never cached. Always
// present on Balance and BalanceHistory JSON responses.
//
// AvailableOverdraftLimit has three documented cases:
//
//   - "0"          when overdraft is disabled (Settings nil OR
//     Settings.AllowOverdraft = false). The customer has no
//     headroom because no overdraft is permitted.
//   - positive     when overdraft is limited (AllowOverdraft = true AND
//     OverdraftLimitEnabled = true). Value is
//     OverdraftLimit − OverdraftUsed. May be zero when used
//     exactly equals limit, or negative on malformed limit
//     strings (read-side resilience: never crash).
//   - omitted/null when overdraft is unlimited (AllowOverdraft = true AND
//     OverdraftLimitEnabled = false). The field is absent
//     from the JSON wire (omitempty on a nil pointer).
//
// swagger:model Position
// @Description Calculated balance position. Derived from Available, OnHold, OverdraftUsed, and Settings — never stored. The position object centralises customer-facing arithmetic so consumers don't have to reimplement it client-side.
type Position struct {
	// Available is the net spendable amount: Balance.Available minus
	// Balance.OverdraftUsed. Negative when the balance is overdrafted
	// (Available pinned at 0, OverdraftUsed > 0). Always present.
	// example: 950
	Available decimal.Decimal `json:"available" example:"950"`

	// OnHold mirrors Balance.OnHold verbatim. Always present.
	// example: 100
	OnHold decimal.Decimal `json:"onHold" example:"100"`

	// AvailableOverdraftLimit is the remaining overdraft headroom.
	// Omitted from JSON when overdraft is unlimited (nil pointer).
	// example: 500
	AvailableOverdraftLimit *decimal.Decimal `json:"availableOverdraftLimit,omitempty" example:"500"`
} // @name Position

// ComputePosition derives a Position from the current Balance state.
// Pure function, no side effects, no I/O.
//
// See the Position type doc for the full semantics of each field.
func (b Balance) ComputePosition() Position {
	return computePosition(b.Available, b.OnHold, b.OverdraftUsed, b.Settings)
}

// ComputePosition derives a Position from the BalanceHistory snapshot.
// Mirrors Balance.ComputePosition so historical responses carry the same
// position shape as live reads.
func (bh BalanceHistory) ComputePosition() Position {
	return computePosition(bh.Available, bh.OnHold, bh.OverdraftUsed, bh.Settings)
}

// computePosition is the shared implementation used by both
// Balance.ComputePosition and BalanceHistory.ComputePosition. Keeping the
// logic in one place prevents drift between the live and historical
// surfaces.
//
// Resilience policy: a malformed Settings.OverdraftLimit string falls
// back to decimal.Zero (matching the existing posture in
// Balance.ToTransactionBalance). The position field is read-only audit
// context — never a correctness gate — so unparseable strings on
// historical or corrupted rows surface as conservative numeric values
// rather than crashing the read path.
func computePosition(available, onHold, overdraftUsed decimal.Decimal, settings *BalanceSettings) Position {
	pos := Position{
		Available: available.Sub(overdraftUsed),
		OnHold:    onHold,
	}

	// Settings nil OR overdraft disabled → no headroom permitted.
	// Emit an explicit "0" so consumers can distinguish "no overdraft"
	// from "unlimited overdraft" (the latter omits the field entirely).
	if settings == nil || !settings.AllowOverdraft {
		zero := decimal.Zero
		pos.AvailableOverdraftLimit = &zero

		return pos
	}

	// Overdraft enabled but unlimited → omit the field entirely
	// (nil pointer + omitempty). Absence on the wire = unlimited.
	if !settings.OverdraftLimitEnabled {
		return pos
	}

	// Overdraft enabled AND limited → headroom = limit − used.
	var limit decimal.Decimal
	if settings.OverdraftLimit != nil {
		if parsed, err := decimal.NewFromString(*settings.OverdraftLimit); err == nil {
			limit = parsed
		}
		// Parse failure: limit stays at decimal.Zero. Conservative —
		// headroom = 0 − overdraftUsed, possibly negative.
	}

	headroom := limit.Sub(overdraftUsed)
	pos.AvailableOverdraftLimit = &headroom

	return pos
}

// MarshalJSON injects the computed Position block into the JSON output
// alongside the standard Balance fields.
//
// Implementation detail: the alias-type pattern (`type Alias Balance`)
// strips the MarshalJSON receiver from the inner struct so the encoder
// falls back to default tag-based serialization, avoiding infinite
// recursion. The outer anonymous struct embeds the alias and adds the
// computed position field with its own json tag.
//
// This receiver only fires for `mmodel.Balance` values. The cache
// envelope `BalanceRedis` is a separate type and is unaffected. The DB
// model `BalancePostgreSQLModel` is never JSON-marshalled (SQL only).
// The queue path uses msgpack, not JSON. The wire-shape contract for
// the position block is therefore JSON-response-only by construction.
func (b Balance) MarshalJSON() ([]byte, error) {
	type Alias Balance

	return json.Marshal(struct {
		Alias
		Position Position `json:"position"`
	}{
		Alias:    Alias(b),
		Position: b.ComputePosition(),
	})
}

// MarshalJSON injects the computed Position block into BalanceHistory's
// JSON output. Mirrors Balance.MarshalJSON so historical responses
// carry the same wire shape as live responses.
func (bh BalanceHistory) MarshalJSON() ([]byte, error) {
	type Alias BalanceHistory

	return json.Marshal(struct {
		Alias
		Position Position `json:"position"`
	}{
		Alias:    Alias(bh),
		Position: bh.ComputePosition(),
	})
}
