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

// AccountUpdatedDefinition is the routing contract for account.updated.
// Emission anchor:
// components/ledger/internal/services/command/update_account.go,
// immediately after AccountRepo.Update succeeds (post-commit) and
// before UpdateOnboardingMetadata runs (pre-metadata-write), so a
// downstream Mongo failure cannot mask the event. IMPORTANT posture:
// emit failures MUST NOT fail the request; durability is owned by PG +
// (follow-up task) the outbox subsystem.
//
// External-type accounts cannot be updated and never reach this anchor.
var AccountUpdatedDefinition = Definition{
	ResourceType:  "account",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// AccountUpdatedPayload is the wire payload for account.updated. Like
// AccountCreatedPayload, this struct is the canonical wire contract —
// intentionally typed independently of mmodel.Account so domain growth
// does not silently leak onto the bus.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
type AccountUpdatedPayload struct {
	// Required core identity fields.
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	Name           string `json:"name"`

	// Nullable references — encoded as JSON null when unset.
	PortfolioID *string `json:"portfolioId"`
	SegmentID   *string `json:"segmentId"`
	EntityID    *string `json:"entityId"`

	// Status block. Decoupled from mmodel.Status so domain-side fields
	// (e.g. internal-only flags introduced later) do not leak onto the
	// wire until explicitly mirrored here.
	Status AccountStatusPayload `json:"status"`

	// Blocked is a pointer so absence is distinguishable from explicit
	// false. Contract emits the field with value false when unset on
	// the request.
	Blocked *bool `json:"blocked"`

	// RFC3339-formatted timestamp of the post-update state. Pinned by
	// the persisted UpdatedAt on the row, not by time.Now() at the
	// emit site.
	UpdatedAt string `json:"updatedAt"`
}

// NewAccountUpdated maps the post-update account record into the wire
// payload. The JSONShape test (account_updated_test.go) locks the field
// count so additive/destructive drift surfaces at test time.
//
// Caller invariant: acc must be the value returned by AccountRepo.Update
// (post-commit), not the input struct. Specifically acc.UpdatedAt must
// reflect the persisted timestamp.
func NewAccountUpdated(acc *mmodel.Account) AccountUpdatedPayload {
	return AccountUpdatedPayload{
		ID:             acc.ID,
		OrganizationID: acc.OrganizationID,
		LedgerID:       acc.LedgerID,
		Name:           acc.Name,
		PortfolioID:    acc.PortfolioID,
		SegmentID:      acc.SegmentID,
		EntityID:       acc.EntityID,
		Status: AccountStatusPayload{
			Code:        acc.Status.Code,
			Description: acc.Status.Description,
		},
		Blocked:   acc.Blocked,
		UpdatedAt: acc.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEvent assembles a libStreaming.Event ready for the Emitter. tenantID
// comes from pkgStreaming.ResolveTenantID(ctx); source is the per-component
// CloudEvents ce-source attribute; ts is the timestamp lib-streaming
// stamps on the ce-time header — typically the persisted UpdatedAt for
// "updated" events.
//
// Returns a wrapped json.Marshal error so callers can decide whether to
// log Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p AccountUpdatedPayload) ToEvent(tenantID, source string, ts time.Time) (libStreaming.Event, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.Event{}, fmt.Errorf("marshal %s payload: %w", AccountUpdatedDefinition.Key(), err)
	}

	return libStreaming.Event{
		TenantID:      tenantID,
		Source:        source,
		ResourceType:  AccountUpdatedDefinition.ResourceType,
		EventType:     AccountUpdatedDefinition.EventType,
		SchemaVersion: AccountUpdatedDefinition.SchemaVersion,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
