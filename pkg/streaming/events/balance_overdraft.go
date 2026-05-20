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

// Overdraft action constants stamped onto BalanceOverdraftPayload.Action.
// Wire-stable strings; consumers switch on them.
const (
	OverdraftWireActionDrawn   = "drawn"
	OverdraftWireActionRepaid  = "repaid"
	OverdraftWireActionCleared = "cleared"
)

// BalanceOverdraftDrawnDefinition is the routing contract for
// balance.overdraft_drawn.
//
// Emission anchor: components/ledger/internal/services/command/send_overdraft_events.go,
// inside SendOverdraftEvents alongside the legacy rabbit publish. The
// classifier at classifyOverdraftOperation returns OverdraftActionDrawn
// when op.Direction == DirectionDebit on a direction=debit companion
// balance (overdraft consumed by a debit that exceeded the credit
// balance's available funds).
//
// Emission ordering: the lib-streaming Emit MUST occur AFTER the parent
// transaction.{posted,committed,reverted} has been emitted in the same
// goroutine cascade (SendTransactionEvents and SendOverdraftEvents are
// launched together from create_balance_transaction_operations_async.go).
// During the cutover window this event coexists with the legacy
// transaction.overdraft_events rabbit publish; the rabbit publish is
// removed in a follow-up task once consumers migrate.
//
// IMPORTANT posture: emit failures MUST NOT fail the parent transaction.
// EventType uses the HYPHEN form `overdraft-drawn` because the
// lib-streaming route-key regex rejects underscores. Wire topic:
// `lerian.streaming.balance.overdraft-drawn`. The Action payload field
// keeps the unsuffixed value "drawn" (consumers can match on either
// the topic or the action field).
var BalanceOverdraftDrawnDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "overdraft-drawn",
	SchemaVersion: "1.0.0",
}

// BalanceOverdraftRepaidDefinition is the routing contract for
// balance.overdraft_repaid.
//
// Emission anchor: same as BalanceOverdraftDrawnDefinition. The
// classifier returns OverdraftActionRepaid when op.Direction ==
// DirectionCredit AND the companion balance's afterAvail is non-zero
// (overdraft usage decreased but not fully cleared).
//
// IMPORTANT posture: emit failures MUST NOT fail the parent transaction.
// EventType uses the HYPHEN form `overdraft-repaid` per the
// lib-streaming route-key regex (no underscores allowed in routing
// identifiers).
var BalanceOverdraftRepaidDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "overdraft-repaid",
	SchemaVersion: "1.0.0",
}

// BalanceOverdraftClearedDefinition is the routing contract for
// balance.overdraft_cleared.
//
// Emission anchor: same as BalanceOverdraftDrawnDefinition. The
// classifier returns OverdraftActionCleared when op.Direction ==
// DirectionCredit AND the companion balance's afterAvail is zero
// (overdraft fully repaid — terminal signal).
//
// IMPORTANT posture: emit failures MUST NOT fail the parent transaction.
// EventType uses the HYPHEN form `overdraft-cleared` per the
// lib-streaming route-key regex.
var BalanceOverdraftClearedDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "overdraft-cleared",
	SchemaVersion: "1.0.0",
}

// BalanceOverdraftPayload is the shared wire payload for the three
// overdraft lifecycle events (drawn, repaid, cleared). The Action field
// discriminates which lifecycle stage this row reports — its value
// matches the EventType suffix of the corresponding Definition (e.g.
// Action="drawn" for balance.overdraft_drawn). Consumers can branch on
// either the topic name or the Action field; both are guaranteed
// consistent by the per-event constructors below.
//
// All three lifecycle events carry the same set of fields because
// downstream partners use them for the same purpose: tracking the
// companion overdraft balance's running state. Defining three
// near-identical mirror payload types would have been disproportionate
// boilerplate; one shared payload with an explicit Action discriminator
// is cheaper and equally type-safe.
//
// Idempotency hint for consumers: `transactionId + operationId` is
// unique per overdraft movement; consumers safe-deduping on that pair
// can replay any of the three events without effect.
//
// OverdraftLimit is *decimal.Decimal so the wire shape distinguishes
// "limit not yet known to this event" (null) from "limit is zero" (0).
// The current emission path leaves OverdraftLimit nil — the persisted
// operation row does not (yet) carry the configured limit, and a fresh
// repo lookup on the hot path is not justified. A future task (T-010)
// will extend the operation snapshot to carry the limit alongside
// OverdraftUsed; this payload accepts the field today so consumers can
// rely on the shape once that data lands.
//
// Scale is intentionally OMITTED: it is an asset-level property, not a
// balance-level one. Consumers needing scale should join against the
// corresponding asset.created event.
type BalanceOverdraftPayload struct {
	BalanceID        string           `json:"balanceId"`
	AccountID        string           `json:"accountId"`
	OrganizationID   string           `json:"organizationId"`
	LedgerID         string           `json:"ledgerId"`
	AssetCode        string           `json:"assetCode"`
	TransactionID    string           `json:"transactionId"`
	OperationID      string           `json:"operationId"`
	Action           string           `json:"action"`
	Amount           decimal.Decimal  `json:"amount"`
	OverdraftBalance decimal.Decimal  `json:"overdraftBalance"`
	OverdraftLimit   *decimal.Decimal `json:"overdraftLimit,omitempty"`
	OccurredAt       string           `json:"occurredAt"`
}

