// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	cryptoTink "github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// Package-level errors for encryption operations.
var (
	// ErrFieldContextInvalid is returned when the FieldContext fails validation.
	ErrFieldContextInvalid = errors.New("field context is invalid")

	// ErrSearchContextInvalid is returned when the SearchTokenContext fails validation.
	ErrSearchContextInvalid = errors.New("search token context is invalid")

	// ErrLegacyReadNotAllowed is returned when attempting to decrypt legacy ciphertext
	// but the organization's protection state does not allow legacy reads.
	ErrLegacyReadNotAllowed = errors.New("legacy read not allowed for this organization")

	// ErrEnvelopeDecryptFailed is returned when envelope decryption fails.
	// No fallback to legacy decryption is attempted when this error occurs.
	ErrEnvelopeDecryptFailed = errors.New("envelope decryption failed")
)

// LegacyCrypto defines the interface for legacy encryption operations.
// Implemented by:
//   - lib-commons crypto.Crypto for KMS_VENDOR=none
//   - LegacyKeyMaterial (Tink-backed) for KMS_VENDOR=hashicorp-vault
type LegacyCrypto interface {
	Encrypt(plaintext *string) (*string, error)
	Decrypt(ciphertext *string) (*string, error)
	GenerateHash(value *string) string
}

// LegacyKeyMaterial holds Tink-backed primitives for legacy encryption operations.
// Used only in envelope mode (KMS_VENDOR=hashicorp-vault) for reading legacy data
// during migration. Implements LegacyCrypto interface.
type LegacyKeyMaterial struct {
	aead *cryptoTink.AEADPrimitive
	mac  *cryptoTink.LegacyMACPrimitive
}

// NewLegacyKeyMaterial creates Tink-backed primitives from legacy key material.
// The encryptHexKey is a hex-encoded AES key (32 bytes for AES-256).
// The hashSecretKey is the plain string secret for HMAC-SHA256.
func NewLegacyKeyMaterial(encryptHexKey, hashSecretKey string) (*LegacyKeyMaterial, error) {
	aeadPrimitive, err := cryptoTink.NewLegacyAESGCMPrimitiveFromHexKey(encryptHexKey)
	if err != nil {
		return nil, fmt.Errorf("initialize legacy AES-GCM import: %w", err)
	}

	macPrimitive, err := cryptoTink.NewLegacyMACPrimitiveFromSecret(hashSecretKey)
	if err != nil {
		return nil, fmt.Errorf("initialize legacy HMAC import: %w", err)
	}

	return &LegacyKeyMaterial{aead: aeadPrimitive, mac: macPrimitive}, nil
}

// Encrypt implements LegacyCrypto interface using Tink AES-GCM with RAW prefix.
// Output is base64-encoded and compatible with lib-commons format.
func (m *LegacyKeyMaterial) Encrypt(plaintext *string) (*string, error) {
	if plaintext == nil {
		return nil, nil
	}

	cipherBytes, err := m.aead.Encrypt([]byte(*plaintext), nil)
	if err != nil {
		return nil, err
	}

	result := base64.StdEncoding.EncodeToString(cipherBytes)

	return &result, nil
}

// Decrypt implements LegacyCrypto interface using Tink AES-GCM with RAW prefix.
// Decrypts base64-encoded ciphertext from lib-commons Encrypt.
func (m *LegacyKeyMaterial) Decrypt(ciphertext *string) (*string, error) {
	if ciphertext == nil {
		return nil, nil
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(*ciphertext)
	if err != nil {
		return nil, err
	}

	plainBytes, err := m.aead.Decrypt(cipherBytes, nil)
	if err != nil {
		return nil, err
	}

	result := string(plainBytes)

	return &result, nil
}

// GenerateHash implements LegacyCrypto interface.
// Computes a lowercase hex HMAC-SHA256 token matching lib-commons output.
func (m *LegacyKeyMaterial) GenerateHash(value *string) string {
	if value == nil {
		return ""
	}

	token, err := m.mac.ComputeLegacyHexToken([]byte(*value))
	if err != nil {
		return ""
	}

	return token
}

// EncryptionService defines the contract for encryption operations.
//
// Core operations (used by repositories):
//   - Encrypt: encrypts plaintext for storage
//   - Decrypt: decrypts ciphertext for retrieval
//   - GenerateSearchToken: creates deterministic tokens for encrypted field search
//
// Inspection operations (for admin tooling and diagnostics):
//   - MustUseEnvelope: checks if organization requires envelope encryption
//   - GetProtectionState: returns full protection state for an organization
//   - GetKeysetInfo: returns keyset metadata (key IDs, status) without key material
type EncryptionService interface {
	Encrypt(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error)
	Decrypt(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error)
	GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error)
	MustUseEnvelope(ctx context.Context, organizationID string) (bool, error)
	GetProtectionState(ctx context.Context, organizationID string) (ProtectionState, error)
	GetKeysetInfo(ctx context.Context, organizationID string) (*mmodel.KeysetInfo, error)
}

