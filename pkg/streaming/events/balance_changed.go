// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/shopspring/decimal"
)

// Reason values stamped onto BalanceChangedPayload.Reason. Domain-agnostic,
// wire-stable discriminators of WHAT changed the balance. Consumers switch on
// these. NO third-party domain semantics belong here — the Midaz ledger only
// announces "the balance moved, and generically why".
const (
	BalanceChangeReasonCredit    = "credit"
	BalanceChangeReasonDebit     = "debit"
	BalanceChangeReasonBlock     = "block"
	BalanceChangeReasonUnblock   = "unblock"
	BalanceChangeReasonHold      = "hold"
	BalanceChangeReasonRelease   = "release"
	BalanceChangeReasonOverdraft = "overdraft"
	BalanceChangeReasonAdjust    = "adjust"
)

// BalanceChangedDefinition is the routing contract for balance.changed.
//
// Emission anchor: components/ledger/internal/services/command/send_balance_changed_events.go,
// inside SendBalanceChangedEvents, launched as a goroutine alongside
// SendTransactionEvents / SendOverdraftEvents from
// create_balance_transaction_operations_async.go and
// create_bulk_transaction_operations_async.go. One balance.changed is emitted
// per balance-affecting operation of the committed transaction.
//
// Design intent: this event is INTENTIONALLY generic and domain-agnostic. It
// carries only Midaz identities (org/ledger/account/balance/asset) plus a
// generic Reason discriminator. It carries NO consumer-domain fields (no
// cpf_hash, no judicial-order id, nothing SISBAJUD-specific). Translating this
// generic signal into a domain trigger is the consumer's job.
//
// Relationship to balance.created: COMPLEMENTARY, not a replacement.
// balance.created fires only on CreateAdditionalBalance (lifecycle create);
// balance.changed fires on transaction-driven mutations of an existing balance.
//
// IMPORTANT posture: emit failures MUST NOT fail the parent transaction.
// EventType "changed" is a single word — no underscore/hyphen concern with the
// lib-streaming route-key regex. Wire topic: lerian.streaming.balance.changed.
var BalanceChangedDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "changed",
	SchemaVersion: "1.0.0",
}

// BalanceChangedPayload is the wire payload for balance.changed.
//
// Available/OnHold/Amount are decimal.Decimal so the wire encodes them as JSON
// numbers (matching the HTTP balance contract). They reflect the state AFTER
// the operation.
//
// Scale is intentionally OMITTED: it is an asset-level property, not a
// balance-level one (see balance_created.go). Consumers needing scale should
// join against the corresponding asset.created event. Version IS included so
// consumers can order/dedup mutations of the same balance.
type BalanceChangedPayload struct {
	OrganizationID string          `json:"organizationId"`
	LedgerID       string          `json:"ledgerId"`
	AccountID      string          `json:"accountId"`
	BalanceID      string          `json:"balanceId"`
	Alias          string          `json:"alias,omitempty"`
	AssetCode      string          `json:"assetCode"`
	BalanceKey     string          `json:"balanceKey"`
	Available      decimal.Decimal `json:"available"`
	OnHold         decimal.Decimal `json:"onHold"`
	Version        int64           `json:"version"`
	Reason         string          `json:"reason"`
	OperationType  string          `json:"operationType"`
	Direction      string          `json:"direction,omitempty"`
	Amount         decimal.Decimal `json:"amount"`
	TransactionID  string          `json:"transactionId"`
	OperationID    string          `json:"operationId"`
	OccurredAt     string          `json:"occurredAt"`
}

// BalanceChangedSource carries the identifiers + amounts the constructor needs
// to assemble a payload. The use case populates this from one operation + its
// parent transaction. Keeping it a struct keeps the constructor from growing
// into positional-argument territory (mirrors BalanceOverdraftSource).
type BalanceChangedSource struct {
	OrganizationID string
	LedgerID       string
	AccountID      string
	BalanceID      string
	Alias          string
	AssetCode      string
	BalanceKey     string
	Available      decimal.Decimal
	OnHold         decimal.Decimal
	Version        int64
	Reason         string
	OperationType  string
	Direction      string
	Amount         decimal.Decimal
	TransactionID  string
	OperationID    string
	OccurredAt     time.Time
}

// NewBalanceChanged maps the source into the wire payload.
func NewBalanceChanged(src BalanceChangedSource) BalanceChangedPayload {
	return BalanceChangedPayload{
		OrganizationID: src.OrganizationID,
		LedgerID:       src.LedgerID,
		AccountID:      src.AccountID,
		BalanceID:      src.BalanceID,
		Alias:          src.Alias,
		AssetCode:      src.AssetCode,
		BalanceKey:     src.BalanceKey,
		Available:      src.Available,
		OnHold:         src.OnHold,
		Version:        src.Version,
		Reason:         src.Reason,
		OperationType:  src.OperationType,
		Direction:      src.Direction,
		Amount:         src.Amount,
		TransactionID:  src.TransactionID,
		OperationID:    src.OperationID,
		OccurredAt:     src.OccurredAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
// Subject combines transactionId + operationId so consumers safe-deduping on
// Subject match the canonical idempotency key for this event.
func (p BalanceChangedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", BalanceChangedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: BalanceChangedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.TransactionID + ":" + p.OperationID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
