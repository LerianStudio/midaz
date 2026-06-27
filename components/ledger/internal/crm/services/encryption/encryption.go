// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/crypto"
	cryptoTink "github.com/LerianStudio/midaz/v4/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// Protection path labels for the encrypt_decrypt metric and span output attr.
const (
	protectionPathLegacy   = "legacy"
	protectionPathEnvelope = "envelope"
	// protectionPathUnknown is recorded for failures that occur BEFORE a path is
	// chosen (e.g. field-context validation), so no legacy/envelope path applies.
	protectionPathUnknown = "unknown"
)

// Encrypt/decrypt outcome labels.
const (
	protectionOutcomeSuccess = "success"
	protectionOutcomeFailure = "failure"
)

// Stable error_type classifiers for the encrypt_decrypt metric. These are short,
// stable strings — never the raw error text — and carry no sensitive data.
const (
	errorTypeFieldContextInvalid = "field_context_invalid"
	errorTypeEnvelopeDecrypt     = "envelope_decrypt_failed"
	errorTypeLegacyReadNotAllow  = "legacy_read_not_allowed"
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
	aead    *cryptoTink.AEADPrimitive
	hashPRF *cryptoTink.LegacyPRFPrimitive
}

// NewLegacyKeyMaterial creates Tink-backed primitives from legacy key material.
// The encryptHexKey is a hex-encoded AES key (32 bytes for AES-256).
// The hashSecretKey is the plain string secret for HMAC-SHA256.
func NewLegacyKeyMaterial(encryptHexKey, hashSecretKey string) (*LegacyKeyMaterial, error) {
	aeadPrimitive, err := cryptoTink.NewLegacyAESGCMPrimitiveFromHexKey(encryptHexKey)
	if err != nil {
		return nil, fmt.Errorf("initialize legacy AES-GCM import: %w", err)
	}

	hashPRFPrimitive, err := cryptoTink.NewLegacyPRFPrimitiveFromSecret(hashSecretKey)
	if err != nil {
		return nil, fmt.Errorf("initialize legacy HMAC import: %w", err)
	}

	return &LegacyKeyMaterial{aead: aeadPrimitive, hashPRF: hashPRFPrimitive}, nil
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

	token, err := m.hashPRF.ComputeLegacyHexToken([]byte(*value))
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
//   - GenerateSearchToken: creates deterministic token for encrypted field search (primary key only).
//     Used on WRITE PATH for indexing — intentionally single-key so new records are always
//     indexed with the current primary key.
//   - GenerateSearchTokenCandidates: creates tokens for all enabled keys.
//     Used on READ PATH for queries — returns tokens for all enabled keys so searches match
//     records indexed with any key version during rotation windows.
//
// Inspection operations (for admin tooling and diagnostics):
//   - MustUseEnvelope: checks if organization requires envelope encryption
//   - GetProtectionState: returns full protection state for an organization
//   - GetKeysetInfo: returns keyset metadata (key IDs, status) without key material
type EncryptionService interface {
	Encrypt(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error)
	Decrypt(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error)
	// GenerateSearchToken creates a single search token using the primary key (WRITE PATH - indexing).
	// It returns the token and the PRF keyset primary key ID it was computed with. The
	// returned key version is non-zero for the envelope path and 0 only on the legacy-hash
	// branch.
	GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, uint32, error)
	// GenerateSearchTokenCandidates creates tokens for all enabled keys (READ PATH - queries).
	GenerateSearchTokenCandidates(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) ([]string, error)
	MustUseEnvelope(ctx context.Context, organizationID string) (bool, error)
	GetProtectionState(ctx context.Context, organizationID string) (ProtectionState, error)
	GetKeysetInfo(ctx context.Context, organizationID string) (*mmodel.KeysetInfo, error)
}

