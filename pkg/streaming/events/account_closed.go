// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
)

// AccountClosedDefinition is the routing contract for account.closed.
// Emission anchor:
// components/ledger/internal/services/command/close_account.go, after the
// closing transition is finalized (post-commit).
//
// IMPORTANT posture: emit failures MUST NOT fail the request; the closing is
// durable in the account row and the API answers 204 regardless.
//
// Closing is a state of the account, not a movement: it writes no transaction
// and no operation, so this event carries no transaction reference. It is also
// the only event the closing publishes — no account.updated and no
// balance.deleted follow from it.
var AccountClosedDefinition = Definition{
	ResourceType:  "account",
	EventType:     "closed",
	SchemaVersion: "1.0.0",
}

// AccountClosedPayload is the wire payload for account.closed. Kept
// intentionally minimal: identity, tenant scope (org/ledger), and the closing
// instant.
//
// Idempotency hint for consumers: a closing is single-shot, so `id` alone
// identifies it and `closedAt` never changes once recorded; a replayed event
// carries the same pair.
type AccountClosedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`

	// RFC3339-formatted instant the database recorded on the account row and
	// returned to the use case. No clock of the producing process reaches it.
	ClosedAt string `json:"closedAt"`
}

// NewAccountClosed maps the account identity and the persisted closing instant
// into the wire payload.
//
// Caller invariant: closedAt is the value the conditional close statement
// returned, never a locally captured time.
func NewAccountClosed(id, organizationID, ledgerID string, closedAt time.Time) AccountClosedPayload {
	return AccountClosedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		ClosedAt:       closedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
// tenantID comes from pkgStreaming.ResolveTenantID(ctx); ts is the timestamp
// lib-streaming stamps on the ce-time header — the persisted closing instant.
//
// Source, ResourceType, EventType, and SchemaVersion are NOT carried on the
// request. Source flows from the Builder at construction time; the other three
// resolve from the Catalog by DefinitionKey at emit time.
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p AccountClosedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountClosedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountClosedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
