// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// AccountExceptionCreatedDefinition is the routing contract for account_exception.created.
// Emission anchor: components/ledger/internal/services/command/create_account_exception.go,
// immediately after AccountExceptionRepo.Create succeeds.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Note on resource type: the canonical wire name (`account_exception`) diverges
// from the JSON entity name (`AccountException`) and the HTTP route segment
// (`account-exceptions`), matching the operation_route convention.
var AccountExceptionCreatedDefinition = Definition{
	ResourceType:  "account_exception",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// AccountExceptionCreatedPayload is the wire payload for account_exception.created.
//
// Optional fields (BalanceKey, EffectiveAt, ExpiresAt) use omitempty to mirror the
// HTTP response contract: an exception with no balance restriction or validity bound
// does not leak an empty string or a nil pointer onto the wire.
type AccountExceptionCreatedPayload struct {
	ID                   string   `json:"id"`
	OrganizationID       string   `json:"organizationId"`
	LedgerID             string   `json:"ledgerId"`
	AccountID            string   `json:"accountId"`
	OperationalTypeCodes []string `json:"operationalTypeCodes"`
	BalanceKey           *string  `json:"balanceKey,omitempty"`
	Context              string   `json:"context"`
	EffectiveAt          *string  `json:"effectiveAt,omitempty"`
	ExpiresAt            *string  `json:"expiresAt,omitempty"`
	CreatedAt            string   `json:"createdAt"`
	UpdatedAt            string   `json:"updatedAt"`
}

// NewAccountExceptionCreated maps a persisted account exception into the wire payload.
//
// Caller invariant: e must be the value returned by AccountExceptionRepo.Create
// (post-commit), so e.ID, e.CreatedAt and e.UpdatedAt reflect the stored row.
func NewAccountExceptionCreated(e *mmodel.AccountException) AccountExceptionCreatedPayload {
	return AccountExceptionCreatedPayload{
		ID:                   e.ID,
		OrganizationID:       e.OrganizationID,
		LedgerID:             e.LedgerID,
		AccountID:            e.AccountID,
		OperationalTypeCodes: e.OperationalTypeCodes,
		BalanceKey:           e.BalanceKey,
		Context:              e.Context,
		EffectiveAt:          formatOptionalTime(e.EffectiveAt),
		ExpiresAt:            formatOptionalTime(e.ExpiresAt),
		CreatedAt:            e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            e.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter. Source,
// ResourceType, EventType, and SchemaVersion live in the Catalog under DefinitionKey.
func (p AccountExceptionCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountExceptionCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountExceptionCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

// formatOptionalTime renders a nullable timestamp as an RFC3339 string pointer, or nil
// when the source is nil, so omitempty keeps unbounded windows off the wire.
func formatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}

	formatted := t.Format(time.RFC3339)

	return &formatted
}