// KeysetRepository is the keyset persistence contract consumed by the encryption
// services (encryptionService, KeysetManager, provisioningService). It is defined
// here, in the consuming service package, so dependencies flow inward: the MongoDB
// adapter satisfies it structurally without the service importing the adapter for
// the type.
type KeysetRepository interface {
	Save(ctx context.Context, keyset *mmodel.OrganizationKeyset) error
	Get(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error)
	GetByVersion(ctx context.Context, organizationID string, version int) (*mmodel.OrganizationKeyset, error)
	GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error)
	Update(ctx context.Context, keyset *mmodel.OrganizationKeyset, expectedRevision int64) error
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
	keysetRepo     KeysetRepository
	legacyCrypto   LegacyCrypto
	metrics        *protectionMetrics
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
//
// metrics is the nil-safe protection metrics seam, passed as a non-variadic param
// before the variadic encryptionMode to avoid ambiguity. A nil value defaults to
// NewProtectionMetrics(nil) so emission is a no-op when telemetry is disabled.
func NewEncryptionService(
	stateResolver *ProtectionStateResolver,
	keysetManager *KeysetManager,
	keysetRepo KeysetRepository,
	legacyCrypto LegacyCrypto,
	metrics *protectionMetrics,
	encryptionMode ...crypto.EncryptionMode,
) EncryptionService {
	mode := crypto.EncryptionModeLegacy
	if len(encryptionMode) > 0 {
		mode = encryptionMode[0]
	}

	if metrics == nil {
		metrics = NewProtectionMetrics(nil)
	}

	return &encryptionService{
		stateResolver:  stateResolver,
		keysetManager:  keysetManager,
		keysetRepo:     keysetRepo,
		legacyCrypto:   legacyCrypto,
		metrics:        metrics,
		encryptionMode: mode,
	}
}

// Encrypt encrypts a plaintext value for the given field context.
// Uses envelope encryption if encryptionMode is envelope or organization is active,
// legacy otherwise.
//
// When encryptionMode is EncryptionModeEnvelope:
//   - Always uses envelope encryption regardless of per-org registry state
//   - Triggers lazy provisioning via KeysetManager.GetActivePrimitives() for non-provisioned orgs
//
// When encryptionMode is EncryptionModeLegacy (or unset):
//   - Routes based on per-organization protection state
//   - Uses envelope if organization registry shows active status
//   - Uses legacy if organization has no registry record
//
// For envelope mode:
//   - Validates field context
//   - Gets the active-version AEAD primitive from KeysetManager
//   - Encrypts with canonical AAD
//   - Returns marked ciphertext (tink:v{keysetVersion}:{base64})
//
// For legacy mode:
//   - Uses libCommons crypto.Encrypt
//   - Returns unmarked ciphertext
func (s *encryptionService) Encrypt(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.protection.encrypt_field")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.organization_id", fieldCtx.OrganizationID))
	// FieldName is a non-sensitive field identifier (NAME, never the value).
	span.SetAttributes(attribute.String("app.request.field", fieldCtx.FieldName))

	// Check context before any work
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Validate field context. This happens BEFORE a path is chosen, so a failure
	// is attributed to path="unknown" with the field_context_invalid classifier.
	if err := fieldCtx.Validate(); err != nil {
		errWrap := fmt.Errorf("%w: %v", ErrFieldContextInvalid, err)
		libOpenTelemetry.HandleSpanError(span, "invalid field context", errWrap)
		s.metrics.recordEncryptDecrypt(ctx, protectionPathUnknown, protectionOutcomeFailure, errorTypeFieldContextInvalid)

		return "", errWrap
	}

	// Resolve the encryption path (envelope vs legacy).
	path, useEnvelope, err := s.resolveEncryptPath(ctx, fieldCtx.OrganizationID)
	if err != nil {
		// Path resolution failure has no stable per-branch classifier; record the
		// failure on the resolved-or-unknown path without a raw-error label.
		libOpenTelemetry.HandleSpanError(span, "failed to resolve protection state", err)
		s.metrics.recordEncryptDecrypt(ctx, path, protectionOutcomeFailure, "")

		return "", err
	}

	span.SetAttributes(attribute.String("app.protection.path", path))

	var result string

	if useEnvelope {
		result, err = s.encryptEnvelope(ctx, fieldCtx, plaintext)
	} else {
		result, err = s.encryptLegacy(ctx, plaintext)
	}

	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "encrypt failed", err)
		s.metrics.recordEncryptDecrypt(ctx, path, protectionOutcomeFailure, "")

		return "", err
	}

	s.metrics.recordEncryptDecrypt(ctx, path, protectionOutcomeSuccess, "")

	return result, nil
}

