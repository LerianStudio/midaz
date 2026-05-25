// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

type RegistryStatus string

type ProtectionModel string

const (
	RegistryStatusLegacy            RegistryStatus = "legacy"
	RegistryStatusPendingMigration  RegistryStatus = "pending_migration"
	RegistryStatusActive            RegistryStatus = "active"
	RegistryStatusPartiallyMigrated RegistryStatus = "partially_migrated"
	RegistryStatusMigrationComplete RegistryStatus = "migration_complete"
	RegistryStatusFailed            RegistryStatus = "failed"
	RegistryStatusBlocked           RegistryStatus = "blocked"
)

const (
	ProtectionModelLegacy   ProtectionModel = "legacy"
	ProtectionModelEnvelope ProtectionModel = "envelope"
)

// Re-export errors from constant package for backward compatibility.
// Callers should migrate to using constant.ErrRegistry* directly.
var (
	ErrRegistryRevisionConflict = constant.ErrRegistryRevisionConflict
	ErrRegistryNotFound         = constant.ErrRegistryNotFound
	ErrRegistryAlreadyExists    = constant.ErrRegistryAlreadyExists
)

// OrganizationRegistryRecord tracks the encryption state of an organization.
type OrganizationRegistryRecord struct {
	TenantID             string
	OrganizationID       string
	Status               RegistryStatus
	ProtectionModel      ProtectionModel
	CurrentVersion       int
	ReadableVersions     []int
	Revision             int64
	LegacyReadable       bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CreatedBy            string
	UpdatedBy            string
	LastTransitionReason string
}

// NewOrganizationRegistryRecord creates a new registry record with initial state.
func NewOrganizationRegistryRecord(tenantID, organizationID, actor, reason string) (*OrganizationRegistryRecord, error) {
	tenantID = strings.TrimSpace(tenantID)
	organizationID = strings.TrimSpace(organizationID)
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)

	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	if organizationID == "" {
		return nil, fmt.Errorf("organization_id is required")
	}

	if actor == "" {
		return nil, fmt.Errorf("actor is required")
	}

	if reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	now := time.Now().UTC()

	return &OrganizationRegistryRecord{
		TenantID:             tenantID,
		OrganizationID:       organizationID,
		Status:               RegistryStatusPendingMigration,
		ProtectionModel:      ProtectionModelLegacy,
		Revision:             1,
		LegacyReadable:       true,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            actor,
		UpdatedBy:            actor,
		LastTransitionReason: reason,
	}, nil
}

// Activate transitions the organization to envelope encryption mode.
// Only records in pending_migration status can be activated.
func (r *OrganizationRegistryRecord) Activate(expectedRevision int64, actor, reason string) error {
	if r.Status != RegistryStatusPendingMigration {
		return fmt.Errorf("%w: cannot activate from status %s, only pending_migration can be activated",
			constant.ErrRegistryInvalidTransition, r.Status)
	}

	if r.Revision != expectedRevision {
		return ErrRegistryRevisionConflict
	}

	r.Status = RegistryStatusActive
	r.ProtectionModel = ProtectionModelEnvelope
	r.CurrentVersion = 1
	r.ReadableVersions = []int{1}
	r.Revision++
	r.UpdatedAt = time.Now().UTC()
	r.UpdatedBy = strings.TrimSpace(actor)
	r.LastTransitionReason = strings.TrimSpace(reason)

	return nil
}

// UsesEnvelopeMode returns true if organization uses envelope encryption.
func (r *OrganizationRegistryRecord) UsesEnvelopeMode() bool {
	return r.ProtectionModel == ProtectionModelEnvelope
}

// CanReadLegacy returns true if legacy-encrypted data can still be read.
func (r *OrganizationRegistryRecord) CanReadLegacy() bool {
	return r.LegacyReadable
}

// CurrentWriteKeysetVersion returns the keyset version for new encryptions.
func (r *OrganizationRegistryRecord) CurrentWriteKeysetVersion() int {
	return r.CurrentVersion
}

// ReadableKeysetVersions returns all keyset versions that can be decrypted.
func (r *OrganizationRegistryRecord) ReadableKeysetVersions() []int {
	versions := make([]int, len(r.ReadableVersions))
	copy(versions, r.ReadableVersions)

	return versions
}