// encryptionService provides field-level encryption for CRM sensitive data.
// It routes between Tink-backed legacy key material and envelope (Tink/KMS) encryption
// based on organization protection state or encryption mode.
//
// When encryptionMode is EncryptionModeEnvelope, the service always uses envelope
// encryption regardless of per-organization registry state. This enables lazy
// provisioning via KeysetManager for organizations that have not been explicitly
// provisioned yet.
type encryptionService struct {
	stateResolver  *ProtectionStateResolver
	keysetManager  *KeysetManager
	keysetRepo     mongoEncryption.KeysetRepository
	legacyCrypto   LegacyCrypto
	encryptionMode crypto.EncryptionMode
}

// NewEncryptionService creates a new encryption service with the given dependencies.
//
// The legacyCrypto parameter provides legacy encryption operations:
//   - For KMS_VENDOR=none: pass lib-commons *crypto.Crypto directly
//   - For KMS_VENDOR=hashicorp-vault: pass *LegacyKeyMaterial (Tink-backed)
//
// The encryptionMode parameter determines the encryption strategy:
//   - EncryptionModeEnvelope: Always use envelope encryption, triggering lazy
//     provisioning via KeysetManager for non-provisioned organizations.
//   - EncryptionModeLegacy: Fall back to per-organization protection state
//     resolution (default behavior for backwards compatibility).
//
// When encryptionMode is omitted or set to EncryptionModeLegacy, the service routes
// encryption based on the per-organization registry state resolved by stateResolver.
func NewEncryptionService(
	stateResolver *ProtectionStateResolver,
	keysetManager *KeysetManager,
	keysetRepo mongoEncryption.KeysetRepository,
	legacyCrypto LegacyCrypto,
	encryptionMode ...crypto.EncryptionMode,
) EncryptionService {
	mode := crypto.EncryptionModeLegacy
	if len(encryptionMode) > 0 {
		mode = encryptionMode[0]
	}

	return &encryptionService{
		stateResolver:  stateResolver,
		keysetManager:  keysetManager,
		keysetRepo:     keysetRepo,
		legacyCrypto:   legacyCrypto,
		encryptionMode: mode,
	}
}

// Encrypt encrypts a plaintext value for the given field context.
// Uses envelope encryption if encryptionMode is envelope or organization is active,
// legacy otherwise.
//
// When encryptionMode is EncryptionModeEnvelope:
//   - Always uses envelope encryption regardless of per-org registry state
//   - Triggers lazy provisioning via KeysetManager.GetPrimitives() for non-provisioned orgs
//
// When encryptionMode is EncryptionModeLegacy (or unset):
//   - Routes based on per-organization protection state
//   - Uses envelope if organization registry shows active status
//   - Uses legacy if organization has no registry record
//
// For envelope mode:
//   - Validates field context
//   - Gets AEAD primitive from KeysetManager
//   - Encrypts with canonical AAD
//   - Returns marked ciphertext (tink:v{keyID}:{base64})
//
// For legacy mode:
//   - Uses libCommons crypto.Encrypt
//   - Returns unmarked ciphertext
func (s *encryptionService) Encrypt(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Validate field context
	if err := fieldCtx.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrFieldContextInvalid, err)
	}

	// Encryption mode envelope: always use envelope encryption (triggers lazy provisioning)
	if s.encryptionMode == crypto.EncryptionModeEnvelope {
		return s.encryptEnvelope(ctx, fieldCtx, plaintext)
	}

	// Legacy encryption mode: resolve protection state per organization
	state, err := s.stateResolver.Resolve(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve protection state: %w", err)
	}

	// Route based on per-org mode
	if state.MustUseEnvelope() {
		return s.encryptEnvelope(ctx, fieldCtx, plaintext)
	}

	return s.encryptLegacy(ctx, plaintext)
}

