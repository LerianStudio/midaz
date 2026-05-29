// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// Package-level errors for provisioning operations.
var (
	// ErrOrganizationAlreadyProvisioned is returned when attempting to provision
	// an organization that already has a keyset.
	ErrOrganizationAlreadyProvisioned = errors.New("organization already provisioned for envelope encryption")

	// ErrOrganizationNotProvisioned is returned when attempting to activate
	// an organization that has not been provisioned.
	ErrOrganizationNotProvisioned = errors.New("organization not provisioned for envelope encryption")

	// ErrProvisioningFailed is returned when keyset generation or storage fails.
	ErrProvisioningFailed = errors.New("provisioning failed")

	// ErrActivationFailed is returned when registry activation fails.
	ErrActivationFailed = errors.New("activation failed")
)

// KeysetWriter defines the interface for persisting organization keysets.
// Compatible with the KeysetRepository in the MongoDB adapter.
type KeysetWriter interface {
	Save(ctx context.Context, keyset *mmodel.OrganizationKeyset) error
}

// RegistryWriter defines the interface for persisting organization registry records.
// Compatible with the RegistryRepository in the MongoDB adapter.
type RegistryWriter interface {
	Save(ctx context.Context, record *mmodel.OrganizationRegistryRecord) error
	Get(ctx context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error)
	Update(ctx context.Context, record *mmodel.OrganizationRegistryRecord, expectedRevision int64) error
}

// KeysetGenerator defines the interface for generating and wrapping keysets.
// Compatible with pkg/crypto/tink.KeysetFactory.
type KeysetGenerator interface {
	GenerateAEADKeyset(ctx context.Context, keyName string) (KeysetBundle, error)
	GenerateMACKeyset(ctx context.Context, keyName string) (KeysetBundle, error)
}

// KeysetBundle contains a generated keyset with its wrapped form and metadata.
// Mirrors pkg/crypto/tink.KeysetBundle.
type KeysetBundle struct {
	Wrapped   WrappedKeyset
	RawKeyset []byte
}

// WrappedKeyset represents a keyset wrapped by KMS.
// Mirrors pkg/crypto/tink.WrappedKeyset.
type WrappedKeyset struct {
	WrappedData       string
	Info              KeysetInfo
	LegacyKeyImported bool
}

// KeysetInfo contains keyset metadata without key material.
// Mirrors pkg/crypto/tink.KeysetInfo.
type KeysetInfo struct {
	PrimaryKeyID uint32
	Keys         []KeyInfoEntry
}

// KeyInfoEntry describes a single key in the keyset.
type KeyInfoEntry struct {
	KeyID     uint32
	Status    string
	Type      string
	IsPrimary bool
}

// ProvisioningConfig holds configuration for the ProvisioningService.
type ProvisioningConfig struct {
	// KEKMountPath is the KMS mount path (e.g., "transit" for Vault Transit).
	KEKMountPath string
}

// DefaultProvisioningConfig returns the default configuration.
func DefaultProvisioningConfig() ProvisioningConfig {
	return ProvisioningConfig{
		KEKMountPath: "transit",
	}
}

// ProvisioningService handles organization encryption provisioning and activation.
// It coordinates keyset generation, KMS wrapping, and registry state management.
type ProvisioningService struct {
	keysetWriter    KeysetWriter
	registryWriter  RegistryWriter
	keysetGenerator KeysetGenerator
	kekMountPath    string
}

// NewProvisioningService creates a new provisioning service with the given dependencies.
func NewProvisioningService(
	keysetWriter KeysetWriter,
	registryWriter RegistryWriter,
	keysetGenerator KeysetGenerator,
	config ProvisioningConfig,
) *ProvisioningService {
	mountPath := config.KEKMountPath
	if mountPath == "" {
		mountPath = "transit"
	}

	return &ProvisioningService{
		keysetWriter:    keysetWriter,
		registryWriter:  registryWriter,
		keysetGenerator: keysetGenerator,
		kekMountPath:    mountPath,
	}
}

// ProvisionRequest contains the parameters for provisioning an organization.
type ProvisionRequest struct {
	TenantID       string
	OrganizationID string
	Actor          string // Who initiated the provisioning
	Reason         string // Why provisioning was requested
}

// Validate validates the provision request.
func (r ProvisionRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}

	if r.OrganizationID == "" {
		return fmt.Errorf("organization_id is required")
	}

	if r.Actor == "" {
		return fmt.Errorf("actor is required")
	}

	if r.Reason == "" {
		return fmt.Errorf("reason is required")
	}

	return nil
}

