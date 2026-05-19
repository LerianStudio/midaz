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

// AccountTypeCreatedDefinition is the routing contract for account-type.created.
// Emission anchor: components/ledger/internal/services/command/create_account_type.go,
// immediately after AccountTypeRepo.Create succeeds and before CreateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var AccountTypeCreatedDefinition = Definition{
	ResourceType:  "account-type",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// AccountTypeCreatedPayload is the wire payload for account-type.created.
// Description is optional and omitted from the wire when empty.
type AccountTypeCreatedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	KeyValue       string `json:"keyValue"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// NewAccountTypeCreated maps a persisted account type into the wire
// payload.
//
// Caller invariant: a must be the value returned by AccountTypeRepo.Create
// (post-commit), not the input struct. Specifically a.ID, a.CreatedAt,
// and a.UpdatedAt must reflect the persisted state.
func NewAccountTypeCreated(a *mmodel.AccountType) AccountTypeCreatedPayload {
	return AccountTypeCreatedPayload{
		ID:             a.ID.String(),
		OrganizationID: a.OrganizationID.String(),
		LedgerID:       a.LedgerID.String(),
		Name:           a.Name,
		Description:    a.Description,
		KeyValue:       a.KeyValue,
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p AccountTypeCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountTypeCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountTypeCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
