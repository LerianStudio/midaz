// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// AccountCreatedDefinition is the routing contract for account.created.
// Emission anchor:
// components/ledger/internal/services/command/create_account.go,
// between the default-balance success branch (post-commit, past the
// compensating-delete window) and the CreateOnboardingMetadata call
// (pre-metadata-write, so a Mongo metadata failure cannot block the
// event). IMPORTANT posture: emit failures MUST NOT fail the request;
// durability is owned by PG + (follow-up task) the outbox subsystem.
var AccountCreatedDefinition = Definition{
	ResourceType:  "account",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// AccountCreatedPayload is the wire payload for account.created. This
// struct is the canonical contract; consumers and tests read it as the
// source of truth.
//
// Fields are intentionally typed independently of mmodel.Account so
// domain evolution does not silently shift the wire contract — e.g. if
// mmodel.Account grows a new internal-only field, it does NOT leak onto
// the wire until a new field is explicitly added here.
//
// Optional references use *string so JSON null (absent on this code
// path) is distinguishable from an empty string.
type AccountCreatedPayload struct {
	// Required core identity fields.
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	Name           string `json:"name"`
	AssetCode      string `json:"assetCode"`
	Type           string `json:"type"`

	// Nullable references — encoded as JSON null when unset.
	PortfolioID     *string `json:"portfolioId"`
	SegmentID       *string `json:"segmentId"`
	ParentAccountID *string `json:"parentAccountId"`
	EntityID        *string `json:"entityId"`
	HolderID        *string `json:"holderId"`
	Alias           *string `json:"alias"`

	// Status block. Decoupled from mmodel.Status so domain-side fields
	// (e.g. internal-only flags introduced later) do not leak onto the
	// wire until explicitly mirrored here.
	Status AccountStatusPayload `json:"status"`

	// Blocked is a pointer so absence is distinguishable from explicit
	// false. Contract emits the field with value false when unset on
	// the request.
	Blocked *bool `json:"blocked"`

	// RFC3339-formatted timestamps. Producer formats once at the emit
	// site; consumers parse with time.Parse(time.RFC3339, ...).
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// AccountStatusPayload is the nested status object on account payloads.
// Code is mandatory; Description is optional and omitted when nil.
type AccountStatusPayload struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// NewAccountCreated maps a persisted account into the wire payload.
// Every field declared on AccountCreatedPayload must be assigned here;
// the JSONShape test (account_created_test.go) locks the field count so
// additions and deletions surface at test time.
//
// Caller invariant: acc must be the value returned by AccountRepo.Create
// (post-commit), not the input struct. Specifically acc.ID, acc.CreatedAt,
// and acc.UpdatedAt must reflect the persisted state.
func NewAccountCreated(acc *mmodel.Account) AccountCreatedPayload {
	return AccountCreatedPayload{
		ID:              acc.ID,
		OrganizationID:  acc.OrganizationID,
		LedgerID:        acc.LedgerID,
		Name:            acc.Name,
		AssetCode:       acc.AssetCode,
		Type:            acc.Type,
		PortfolioID:     acc.PortfolioID,
		SegmentID:       acc.SegmentID,
		ParentAccountID: acc.ParentAccountID,
		EntityID:        acc.EntityID,
		HolderID:        acc.HolderID,
		Alias:           acc.Alias,
		Status: AccountStatusPayload{
			Code:        acc.Status.Code,
			Description: acc.Status.Description,
		},
		Blocked:   acc.Blocked,
		CreatedAt: acc.CreatedAt.Format(time.RFC3339),
		UpdatedAt: acc.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. tenantID comes from pkgStreaming.ResolveTenantID(ctx); ts is
// the timestamp lib-streaming stamps on the ce-time header — typically
// the persisted CreatedAt for "created" events.
//
// Source, ResourceType, EventType, and SchemaVersion are NOT carried on
// the request. Source flows from the Builder at construction time; the
// other three resolve from the Catalog by
// DefinitionKey at emit time.
//
// Returns a wrapped json.Marshal error so callers can decide whether to
// log Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p AccountCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AccountCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AccountCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
