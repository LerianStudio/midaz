// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fakes and Helpers
// ---------------------------------------------------------------------------

// fakeKeysetRepoForProv implements mongoEncryption.KeysetRepository for provisioning tests.
type fakeKeysetRepoForProv struct {
	keysets    map[string]*mmodel.OrganizationKeyset
	saveErr    error
	saveCalled int
}

func newFakeKeysetRepoForProv() *fakeKeysetRepoForProv {
	return &fakeKeysetRepoForProv{
		keysets: make(map[string]*mmodel.OrganizationKeyset),
	}
}

func (f *fakeKeysetRepoForProv) Save(_ context.Context, keyset *mmodel.OrganizationKeyset) error {
	f.saveCalled++

	if f.saveErr != nil {
		return f.saveErr
	}

	if _, exists := f.keysets[keyset.OrganizationID]; exists {
		return mmodel.ErrKeysetAlreadyExists
	}

	f.keysets[keyset.OrganizationID] = keyset

	return nil
}

func (f *fakeKeysetRepoForProv) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	keyset, ok := f.keysets[organizationID]
	if !ok {
		return nil, mmodel.ErrKeysetNotFound
	}

	return keyset, nil
}

func (f *fakeKeysetRepoForProv) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// fakeRegistryRepoForProv implements mongoEncryption.RegistryRepository for provisioning tests.
type fakeRegistryRepoForProv struct {
	records      map[string]*mmodel.OrganizationRegistryRecord
	saveErr      error
	getErr       error
	updateErr    error
	saveCalled   int
	returnNilNil bool // When true, Get returns (nil, nil) to simulate edge case
}

func newFakeRegistryRepoForProv() *fakeRegistryRepoForProv {
	return &fakeRegistryRepoForProv{
		records: make(map[string]*mmodel.OrganizationRegistryRecord),
	}
}

func (f *fakeRegistryRepoForProv) Save(_ context.Context, record *mmodel.OrganizationRegistryRecord) error {
	f.saveCalled++

	if f.saveErr != nil {
		return f.saveErr
	}

	if _, exists := f.records[record.OrganizationID]; exists {
		return mmodel.ErrRegistryAlreadyExists
	}

	f.records[record.OrganizationID] = record

	return nil
}

func (f *fakeRegistryRepoForProv) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	// Simulate edge case where repository returns (nil, nil)
	if f.returnNilNil {
		return nil, nil
	}

	record, ok := f.records[organizationID]
	if !ok {
		return nil, mmodel.ErrRegistryNotFound
	}

	// Return a copy to avoid mutation affecting the stored record
	copyRecord := *record
	copyRecord.ReadableVersions = make([]int, len(record.ReadableVersions))
	copy(copyRecord.ReadableVersions, record.ReadableVersions)

	return &copyRecord, nil
}

func (f *fakeRegistryRepoForProv) Update(_ context.Context, record *mmodel.OrganizationRegistryRecord, expectedRevision int64) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	existing, ok := f.records[record.OrganizationID]
	if !ok {
		return mmodel.ErrRegistryNotFound
	}

	if existing.Revision != expectedRevision {
		return mmodel.ErrRegistryRevisionConflict
	}

	f.records[record.OrganizationID] = record

	return nil
}

// fakeKeysetGenerator implements KeysetGenerator for tests.
type fakeKeysetGenerator struct {
	aeadBundle    tink.KeysetBundle
	macBundle     tink.KeysetBundle
	aeadErr       error
	macErr        error
	aeadCalled    int
	macCalled     int
	aeadMountPath string // records the mountPath received by GenerateAEADKeyset
	macMountPath  string // records the mountPath received by GenerateMACKeyset
	nextKeyID     uint32
	callSequencer int
}

func newFakeKeysetGenerator() *fakeKeysetGenerator {
	return &fakeKeysetGenerator{
		nextKeyID: 1000,
	}
}

