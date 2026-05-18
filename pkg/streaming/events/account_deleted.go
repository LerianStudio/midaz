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

// AccountDeletedDefinition is the routing contract for account.deleted.
// Emission anchor:
// components/ledger/internal/services/command/delete_account.go,
// immediately after AccountRepo.Delete succeeds (post-commit).
// IMPORTANT posture: emit failures MUST NOT fail the request;
// durability is owned by PG + (follow-up task) the outbox subsystem.
//
// External-type accounts cannot be deleted and never reach this anchor.
// The cascade DeleteAllBalancesByAccountID step earlier in the use case
// is internal cleanup — it does NOT generate per-balance balance.deleted
// events. The user-visible fact is the account removal.
var AccountDeletedDefinition = Definition{
	ResourceType:  "account",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// AccountDeletedPayload is the wire payload for account.deleted. Kept
// intentionally minimal: identity, tenant scope (org/ledger), portfolio
// reference (when present), and the soft-delete timestamp.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type AccountDeletedPayload struct {
	// Required core identity fields.
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`

	// Nullable reference — encoded as JSON null when the account is
	// not portfolio-scoped.
	PortfolioID *string `json:"portfolioId"`

	// RFC3339-formatted timestamp of the soft-delete. Producer-side
	// time.Now() captured at the emit site (post-commit) — PG's
	// deleted_at column is populated by the same wall clock, so the
	// values are effectively the same up to clock skew.
	DeletedAt string `json:"deletedAt"`
}

// NewAccountDeleted maps the pre-delete account record + the post-commit
// deletedAt timestamp into the wire payload. The JSONShape test
// (account_deleted_test.go) locks the field count.
//
// Caller invariant: acc must be the pre-delete record (the value
// returned by AccountRepo.Find before AccountRepo.Delete ran).
// deletedAt is the wall-clock instant captured by the emit site after
// the Delete call succeeded.
func NewAccountDeleted(acc *mmodel.Account, deletedAt time.Time) AccountDeletedPayload {
	return AccountDeletedPayload{
		ID:             acc.ID,
		OrganizationID: acc.OrganizationID,
		LedgerID:       acc.LedgerID,
		PortfolioID:    acc.PortfolioID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. tenantID comes from pkgStreaming.ResolveTenantID(ctx); ts is
// the timestamp lib-streaming stamps on the ce-time header — typically
// the same wall-clock instant that was passed into NewAccountDeleted as
// deletedAt.
//
// Source, ResourceType, EventType, and SchemaVersion are NOT carried on
// the request. Source flows from the Builder at construction time; the
// other three resolve from the Catalog by
// DefinitionKey at emit time.
//
// Returns a wrapped json.Marshal error so callers can decide whether to
// log Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p AccountDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
