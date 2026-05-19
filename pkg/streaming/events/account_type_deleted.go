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

// AccountTypeDeletedDefinition is the routing contract for account-type.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_account_type.go,
// immediately after AccountTypeRepo.Delete succeeds (post-commit).
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var AccountTypeDeletedDefinition = Definition{
	ResourceType:  "account-type",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// AccountTypeDeletedPayload is the wire payload for account-type.deleted.
// Kept intentionally minimal: identity, tenant scope (org/ledger), and
// the soft-delete timestamp.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type AccountTypeDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewAccountTypeDeleted maps the account type identity and post-commit
// deletedAt timestamp into the wire payload. The use case does not
// return the persisted struct on delete, so the caller captures
// deletedAt at the emit site.
func NewAccountTypeDeleted(id, organizationID, ledgerID string, deletedAt time.Time) AccountTypeDeletedPayload {
	return AccountTypeDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p AccountTypeDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountTypeDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountTypeDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
