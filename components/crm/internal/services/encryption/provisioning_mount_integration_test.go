//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	vaulttestutil "github.com/LerianStudio/midaz/v3/tests/utils/vault"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Provision_AutoCreatesMissingMount proves the reactive mount
// recovery end-to-end against a real Vault Transit engine: provisioning a tenant
// whose per-tenant mount was never enabled creates the mount and succeeds, and a
// re-provision of the same organization is idempotent.
func TestIntegration_Provision_AutoCreatesMissingMount(t *testing.T) {
	ctx := context.Background()

	// Vault: enable only the base mount, NOT the per-tenant sub-mount.
	vaultContainer := vaulttestutil.SetupContainer(t)
	vaultClient := vaulttestutil.CreateClient(t, vaultContainer)
	vaulttestutil.EnableTransitMount(t, vaultContainer, "transit")

	// MongoDB-backed keyset + registry repositories.
	mongoContainer := mongotestutil.SetupContainer(t)
	conn := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)

	keysetRepo, err := mongoEncryption.NewKeysetMongoDBRepository(conn)
	require.NoError(t, err)

	registryRepo, err := mongoEncryption.NewRegistryMongoDBRepository(conn)
	require.NoError(t, err)

	keysetFactory := tink.NewKeysetFactory(vaultClient)

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetFactory,
		ProvisioningConfig{KEKMountPath: "transit", MultiTenant: true},
		newSpyAuditWriter(), NewProtectionMetrics(nil), nil, vaultClient)

	tenantID := uuid.New().String()
	orgID := "org-" + uuid.New().String()[:8]

	req := ProvisionInput{
		TenantID:       tenantID,
		OrganizationID: orgID,
		Actor:          "integration-test",
		Reason:         "auto-create missing mount",
	}

	// The per-tenant mount does not exist yet: provisioning must create it.
	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "provisioning must auto-create the missing per-tenant mount and succeed")
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Auto-creation is proven by the provisioning success above. Here we assert
	// the separate idempotency property: creating the same mount again succeeds.
	require.NoError(t, vaultClient.EnsureTransitMount(ctx, "transit/"+tenantID),
		"creating an already-existing mount must be idempotent success")

	// Re-provisioning the same organization is idempotent (no new keyset).
	resultAgain, err := svc.Provision(ctx, req)
	require.NoError(t, err, "re-provisioning the same org must be idempotent")
	assert.Equal(t, result.OrganizationID, resultAgain.OrganizationID)
	assert.Equal(t, result.AEADPrimaryKeyID, resultAgain.AEADPrimaryKeyID, "idempotent re-provision must return the same primary key id")
}