// resolveEncryptPath determines whether encryption uses the envelope or legacy
// path and returns the matching path label. In EncryptionModeEnvelope the path is
// always envelope; otherwise it resolves per-organization protection state. On a
// resolution error the path label is "unknown".
func (s *encryptionService) resolveEncryptPath(ctx context.Context, organizationID string) (path string, useEnvelope bool, err error) {
	if s.encryptionMode == crypto.EncryptionModeEnvelope {
		return protectionPathEnvelope, true, nil
	}

	state, err := s.stateResolver.Resolve(ctx, organizationID)
	if err != nil {
		return protectionPathUnknown, false, fmt.Errorf("failed to resolve protection state: %w", err)
	}

	if state.MustUseEnvelope() {
		return protectionPathEnvelope, true, nil
	}

	return protectionPathLegacy, false, nil
}

// encryptEnvelope performs envelope encryption using the active keyset's Tink AEAD.
func (s *encryptionService) encryptEnvelope(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	// Get the active-version primitives (auto-provisions version 1 on first access).
	prims, err := s.keysetManager.GetActivePrimitives(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get AEAD primitive: %w", err)
	}

	// Encrypt with canonical AAD
	aad := fieldCtx.CanonicalAAD()

	ciphertext, err := prims.AEAD.Encrypt([]byte(plaintext), aad)
	if err != nil {
		return "", fmt.Errorf("AEAD encryption failed: %w", err)
	}

	// Stamp the marker with the active keyset VERSION (NOT the Tink primary key id);
	// decrypt routes on this version.
	marked := FormatEnvelopeMarker(prims.Version, ciphertext)

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
//   - Parse marker to get the keyset version
//   - Fail closed if the version is not in the org's readable versions
//   - Load the version-specific AEAD primitive from KeysetManager
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
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.protection.decrypt_field")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.organization_id", fieldCtx.OrganizationID))
	// FieldName is a non-sensitive field identifier (NAME, never the value).
	span.SetAttributes(attribute.String("app.request.field", fieldCtx.FieldName))

	// Check context before any work
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Validate field context. This happens BEFORE a path is chosen, so a failure
	// is attributed to path="unknown" with the field_context_invalid classifier.
	if err := fieldCtx.Validate(); err != nil {
		errWrap := fmt.Errorf("%w: %v", ErrFieldContextInvalid, err)
		libOpenTelemetry.HandleSpanError(span, "invalid field context", errWrap)
		s.metrics.recordEncryptDecrypt(ctx, protectionPathUnknown, protectionOutcomeFailure, errorTypeFieldContextInvalid)

		return "", errWrap
	}

	// Check for envelope marker
	marker, hasMarker, err := ParseEnvelopeMarker(ciphertext)
	if err != nil {
		// Marker parse failure is unclassified and precedes path selection.
		libOpenTelemetry.HandleSpanError(span, "failed to parse envelope marker", err)
		s.metrics.recordEncryptDecrypt(ctx, protectionPathUnknown, protectionOutcomeFailure, "")

		return "", fmt.Errorf("failed to parse envelope marker: %w", err)
	}

	// Path derived from marker presence: marker -> envelope, else legacy.
	if hasMarker {
		span.SetAttributes(attribute.String("app.protection.path", protectionPathEnvelope))

		plaintext, derr := s.decryptEnvelope(ctx, fieldCtx, marker)
		if derr != nil {
			libOpenTelemetry.HandleSpanError(span, "envelope decrypt failed", derr)
			s.metrics.recordEncryptDecrypt(ctx, protectionPathEnvelope, protectionOutcomeFailure, errorTypeEnvelopeDecrypt)

			return "", derr
		}

		s.metrics.recordEncryptDecrypt(ctx, protectionPathEnvelope, protectionOutcomeSuccess, "")

		return plaintext, nil
	}

	span.SetAttributes(attribute.String("app.protection.path", protectionPathLegacy))

	plaintext, derr := s.decryptLegacy(ctx, fieldCtx, ciphertext)
	if derr != nil {
		libOpenTelemetry.HandleSpanError(span, "legacy decrypt failed", derr)
		s.metrics.recordEncryptDecrypt(ctx, protectionPathLegacy, protectionOutcomeFailure, classifyLegacyDecryptError(derr))

		return "", derr
	}

	s.metrics.recordEncryptDecrypt(ctx, protectionPathLegacy, protectionOutcomeSuccess, "")

	return plaintext, nil
}

