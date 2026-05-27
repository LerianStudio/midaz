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
	"github.com/shopspring/decimal"
)

// BalanceCreatedDefinition is the routing contract for balance.created.
//
// Emission anchor: components/ledger/internal/services/command/create_balance_additional.go,
// immediately after BalanceRepo.Create succeeds in the CreateAdditionalBalance
// use case (POST .../accounts/:account_id/balances).
//
// Suppressed paths:
//   - CreateDefaultBalance: the implicit default-balance auto-create from
//     create_account.go + create_asset.go. That scenario emits
//     account.created or asset.created instead.
//   - ensureOverdraftBalance: the system-managed overdraft companion
//     auto-create. That row is reported via balance.config_changed with
//     changeType=overdraft_enabled (the companion's identity becoming
//     known IS the "config changed" signal).
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var BalanceCreatedDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// BalanceCreatedPayload is the wire payload for balance.created.
//
// Available/OnHold are decimal.Decimal so the wire encodes them as a
// JSON number (matching the HTTP response contract). Newly-created
// balances always have zero Available/OnHold, but the fields are still
// present on the wire so consumers don't need a schema branch for the
// create event vs subsequent state.
//
// Optional fields (Description-like extensions, Settings) use omitempty
// to mirror the HTTP response: a balance created with only the
// required fields does not leak nil pointers onto the wire.
//
// Scale is intentionally OMITTED: it is an asset-level property
// (mmodel.Asset.Scale), not a balance-level one. Consumers needing
// scale should join against the corresponding asset.created event.
// Including scale here would force an asset lookup on the hot create
// path solely to populate the event.
type BalanceCreatedPayload struct {
	ID             string                  `json:"id"`
	OrganizationID string                  `json:"organizationId"`
	LedgerID       string                  `json:"ledgerId"`
	AccountID      string                  `json:"accountId"`
	Alias          string                  `json:"alias,omitempty"`
	Key            string                  `json:"key"`
	AssetCode      string                  `json:"assetCode"`
	AccountType    string                  `json:"accountType,omitempty"`
	Available      decimal.Decimal         `json:"available"`
	OnHold         decimal.Decimal         `json:"onHold"`
	AllowSending   bool                    `json:"allowSending"`
	AllowReceiving bool                    `json:"allowReceiving"`
	Direction      string                  `json:"direction,omitempty"`
	Settings       *mmodel.BalanceSettings `json:"settings,omitempty"`
	CreatedAt      string                  `json:"createdAt"`
	UpdatedAt      string                  `json:"updatedAt"`
}

// NewBalanceCreated maps a persisted balance into the wire payload.
//
// Caller invariant: b must be the value returned by BalanceRepo.Create
// (post-commit), not the input struct. Specifically b.ID, b.CreatedAt
// and b.UpdatedAt must reflect the persisted state.
func NewBalanceCreated(b *mmodel.Balance) BalanceCreatedPayload {
	return BalanceCreatedPayload{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            b.Key,
		AssetCode:      b.AssetCode,
		AccountType:    b.AccountType,
		Available:      b.Available,
		OnHold:         b.OnHold,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		Direction:      b.Direction,
		Settings:       b.Settings,
		CreatedAt:      b.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      b.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p BalanceCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", BalanceCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: BalanceCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