func (f *fakeKeysetGenerator) GenerateAEADKeyset(_ context.Context, mountPath, _ string) (tink.KeysetBundle, error) {
	f.aeadCalled++
	f.callSequencer++
	f.aeadMountPath = mountPath

	if f.aeadErr != nil {
		return tink.KeysetBundle{}, f.aeadErr
	}

	if f.aeadBundle.Wrapped.WrappedData != "" {
		return f.aeadBundle, nil
	}

	// Generate a unique keyset
	keyID := f.nextKeyID
	f.nextKeyID++

	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "vault:v1:wrapped-aead-data",
			Info: tink.KeysetInfo{
				PrimaryKeyID: keyID,
				Keys: []tink.KeyInfo{
					{
						KeyID:     keyID,
						Status:    tink.KeyStatusEnabled,
						Type:      tink.KeyTypeAES256GCM,
						IsPrimary: true,
					},
				},
			},
			LegacyKeyImported: false,
		},
		RawKeyset: []byte("raw-aead-keyset"),
	}, nil
}

func (f *fakeKeysetGenerator) GenerateMACKeyset(_ context.Context, mountPath, _ string) (tink.KeysetBundle, error) {
	f.macCalled++
	f.callSequencer++
	f.macMountPath = mountPath

	if f.macErr != nil {
		return tink.KeysetBundle{}, f.macErr
	}

	if f.macBundle.Wrapped.WrappedData != "" {
		return f.macBundle, nil
	}

	// Generate a unique keyset
	keyID := f.nextKeyID
	f.nextKeyID++

	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "vault:v1:wrapped-mac-data",
			Info: tink.KeysetInfo{
				PrimaryKeyID: keyID,
				Keys: []tink.KeyInfo{
					{
						KeyID:     keyID,
						Status:    tink.KeyStatusEnabled,
						Type:      tink.KeyTypeHMACSHA256,
						IsPrimary: true,
					},
				},
			},
			LegacyKeyImported: false,
		},
		RawKeyset: []byte("raw-mac-keyset"),
	}, nil
}

// ---------------------------------------------------------------------------
// ProvisionRequest Validation Tests
// ---------------------------------------------------------------------------

func TestProvisionRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     ProvisionInput
		wantErr bool
	}{
		{
			name: "valid request",
			req: ProvisionInput{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				Actor:          "admin@example.com",
				Reason:         "Initial provisioning",
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			req: ProvisionInput{
				TenantID:       "",
				OrganizationID: "org-456",
				Actor:          "admin@example.com",
				Reason:         "Initial provisioning",
			},
			wantErr: true,
		},
		{
			name: "missing organization_id",
			req: ProvisionInput{
				TenantID:       "tenant-123",
				OrganizationID: "",
				Actor:          "admin@example.com",
				Reason:         "Initial provisioning",
			},
			wantErr: true,
		},
		{
			name: "missing actor",
			req: ProvisionInput{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				Actor:          "",
				Reason:         "Initial provisioning",
			},
			wantErr: true,
		},
		{
			name: "missing reason",
			req: ProvisionInput{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				Actor:          "admin@example.com",
				Reason:         "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.req.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Provision Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_Provision_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.NotZero(t, result.AEADPrimaryKeyID)
	assert.NotZero(t, result.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Verify keyset was saved
	assert.Equal(t, 1, keysetRepo.saveCalled)
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "tenant-123", savedKeyset.TenantID)
	assert.Equal(t, "org-456", savedKeyset.OrganizationID)
	assert.NotEmpty(t, savedKeyset.WrappedKeyset)
	assert.NotEmpty(t, savedKeyset.WrappedHMACKeyset)

	// Verify registry was saved
	assert.Equal(t, 1, registryRepo.saveCalled)
	savedRegistry := registryRepo.records["org-456"]
	require.NotNil(t, savedRegistry)
	assert.Equal(t, "tenant-123", savedRegistry.TenantID)
	assert.Equal(t, "org-456", savedRegistry.OrganizationID)
	assert.Equal(t, mmodel.RegistryStatusActive, savedRegistry.Status)
	assert.Equal(t, mmodel.ProtectionModelEnvelope, savedRegistry.ProtectionModel,
		"provisioned registry must have ProtectionModel=envelope")
	assert.True(t, savedRegistry.LegacyReadable, "provisioned registry must have LegacyReadable=true")

	// Verify generators were called
	assert.Equal(t, 1, keysetGenerator.aeadCalled)
	assert.Equal(t, 1, keysetGenerator.macCalled)
}

func TestProvisioningService_Provision_SingleTenant_FlatMount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator,
		ProvisioningConfig{KEKMountPath: "transit"}, newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "default",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.NoError(t, err)

	// Single-tenant ("default") resolves to the flat base mount for both wrap calls.
	assert.Equal(t, "transit", keysetGenerator.aeadMountPath, "AEAD wrap must use flat base mount for default tenant")
	assert.Equal(t, "transit", keysetGenerator.macMountPath, "MAC wrap must use flat base mount for default tenant")

	// The mount must NOT be stored on the keyset record (unwrap re-derives it).
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "org-org-456", savedKeyset.KEKPath, "key name stays org-{id}, unchanged by mount resolution")
}

func TestProvisioningService_Provision_MultiTenant_SubMount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator,
		ProvisioningConfig{KEKMountPath: "transit"}, newSpyAuditWriter(), NewProtectionMetrics(nil))

	const tenantID = "11111111-2222-3333-4444-555555555555"

	req := ProvisionInput{
		TenantID:       tenantID,
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.NoError(t, err)

	// Multi-tenant resolves to a per-tenant sub-mount for both wrap calls.
	want := "transit/" + tenantID
	assert.Equal(t, want, keysetGenerator.aeadMountPath, "AEAD wrap must use per-tenant sub-mount")
	assert.Equal(t, want, keysetGenerator.macMountPath, "MAC wrap must use per-tenant sub-mount")

	// Key name remains org-{id}; mount is not part of it and not stored on the record.
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "org-org-456", savedKeyset.KEKPath)
}

