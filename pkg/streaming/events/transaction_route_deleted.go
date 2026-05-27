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

// TransactionRouteDeletedDefinition is the routing contract for transaction-route.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_transaction_route.go,
// immediately after TransactionRouteRepo.Delete succeeds (post-commit).
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var TransactionRouteDeletedDefinition = Definition{
	ResourceType:  "transaction-route",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// TransactionRouteDeletedPayload is the wire payload for transaction-route.deleted.
// Kept intentionally minimal: identity, tenant scope (org/ledger), and
// the soft-delete timestamp. The cascade soft-delete of the
// operation_transaction_route relations earlier in the use case is
// internal cleanup and does NOT generate per-relation events on this
// code path — consumers infer the link removal from the deleted-fact.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type TransactionRouteDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewTransactionRouteDeleted maps the transaction route identity and
// post-commit deletedAt timestamp into the wire payload. The use case
// does not return the persisted struct on delete, so the caller
// captures deletedAt at the emit site.
func NewTransactionRouteDeleted(id, organizationID, ledgerID string, deletedAt time.Time) TransactionRouteDeletedPayload {
	return TransactionRouteDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p TransactionRouteDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", TransactionRouteDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: TransactionRouteDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