// classifyLegacyDecryptError maps a legacy decrypt error to a short, stable
// error_type classifier (never the raw error text). Rejected legacy reads map to
// legacy_read_not_allowed; other failures carry no classifier label.
func classifyLegacyDecryptError(err error) string {
	if errors.Is(err, ErrLegacyReadNotAllowed) {
		return errorTypeLegacyReadNotAllow
	}

	return ""
}

// decryptEnvelope performs version-routed envelope decryption.
//
// The marker carries the organization keyset VERSION that produced the ciphertext.
// Decrypt resolves the organization's protection state and fails closed when the
// marker version is not in state.ReadableVersions (no fallback to legacy). It then
// loads the exact-version primitives via GetPrimitivesForVersion and decrypts. Tink
// still selects the concrete key internally from the ciphertext prefix; the version
// only routes which keyset is loaded. Returns ErrEnvelopeDecryptFailed on failure.
func (s *encryptionService) decryptEnvelope(ctx context.Context, fieldCtx FieldContext, marker EnvelopeMarker) (string, error) {
	// Resolve protection state to enforce readable-version routing (fail-closed).
	state, err := s.stateResolver.Resolve(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("%w: failed to resolve protection state: %v", ErrEnvelopeDecryptFailed, err)
	}

	if !versionIsReadable(marker.Version, state.ReadableVersions) {
		return "", fmt.Errorf("%w: marker version %d is not in the organization's readable versions", ErrEnvelopeDecryptFailed, marker.Version)
	}

	// Load the primitives for the exact marker version (no auto-provision).
	prims, err := s.keysetManager.GetPrimitivesForVersion(ctx, fieldCtx.OrganizationID, int(marker.Version))
	if err != nil {
		return "", fmt.Errorf("%w: failed to get AEAD primitive: %v", ErrEnvelopeDecryptFailed, err)
	}

	// Decrypt with canonical AAD
	aad := fieldCtx.CanonicalAAD()

	plaintext, err := prims.AEAD.Decrypt(marker.Payload, aad)
	if err != nil {
		// NO FALLBACK to legacy - envelope decryption must succeed or fail definitively
		return "", fmt.Errorf("%w: %v", ErrEnvelopeDecryptFailed, err)
	}

	return string(plaintext), nil
}