func TestProvisioningService_Provision_MountNotFound_FailsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.aeadErr = fmt.Errorf("wrap aead: %w", vault.ErrMountNotFound)

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator,
		ProvisioningConfig{KEKMountPath: "transit"}, newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "11111111-2222-3333-4444-555555555555",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, vault.ErrMountNotFound, "missing per-tenant mount must fail provisioning closed")

	// No keyset/registry should be persisted on a failed-closed provision.
	assert.Empty(t, keysetRepo.keysets, "no keyset should be saved when mount is missing")
	assert.Empty(t, registryRepo.records, "no registry should be saved when mount is missing")
}

func TestProvisioningService_Provision_AlreadyProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset AND registry (truly already provisioned)
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}
	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	// Idempotent behavior: should succeed and return existing info
	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "provision should be idempotent - succeeds if already provisioned")

	// Verify returned info matches existing keyset/registry
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.Equal(t, uint32(12345), result.AEADPrimaryKeyID)
	assert.Equal(t, uint32(67890), result.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Verify no new keyset was generated (idempotent - no work done)
	assert.Equal(t, 0, keysetGenerator.aeadCalled, "should not generate new keyset for already provisioned org")
	assert.Equal(t, 0, keysetGenerator.macCalled, "should not generate new keyset for already provisioned org")
}

func TestProvisioningService_Provision_RecoveryFromPartialFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset but NO registry (partial failure scenario)
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Recovery from partial failure",
	}

	// Should succeed by completing the interrupted provisioning
	result, err := svc.Provision(ctx, req)
	require.NoError(t, err)

	// Verify result uses existing keyset's key IDs
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.Equal(t, uint32(12345), result.AEADPrimaryKeyID)
	assert.Equal(t, uint32(67890), result.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Verify registry was created
	savedRegistry := registryRepo.records["org-456"]
	require.NotNil(t, savedRegistry)
	assert.Equal(t, mmodel.RegistryStatusActive, savedRegistry.Status)
	assert.True(t, savedRegistry.LegacyReadable, "provisioned registry must have LegacyReadable=true")
}

func TestProvisioningService_Provision_InvalidRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid provision request")
}

func TestProvisioningService_Provision_AEADGenerationFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.aeadErr = errors.New("KMS unavailable")

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_Provision_MACGenerationFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.macErr = errors.New("KMS unavailable")

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_Provision_KeysetSaveFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	keysetRepo.saveErr = errors.New("database error")
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_Provision_RegistrySaveFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	registryRepo.saveErr = errors.New("database error")
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_Provision_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// GetProvisioningStatus Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_GetProvisioningStatus_Provisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with registry
	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	status, err := svc.GetProvisioningStatus(ctx, "org-456")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, mmodel.RegistryStatusActive, *status)
}

