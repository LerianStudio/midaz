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

// FeesBillingPackageCreatedDefinition is the routing contract for
// fee-billing-packages.created. IMPORTANT posture: emit failures MUST NOT fail
// the request.
var FeesBillingPackageCreatedDefinition = Definition{
	ResourceType:  "fee-billing-packages",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// FeesBillingPackageCreatedPayload is the wire payload for
// fee-billing-packages.created. Only stable identifiers, the org/ledger scope,
// the type/pricing/count classification, the enable flag, and timestamps cross
// the wire. Fee-detail surface (label, description, assetCode, feeAmount, tiers,
// discountTiers, freeQuota, eventFilter, accountTarget, account aliases) is
// DELIBERATELY ABSENT. Fields are typed independently of model.BillingPackage
// so domain evolution does not silently shift the wire contract.
type FeesBillingPackageCreatedPayload struct {
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

// NewFeesBillingPackageCreated maps identifiers and classifications into the
// wire payload. Params are primitives so this shared package never imports the
// internal fees domain. Timestamps are already RFC3339 strings on the domain
// model and pass through unchanged.
func NewFeesBillingPackageCreated(id, organizationID, ledgerID, typ string, pricingModel, countMode *string, enable bool, createdAt, updatedAt string) FeesBillingPackageCreatedPayload {
	return FeesBillingPackageCreatedPayload{
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
func (p FeesBillingPackageCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesBillingPackageCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesBillingPackageCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
