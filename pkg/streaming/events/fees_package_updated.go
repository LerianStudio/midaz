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

// FeesPackageUpdatedDefinition is the routing contract for fee-packages.updated.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var FeesPackageUpdatedDefinition = Definition{
	ResourceType:  "fee-packages",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// FeesPackageUpdatedPayload is the wire payload for fee-packages.updated. Only
// stable identifiers, the org/ledger scope, the segment/route classification,
// the enable flag, and timestamps cross the wire. Fee-detail surface
// (feeGroupLabel, description, minimum/maximum amount, fees, waivedAccounts) is
// DELIBERATELY ABSENT. The JSONShape test locks both the present key set and the
// absence of every excluded key.
type FeesPackageUpdatedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`

	// Nullable references — encoded as JSON null when unset.
	SegmentID        *string `json:"segmentId"`
	TransactionRoute *string `json:"transactionRoute"`

	Enable bool `json:"enable"`

	// RFC3339-formatted timestamps.
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// NewFeesPackageUpdated maps identifiers and classifications into the wire
// payload. Params are primitives so this shared package never imports the
// internal fees domain.
func NewFeesPackageUpdated(id, organizationID, ledgerID string, segmentID, transactionRoute *string, enable bool, createdAt, updatedAt time.Time) FeesPackageUpdatedPayload {
	return FeesPackageUpdatedPayload{
		ID:               id,
		OrganizationID:   organizationID,
		LedgerID:         ledgerID,
		SegmentID:        segmentID,
		TransactionRoute: transactionRoute,
		Enable:           enable,
		CreatedAt:        createdAt.Format(time.RFC3339),
		UpdatedAt:        updatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
func (p FeesPackageUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesPackageUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesPackageUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