func TestProvisioningService_GetProvisioningStatus_NotProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	status, err := svc.GetProvisioningStatus(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.Nil(t, status)
}

func TestProvisioningService_GetProvisioningStatus_EmptyOrgID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	_, err := svc.GetProvisioningStatus(ctx, "")
	require.Error(t, err)
}

func TestProvisioningService_GetProvisioningStatus_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	_, err := svc.GetProvisioningStatus(ctx, "org-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// IsProvisioned Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_IsProvisioned_True(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	provisioned, err := svc.IsProvisioned(ctx, "org-456")
	require.NoError(t, err)
	assert.True(t, provisioned)
}

func TestProvisioningService_IsProvisioned_False(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	provisioned, err := svc.IsProvisioned(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.False(t, provisioned)
}

// ---------------------------------------------------------------------------
// IsActive Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_IsActive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	active, err := svc.IsActive(ctx, "org-456")
	require.NoError(t, err)
	assert.True(t, active, "active status should return true")
}

func TestProvisioningService_IsActive_NotProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	active, err := svc.IsActive(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.False(t, active)
}

// ---------------------------------------------------------------------------
// Configuration Tests
// ---------------------------------------------------------------------------

func TestNewProvisioningService_DefaultMountPath(t *testing.T) {
	t.Parallel()

	svc := NewProvisioningService(nil, nil, nil, ProvisioningConfig{}, newSpyAuditWriter(), NewProtectionMetrics(nil))

	// Type assert to access internal method for testing
	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok, "NewProvisioningService must return *provisioningService")

	// Verify key name format
	kekPath := concreteSvc.buildKEKPath("org-123")
	assert.Equal(t, "org-org-123", kekPath)
}

func TestNewProvisioningService_CustomMountPath(t *testing.T) {
	t.Parallel()

	config := ProvisioningConfig{
		KEKMountPath: "custom-transit",
	}
	svc := NewProvisioningService(nil, nil, nil, config, newSpyAuditWriter(), NewProtectionMetrics(nil))

	// Type assert to access internal method for testing
	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok, "NewProvisioningService must return *provisioningService")

	// Verify key name format (mount path is used internally by Vault client, not in key name)
	kekPath := concreteSvc.buildKEKPath("org-123")
	assert.Equal(t, "org-org-123", kekPath)
}

func TestDefaultProvisioningConfig(t *testing.T) {
	t.Parallel()

	config := DefaultProvisioningConfig()
	assert.Equal(t, "transit", config.KEKMountPath)
}

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

func TestProvisioningService_Provision_RegistryAlreadyExists_ButKeysetMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate registry only (keyset doesn't exist) - inconsistent state
	// This is an edge case where registry was created but keyset wasn't persisted
	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	// Registry exists so IsProvisioned returns true, but getExistingProvisionResult
	// fails because keyset is missing. This is an inconsistent state that should error.
	_, err := svc.Provision(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get keyset")
}

func TestProvisioningService_GetProvisioningStatus_DatabaseError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	registryRepo.getErr = errors.New("database error")
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	_, err := svc.GetProvisioningStatus(ctx, "org-456")
	require.Error(t, err)
}

func TestProvisioningService_GetProvisioningStatus_NilNilFromRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	registryRepo.returnNilNil = true // Simulate edge case: repository returns (nil, nil)
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	// Should handle (nil, nil) gracefully without panic
	status, err := svc.GetProvisioningStatus(ctx, "org-456")
	require.NoError(t, err)
	assert.Nil(t, status, "nil registry should be treated as not provisioned")
}

// ---------------------------------------------------------------------------
// Constant Package Error Compatibility Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_Provision_ConstantPackageErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset AND registry to test idempotent handling
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 11111},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 22222},
	}
	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	// Idempotent behavior: should succeed and return existing info
	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "provision should be idempotent - succeeds if already provisioned")

	// Verify returned info matches existing keyset/registry
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, uint32(11111), result.AEADPrimaryKeyID)
	assert.Equal(t, uint32(22222), result.MACPrimaryKeyID)
}

