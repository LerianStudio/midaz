// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// BalanceConfigChangeType discriminates the two source branches that
// emit balance.config_changed. Wire values are stable strings; consumers
// switch on them to route into their state-replication logic.
const (
	// BalanceConfigChangeTypeSettingsUpdated is emitted for ordinary
	// settings PATCH operations on a balance (AllowSending,
	// AllowReceiving, Settings.AllowOverdraft, Settings.OverdraftLimit,
	// Settings.BalanceScope).
	BalanceConfigChangeTypeSettingsUpdated = "settings_updated"

	// BalanceConfigChangeTypeOverdraftEnabled is emitted exactly once on
	// the false->true overdraft transition for the parent balance, from
	// the auto-create branch in ensureOverdraftBalance. The payload's
	// id field carries the newly-materialized companion balance ID —
	// the companion's identity becoming known IS the "config changed"
	// signal. This event substitutes for a balance.created on the
	// companion (which is suppressed because companions are
	// system-managed).
	BalanceConfigChangeTypeOverdraftEnabled = "overdraft_enabled"
)

// BalanceConfigChangedDefinition is the routing contract for
// balance.config_changed.
//
// Emission anchors (two, sharing this single definition):
//
//   - components/ledger/internal/services/command/update_balance.go,
//     immediately after BalanceRepo.Update succeeds in UseCase.Update.
//     ChangeType = settings_updated, id = the updated parent balance.
//
//   - components/ledger/internal/services/command/update_balance_overdraft.go,
//     immediately after the companion BalanceRepo.Create succeeds in
//     UseCase.ensureOverdraftBalance. ChangeType = overdraft_enabled,
//     id = the newly-created companion balance.
//
// A single PATCH that toggles AllowOverdraft false->true therefore
// produces TWO config_changed events: one for the auto-created
// companion (changeType=overdraft_enabled) followed by one for the
// parent balance whose settings now hold AllowOverdraft=true
// (changeType=settings_updated). The ordering is enforced by the use
// case (ensureOverdraftBalance runs before BalanceRepo.Update).
//
// Internal-scope balances cannot be updated via the public API
// (rejected by the scope guard in UseCase.Update). The companion
// auto-create runs from the system, not the public PATCH path.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
// EventType uses the HYPHEN form `config-changed` because the
// lib-streaming route-key regex `^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)+$`
// rejects underscores. The wire topic is
// `lerian.streaming.ledger_balance.config_changed` (hyphens in the route
// key become underscores in the topic name only). Payload field VALUES
// (e.g. changeType="settings_updated") may keep snake_case because
// they are payload data, not routing identifiers.
var BalanceConfigChangedDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "config-changed",
	SchemaVersion: "1.0.0",
}

// BalanceConfigChangedPayload is the wire payload for
// balance.config_changed.
//
// The payload carries the full mutable configuration surface so
// consumers don't need to join against balance.created to render the
// row. Monetary fields (available, onHold) are intentionally OMITTED:
// this event is a configuration mutation signal, not a money-movement
// signal. Money movement lives inside transaction.{posted,committed,...}
// in the transaction-engine segment.
//
// Settings uses the *BalanceSettings pointer because the wire shape
// distinguishes "settings was cleared" (settings:null) from "settings
// was left unchanged on this PATCH" (consumers cannot disambiguate that
// from this event alone — they must merge with the prior known state).
type BalanceConfigChangedPayload struct {
	ID             string                  `json:"id"`
	OrganizationID string                  `json:"organizationId"`
	LedgerID       string                  `json:"ledgerId"`
	AccountID      string                  `json:"accountId"`
	Alias          string                  `json:"alias,omitempty"`
	Key            string                  `json:"key"`
	AllowSending   bool                    `json:"allowSending"`
	AllowReceiving bool                    `json:"allowReceiving"`
	Direction      string                  `json:"direction,omitempty"`
	Settings       *mmodel.BalanceSettings `json:"settings,omitempty"`
	ChangeType     string                  `json:"changeType"`
	UpdatedAt      string                  `json:"updatedAt"`
}

// NewBalanceConfigChanged maps a persisted balance plus the
// discriminating changeType into the wire payload.
//
// Caller invariant: b must be the value returned by BalanceRepo.Update
// (settings_updated branch) or by BalanceRepo.Create (overdraft_enabled
// branch — i.e. the companion's persisted record). In both cases
// b.UpdatedAt must reflect the persisted row state.
//
// changeType must be one of BalanceConfigChangeType{SettingsUpdated,
// OverdraftEnabled}; other values are passed through verbatim so this
// function can be extended without changing the signature.
func NewBalanceConfigChanged(b *mmodel.Balance, changeType string) BalanceConfigChangedPayload {
	return BalanceConfigChangedPayload{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            b.Key,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		Direction:      b.Direction,
		Settings:       b.Settings,
		ChangeType:     changeType,
		UpdatedAt:      b.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p BalanceConfigChangedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", BalanceConfigChangedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: BalanceConfigChangedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