// ProvisionResult contains the result of a successful provisioning operation.
type ProvisionResult struct {
	OrganizationID   string
	KEKPath          string
	AEADPrimaryKeyID uint32
	MACPrimaryKeyID  uint32
	RegistryStatus   mmodel.RegistryStatus
}

// Provision creates keysets for an organization and registers it for envelope encryption.
// The organization starts in pending_migration status after provisioning.
//
// This operation:
//  1. Generates AEAD and MAC keysets
//  2. Wraps keysets with the organization's KEK via KMS
//  3. Persists wrapped keysets to storage
//  4. Creates registry record in pending_migration status
//
// Returns ErrOrganizationAlreadyProvisioned if the organization already has a keyset.
func (s *ProvisioningService) Provision(ctx context.Context, req ProvisionRequest) (ProvisionResult, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return ProvisionResult{}, err
	}

	// Validate request
	if err := req.Validate(); err != nil {
		return ProvisionResult{}, fmt.Errorf("invalid provision request: %w", err)
	}

	// Generate KEK path for this organization
	kekPath := s.buildKEKPath(req.OrganizationID)

	// Generate AEAD keyset
	aeadBundle, err := s.keysetGenerator.GenerateAEADKeyset(ctx, kekPath)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("%w: failed to generate AEAD keyset: %v", ErrProvisioningFailed, err)
	}

	// Check context before generating MAC keyset
	if err := ctx.Err(); err != nil {
		return ProvisionResult{}, err
	}

	// Generate MAC keyset
	macBundle, err := s.keysetGenerator.GenerateMACKeyset(ctx, kekPath)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("%w: failed to generate MAC keyset: %v", ErrProvisioningFailed, err)
	}

	// Build organization keyset
	now := time.Now().UTC()
	keyset := &mmodel.OrganizationKeyset{
		TenantID:          req.TenantID,
		OrganizationID:    req.OrganizationID,
		KEKPath:           kekPath,
		WrappedKeyset:     aeadBundle.Wrapped.WrappedData,
		KeysetInfo:        convertKeysetInfo(aeadBundle.Wrapped.Info),
		LegacyKeyImported: false,
		WrappedHMACKeyset: macBundle.Wrapped.WrappedData,
		HMACKeysetInfo:    convertKeysetInfo(macBundle.Wrapped.Info),
		Revision:          1,
		CreatedAt:         now,
	}

	// Save keyset
	if err := s.keysetWriter.Save(ctx, keyset); err != nil {
		if errors.Is(err, constant.ErrKeysetAlreadyExists) || errors.Is(err, mmodel.ErrKeysetAlreadyExists) {
			return ProvisionResult{}, ErrOrganizationAlreadyProvisioned
		}

		return ProvisionResult{}, fmt.Errorf("%w: failed to save keyset: %v", ErrProvisioningFailed, err)
	}

	// Create registry record in pending_migration status
	registry, err := mmodel.NewOrganizationRegistryRecord(req.TenantID, req.OrganizationID, req.Actor, req.Reason)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("%w: failed to create registry record: %v", ErrProvisioningFailed, err)
	}

	// Save registry record
	if err := s.registryWriter.Save(ctx, registry); err != nil {
		// Registry save failed - keyset is orphaned but that's acceptable
		// (keyset without registry = organization still in legacy mode)
		if errors.Is(err, constant.ErrRegistryAlreadyExists) || errors.Is(err, mmodel.ErrRegistryAlreadyExists) {
			return ProvisionResult{}, ErrOrganizationAlreadyProvisioned
		}

		return ProvisionResult{}, fmt.Errorf("%w: failed to save registry: %v", ErrProvisioningFailed, err)
	}

	return ProvisionResult{
		OrganizationID:   req.OrganizationID,
		KEKPath:          kekPath,
		AEADPrimaryKeyID: aeadBundle.Wrapped.Info.PrimaryKeyID,
		MACPrimaryKeyID:  macBundle.Wrapped.Info.PrimaryKeyID,
		RegistryStatus:   mmodel.RegistryStatusPendingMigration,
	}, nil
}

// ActivateRequest contains the parameters for activating an organization.
type ActivateRequest struct {
	OrganizationID string
	Actor          string // Who initiated the activation
	Reason         string // Why activation was requested
}