// ---------------------------------------------------------------------------
// Registry Save Failure + Retry Recovery Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_Provision_RegistrySaveFailure_ThenRetryRecovers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	// STEP 1: First attempt - keyset saves successfully, registry save fails
	registryRepo.saveErr = errors.New("transient database error")

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	// Verify keyset was saved (this is the partial failure state)
	assert.Equal(t, 1, keysetRepo.saveCalled)
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset, "keyset should be persisted despite registry failure")
	assert.Equal(t, "tenant-123", savedKeyset.TenantID)
	assert.Equal(t, "org-456", savedKeyset.OrganizationID)

	// Verify registry was NOT saved
	assert.Equal(t, 1, registryRepo.saveCalled)
	_, registryExists := registryRepo.records["org-456"]
	assert.False(t, registryExists, "registry should NOT exist after failed save")

	// STEP 2: Second attempt (retry) - should recover by detecting existing keyset
	registryRepo.saveErr = nil // Clear the error for retry

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "retry should succeed by recovering from partial failure")

	// Verify result uses existing keyset's key IDs
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.Equal(t, savedKeyset.KeysetInfo.PrimaryKeyID, result.AEADPrimaryKeyID)
	assert.Equal(t, savedKeyset.HMACKeysetInfo.PrimaryKeyID, result.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Verify keyset was NOT regenerated (save called only once more for the existing check)
	assert.Equal(t, 2, keysetRepo.saveCalled, "keyset save should be called again (hitting already exists)")

	// Verify registry was created on retry
	assert.Equal(t, 2, registryRepo.saveCalled)
	savedRegistry := registryRepo.records["org-456"]
	require.NotNil(t, savedRegistry, "registry should be created on retry")
	assert.Equal(t, mmodel.RegistryStatusActive, savedRegistry.Status)
	assert.True(t, savedRegistry.LegacyReadable, "provisioned registry must have LegacyReadable=true")

	// STEP 3: Third attempt should succeed (idempotent behavior)
	thirdResult, err := svc.Provision(ctx, req)
	require.NoError(t, err, "third attempt should succeed - idempotent behavior")

	// Verify it returns the same result as second attempt
	assert.Equal(t, result.OrganizationID, thirdResult.OrganizationID)
	assert.Equal(t, result.KEKPath, thirdResult.KEKPath)
	assert.Equal(t, result.AEADPrimaryKeyID, thirdResult.AEADPrimaryKeyID)
	assert.Equal(t, result.MACPrimaryKeyID, thirdResult.MACPrimaryKeyID)
}

// ---------------------------------------------------------------------------
// Idempotent Provision Tests (TDD-RED: getExistingProvisionResult does not exist)
// ---------------------------------------------------------------------------

func TestProvisioningService_Provision_IdempotentMultipleCalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-idempotent",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	// STEP 1: First provision call - should succeed
	firstResult, err := svc.Provision(ctx, req)
	require.NoError(t, err, "first provision call should succeed")

	assert.Equal(t, "org-idempotent", firstResult.OrganizationID)
	assert.Equal(t, "org-org-idempotent", firstResult.KEKPath)
	assert.NotZero(t, firstResult.AEADPrimaryKeyID)
	assert.NotZero(t, firstResult.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, firstResult.RegistryStatus)

	// Capture generator call counts after first provision
	aeadCallsAfterFirst := keysetGenerator.aeadCalled
	macCallsAfterFirst := keysetGenerator.macCalled

	// STEP 2: Second provision call (same org) - should succeed with same result (idempotent)
	secondResult, err := svc.Provision(ctx, req)
	require.NoError(t, err, "second provision call should succeed (idempotent behavior)")

	// Verify second call returns same info as first
	assert.Equal(t, firstResult.OrganizationID, secondResult.OrganizationID)
	assert.Equal(t, firstResult.KEKPath, secondResult.KEKPath)
	assert.Equal(t, firstResult.AEADPrimaryKeyID, secondResult.AEADPrimaryKeyID)
	assert.Equal(t, firstResult.MACPrimaryKeyID, secondResult.MACPrimaryKeyID)
	assert.Equal(t, firstResult.RegistryStatus, secondResult.RegistryStatus)

	// Verify no new keyset was generated on second call
	assert.Equal(t, aeadCallsAfterFirst, keysetGenerator.aeadCalled,
		"AEAD keyset generator should NOT be called again for idempotent provision")
	assert.Equal(t, macCallsAfterFirst, keysetGenerator.macCalled,
		"MAC keyset generator should NOT be called again for idempotent provision")

	// STEP 3: Third provision call - should still succeed (idempotent)
	thirdResult, err := svc.Provision(ctx, req)
	require.NoError(t, err, "third provision call should succeed (idempotent behavior)")

	assert.Equal(t, firstResult.AEADPrimaryKeyID, thirdResult.AEADPrimaryKeyID)
	assert.Equal(t, firstResult.MACPrimaryKeyID, thirdResult.MACPrimaryKeyID)
}

