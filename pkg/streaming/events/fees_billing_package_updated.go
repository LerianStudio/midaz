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

// FeesBillingPackageUpdatedDefinition is the routing contract for
// fee-billing-packages.updated. IMPORTANT posture: emit failures MUST NOT fail
// the request.
var FeesBillingPackageUpdatedDefinition = Definition{
	ResourceType:  "fee-billing-packages",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// FeesBillingPackageUpdatedPayload is the wire payload for
// fee-billing-packages.updated. It mirrors the created payload: only
// identifiers, scope, classifications, the enable flag, and timestamps cross
// the wire. Fee-detail surface is DELIBERATELY ABSENT.
type FeesBillingPackageUpdatedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`

	// Type is the package classification: "volume" or "maintenance".
	Type string `json:"type"`

	// Nullable classifications — encoded as JSON null when unset.
	PricingModel *string `json:"pricingModel"`
	CountMode    *string `json:"countMode"`

	Enable bool `json:"enable"`

	// RFC3339-formatted timestamps.
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// NewFeesBillingPackageUpdated maps identifiers and classifications into the
// wire payload. Params are primitives so this shared package never imports the
// internal fees domain. Timestamps are already RFC3339 strings on the domain
// model and pass through unchanged.
func NewFeesBillingPackageUpdated(id, organizationID, ledgerID, typ string, pricingModel, countMode *string, enable bool, createdAt, updatedAt string) FeesBillingPackageUpdatedPayload {
	return FeesBillingPackageUpdatedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Type:           typ,
		PricingModel:   pricingModel,
		CountMode:      countMode,
		Enable:         enable,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
func (p FeesBillingPackageUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesBillingPackageUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesBillingPackageUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
