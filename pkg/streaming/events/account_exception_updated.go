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

// AccountExceptionUpdatedDefinition is the routing contract for account_exception.updated.
// Emission anchor: components/ledger/internal/services/command/update_account_exception.go,
// immediately after AccountExceptionRepo.Update succeeds.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var AccountExceptionUpdatedDefinition = Definition{
	ResourceType:  "account_exception",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// AccountExceptionUpdatedPayload is the wire payload for account_exception.updated.
// It mirrors the created payload so consumers materialize the full post-update state
// from a single event.
type AccountExceptionUpdatedPayload struct {
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

// NewAccountExceptionUpdated maps a persisted account exception into the wire payload.
//
// Caller invariant: e must be the value returned by AccountExceptionRepo.Update
// (post-commit), so e.UpdatedAt and the persisted fields reflect the stored row.
func NewAccountExceptionUpdated(e *mmodel.AccountException) AccountExceptionUpdatedPayload {
	return AccountExceptionUpdatedPayload{
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
func (p AccountExceptionUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountExceptionUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountExceptionUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