// encryptEnvelope performs envelope encryption using Tink AEAD.
func (s *encryptionService) encryptEnvelope(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	// Get AEAD primitive and primary key ID (cached together to avoid redundant DB calls)
	aead, _, primaryKeyID, err := s.keysetManager.GetPrimitives(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get AEAD primitive: %w", err)
	}

	// Encrypt with canonical AAD
	aad := fieldCtx.CanonicalAAD()

	ciphertext, err := aead.Encrypt([]byte(plaintext), aad)
	if err != nil {
		return "", fmt.Errorf("AEAD encryption failed: %w", err)
	}

	// Format with envelope marker
	marked := FormatEnvelopeMarker(primaryKeyID, ciphertext)

	return marked, nil
}

// encryptLegacy performs legacy encryption using the LegacyCrypto interface.
// Uses lib-commons crypto for KMS_VENDOR=none, Tink for KMS_VENDOR=hashicorp-vault.
func (s *encryptionService) encryptLegacy(ctx context.Context, plaintext string) (string, error) {
	// Check context before crypto operation
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if s.legacyCrypto == nil {
		return "", fmt.Errorf("legacy crypto is required")
	}

	result, err := s.legacyCrypto.Encrypt(&plaintext)
	if err != nil {
		return "", fmt.Errorf("legacy encrypt: %w", err)
	}

	if result == nil {
		return "", nil
	}

	return *result, nil
}

// Decrypt decrypts a ciphertext value for the given field context.
// Routes based on envelope marker presence and organization state.
//
// If ciphertext has envelope marker:
//   - Parse marker to get key ID
//   - Get AEAD primitive from KeysetManager
//   - Decrypt with canonical AAD
//   - Return plaintext
//
// If ciphertext has no marker (legacy candidate):
//   - Check if organization allows legacy read
//   - Use libCommons crypto.Decrypt
//   - Return plaintext
//
// Error cases:
//   - Marked ciphertext with envelope decrypt failure returns error (no fallback)
//   - Unmarked ciphertext when legacy read not allowed returns error
func (s *encryptionService) Decrypt(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Validate field context
	if err := fieldCtx.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrFieldContextInvalid, err)
	}

	// Check for envelope marker
	marker, hasMarker, err := ParseEnvelopeMarker(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to parse envelope marker: %w", err)
	}

	if hasMarker {
		// Envelope-encrypted: decrypt using Tink AEAD
		return s.decryptEnvelope(ctx, fieldCtx, marker)
	}

	// No marker: legacy candidate
	return s.decryptLegacy(ctx, fieldCtx, ciphertext)
}

// decryptEnvelope performs envelope decryption using Tink AEAD.
// Returns ErrEnvelopeDecryptFailed on failure - NO fallback to legacy.
func (s *encryptionService) decryptEnvelope(ctx context.Context, fieldCtx FieldContext, marker EnvelopeMarker) (string, error) {
	// Get AEAD primitive (ignoring primaryKeyID as we use the marker's key ID for decryption)
	aead, _, _, err := s.keysetManager.GetPrimitives(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("%w: failed to get AEAD primitive: %v", ErrEnvelopeDecryptFailed, err)
	}

	// Decrypt with canonical AAD
	aad := fieldCtx.CanonicalAAD()

	plaintext, err := aead.Decrypt(marker.Payload, aad)
	if err != nil {
		// NO FALLBACK to legacy - envelope decryption must succeed or fail definitively
		return "", fmt.Errorf("%w: %v", ErrEnvelopeDecryptFailed, err)
	}

	return string(plaintext), nil
}