// versionIsReadable reports whether the marker version is present in the
// organization's readable-versions set. An empty/nil set is never readable
// (fail-closed for legacy/unprovisioned organizations).
func versionIsReadable(version uint32, readable []int) bool {
	target := int(version)
	for _, v := range readable {
		if v == target {
			return true
		}
	}

	return false
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

	// Record an attempted (allowed) legacy read. The organization_status label
	// carries the organization's resolved protection mode (state.Mode.String():
	// "legacy"/"envelope"); the legacy read happens within that mode. This is
	// emitted only for allowed reads, never on the ErrLegacyReadNotAllowed
	// rejection above. Emission is nil-safe and best-effort.
	s.metrics.recordLegacyRead(ctx, state.Mode.String())

	// Check context before crypto operation
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Branch the legacy decrypt by source of legacy material (per-org resolution,
	// not the service-level mode). The CanReadLegacy gate and legacy-read metric
	// above are unchanged. An envelope-mode org (a keyset exists) carries any
	// imported legacy key inside its per-org composite keyset, so unmarked legacy
	// bytes decrypt through the keyset AEAD; the process-global legacyCrypto is
	// consulted only in pure legacy mode (KMS_VENDOR=none, no keyset). For a
	// lazy-provisioned envelope org that never migrated legacy data, the keyset
	// has no legacy key and the AEAD decrypt fails on the legacy path - correct,
	// because that org never wrote legacy data.
	if state.MustUseEnvelope() {
		return s.decryptLegacyFromKeyset(ctx, fieldCtx, ciphertext)
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

// decryptLegacyFromKeyset decrypts unmarked legacy ciphertext using the
// organization's per-org composite AEAD primitive instead of the process-global
// legacyCrypto. Migrated organizations carry the imported legacy RAW-prefix
// AES-GCM key inside their composite keyset, so the legacy bytes decrypt through
// prims.AEAD.Decrypt.
//
// The input format mirrors LegacyKeyMaterial.Decrypt: base64(nonce||ciphertext||
// tag) written with nil associated data. The canonical envelope AAD MUST NOT be
// used for legacy bytes. This helper performs decode + crypto ONLY; the
// CanReadLegacy gate and legacy-read metric remain in the caller.
func (s *encryptionService) decryptLegacyFromKeyset(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Mirror LegacyKeyMaterial.Decrypt: base64 decode before AEAD decrypt.
	cipherBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode legacy ciphertext: %w", err)
	}

	prims, err := s.keysetManager.GetActivePrimitives(ctx, fieldCtx.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get AEAD primitive: %w", err)
	}

	// Legacy data was written with nil associated data; the canonical envelope
	// AAD MUST NOT be applied here.
	plainBytes, err := prims.AEAD.Decrypt(cipherBytes, nil)
	if err != nil {
		return "", fmt.Errorf("keyset legacy decrypt: %w", err)
	}

	return string(plainBytes), nil
}

// GenerateSearchToken generates a deterministic search token for a field value.
// Used for encrypted field searching without exposing plaintext.
//
// When encryptionMode is EncryptionModeEnvelope:
//   - Always uses envelope PRF generation regardless of per-org registry state
//   - Triggers lazy provisioning via KeysetManager.GetActivePrimitives() for non-provisioned orgs
//
// When encryptionMode is EncryptionModeLegacy (or unset):
//   - Routes based on per-organization protection state
//
// For envelope mode:
//   - Gets PRF primitive from KeysetManager
//   - Computes PRF of canonical input (tenant:org:field:value)
//   - Returns base64-encoded token
//
// For legacy mode:
//   - Uses libCommons crypto hash function
//   - Returns legacy hash token
func (s *encryptionService) GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, uint32, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	// Validate search context
	if err := searchCtx.Validate(); err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrSearchContextInvalid, err)
	}

	// encryption mode envelope: always use envelope PRF (triggers lazy provisioning)
	if s.encryptionMode == crypto.EncryptionModeEnvelope {
		return s.generateSearchTokenEnvelope(ctx, searchCtx, normalizedValue)
	}

	// Legacy encryption mode: resolve protection state per organization
	state, err := s.stateResolver.Resolve(ctx, searchCtx.OrganizationID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve protection state: %w", err)
	}

	// Route based on per-org mode
	if state.MustUseEnvelope() {
		return s.generateSearchTokenEnvelope(ctx, searchCtx, normalizedValue)
	}

	// True legacy-hash branch: key version is 0.
	token, err := s.generateSearchTokenLegacy(normalizedValue)
	if err != nil {
		return "", 0, err
	}

	return token, 0, nil
}

// generateSearchTokenEnvelope generates a PRF-based search token using Tink and
// returns the PRF keyset primary key ID it was computed with.
func (s *encryptionService) generateSearchTokenEnvelope(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, uint32, error) {
	// Write path: index with the ACTIVE version's PRF primary key (auto-provisions v1).
	prims, err := s.keysetManager.GetActivePrimitives(ctx, searchCtx.OrganizationID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get PRF primitive: %w", err)
	}

	// Compute PRF of canonical input
	canonicalInput := searchCtx.CanonicalInput(normalizedValue)

	token, err := prims.PRF.ComputeSearchToken(canonicalInput)
	if err != nil {
		return "", 0, fmt.Errorf("failed to compute search token: %w", err)
	}

	return token, prims.PRFPrimaryKeyID, nil
}

