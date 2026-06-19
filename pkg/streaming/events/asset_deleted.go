// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
)

// AssetDeletedDefinition is the routing contract for asset.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_asset.go,
// immediately after AssetRepo.Delete succeeds (post-commit). The
// cascade-delete of the implicit external account earlier in the same
// use case is internal plumbing and does NOT produce a separate
// account.deleted event — it goes through AccountRepo directly, not
// through UseCase.DeleteAccountByID. The user-visible fact is the asset
// removal.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var AssetDeletedDefinition = Definition{
	ResourceType:  "asset",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// AssetDeletedPayload is the wire payload for asset.deleted. Kept
// intentionally minimal: identity, tenant scope (org/ledger), and the
// soft-delete timestamp.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type AssetDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewAssetDeleted maps the asset identity and post-commit deletedAt
// timestamp into the wire payload. The use case does not return the
// persisted struct on delete, so the caller captures deletedAt at the
// emit site.
func NewAssetDeleted(id, organizationID, ledgerID string, deletedAt time.Time) AssetDeletedPayload {
	return AssetDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p AssetDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AssetDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AssetDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
