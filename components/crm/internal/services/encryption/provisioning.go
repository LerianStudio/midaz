// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// defaultAuditActor is the ActorID recorded when the provision request omits an
// actor. It documents that the action was initiated by the service itself
// rather than an authenticated principal.
const defaultAuditActor = "system"

// auditActorTypeService classifies the actor as a service principal. It matches
// the default applied by mmodel.NewProtectionAuditEvent for an empty ActorType.
const auditActorTypeService = "service"

// Static, error-free Reason phrases per outcome. They never carry dynamic error
// text or PII, so they are safe to persist in the audit trail.
const (
	reasonProvisionSuccess       = "organization encryption provisioned"
	reasonProvisionAlreadyExists = "organization encryption already provisioned"
	reasonProvisionFailure       = "organization encryption provisioning failed"
)

// EntityOrganizationEncryption is the entity type for encryption-related errors.
const EntityOrganizationEncryption = "OrganizationEncryption"

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
//   - GetProvisioningStatus: returns current status for an organization
//
// Convenience operations (for admin tooling and conditional logic):
//   - IsProvisioned: quick check if organization has been provisioned
//   - IsActive: quick check if organization uses envelope encryption
type ProvisioningService interface {
	Provision(ctx context.Context, req ProvisionInput) (ProvisionResult, error)
	GetProvisioningStatus(ctx context.Context, organizationID string) (*mmodel.RegistryStatus, error)
	IsProvisioned(ctx context.Context, organizationID string) (bool, error)
	IsActive(ctx context.Context, organizationID string) (bool, error)
}

// provisioningService handles organization encryption provisioning and activation.
// It coordinates keyset generation, KMS wrapping, and registry state management.
type provisioningService struct {
	keysetRepo      mongoEncryption.KeysetRepository
	registryRepo    mongoEncryption.RegistryRepository
	keysetGenerator KeysetGenerator
	kekMountPath    string
	auditWriter     AuditWriter
	metrics         *protectionMetrics
}

