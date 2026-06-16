// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestService_GenerateSearchToken_Envelope_ReturnsPRFKeyVersion verifies that the
// envelope path returns a non-zero keyVersion equal to the provisioned PRF keyset
// primary key ID, and that the token base64url-decodes to a RAW 32-byte PRF value
// (not a TINK-prefixed MAC tag).
func TestService_GenerateSearchToken_Envelope_ReturnsPRFKeyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-prf",
		TenantID:             "tenant-prf",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, keyset := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-prf",
		OrganizationID: "org-prf",
		FieldName:      "document",
	}

	token, keyVersion, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// keyVersion must equal the provisioned PRF (HMAC) keyset primary key ID.
	assert.NotZero(t, keyVersion, "envelope search token must carry a non-zero PRF key version")
	assert.Equal(t, keyset.HMACKeysetInfo.PrimaryKeyID, keyVersion)

	// PRF output is RAW (no Tink key-id prefix) and fixed at 32 bytes.
	raw, decErr := base64.URLEncoding.DecodeString(token)
	require.NoError(t, decErr)
	assert.Len(t, raw, 32, "PRF token must decode to RAW 32 bytes")
}

// TestService_GenerateSearchToken_Legacy_ReturnsZeroKeyVersion verifies that the
// true legacy-hash branch returns keyVersion == 0.
func TestService_GenerateSearchToken_Legacy_ReturnsZeroKeyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Empty registry (no record) -> legacy mode.
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	token, keyVersion, err := svc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	assert.Zero(t, keyVersion, "legacy-hash branch must return key version 0")
	assert.Equal(t, legacyKeys.GenerateHash(&normalizedValue), token)
}

// TestService_GenerateSearchTokenCandidates_MigratedOrg_PreservesLegacyUnion
// verifies the Phase-1 union: a migrated org (envelope + CanReadLegacy) yields the
// PRF candidate(s) AND a trailing legacy bare-value token.
func TestService_GenerateSearchTokenCandidates_MigratedOrg_PreservesLegacyUnion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        true,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-migrated",
		TenantID:             "tenant-migrated",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-migrated",
		OrganizationID: "org-migrated",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tokens), 2, "must include at least one PRF candidate plus the legacy token")

	// Trailing element must be the legacy bare-value token (Phase-1 union preserved).
	legacyToken := legacyKeys.GenerateHash(&normalizedValue)
	assert.Equal(t, legacyToken, tokens[len(tokens)-1], "last candidate must be the legacy bare-value token")

	// Leading PRF candidate(s) must be RAW 32-byte PRF values, distinct from the legacy hex token.
	raw, decErr := base64.URLEncoding.DecodeString(tokens[0])
	require.NoError(t, decErr)
	assert.Len(t, raw, 32)
	assert.NotEqual(t, legacyToken, tokens[0])
}
