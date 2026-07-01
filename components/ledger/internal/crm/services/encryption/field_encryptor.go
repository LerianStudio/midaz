// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"strings"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// EncryptionContext provides the context for field encryption operations.
// It carries tenant, organization, and record identifiers used to bind ciphertext.
type EncryptionContext struct {
	TenantID       string
	OrganizationID string
	RecordID       string
}

// ExtractTenantID extracts tenant ID from context or returns "default" for single-tenant mode.
// "default" is a reserved sentinel mapping to the flat-base mount; externally supplied
// "default" is rejected at the provisioning boundary to avoid collision.
func ExtractTenantID(ctx context.Context) string {
	if tenantID := tmcore.GetTenantIDContext(ctx); tenantID != "" {
		return tenantID
	}

	return "default"
}

// ResolveProvisionTenantID resolves the tenant id for provisioning. It rejects an
// externally-supplied "default" (the reserved single-tenant sentinel) with
// ErrReservedTenantID; an empty context maps to the "default" sentinel. Shared by the
// HTTP handler and lazy autoProvision so the rule is single-sourced.
func ResolveProvisionTenantID(ctx context.Context) (string, error) {
	raw := strings.Trim(tmcore.GetTenantIDContext(ctx), "/ \t")

	if raw == "default" {
		return "", pkg.ValidateBusinessError(constant.ErrReservedTenantID, EntityOrganizationEncryption)
	}

	if raw == "" {
		return "default", nil
	}

	return raw, nil
}

// FieldEncryptor provides field-level encryption for repository adapters.
// It wraps EncryptionService and routes based on organization state.
// Repository adapters use this interface to encrypt/decrypt sensitive fields
// without coupling to the underlying encryption implementation.
type FieldEncryptor interface {
	// EncryptField encrypts a plaintext value using the provided field context.
	// The field context binds the ciphertext to tenant, organization, record, and field.
	EncryptField(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error)

	// DecryptField decrypts a ciphertext value using the provided field context.
	// The field context must match the context used during encryption.
	DecryptField(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error)

	// GenerateSearchToken generates a deterministic search token for a normalized value.
	// Search tokens enable encrypted field searching without exposing plaintext.
	// It also returns the PRF keyset primary key ID the token was computed with (0 on the
	// legacy-hash branch).
	GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, uint32, error)

	// GenerateSearchTokenCandidates generates search tokens using all enabled keys for key rotation support.
	// Returns tokens from all enabled HMAC keys to support searching records indexed with any key version.
	GenerateSearchTokenCandidates(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) ([]string, error)
}

// fieldEncryptorAdapter implements FieldEncryptor using EncryptionService.
// It delegates all encryption operations to the underlying service while
// providing a repository-friendly interface.
type fieldEncryptorAdapter struct {
	encryptionService EncryptionService
}

// NewFieldEncryptorAdapter creates a new FieldEncryptor.
// The encryptionService parameter provides the actual encryption implementation.
func NewFieldEncryptorAdapter(encryptionService EncryptionService) FieldEncryptor {
	return &fieldEncryptorAdapter{
		encryptionService: encryptionService,
	}
}

// EncryptField encrypts a plaintext value using the provided field context.
func (a *fieldEncryptorAdapter) EncryptField(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	return a.encryptionService.Encrypt(ctx, fieldCtx, plaintext)
}

// DecryptField decrypts a ciphertext value using the provided field context.
func (a *fieldEncryptorAdapter) DecryptField(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error) {
	return a.encryptionService.Decrypt(ctx, fieldCtx, ciphertext)
}

// GenerateSearchToken generates a deterministic search token for a normalized value.
func (a *fieldEncryptorAdapter) GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, uint32, error) {
	return a.encryptionService.GenerateSearchToken(ctx, searchCtx, normalizedValue)
}

// GenerateSearchTokenCandidates generates search tokens using all enabled keys.
func (a *fieldEncryptorAdapter) GenerateSearchTokenCandidates(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) ([]string, error) {
	return a.encryptionService.GenerateSearchTokenCandidates(ctx, searchCtx, normalizedValue)
}
