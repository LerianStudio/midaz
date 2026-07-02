// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// FeesBillingPackageUpdatedDefinition is the routing contract for
// fees-billing-package.updated. IMPORTANT posture: emit failures MUST NOT fail
// the request.
var FeesBillingPackageUpdatedDefinition = Definition{
	ResourceType:  "fees-billing-package",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// FeesBillingPackageUpdatedPayload is the wire payload for
// fees-billing-package.updated. It mirrors the created payload: only
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

// NewFeesBillingPackageUpdated maps a persisted billing package into the wire
// payload. Enable resolves nil to false. Timestamps are already RFC3339 strings
// on the domain model and pass through unchanged.
func NewFeesBillingPackageUpdated(bp *model.BillingPackage) FeesBillingPackageUpdatedPayload {
	return FeesBillingPackageUpdatedPayload{
		ID:             bp.ID,
		OrganizationID: bp.OrganizationID,
		LedgerID:       bp.LedgerID,
		Type:           bp.Type,
		PricingModel:   bp.PricingModel,
		CountMode:      bp.CountMode,
		Enable:         bp.Enable != nil && *bp.Enable,
		CreatedAt:      bp.CreatedAt,
		UpdatedAt:      bp.UpdatedAt,
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