// generateSearchTokenLegacy generates a hash-based search token using the LegacyCrypto interface.
// Fails closed when no legacy crypto is configured: a missing impl must never be
// treated as a valid (empty) token, mirroring encryptLegacy/decryptLegacy.
func (s *encryptionService) generateSearchTokenLegacy(normalizedValue string) (string, error) {
	if s.legacyCrypto == nil {
		return "", fmt.Errorf("legacy crypto is required")
	}

	return s.legacyCrypto.GenerateHash(&normalizedValue), nil
}

// GenerateSearchTokenCandidates generates search tokens for all enabled keys, routing to
// envelope or legacy based on encryptionMode and per-org protection state.
func (s *encryptionService) GenerateSearchTokenCandidates(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) ([]string, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate search context
	if err := searchCtx.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchContextInvalid, err)
	}

	// Resolve state in both paths so CanReadLegacy is known before producing envelope candidates.
	state, err := s.stateResolver.Resolve(ctx, searchCtx.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve protection state: %w", err)
	}

	if s.encryptionMode == crypto.EncryptionModeEnvelope || state.MustUseEnvelope() {
		return s.generateSearchTokenCandidatesEnvelope(ctx, searchCtx, normalizedValue, state.CanReadLegacy)
	}

	return s.generateSearchTokenCandidatesLegacy(normalizedValue)
}

// generateSearchTokenCandidatesEnvelope generates PRF tokens for all enabled search-token keys.
//
// When canReadLegacy is true and the per-organization keyset carries an imported legacy
// HMAC key (migrated org), it appends that keyset's legacy hex token (over the bare value)
// as the final candidate. This token is byte-identical to the indexed legacy token, so
// migrated orgs can still find their pre-migration rows.
//
// Envelope-only organizations (LegacyHexTokenPRF nil) never wrote legacy tokens, so no
// legacy candidate is appended and the process-global legacyCrypto is never consulted.
func (s *encryptionService) generateSearchTokenCandidatesEnvelope(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string, canReadLegacy bool) ([]string, error) {
	// Read path: candidates are generated from the ACTIVE version's MultiKeyPRF.
	// Multi-version search fan-out (querying older keyset versions after rotation)
	// is rotation work and is deferred/out of scope here.
	prims, err := s.keysetManager.GetActivePrimitives(ctx, searchCtx.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MultiKeyPRF primitive: %w", err)
	}

	// Compute PRF of canonical input using all enabled keys
	canonicalInput := searchCtx.CanonicalInput(normalizedValue)

	tokens, err := prims.MultiKeyPRF.ComputeSearchTokenCandidates(canonicalInput)
	if err != nil {
		return nil, fmt.Errorf("failed to compute search token candidates: %w", err)
	}

	// Union the per-org keyset legacy token when legacy reads are permitted AND the
	// keyset carries an imported legacy key (migrated org). Envelope-only orgs never
	// wrote legacy tokens, so no legacy candidate is appended.
	if canReadLegacy && prims.LegacyHexTokenPRF != nil {
		legacyToken, err := prims.LegacyHexTokenPRF.ComputeLegacyHexToken([]byte(normalizedValue))
		if err != nil {
			// Fail loud: a migrated org that cannot produce its legacy candidate would
			// silently fail to find legacy rows.
			return nil, fmt.Errorf("failed to compute legacy search token candidate: %w", err)
		}

		tokens = append(tokens, legacyToken)
	}

	return tokens, nil
}

// generateSearchTokenCandidatesLegacy generates a single hash-based search token for legacy mode.
// Returns a single-element slice containing the legacy hash token, or an error when
// no legacy crypto is configured (fail-closed: never returns an empty token).
func (s *encryptionService) generateSearchTokenCandidatesLegacy(normalizedValue string) ([]string, error) {
	token, err := s.generateSearchTokenLegacy(normalizedValue)
	if err != nil {
		return nil, err
	}

	return []string{token}, nil
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

	// Legacy/no-keyset setups leave keysetRepo nil. Fail closed with the not-found
	// sentinel rather than dereferencing nil.
	if s.keysetRepo == nil {
		return nil, constant.ErrKeysetNotFound
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
