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

// FeesBillingPackageDeletedDefinition is the routing contract for
// fee-billing-packages.deleted. IMPORTANT posture: emit failures MUST NOT fail
// the request.
var FeesBillingPackageDeletedDefinition = Definition{
	ResourceType:  "fee-billing-packages",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// FeesBillingPackageDeletedPayload is the wire payload for
// fee-billing-packages.deleted. Only identifiers, scope, and the deletion
// timestamp cross the wire.
type FeesBillingPackageDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`

	// DeletedAt is the RFC3339-formatted deletion timestamp.
	DeletedAt string `json:"deletedAt"`
}

// NewFeesBillingPackageDeleted maps identifiers and the deletion timestamp into
// the wire payload. Primitives are taken directly because a delete path may not
// carry the full domain object.
func NewFeesBillingPackageDeleted(id, organizationID, ledgerID string, deletedAt time.Time) FeesBillingPackageDeletedPayload {
	return FeesBillingPackageDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
func (p FeesBillingPackageDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesBillingPackageDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesBillingPackageDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
