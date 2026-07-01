// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// AssetCreatedDefinition is the routing contract for asset.created.
// Emission anchor: components/ledger/internal/services/command/create_asset.go,
// immediately after AssetRepo.Create succeeds and before CreateOnboardingMetadata.
// The implicit external account auto-created later in the same use case is
// internal plumbing and does NOT generate a separate account.created event —
// it goes through AccountRepo directly, not through UseCase.CreateAccount.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var AssetCreatedDefinition = Definition{
	ResourceType:  "asset",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// AssetStatusPayload mirrors mmodel.Status for asset events without
// embedding domain types directly into the wire contract. Description is
// optional and omitted when nil.
type AssetStatusPayload struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// AssetCreatedPayload is the wire payload for asset.created.
type AssetCreatedPayload struct {
	ID             string             `json:"id"`
	OrganizationID string             `json:"organizationId"`
	LedgerID       string             `json:"ledgerId"`
	Name           string             `json:"name"`
	Type           string             `json:"type"`
	Code           string             `json:"code"`
	Status         AssetStatusPayload `json:"status"`
	CreatedAt      string             `json:"createdAt"`
	UpdatedAt      string             `json:"updatedAt"`
}

// NewAssetCreated maps a persisted asset into the wire payload.
//
// Caller invariant: a must be the value returned by AssetRepo.Create
// (post-commit), not the input struct. Specifically a.ID, a.CreatedAt,
// and a.UpdatedAt must reflect the persisted state.
func NewAssetCreated(a *mmodel.Asset) AssetCreatedPayload {
	return AssetCreatedPayload{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		LedgerID:       a.LedgerID,
		Name:           a.Name,
		Type:           a.Type,
		Code:           a.Code,
		Status:         newAssetStatusPayload(a.Status),
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p AssetCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AssetCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AssetCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

func newAssetStatusPayload(status mmodel.Status) AssetStatusPayload {
	return AssetStatusPayload{
		Code:        status.Code,
		Description: status.Description,
	}
}
