//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"

	mongoEncryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	vaulttestutil "github.com/LerianStudio/midaz/v4/tests/utils/vault"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Provision_MultiTenant_SharedEngine proves the shared-engine
// topology end-to-end against a real Vault Transit engine: with only the single
// pre-provisioned "transit-mt" engine enabled (and NO per-tenant mount creation),
// provisioning a multi-tenant organization succeeds and its wrapped AEAD keyset
// round-trips (unwrap + encrypt/decrypt). The KEK therefore resolves to mount
// "transit-mt" with the tenant-scoped key name "{tenant}_org-{org}".
func TestIntegration_Provision_MultiTenant_SharedEngine(t *testing.T) {
	ctx := context.Background()

	// Vault: enable ONLY the shared multi-tenant engine. No per-tenant mounts.
	vaultContainer := vaulttestutil.SetupContainer(t)
	vaultClient := vaulttestutil.CreateClient(t, vaultContainer)
	vaulttestutil.EnableTransitMount(t, vaultContainer, "transit-mt")

	// MongoDB-backed keyset + registry repositories.
	mongoContainer := mongotestutil.SetupContainer(t)
	conn := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)

	keysetRepo, err := mongoEncryption.NewKeysetMongoDBRepository(conn)
	require.NoError(t, err)

	registryRepo, err := mongoEncryption.NewRegistryMongoDBRepository(conn)
	require.NoError(t, err)

	keysetFactory := tink.NewKeysetFactory(vaultClient)

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetFactory,
		ProvisioningConfig{KEKMountPath: "transit-mt", MultiTenant: true},
		newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	tenantID := uuid.New().String()
	orgID := "org-" + uuid.New().String()[:8]

	req := ProvisionInput{
		TenantID:       tenantID,
		OrganizationID: orgID,
		Actor:          "integration-test",
		Reason:         "shared-engine provisioning",
	}

	// Provisioning uses the shared engine verbatim; the transit key is auto-created
	// on first encrypt. No mount creation happens.
	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "provisioning must succeed against the shared engine")
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// The persisted keyset must carry the shared engine and the tenant-scoped key name.
	saved, err := keysetRepo.Get(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "transit-mt", saved.KEKMountPath, "wrap mount must be the shared engine verbatim")
	assert.Equal(t, tenantID+"_org-"+orgID, saved.KEKPath, "key name must be tenant-scoped")

	// Round-trip proof: the wrapped AEAD keyset unwraps via mount "transit-mt" + key
	// "{tenant}_org-{org}", and the unwrapped primitive encrypts and decrypts.
	wrapped := tink.WrappedKeyset{WrappedData: saved.WrappedKeyset}

	aead, err := keysetFactory.UnwrapAEAD(ctx, saved.KEKMountPath, saved.KEKPath, wrapped)
	require.NoError(t, err, "wrapped keyset must unwrap via the shared engine and tenant-scoped key")

	plaintext := []byte("shared-engine round-trip")
	associatedData := []byte("context:integration")

	ciphertext, err := aead.Encrypt(plaintext, associatedData)
	require.NoError(t, err)

	decrypted, err := aead.Decrypt(ciphertext, associatedData)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted, "round-trip must recover the original plaintext")
}
