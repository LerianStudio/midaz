// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// AccountTypeUpdatedDefinition is the routing contract for account-type.updated.
// Emission anchor: components/ledger/internal/services/command/update_account_type.go,
// immediately after AccountTypeRepo.Update succeeds and before UpdateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
var AccountTypeUpdatedDefinition = Definition{
	ResourceType:  "account-type",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// AccountTypeUpdatedPayload is the wire payload for account-type.updated.
// The payload carries the full mutable surface (name, description) plus
// the immutable keyValue so consumers don't need to join against
// account-type.created to render the row. CreatedAt is intentionally
// omitted — pinned at create time and not part of the update fact.
//
// Description uses omitempty to mirror the create-time contract;
// a metadata-only or name-only PATCH may leave it empty if the original
// row had no description.
type AccountTypeUpdatedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	KeyValue       string `json:"keyValue"`
	UpdatedAt      string `json:"updatedAt"`
}

// NewAccountTypeUpdated maps the post-update account type record into
// the wire payload.
//
// Caller invariant: a must be the value returned by AccountTypeRepo.Update
// (post-commit), not the input struct. Specifically a.UpdatedAt must
// reflect the persisted timestamp and a.KeyValue must carry the
// preserved create-time value.
func NewAccountTypeUpdated(a *mmodel.AccountType) AccountTypeUpdatedPayload {
	return AccountTypeUpdatedPayload{
		ID:             a.ID.String(),
		OrganizationID: a.OrganizationID.String(),
		LedgerID:       a.LedgerID.String(),
		Name:           a.Name,
		Description:    a.Description,
		KeyValue:       a.KeyValue,
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p AccountTypeUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountTypeUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountTypeUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
