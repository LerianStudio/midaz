// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
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
// This is compatible with libCommons crypto.Crypto.
type LegacyCrypto interface {
	Encrypt(value *string) (*string, error)
	Decrypt(value *string) (*string, error)
	GenerateHash(value *string) string
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
// It routes between legacy (libCommons) and envelope (Tink/KMS) encryption
// based on organization protection state.
type encryptionService struct {
	stateResolver *ProtectionStateResolver
	keysetManager *KeysetManager
	keysetReader  KeysetReader
	legacyCrypto  LegacyCrypto
}

// NewEncryptionService creates a new encryption service with the given dependencies.
func NewEncryptionService(
	stateResolver *ProtectionStateResolver,
	keysetManager *KeysetManager,
	keysetReader KeysetReader,
	legacyCrypto LegacyCrypto,
) EncryptionService {
	return &encryptionService{
		stateResolver: stateResolver,
		keysetManager: keysetManager,
		keysetReader:  keysetReader,
		legacyCrypto:  legacyCrypto,
	}
}

// Encrypt encrypts a plaintext value for the given field context.
// Uses envelope encryption if organization is active, legacy otherwise.
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

	// Resolve protection state
	state, err := s.stateResolver.Resolve(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve protection state: %w", err)
	}

	// Route based on mode
	if state.MustUseEnvelope() {
		return s.encryptEnvelope(ctx, fieldCtx, plaintext)
	}

	return s.encryptLegacy(ctx, plaintext)
}

// encryptEnvelope performs envelope encryption using Tink AEAD.
func (s *encryptionService) encryptEnvelope(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	// Get AEAD primitive
	aead, _, err := s.keysetManager.GetPrimitives(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get AEAD primitive: %w", err)
	}

	// Get keyset info for primary key ID
	keyset, err := s.keysetReader.Get(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get keyset info: %w", err)
	}

	// Guard against nil keyset (repository returned nil without error)
	if keyset == nil {
		return "", fmt.Errorf("failed to get keyset info: %w", constant.ErrKeysetNotFound)
	}

	// Encrypt with canonical AAD
	aad := fieldCtx.CanonicalAAD()

	ciphertext, err := aead.Encrypt([]byte(plaintext), aad)
	if err != nil {
		return "", fmt.Errorf("AEAD encryption failed: %w", err)
	}

	// Format with envelope marker
	marked := FormatEnvelopeMarker(keyset.KeysetInfo.PrimaryKeyID, ciphertext)

	return marked, nil
}

// encryptLegacy performs legacy encryption using libCommons crypto.
func (s *encryptionService) encryptLegacy(ctx context.Context, plaintext string) (string, error) {
	// Check context before crypto operation
	if err := ctx.Err(); err != nil {
		return "", err
	}

	encrypted, err := s.legacyCrypto.Encrypt(&plaintext)
	if err != nil {
		return "", fmt.Errorf("legacy encryption failed: %w", err)
	}

	if encrypted == nil {
		return "", nil
	}

	return *encrypted, nil
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
	// Get AEAD primitive
	aead, _, err := s.keysetManager.GetPrimitives(ctx, fieldCtx.OrganizationID)
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

// decryptLegacy performs legacy decryption using libCommons crypto.
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

	// Decrypt using legacy crypto
	decrypted, err := s.legacyCrypto.Decrypt(&ciphertext)
	if err != nil {
		return "", fmt.Errorf("legacy decryption failed: %w", err)
	}

	if decrypted == nil {
		return "", nil
	}

	return *decrypted, nil
}

// GenerateSearchToken generates a deterministic search token for a field value.
// Used for encrypted field searching without exposing plaintext.
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

	// Resolve protection state
	state, err := s.stateResolver.Resolve(ctx, searchCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve protection state: %w", err)
	}

	// Route based on mode
	if state.MustUseEnvelope() {
		return s.generateSearchTokenEnvelope(ctx, searchCtx, normalizedValue)
	}

	return s.generateSearchTokenLegacy(normalizedValue), nil
}

// generateSearchTokenEnvelope generates a MAC-based search token using Tink.
func (s *encryptionService) generateSearchTokenEnvelope(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error) {
	// Get MAC primitive
	_, mac, err := s.keysetManager.GetPrimitives(ctx, searchCtx.OrganizationID)
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

// generateSearchTokenLegacy generates a hash-based search token using legacy crypto.
func (s *encryptionService) generateSearchTokenLegacy(normalizedValue string) string {
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

	keyset, err := s.keysetReader.Get(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get keyset: %w", err)
	}

	// Guard against nil keyset (repository returned nil without error)
	if keyset == nil {
		return nil, constant.ErrKeysetNotFound
	}

	return &keyset.KeysetInfo, nil
}