// NewProvisioningService creates a new provisioning service with the given dependencies.
//
// auditWriter receives a best-effort, non-blocking provisioning audit event for
// every terminal outcome of Provision. It MUST be non-nil: the provisioning
// service exists only in envelope mode, where bootstrap always constructs a
// repository-backed AuditWriter. Audit emission never affects the provisioning
// result, return value, or error path.
//
// metrics is the nil-safe protection metrics seam; a nil value defaults to
// NewProtectionMetrics(nil) so emission is a no-op when telemetry is disabled.
func NewProvisioningService(
	keysetRepo mongoEncryption.KeysetRepository,
	registryRepo mongoEncryption.RegistryRepository,
	keysetGenerator KeysetGenerator,
	config ProvisioningConfig,
	auditWriter AuditWriter,
	metrics *protectionMetrics,
) ProvisioningService {
	mountPath := config.KEKMountPath
	if mountPath == "" {
		mountPath = "transit"
	}

	if metrics == nil {
		metrics = NewProtectionMetrics(nil)
	}

	return &provisioningService{
		keysetRepo:      keysetRepo,
		registryRepo:    registryRepo,
		keysetGenerator: keysetGenerator,
		kekMountPath:    mountPath,
		auditWriter:     auditWriter,
		metrics:         metrics,
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
// The organization starts in active status after provisioning.
//
// This operation is idempotent: if the organization is already provisioned, it returns
// the existing provisioning info without error.
//
// Steps for new provisioning:
//  1. Generates AEAD and MAC keysets
//  2. Wraps keysets with the organization's KEK via KMS
//  3. Persists wrapped keysets to storage
//  4. Creates registry record in active status
func (s *provisioningService) Provision(ctx context.Context, req ProvisionInput) (ProvisionResult, error) {
	result, outcome, err := s.provision(ctx, req)

	// Single-point, best-effort audit emission: exactly one event per terminal
	// path, based on the outcome we are about to return. This must never alter
	// the provisioning result, return value, or error path.
	s.emitProvisioningAudit(ctx, req, result, outcome, err)

	return result, err
}

// provision performs the actual provisioning work and reports the terminal
// audit outcome alongside the result/error. It is the single internal entry
// point so the exported Provision can emit exactly one audit event. The helpers
// return their own outcome; this method never emits.
func (s *provisioningService) provision(ctx context.Context, req ProvisionInput) (ProvisionResult, mmodel.AuditOutcome, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, err
	}

	// Validate request
	if err := req.Validate(); err != nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, fmt.Errorf("invalid provision request: %w", err)
	}

	// Check if already provisioned (idempotent behavior)
	provisioned, err := s.IsProvisioned(ctx, req.OrganizationID)
	if err != nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, fmt.Errorf("failed to check provisioning status: %w", err)
	}

	if provisioned {
		result, err := s.getExistingProvisionResult(ctx, req.OrganizationID)
		if err != nil {
			return result, mmodel.AuditOutcomeFailure, err
		}

		return result, mmodel.AuditOutcomeAlreadyExists, nil
	}

	// Generate KEK path for this organization
	kekPath := s.buildKEKPath(req.OrganizationID)

	// Generate AEAD keyset. The KMS wrap happens inside the generator; provider
	// timing is measured here at the service boundary (pkg/crypto/tink MUST NOT
	// emit metrics). Provider is "vault" today (envelope is Vault-only); this
	// becomes dynamic when other KMS providers land. Timing is recorded even on
	// failure.
	aeadWrapStart := time.Now()
	aeadBundle, err := s.keysetGenerator.GenerateAEADKeyset(ctx, kekPath)
	s.metrics.recordProviderOperation(ctx, providerOperationWrap, providerVault, time.Since(aeadWrapStart).Milliseconds())

	if err != nil {
		s.metrics.recordProviderFailure(ctx, providerOperationWrap, errorCodeWrapAEADFailed)

		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Check context before generating MAC keyset
	if err := ctx.Err(); err != nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, err
	}

	// Generate MAC keyset. Provider timing is measured here at the service
	// boundary and recorded even on failure.
	macWrapStart := time.Now()
	macBundle, err := s.keysetGenerator.GenerateMACKeyset(ctx, kekPath)
	s.metrics.recordProviderOperation(ctx, providerOperationWrap, providerVault, time.Since(macWrapStart).Milliseconds())

	if err != nil {
		s.metrics.recordProviderFailure(ctx, providerOperationWrap, errorCodeWrapMACFailed)

		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Build organization keyset
	now := time.Now().UTC()
	keyset := &mmodel.OrganizationKeyset{
		TenantID:          req.TenantID,
		OrganizationID:    req.OrganizationID,
		KEKPath:           kekPath,
		WrappedKeyset:     aeadBundle.Wrapped.WrappedData,
		KeysetInfo:        convertKeysetInfo(aeadBundle.Wrapped.Info),
		WrappedHMACKeyset: macBundle.Wrapped.WrappedData,
		HMACKeysetInfo:    convertKeysetInfo(macBundle.Wrapped.Info),
		Revision:          1,
		CreatedAt:         now,
	}

	// Save keyset
	if err := s.keysetRepo.Save(ctx, keyset); err != nil {
		if errors.Is(err, constant.ErrKeysetAlreadyExists) || errors.Is(err, mmodel.ErrKeysetAlreadyExists) {
			// Keyset already exists - check if this is a recovery from partial failure
			// or a true duplicate provisioning attempt
			return s.handleExistingKeyset(ctx, req)
		}

		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Create and save registry record
	return s.createAndSaveRegistry(ctx, req, kekPath, aeadBundle.Wrapped.Info.PrimaryKeyID, macBundle.Wrapped.Info.PrimaryKeyID)
}

// emitProvisioningAudit builds and emits exactly one best-effort provisioning
// audit event for the terminal outcome. It is intentionally tolerant: if the
// event cannot be built it debug-logs and skips emission, and it never inspects
// the writer's result. Emission MUST NOT affect the provisioning outcome.
func (s *provisioningService) emitProvisioningAudit(ctx context.Context, req ProvisionInput, result ProvisionResult, outcome mmodel.AuditOutcome, provErr error) {
	logger, _, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	actorID := req.Actor
	if actorID == "" {
		actorID = defaultAuditActor
	}

	input := mmodel.ProtectionAuditEventInput{
		TenantID:       req.TenantID,
		OrganizationID: req.OrganizationID,
		EventType:      mmodel.AuditEventTypeProvisioning,
		Action:         mmodel.AuditActionProvision,
		Outcome:        outcome,
		ActorID:        actorID,
		ActorType:      auditActorTypeService,
		Reason:         reasonForOutcome(outcome),
		RequestID:      reqID,
		Details:        provisioningAuditDetails(result, outcome, provErr),
	}

	event, err := mmodel.NewProtectionAuditEvent(input)
	if err != nil {
		if logger != nil {
			logger.Log(ctx, libLog.LevelDebug, "audit event build skipped",
				libLog.String("organization_id", req.OrganizationID),
				libLog.String("outcome", string(outcome)))
		}

		return
	}

	s.auditWriter.EmitAsync(ctx, event)
}

// reasonForOutcome returns the static, error-free Reason phrase for an outcome.
func reasonForOutcome(outcome mmodel.AuditOutcome) string {
	switch outcome {
	case mmodel.AuditOutcomeSuccess:
		return reasonProvisionSuccess
	case mmodel.AuditOutcomeAlreadyExists:
		return reasonProvisionAlreadyExists
	default:
		return reasonProvisionFailure
	}
}

// provisioningAuditDetails builds the non-sensitive AuditDetails for the event.
// On success it records NewStatus=active and the affected primary key IDs; on
// failure it records the static provisioning error code. It never includes
// dynamic error text, keysets, or PII.
func provisioningAuditDetails(result ProvisionResult, outcome mmodel.AuditOutcome, provErr error) *mmodel.AuditDetails {
	switch outcome {
	case mmodel.AuditOutcomeSuccess:
		keyIDs := make([]uint32, 0, 2)
		if result.AEADPrimaryKeyID != 0 {
			keyIDs = append(keyIDs, result.AEADPrimaryKeyID)
		}

		if result.MACPrimaryKeyID != 0 {
			keyIDs = append(keyIDs, result.MACPrimaryKeyID)
		}

		return &mmodel.AuditDetails{
			NewStatus:      string(mmodel.RegistryStatusActive),
			AffectedKeyIDs: keyIDs,
		}
	case mmodel.AuditOutcomeFailure:
		if provErr == nil {
			return nil
		}

		return &mmodel.AuditDetails{
			ErrorCode: constant.ErrProvisioningFailed.Error(),
		}
	default:
		return nil
	}
}

// handleExistingKeyset handles the case where a keyset already exists but registry creation
// failed (partial failure recovery). Completes provisioning by creating the registry.
func (s *provisioningService) handleExistingKeyset(ctx context.Context, req ProvisionInput) (ProvisionResult, mmodel.AuditOutcome, error) {
	// Check if registry exists
	existingRegistry, err := s.registryRepo.Get(ctx, req.OrganizationID)
	if err != nil && !errors.Is(err, constant.ErrRegistryNotFound) && !errors.Is(err, mmodel.ErrRegistryNotFound) {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	if existingRegistry != nil {
		// Both keyset and registry exist - truly already provisioned
		// Return existing info for idempotent behavior
		result, err := s.getExistingProvisionResult(ctx, req.OrganizationID)
		if err != nil {
			return result, mmodel.AuditOutcomeFailure, err
		}

		return result, mmodel.AuditOutcomeAlreadyExists, nil
	}

	// Keyset exists but registry doesn't - recover from partial failure
	// Read the existing keyset to get key IDs
	existingKeyset, err := s.keysetRepo.Get(ctx, req.OrganizationID)
	if err != nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	if existingKeyset == nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Complete provisioning by creating the registry
	return s.createAndSaveRegistry(ctx, req, existingKeyset.KEKPath,
		existingKeyset.KeysetInfo.PrimaryKeyID, existingKeyset.HMACKeysetInfo.PrimaryKeyID)
}

// createAndSaveRegistry creates a registry record and saves it.
// Idempotent: if registry already exists (concurrent provisioning race), returns existing result.
func (s *provisioningService) createAndSaveRegistry(ctx context.Context, req ProvisionInput, kekPath string, aeadKeyID, macKeyID uint32) (ProvisionResult, mmodel.AuditOutcome, error) {
	// Create registry record in active status
	registry, err := mmodel.NewOrganizationRegistryRecord(req.TenantID, req.OrganizationID, req.Actor, req.Reason)
	if err != nil {
		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	// Save registry record
	if err := s.registryRepo.Save(ctx, registry); err != nil {
		// Idempotent: if registry already exists (concurrent provisioning), return existing result
		if errors.Is(err, constant.ErrRegistryAlreadyExists) || errors.Is(err, mmodel.ErrRegistryAlreadyExists) {
			result, err := s.getExistingProvisionResult(ctx, req.OrganizationID)
			if err != nil {
				return result, mmodel.AuditOutcomeFailure, err
			}

			return result, mmodel.AuditOutcomeAlreadyExists, nil
		}

		return ProvisionResult{}, mmodel.AuditOutcomeFailure, pkg.ValidateBusinessError(constant.ErrProvisioningFailed, EntityOrganizationEncryption)
	}

	return ProvisionResult{
		OrganizationID:   req.OrganizationID,
		KEKPath:          kekPath,
		AEADPrimaryKeyID: aeadKeyID,
		MACPrimaryKeyID:  macKeyID,
		RegistryStatus:   mmodel.RegistryStatusActive,
	}, mmodel.AuditOutcomeSuccess, nil
}

// getExistingProvisionResult retrieves provisioning info for an already-provisioned organization.
// Returns error if keyset or registry is missing/nil.
func (s *provisioningService) getExistingProvisionResult(ctx context.Context, organizationID string) (ProvisionResult, error) {
	if organizationID == "" {
		return ProvisionResult{}, fmt.Errorf("organization_id is required")
	}

	keyset, err := s.keysetRepo.Get(ctx, organizationID)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("failed to get keyset: %w", err)
	}

	if keyset == nil {
		return ProvisionResult{}, fmt.Errorf("keyset is nil for organization %s", organizationID)
	}

	registry, err := s.registryRepo.Get(ctx, organizationID)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("failed to get registry: %w", err)
	}

	if registry == nil {
		return ProvisionResult{}, fmt.Errorf("registry is nil for organization %s", organizationID)
	}

	return ProvisionResult{
		OrganizationID:   organizationID,
		KEKPath:          keyset.KEKPath,
		AEADPrimaryKeyID: keyset.KeysetInfo.PrimaryKeyID,
		MACPrimaryKeyID:  keyset.HMACKeysetInfo.PrimaryKeyID,
		RegistryStatus:   registry.Status,
	}, nil
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

	registry, err := s.registryRepo.Get(ctx, organizationID)
	if err != nil {
		if errors.Is(err, constant.ErrRegistryNotFound) || errors.Is(err, mmodel.ErrRegistryNotFound) {
			return nil, nil // Not provisioned
		}

		return nil, fmt.Errorf("failed to get registry: %w", err)
	}

	// Guard against repository returning (nil, nil)
	if registry == nil {
		return nil, nil // Not provisioned
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

// IsActive returns true if the organization is provisioned for envelope encryption.
func (s *provisioningService) IsActive(ctx context.Context, organizationID string) (bool, error) {
	status, err := s.GetProvisioningStatus(ctx, organizationID)
	if err != nil {
		return false, err
	}

	if status == nil {
		return false, nil
	}

	return *status == mmodel.RegistryStatusActive, nil
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