// ---------------------------------------------------------------------------
// getExistingProvisionResult Tests (TDD-RED: method does not exist yet)
// ---------------------------------------------------------------------------

func TestProvisioningService_getExistingProvisionResult_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset and registry (fully provisioned org)
	keysetRepo.keysets["org-existing"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-existing",
		KEKPath:        "org-org-existing",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 11111},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 22222},
	}
	registryRepo.records["org-existing"] = &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: "org-existing",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	// Type assert to access internal method
	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok, "NewProvisioningService must return *provisioningService")

	// Call getExistingProvisionResult (this method does not exist yet - TDD RED)
	result, err := concreteSvc.getExistingProvisionResult(ctx, "org-existing")
	require.NoError(t, err)

	assert.Equal(t, "org-existing", result.OrganizationID)
	assert.Equal(t, "org-org-existing", result.KEKPath)
	assert.Equal(t, uint32(11111), result.AEADPrimaryKeyID)
	assert.Equal(t, uint32(22222), result.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)
}

func TestProvisioningService_getExistingProvisionResult_EmptyOrgID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok)

	// Call with empty organization ID - should return error
	_, err := concreteSvc.getExistingProvisionResult(ctx, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "organization_id")
}

func TestProvisioningService_getExistingProvisionResult_KeysetNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Registry exists but keyset does not
	registryRepo.records["org-no-keyset"] = &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: "org-no-keyset",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok)

	// Call getExistingProvisionResult when keyset not found - should return error
	_, err := concreteSvc.getExistingProvisionResult(ctx, "org-no-keyset")
	require.Error(t, err)
}

func TestProvisioningService_getExistingProvisionResult_NilKeyset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Registry exists, keyset reader returns (nil, nil) edge case
	registryRepo.records["org-nil-keyset"] = &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: "org-nil-keyset",
		Status:         mmodel.RegistryStatusActive,
	}

	// Create a custom fake that returns (nil, nil) from Get
	nilKeysetRepo := &fakeKeysetRepoNilNil{}

	svc := NewProvisioningService(nilKeysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok)

	// Call getExistingProvisionResult when keyset is nil - should return error
	_, err := concreteSvc.getExistingProvisionResult(ctx, "org-nil-keyset")
	require.Error(t, err)
}

func TestProvisioningService_getExistingProvisionResult_RegistryNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	// Keyset exists but registry does not
	keysetRepo.keysets["org-no-registry"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-no-registry",
		KEKPath:        "org-org-no-registry",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 33333},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 44444},
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok)

	// Call getExistingProvisionResult when registry not found - should return error
	_, err := concreteSvc.getExistingProvisionResult(ctx, "org-no-registry")
	require.Error(t, err)
}

func TestProvisioningService_getExistingProvisionResult_NilRegistry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	registryRepo.returnNilNil = true // Simulate (nil, nil) from registry
	keysetGenerator := newFakeKeysetGenerator()

	// Keyset exists but registry returns (nil, nil)
	keysetRepo.keysets["org-nil-registry"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-nil-registry",
		KEKPath:        "org-org-nil-registry",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 55555},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 66666},
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil))

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok)

	// Call getExistingProvisionResult when registry is nil - should return error
	_, err := concreteSvc.getExistingProvisionResult(ctx, "org-nil-registry")
	require.Error(t, err)
}

// fakeKeysetRepoNilNil implements mongoEncryption.KeysetRepository returning (nil, nil) from Get.
type fakeKeysetRepoNilNil struct{}

func (f *fakeKeysetRepoNilNil) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return nil, nil
}

func (f *fakeKeysetRepoNilNil) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

func (f *fakeKeysetRepoNilNil) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}
