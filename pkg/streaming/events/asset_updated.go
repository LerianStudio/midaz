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

// AssetUpdatedDefinition is the routing contract for asset.updated.
// Emission anchor: components/ledger/internal/services/command/update_asset.go,
// immediately after AssetRepo.Update succeeds and before UpdateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
var AssetUpdatedDefinition = Definition{
	ResourceType:  "asset",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// AssetUpdatedPayload is the wire payload for asset.updated. Type and
// Code are immutable post-create but are mirrored here so the wire
// payload is a complete identity snapshot and consumers don't need to
// join against asset.created to render the row.
type AssetUpdatedPayload struct {
	ID             string             `json:"id"`
	OrganizationID string             `json:"organizationId"`
	LedgerID       string             `json:"ledgerId"`
	Name           string             `json:"name"`
	Type           string             `json:"type"`
	Code           string             `json:"code"`
	Status         AssetStatusPayload `json:"status"`
	UpdatedAt      string             `json:"updatedAt"`
}

// NewAssetUpdated maps the post-update asset record into the wire payload.
//
// Caller invariant: a must be the value returned by AssetRepo.Update
// (post-commit), not the input struct. Specifically a.UpdatedAt must
// reflect the persisted timestamp.
func NewAssetUpdated(a *mmodel.Asset) AssetUpdatedPayload {
	return AssetUpdatedPayload{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		LedgerID:       a.LedgerID,
		Name:           a.Name,
		Type:           a.Type,
		Code:           a.Code,
		Status:         newAssetStatusPayload(a.Status),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p AssetUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AssetUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AssetUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
