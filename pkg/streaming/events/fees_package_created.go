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

// FeesPackageCreatedDefinition is the routing contract for fees-package.created.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var FeesPackageCreatedDefinition = Definition{
	ResourceType:  "fees-package",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// FeesPackageCreatedPayload is the wire payload for fees-package.created. Only
// stable identifiers, the org/ledger scope, the segment/route classification,
// the enable flag, and timestamps cross the wire. Fee-detail surface
// (feeGroupLabel, description, minimum/maximum amount, fees, waivedAccounts) is
// DELIBERATELY ABSENT. The JSONShape test locks both the present key set and the
// absence of every excluded key.
type FeesPackageCreatedPayload struct {
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

// NewFeesPackageCreated maps identifiers and classifications into the wire
// payload. Params are primitives so this shared package never imports the
// internal fees domain.
func NewFeesPackageCreated(id, organizationID, ledgerID string, segmentID, transactionRoute *string, enable bool, createdAt, updatedAt time.Time) FeesPackageCreatedPayload {
	return FeesPackageCreatedPayload{
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
func (p FeesPackageCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesPackageCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesPackageCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
