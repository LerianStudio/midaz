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

// BalanceDeletedDefinition is the routing contract for balance.deleted.
//
// Emission anchor: components/ledger/internal/services/command/delete_balance.go,
// immediately after BalanceRepo.Delete succeeds in UseCase.DeleteBalance
// (the explicit DELETE .../balances/:balance_id endpoint).
//
// Suppressed paths:
//   - Cascade delete via account.deleted (DeleteAllByIDs): account
//     deletion already fans out the parent signal; emitting per-balance
//     events from the cascade would multiply traffic for a single
//     user-visible action. Consumers reconcile balances from
//     account.deleted.
//   - Internal-scope balance deletion is impossible by API contract
//     (rejected by the scope guard before reaching BalanceRepo.Delete).
//   - Balances with non-zero Available or OnHold cannot be deleted
//     (rejected by the funds-check before reaching BalanceRepo.Delete).
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var BalanceDeletedDefinition = Definition{
	ResourceType:  "balance",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// BalanceDeletedPayload is the wire payload for balance.deleted.
// Kept intentionally minimal: identity, tenant scope (org/ledger/account),
// and the soft-delete timestamp.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type BalanceDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	AccountID      string `json:"accountId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewBalanceDeleted maps the deleted balance's identity plus a
// post-commit deletedAt timestamp into the wire payload. The use case
// captures deletedAt at the emit site (wall-clock NOW(), matching the
// SQL UPDATE balance SET deleted_at = NOW() the repo executes).
func NewBalanceDeleted(id, organizationID, ledgerID, accountID string, deletedAt time.Time) BalanceDeletedPayload {
	return BalanceDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p BalanceDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", BalanceDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: BalanceDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
