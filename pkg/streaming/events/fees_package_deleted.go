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

// FeesPackageDeletedDefinition is the routing contract for fees-package.deleted.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var FeesPackageDeletedDefinition = Definition{
	ResourceType:  "fees-package",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// FeesPackageDeletedPayload is the wire payload for fees-package.deleted. Kept
// minimal: identity, org/ledger scope, and the deletion timestamp. Fee-detail
// surface never crosses the wire.
//
// Idempotency hint for consumers: id + deletedAt is unique per deletion.
type FeesPackageDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`

	// RFC3339-formatted deletion timestamp.
	DeletedAt string `json:"deletedAt"`
}

// NewFeesPackageDeleted maps identifiers and the post-commit deletion timestamp
// into the wire payload. Params are primitives so this shared package never
// imports the internal fees domain.
func NewFeesPackageDeleted(id, organizationID, ledgerID string, deletedAt time.Time) FeesPackageDeletedPayload {
	return FeesPackageDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
func (p FeesPackageDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesPackageDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesPackageDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
