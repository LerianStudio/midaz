// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// ProtectionState contains the resolved encryption state for an organization.
type ProtectionState struct {
	// Mode indicates whether to use legacy or envelope encryption for new writes.
	Mode crypto.EncryptionMode
	// CanReadLegacy indicates whether legacy-encrypted data can be decrypted.
	CanReadLegacy bool
	// CurrentKeysetVersion is the keyset version for new encryptions (0 for legacy mode).
	CurrentKeysetVersion int
	// OrganizationID is the resolved organization.
	OrganizationID string
	// TenantID is the resolved tenant (from registry record).
	TenantID string
}

// MustUseEnvelope returns true if the organization MUST use envelope encryption for new writes.
func (ps ProtectionState) MustUseEnvelope() bool {
	return ps.Mode == crypto.EncryptionModeEnvelope
}

// ProtectionStateResolver determines the encryption mode for an organization
// based on its registry state.
type ProtectionStateResolver struct {
	registryRepo mongoEncryption.RegistryRepository
}

// NewProtectionStateResolver creates a new resolver with the given registry repository.
func NewProtectionStateResolver(registryRepo mongoEncryption.RegistryRepository) *ProtectionStateResolver {
	return &ProtectionStateResolver{
		registryRepo: registryRepo,
	}
}

// Resolve determines the protection state for an organization.
//
// Returns ProtectionState with Mode=Legacy if:
//   - Registry record not found (organization not provisioned)
//
// Returns ProtectionState with Mode=Envelope if:
//   - Registry record exists with "active" status
//
// Returns error if:
//   - Repository returns an unexpected error
//   - Reader is nil
func (r *ProtectionStateResolver) Resolve(ctx context.Context, organizationID string) (ProtectionState, error) {
	// Nil registry repository indicates KMS_VENDOR=none (legacy-only mode).
	// Return legacy readable state to allow legacy encryption without envelope.
	if r.registryRepo == nil {
		return ProtectionState{
			Mode:                 crypto.EncryptionModeLegacy,
			CanReadLegacy:        true,
			CurrentKeysetVersion: 0,
			OrganizationID:       organizationID,
			TenantID:             "",
		}, nil
	}

	record, err := r.registryRepo.Get(ctx, organizationID)
	if err != nil {
		if errors.Is(err, constant.ErrRegistryNotFound) {
			// Organization hasn't been provisioned for envelope encryption yet.
			// Default to legacy mode with legacy readable.
			return ProtectionState{
				Mode:                 crypto.EncryptionModeLegacy,
				CanReadLegacy:        true,
				CurrentKeysetVersion: 0,
				OrganizationID:       organizationID,
				TenantID:             "",
			}, nil
		}

		return ProtectionState{}, err
	}

	return r.resolveFromRecord(record)
}

// resolveFromRecord maps a registry record to a ProtectionState.
// Returns legacy state if record is nil (organization not provisioned).
func (r *ProtectionStateResolver) resolveFromRecord(record *mmodel.OrganizationRegistryRecord) (ProtectionState, error) {
	// Guard against nil record (repository returned nil without error)
	if record == nil {
		return ProtectionState{
			Mode:                 crypto.EncryptionModeLegacy,
			CanReadLegacy:        true,
			CurrentKeysetVersion: 0,
			OrganizationID:       "",
			TenantID:             "",
		}, nil
	}

	// Registry record exists → organization is provisioned for envelope encryption
	if record.Status == mmodel.RegistryStatusActive {
		return ProtectionState{
			Mode:                 crypto.EncryptionModeEnvelope,
			CanReadLegacy:        record.LegacyReadable,
			CurrentKeysetVersion: record.CurrentVersion,
			OrganizationID:       record.OrganizationID,
			TenantID:             record.TenantID,
		}, nil
	}

	// Unknown status - treat as error to avoid silent misconfiguration
	return ProtectionState{}, fmt.Errorf("unknown registry status: %s", record.Status)
}