// BalanceOverdraftSource carries the identifiers and amounts the
// constructors need to assemble a payload. The use case populates this
// from the operation + parent transaction; keeping it as a struct keeps
// the constructor signatures from growing into ten-positional-arguments
// territory.
//
// Action is NOT a field on this struct: the per-event constructor
// stamps it from the corresponding event-type constant. Callers
// therefore cannot accidentally cross-wire (e.g. invoking
// NewBalanceOverdraftDrawn with action="cleared").
type BalanceOverdraftSource struct {
	BalanceID        string
	AccountID        string
	OrganizationID   string
	LedgerID         string
	AssetCode        string
	TransactionID    string
	OperationID      string
	Amount           decimal.Decimal
	OverdraftBalance decimal.Decimal
	OverdraftLimit   *decimal.Decimal
	OccurredAt       time.Time
}

func newBalanceOverdraftPayload(src BalanceOverdraftSource, action string) BalanceOverdraftPayload {
	return BalanceOverdraftPayload{
		BalanceID:        src.BalanceID,
		AccountID:        src.AccountID,
		OrganizationID:   src.OrganizationID,
		LedgerID:         src.LedgerID,
		AssetCode:        src.AssetCode,
		TransactionID:    src.TransactionID,
		OperationID:      src.OperationID,
		Action:           action,
		Amount:           src.Amount,
		OverdraftBalance: src.OverdraftBalance,
		OverdraftLimit:   src.OverdraftLimit,
		OccurredAt:       src.OccurredAt.Format(time.RFC3339),
	}
}

// NewBalanceOverdraftDrawn stamps Action="drawn" onto the shared payload.
func NewBalanceOverdraftDrawn(src BalanceOverdraftSource) BalanceOverdraftPayload {
	return newBalanceOverdraftPayload(src, OverdraftWireActionDrawn)
}

// NewBalanceOverdraftRepaid stamps Action="repaid" onto the shared payload.
func NewBalanceOverdraftRepaid(src BalanceOverdraftSource) BalanceOverdraftPayload {
	return newBalanceOverdraftPayload(src, OverdraftWireActionRepaid)
}

// NewBalanceOverdraftCleared stamps Action="cleared" onto the shared payload.
func NewBalanceOverdraftCleared(src BalanceOverdraftSource) BalanceOverdraftPayload {
	return newBalanceOverdraftPayload(src, OverdraftWireActionCleared)
}

// ToEmitRequestDrawn assembles a libStreaming.EmitRequest for
// balance.overdraft_drawn. Action MUST equal OverdraftWireActionDrawn —
// callers should construct the payload via NewBalanceOverdraftDrawn
// rather than mutating Action directly.
func (p BalanceOverdraftPayload) ToEmitRequestDrawn(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(BalanceOverdraftDrawnDefinition, tenantID, ts)
}

// ToEmitRequestRepaid assembles a libStreaming.EmitRequest for
// balance.overdraft_repaid. Action MUST equal OverdraftWireActionRepaid.
func (p BalanceOverdraftPayload) ToEmitRequestRepaid(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(BalanceOverdraftRepaidDefinition, tenantID, ts)
}

// ToEmitRequestCleared assembles a libStreaming.EmitRequest for
// balance.overdraft_cleared. Action MUST equal OverdraftWireActionCleared.
func (p BalanceOverdraftPayload) ToEmitRequestCleared(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(BalanceOverdraftClearedDefinition, tenantID, ts)
}

func (p BalanceOverdraftPayload) toEmitRequest(def Definition, tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", def.Key(), err)
	}

	// Subject combines transactionId + operationId so consumers
	// safe-deduping on Subject match the canonical idempotency key for
	// this event family (see consumer_idempotency_key_candidate in the
	// instrumentation map).
	subject := p.TransactionID + ":" + p.OperationID

	return libStreaming.EmitRequest{
		DefinitionKey: def.Key(),
		TenantID:      tenantID,
		Subject:       subject,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
