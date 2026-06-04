// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// midazNamespace is the deterministic UUIDv5 namespace used to derive a stable
// self-holder ID from an organization ID. It must never change: the same org ID
// must always resolve to the same self-holder ID, on the create path, the
// eager-provision path, and the backfill runner. It is shared by the create-account
// default materialisation and the org-create self-holder provisioning.
var midazNamespace = uuid.MustParse("8d9e2c1b-3a47-5f60-9c8b-1d2e3f405162")

// HolderReader is the narrow port the create path uses to assert a holder exists.
//
// It is defined here, in the command package, on purpose: command must not import
// components/crm (dependency-inward). The port is org-scoped — ownership is
// org-global, not ledger-scoped, matching the per-org holder collection. The
// implementation (an adapter over the CRM holder service, wired at bootstrap)
// passes the org ID through to the repository; this contract hides the repository's
// misleadingly-named collection parameter.
type HolderReader interface {
	// Exists reports whether a holder with id exists within the organization.
	Exists(ctx context.Context, organizationID string, id uuid.UUID) (bool, error)
}

// SettingsReader is the narrow port the create path uses to read cached, parsed
// ledger settings without importing the query package's concrete UseCase type.
//
// It mirrors query.GetParsedLedgerSettings so the bootstrap adapter is a thin
// pass-through, keeping the RequireHolder read on the cached path rather than the
// uncached LedgerRepo.GetSettings used by applyAccountingValidations.
type SettingsReader interface {
	// GetParsedLedgerSettings returns the parsed, cached settings for a ledger.
	GetParsedLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (mmodel.LedgerSettings, error)
}

// HolderProvisioner is the narrow port the org-create path uses to provision the
// deterministic self-holder. It is satisfied at bootstrap by the CRM holder
// service's CreateHolderWithID, which treats a duplicate deterministic ID as
// idempotent success — so command never imports components/crm.
type HolderProvisioner interface {
	// CreateHolderWithID provisions a holder with a caller-supplied deterministic ID.
	CreateHolderWithID(ctx context.Context, organizationID string, id uuid.UUID, chi *mmodel.CreateHolderInput) (*mmodel.Holder, error)
}

// deriveSelfHolderID computes the deterministic self-holder ID for an organization.
// The derivation is pure (no I/O), so the create hot path can materialise the
// default holder_id without a Mongo lookup.
func deriveSelfHolderID(organizationID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(midazNamespace, organizationID[:])
}