// decryptLegacy performs legacy decryption using the LegacyCrypto interface.
// Uses lib-commons crypto for KMS_VENDOR=none, Tink for KMS_VENDOR=hashicorp-vault.
// Returns ErrLegacyReadNotAllowed if the organization doesn't permit legacy reads.
func (s *encryptionService) decryptLegacy(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error) {
	// Check protection state for legacy read permission
	state, err := s.stateResolver.Resolve(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve protection state: %w", err)
	}

	if !state.CanReadLegacy {
		return "", fmt.Errorf("%w: organization %s", ErrLegacyReadNotAllowed, fieldCtx.OrganizationID)
	}

	// Check context before crypto operation
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if s.legacyCrypto == nil {
		return "", fmt.Errorf("legacy crypto is required")
	}

	result, err := s.legacyCrypto.Decrypt(&ciphertext)
	if err != nil {
		return "", fmt.Errorf("legacy decrypt: %w", err)
	}

	if result == nil {
		return "", nil
	}

	return *result, nil
}

// GenerateSearchToken generates a deterministic search token for a field value.
// Used for encrypted field searching without exposing plaintext.
//
// When encryptionMode is EncryptionModeEnvelope:
//   - Always uses envelope MAC generation regardless of per-org registry state
//   - Triggers lazy provisioning via KeysetManager.GetPrimitives() for non-provisioned orgs
//
// When encryptionMode is EncryptionModeLegacy (or unset):
//   - Routes based on per-organization protection state
//
// For envelope mode:
//   - Gets MAC primitive from KeysetManager
//   - Computes HMAC of canonical input (tenant:org:field:value)
//   - Returns base64-encoded token
//
// For legacy mode:
//   - Uses libCommons crypto hash function
//   - Returns legacy hash token
func (s *encryptionService) GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Validate search context
	if err := searchCtx.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrSearchContextInvalid, err)
	}

	// encryption mode envelope: always use envelope MAC (triggers lazy provisioning)
	if s.encryptionMode == crypto.EncryptionModeEnvelope {
		return s.generateSearchTokenEnvelope(ctx, searchCtx, normalizedValue)
	}

	// Legacy encryption mode: resolve protection state per organization
	state, err := s.stateResolver.Resolve(ctx, searchCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve protection state: %w", err)
	}

	// Route based on per-org mode
	if state.MustUseEnvelope() {
		return s.generateSearchTokenEnvelope(ctx, searchCtx, normalizedValue)
	}

	return s.generateSearchTokenLegacy(normalizedValue), nil
}

// generateSearchTokenEnvelope generates a MAC-based search token using Tink.
func (s *encryptionService) generateSearchTokenEnvelope(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error) {
	// Get MAC primitive (ignoring primaryKeyID as it's not needed for MAC)
	_, mac, _, err := s.keysetManager.GetPrimitives(ctx, searchCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get MAC primitive: %w", err)
	}

	// Compute MAC of canonical input
	canonicalInput := searchCtx.CanonicalInput(normalizedValue)

	token, err := mac.ComputeSearchToken(canonicalInput)
	if err != nil {
		return "", fmt.Errorf("failed to compute search token: %w", err)
	}

	return token, nil
}

// generateSearchTokenLegacy generates a hash-based search token using the LegacyCrypto interface.
func (s *encryptionService) generateSearchTokenLegacy(normalizedValue string) string {
	if s.legacyCrypto == nil {
		return ""
	}

	return s.legacyCrypto.GenerateHash(&normalizedValue)
}

// MustUseEnvelope returns true if the organization must use envelope encryption.
// Convenience method for checking protection state.
func (s *encryptionService) MustUseEnvelope(ctx context.Context, organizationID string) (bool, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return false, err
	}

	state, err := s.stateResolver.Resolve(ctx, organizationID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve protection state: %w", err)
	}

	return state.MustUseEnvelope(), nil
}

// GetProtectionState returns the current protection state for an organization.
// Useful for callers that need to make decisions based on encryption mode.
func (s *encryptionService) GetProtectionState(ctx context.Context, organizationID string) (ProtectionState, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return ProtectionState{}, err
	}

	return s.stateResolver.Resolve(ctx, organizationID)
}

// GetKeysetInfo returns the keyset info for an organization.
// Useful for callers that need to know the primary key ID.
func (s *encryptionService) GetKeysetInfo(ctx context.Context, organizationID string) (*mmodel.KeysetInfo, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	keyset, err := s.keysetRepo.Get(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get keyset: %w", err)
	}

	// Guard against nil keyset (repository returned nil without error)
	if keyset == nil {
		return nil, constant.ErrKeysetNotFound
	}

	return &keyset.KeysetInfo, nil
}
