// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// EntityOrganizationEncryption is the entity type for encryption-related errors.
const EntityOrganizationEncryption = "OrganizationEncryption"

// KeysetWriter defines the interface for persisting organization keysets.
// Compatible with the KeysetRepository in the MongoDB adapter.
type KeysetWriter interface {
	Save(ctx context.Context, keyset *mmodel.OrganizationKeyset) error
}

// KeysetReaderForProvisioning defines the interface for reading organization keysets.
// Used to support idempotent provisioning when recovering from partial failures.
type KeysetReaderForProvisioning interface {
	Get(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error)
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
	GenerateAEADKeyset(ctx context.Context, keyName string) (tink.KeysetBundle, error)
	GenerateMACKeyset(ctx context.Context, keyName string) (tink.KeysetBundle, error)
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

// ProvisioningService defines the contract for encryption lifecycle management.
//
// Lifecycle operations (exposed via HTTP handlers):
//   - Provision: creates keysets and registry for an organization
//   - Activate: transitions organization from pending_migration to active
//   - GetProvisioningStatus: returns current status for an organization
//
// Convenience operations (for admin tooling and conditional logic):
//   - IsProvisioned: quick check if organization has been provisioned
//   - IsActive: quick check if organization uses envelope encryption
type ProvisioningService interface {
	Provision(ctx context.Context, req ProvisionInput) (ProvisionResult, error)
	Activate(ctx context.Context, req ActivateInput) error
	GetProvisioningStatus(ctx context.Context, organizationID string) (*mmodel.RegistryStatus, error)
	IsProvisioned(ctx context.Context, organizationID string) (bool, error)
	IsActive(ctx context.Context, organizationID string) (bool, error)
}

// provisioningService handles organization encryption provisioning and activation.
// It coordinates keyset generation, KMS wrapping, and registry state management.
type provisioningService struct {
	keysetWriter    KeysetWriter
	keysetReader    KeysetReaderForProvisioning
	registryWriter  RegistryWriter
	keysetGenerator KeysetGenerator
	kekMountPath    string
}

// NewProvisioningService creates a new provisioning service with the given dependencies.
func NewProvisioningService(
	keysetWriter KeysetWriter,
	keysetReader KeysetReaderForProvisioning,
	registryWriter RegistryWriter,
	keysetGenerator KeysetGenerator,
	config ProvisioningConfig,
) ProvisioningService {
	mountPath := config.KEKMountPath
	if mountPath == "" {
		mountPath = "transit"
	}

	return &provisioningService{
		keysetWriter:    keysetWriter,
		keysetReader:    keysetReader,
		registryWriter:  registryWriter,
		keysetGenerator: keysetGenerator,
		kekMountPath:    mountPath,
	}
}

// ProvisionInput contains the parameters for provisioning an organization.
type ProvisionInput struct {
	TenantID       string
	OrganizationID string
	Actor          string // Who initiated the provisioning
	Reason         string // Why provisioning was requested
}

// Validate validates the provision request.
func (r ProvisionInput) Validate() error {
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
func (s *provisioningService) Provision(ctx context.Context, req ProvisionInput) (ProvisionResult, error) {
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
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Check context before generating MAC keyset
	if err := ctx.Err(); err != nil {
		return ProvisionResult{}, err
	}

	// Generate MAC keyset
	macBundle, err := s.keysetGenerator.GenerateMACKeyset(ctx, kekPath)
	if err != nil {
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
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
			// Keyset already exists - check if this is a recovery from partial failure
			// or a true duplicate provisioning attempt
			return s.handleExistingKeyset(ctx, req)
		}

		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Create and save registry record
	return s.createAndSaveRegistry(ctx, req, kekPath, aeadBundle.Wrapped.Info.PrimaryKeyID, macBundle.Wrapped.Info.PrimaryKeyID)
}

// handleExistingKeyset handles the case where a keyset already exists.
// If registry also exists, this is a true duplicate (return conflict).
// If registry doesn't exist, this is recovery from a partial failure (complete provisioning).
func (s *provisioningService) handleExistingKeyset(ctx context.Context, req ProvisionInput) (ProvisionResult, error) {
	// Check if registry exists
	existingRegistry, err := s.registryWriter.Get(ctx, req.OrganizationID)
	if err != nil && !errors.Is(err, constant.ErrRegistryNotFound) && !errors.Is(err, mmodel.ErrRegistryNotFound) {
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	if existingRegistry != nil {
		// Both keyset and registry exist - truly already provisioned
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrRegistryAlreadyExists, EntityOrganizationEncryption)
	}

	// Keyset exists but registry doesn't - recover from partial failure
	// Read the existing keyset to get key IDs
	existingKeyset, err := s.keysetReader.Get(ctx, req.OrganizationID)
	if err != nil {
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	if existingKeyset == nil {
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Complete provisioning by creating the registry
	return s.createAndSaveRegistry(ctx, req, existingKeyset.KEKPath,
		existingKeyset.KeysetInfo.PrimaryKeyID, existingKeyset.HMACKeysetInfo.PrimaryKeyID)
}

// createAndSaveRegistry creates a registry record and saves it.
func (s *provisioningService) createAndSaveRegistry(ctx context.Context, req ProvisionInput, kekPath string, aeadKeyID, macKeyID uint32) (ProvisionResult, error) {
	// Create registry record in pending_migration status
	registry, err := mmodel.NewOrganizationRegistryRecord(req.TenantID, req.OrganizationID, req.Actor, req.Reason)
	if err != nil {
		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Save registry record
	if err := s.registryWriter.Save(ctx, registry); err != nil {
		if errors.Is(err, constant.ErrRegistryAlreadyExists) || errors.Is(err, mmodel.ErrRegistryAlreadyExists) {
			return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrRegistryAlreadyExists, EntityOrganizationEncryption)
		}

		return ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	return ProvisionResult{
		OrganizationID:   req.OrganizationID,
		KEKPath:          kekPath,
		AEADPrimaryKeyID: aeadKeyID,
		MACPrimaryKeyID:  macKeyID,
		RegistryStatus:   mmodel.RegistryStatusPendingMigration,
	}, nil
}

// ActivateInput contains the parameters for activating an organization.
type ActivateInput struct {
	OrganizationID string
	Actor          string // Who initiated the activation
	Reason         string // Why activation was requested
}

// Validate validates the activate request.
func (r ActivateInput) Validate() error {
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
func (s *provisioningService) Activate(ctx context.Context, req ActivateInput) error {
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
			return pkg.ValidateBusinessError(constant.ErrRegistryNotFound, EntityOrganizationEncryption)
		}

		return pkg.ValidateBusinessError(constant.ErrOrganizationEncryptionFailed, EntityOrganizationEncryption)
	}

	// Guard against nil registry (repository returned nil without error)
	if registry == nil {
		return pkg.ValidateBusinessError(constant.ErrRegistryNotFound, EntityOrganizationEncryption)
	}

	// Store current revision for optimistic locking
	currentRevision := registry.Revision

	// Attempt to activate
	if err := registry.Activate(currentRevision, req.Actor, req.Reason); err != nil {
		return pkg.ValidateBusinessError(constant.ErrOrganizationEncryptionFailed, EntityOrganizationEncryption)
	}

	// Update registry
	if err := s.registryWriter.Update(ctx, registry, currentRevision); err != nil {
		if errors.Is(err, constant.ErrRegistryRevisionConflict) || errors.Is(err, mmodel.ErrRegistryRevisionConflict) {
			return pkg.ValidateBusinessError(constant.ErrOrganizationEncryptionFailed, EntityOrganizationEncryption)
		}

		return pkg.ValidateBusinessError(constant.ErrOrganizationEncryptionFailed, EntityOrganizationEncryption)
	}

	return nil
}

// GetProvisioningStatus returns the current provisioning status for an organization.
// Returns nil status if the organization has not been provisioned.
func (s *provisioningService) GetProvisioningStatus(ctx context.Context, organizationID string) (*mmodel.RegistryStatus, error) {
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
func (s *provisioningService) IsProvisioned(ctx context.Context, organizationID string) (bool, error) {
	status, err := s.GetProvisioningStatus(ctx, organizationID)
	if err != nil {
		return false, err
	}

	return status != nil, nil
}

// IsActive returns true if the organization is fully active with envelope encryption.
func (s *provisioningService) IsActive(ctx context.Context, organizationID string) (bool, error) {
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

// buildKEKPath constructs the KEK key name for an organization.
// Format: org-{org-id}
// This is the key name used by Vault Transit for encrypt/decrypt operations.
// The mount path (e.g., "transit") is handled separately by the Vault client.
func (s *provisioningService) buildKEKPath(organizationID string) string {
	return fmt.Sprintf("org-%s", organizationID)
}

// convertKeysetInfo converts tink.KeysetInfo to mmodel.KeysetInfo for persistence.
func convertKeysetInfo(info tink.KeysetInfo) mmodel.KeysetInfo {
	keys := make([]mmodel.KeyInfo, len(info.Keys))
	for i, k := range info.Keys {
		keys[i] = mmodel.KeyInfo{
			KeyID:     k.KeyID,
			Status:    string(k.Status),
			Type:      string(k.Type),
			IsPrimary: k.IsPrimary,
		}
	}

	return mmodel.KeysetInfo{
		PrimaryKeyID: info.PrimaryKeyID,
		Keys:         keys,
	}
}
