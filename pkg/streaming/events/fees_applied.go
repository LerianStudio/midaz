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

// FeesAppliedDefinition is the routing contract for fees.applied.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var FeesAppliedDefinition = Definition{
	ResourceType:  "fees",
	EventType:     "applied",
	SchemaVersion: "1.0.0",
}

// FeesAppliedPayload is the wire payload for fees.applied. Only the transaction
// identity, org/ledger scope, the applied fee package reference, and the
// application timestamp cross the wire. Monetary and detail surface (amounts,
// asset codes, source/destination, operations, metadata, fee lines,
// waivedAccounts) is DELIBERATELY ABSENT. The JSONShape test locks both the
// present key set and the absence of every excluded key.
type FeesAppliedPayload struct {
	TransactionID  string `json:"transactionId"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	FeePackageID   string `json:"feePackageId"`

	// RFC3339-formatted timestamp of when fees were applied.
	AppliedAt string `json:"appliedAt"`
}

// NewFeesApplied maps identifiers and the application timestamp into the wire
// payload. Params are primitives so this shared package never imports the
// internal fees domain.
func NewFeesApplied(transactionID, organizationID, ledgerID, feePackageID string, appliedAt time.Time) FeesAppliedPayload {
	return FeesAppliedPayload{
		TransactionID:  transactionID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		FeePackageID:   feePackageID,
		AppliedAt:      appliedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
func (p FeesAppliedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", FeesAppliedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: FeesAppliedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.TransactionID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