// Validate validates the activate request.
func (r ActivateRequest) Validate() error {
	if r.OrganizationID == "" {
		return fmt.Errorf("organization_id is required")
	}

	if r.Actor == "" {
		return fmt.Errorf("actor is required")
	}

	if r.Reason == "" {
		return fmt.Errorf("reason is required")
	}

	return nil
}

// Activate transitions an organization from pending_migration to active status.
// After activation, the organization uses envelope encryption for all new writes.
//
// Returns ErrOrganizationNotProvisioned if the organization has no registry record.
// Returns ErrActivationFailed if the transition is not allowed.
func (s *ProvisioningService) Activate(ctx context.Context, req ActivateRequest) error {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return err
	}

	// Validate request
	if err := req.Validate(); err != nil {
		return fmt.Errorf("invalid activate request: %w", err)
	}

	// Get current registry record
	registry, err := s.registryWriter.Get(ctx, req.OrganizationID)
	if err != nil {
		if errors.Is(err, constant.ErrRegistryNotFound) || errors.Is(err, mmodel.ErrRegistryNotFound) {
			return ErrOrganizationNotProvisioned
		}

		return fmt.Errorf("%w: failed to get registry: %v", ErrActivationFailed, err)
	}

	// Store current revision for optimistic locking
	currentRevision := registry.Revision

	// Attempt to activate
	if err := registry.Activate(currentRevision, req.Actor, req.Reason); err != nil {
		return fmt.Errorf("%w: %v", ErrActivationFailed, err)
	}

	// Update registry
	if err := s.registryWriter.Update(ctx, registry, currentRevision); err != nil {
		if errors.Is(err, constant.ErrRegistryRevisionConflict) || errors.Is(err, mmodel.ErrRegistryRevisionConflict) {
			return fmt.Errorf("%w: concurrent modification detected", ErrActivationFailed)
		}

		return fmt.Errorf("%w: failed to update registry: %v", ErrActivationFailed, err)
	}

	return nil
}

// GetProvisioningStatus returns the current provisioning status for an organization.
// Returns nil status if the organization has not been provisioned.
func (s *ProvisioningService) GetProvisioningStatus(ctx context.Context, organizationID string) (*mmodel.RegistryStatus, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if organizationID == "" {
		return nil, fmt.Errorf("organization_id is required")
	}

	registry, err := s.registryWriter.Get(ctx, organizationID)
	if err != nil {
		if errors.Is(err, constant.ErrRegistryNotFound) || errors.Is(err, mmodel.ErrRegistryNotFound) {
			return nil, nil // Not provisioned
		}

		return nil, fmt.Errorf("failed to get registry: %w", err)
	}

	return &registry.Status, nil
}

// IsProvisioned returns true if the organization has been provisioned for envelope encryption.
func (s *ProvisioningService) IsProvisioned(ctx context.Context, organizationID string) (bool, error) {
	status, err := s.GetProvisioningStatus(ctx, organizationID)
	if err != nil {
		return false, err
	}

	return status != nil, nil
}

// IsActive returns true if the organization is fully active with envelope encryption.
func (s *ProvisioningService) IsActive(ctx context.Context, organizationID string) (bool, error) {
	status, err := s.GetProvisioningStatus(ctx, organizationID)
	if err != nil {
		return false, err
	}

	if status == nil {
		return false, nil
	}

	return *status == mmodel.RegistryStatusActive ||
		*status == mmodel.RegistryStatusPartiallyMigrated ||
		*status == mmodel.RegistryStatusMigrationComplete, nil
}

// buildKEKPath constructs the KEK path for an organization.
// Format: {mount}/keys/org-{org-id}
// This follows Vault Transit conventions where /keys/ is the standard path segment.
func (s *ProvisioningService) buildKEKPath(organizationID string) string {
	return fmt.Sprintf("%s/keys/org-%s", s.kekMountPath, organizationID)
}

// convertKeysetInfo converts local KeysetInfo to mmodel.KeysetInfo.
func convertKeysetInfo(info KeysetInfo) mmodel.KeysetInfo {
	keys := make([]mmodel.KeyInfo, len(info.Keys))
	for i, k := range info.Keys {
		keys[i] = mmodel.KeyInfo{
			KeyID:     k.KeyID,
			Status:    k.Status,
			Type:      k.Type,
			IsPrimary: k.IsPrimary,
		}
	}

	return mmodel.KeysetInfo{
		PrimaryKeyID: info.PrimaryKeyID,
		Keys:         keys,
	}
}
