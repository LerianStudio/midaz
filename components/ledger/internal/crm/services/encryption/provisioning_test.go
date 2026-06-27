// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/crypto"
	"github.com/LerianStudio/midaz/v4/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v4/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fakes and Helpers
// ---------------------------------------------------------------------------

// fakeKeysetRepoForProv implements encryption.KeysetRepository for provisioning tests.
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

func (f *fakeKeysetRepoForProv) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *fakeKeysetRepoForProv) GetByVersion(_ context.Context, organizationID string, version int) (*mmodel.OrganizationKeyset, error) {
	keyset, ok := f.keysets[organizationID]
	if !ok || keyset.Version != version {
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
	mixedAEAD     int    // records calls to GenerateMixedAEADKeyset
	mixedPRF      int    // records calls to GenerateMixedPRFKeyset
	gotLegacyAES  string // records the raw legacy AES material received by the mixed AEAD generator
	gotLegacyHMAC string // records the raw legacy HMAC material received by the mixed PRF generator
	aeadMountPath string // records the mountPath received by the AEAD generators
	macMountPath  string // records the mountPath received by the PRF generators
	nextKeyID     uint32
	callSequencer int

	// defaultLegacyAES and defaultLegacyHMAC mirror the bootstrap
	// keysetGeneratorAdapter: when the migration path calls the mixed methods
	// with empty legacy material, the adapter substitutes the process-level
	// secrets. The fake substitutes these so the migration arm can succeed.
	defaultLegacyAES  string
	defaultLegacyHMAC string
}

func newFakeKeysetGenerator() *fakeKeysetGenerator {
	return &fakeKeysetGenerator{
		nextKeyID:         1000,
		defaultLegacyAES:  "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		defaultLegacyHMAC: "legacy-hmac-secret",
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

func (f *fakeKeysetGenerator) GeneratePRFKeyset(_ context.Context, mountPath, _ string) (tink.KeysetBundle, error) {
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
			WrappedData: "vault:v1:wrapped-prf-data",
			Info: tink.KeysetInfo{
				PrimaryKeyID: keyID,
				Keys: []tink.KeyInfo{
					{
						KeyID:     keyID,
						Status:    tink.KeyStatusEnabled,
						Type:      tink.KeyTypeHMACPRF,
						IsPrimary: true,
					},
				},
			},
			LegacyKeyImported: false,
		},
		RawKeyset: []byte("raw-prf-keyset"),
	}, nil
}

// GenerateMixedAEADKeyset returns a composite-shaped AEAD bundle (fresh primary +
// legacy non-primary) for the migration seam. It records the mountPath and fails
// closed when the legacy material is empty.
func (f *fakeKeysetGenerator) GenerateMixedAEADKeyset(_ context.Context, mountPath, _, legacyHexKey string) (tink.KeysetBundle, error) {
	f.aeadCalled++
	f.mixedAEAD++
	f.callSequencer++
	f.aeadMountPath = mountPath

	if f.aeadErr != nil {
		return tink.KeysetBundle{}, f.aeadErr
	}

	if legacyHexKey == "" {
		legacyHexKey = f.defaultLegacyAES
	}

	if legacyHexKey == "" {
		return tink.KeysetBundle{}, fmt.Errorf("legacy AES key material is required for mixed generation")
	}

	// Record the raw legacy material that flowed through the seam so a
	// non-disclosure test can prove the secret was genuinely imported yet never
	// surfaces in any persisted or audited output.
	f.gotLegacyAES = legacyHexKey

	primaryID := f.nextKeyID
	f.nextKeyID++

	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "vault:v1:wrapped-mixed-aead-data",
			Info: tink.KeysetInfo{
				PrimaryKeyID: primaryID,
				Keys: []tink.KeyInfo{
					{KeyID: primaryID, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeAES256GCM, IsPrimary: true},
					// 0xFFFFFFFF mirrors the unexported tink.legacyComposedKeyID sentinel
					// assigned to the imported legacy key in the real composer.
					{KeyID: 0xFFFFFFFF, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeLegacyAESGCM, IsPrimary: false},
				},
			},
			LegacyKeyImported: true,
		},
		RawKeyset: []byte("raw-mixed-aead-keyset"),
	}, nil
}

// GenerateMixedPRFKeyset returns a composite-shaped PRF bundle (fresh primary +
// legacy non-primary) for the migration seam. It records the mountPath and fails
// closed when the legacy secret is empty.
func (f *fakeKeysetGenerator) GenerateMixedPRFKeyset(_ context.Context, mountPath, _, legacySecret string) (tink.KeysetBundle, error) {
	f.macCalled++
	f.mixedPRF++
	f.callSequencer++
	f.macMountPath = mountPath

	if f.macErr != nil {
		return tink.KeysetBundle{}, f.macErr
	}

	if legacySecret == "" {
		legacySecret = f.defaultLegacyHMAC
	}

	if legacySecret == "" {
		return tink.KeysetBundle{}, fmt.Errorf("legacy HMAC secret is required for mixed generation")
	}

	// Record the raw legacy material that flowed through the seam so a
	// non-disclosure test can prove the secret was genuinely imported yet never
	// surfaces in any persisted or audited output.
	f.gotLegacyHMAC = legacySecret

	primaryID := f.nextKeyID
	f.nextKeyID++

	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "vault:v1:wrapped-mixed-prf-data",
			Info: tink.KeysetInfo{
				PrimaryKeyID: primaryID,
				Keys: []tink.KeyInfo{
					{KeyID: primaryID, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeHMACPRF, IsPrimary: true},
					// 0xFFFFFFFF mirrors the unexported tink.legacyComposedKeyID sentinel
					// assigned to the imported legacy key in the real composer.
					{KeyID: 0xFFFFFFFF, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeLegacyHMACSHA256, IsPrimary: false},
				},
			},
			LegacyKeyImported: true,
		},
		RawKeyset: []byte("raw-mixed-prf-keyset"),
	}, nil
}

// TestKeysetGenerator_SeamExposesMixedGeneration is the T-1.2.1 seam gate: the
// KeysetGenerator seam MUST expose mixed (legacy + fresh) generation methods so
// the migration path (T-1.2.2) can route to them. This test only exercises the
// CAPABILITY through the seam; it does not change provision() routing.
func TestKeysetGenerator_SeamExposesMixedGeneration(t *testing.T) {
	t.Parallel()

	var gen KeysetGenerator = newFakeKeysetGenerator()

	aead, err := gen.GenerateMixedAEADKeyset(context.Background(), "transit/tenant-x", "org-1", "deadbeef")
	require.NoError(t, err)
	require.True(t, aead.Wrapped.LegacyKeyImported, "mixed AEAD bundle must flag the imported legacy key")
	require.Len(t, aead.Wrapped.Info.Keys, 2, "mixed AEAD keyset must hold two keys")

	prf, err := gen.GenerateMixedPRFKeyset(context.Background(), "transit/tenant-x", "org-1", "legacy-secret")
	require.NoError(t, err)
	require.True(t, prf.Wrapped.LegacyKeyImported, "mixed PRF bundle must flag the imported legacy key")
	require.Len(t, prf.Wrapped.Info.Keys, 2, "mixed PRF keyset must hold two keys")
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

// TestProvisionInput_ImportLegacy_DefaultsFalse verifies that a ProvisionInput
// built without the internal marker (the manual provisioning path used by HTTP
// handlers) defaults importLegacy to false, i.e. the envelope-only path for new
// organizations. The marker is unexported on purpose: external packages cannot
// set it. The mode MUST NOT be inferred from audit fields.
func TestProvisionInput_ImportLegacy_DefaultsFalse(t *testing.T) {
	t.Parallel()

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	if req.importLegacy {
		t.Errorf("ProvisionInput.importLegacy = %v, want false (manual provisioning default)", req.importLegacy)
	}
}

// ---------------------------------------------------------------------------
// buildProvisioningKeysets Tests (single internal branching point)
// ---------------------------------------------------------------------------

// TestProvisioningService_buildProvisioningKeysets_EnvelopeOnly_SingleKeyShape
// pins the contract of the new single keyset-building seam for the lazy,
// envelope-only path: it constructs one fresh AEAD keyset and one fresh PRF
// keyset, each with a non-zero PrimaryKeyID and exactly one enabled key, and
// returns the assembled OrganizationKeyset carrying that single-key metadata.
// This characterizes the branch reached when importLegacy == false (the default).
func TestProvisioningService_buildProvisioningKeysets_EnvelopeOnly_SingleKeyShape(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(newFakeKeysetRepoForProv(), newFakeRegistryRepoForProv(),
		keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok, "NewProvisioningService must return *provisioningService")

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
		importLegacy:   false,
	}

	mount, err := resolveMount(concreteSvc.kekMountPath, req.TenantID, concreteSvc.multiTenant)
	require.NoError(t, err)

	kekPath := concreteSvc.buildKEKPath(req.OrganizationID)

	keyset, verbatim, err := concreteSvc.buildProvisioningKeysets(ctx, req, mount, kekPath)
	require.NoError(t, err)
	require.False(t, verbatim, "successful build must not flag verbatim")
	require.NotNil(t, keyset)

	// Envelope-only must keep the current one-fresh-AEAD + one-fresh-PRF shape.
	assert.Equal(t, 1, keysetGenerator.aeadCalled, "envelope-only must generate exactly one fresh AEAD keyset")
	assert.Equal(t, 1, keysetGenerator.macCalled, "envelope-only must generate exactly one fresh PRF keyset")

	assert.Equal(t, req.TenantID, keyset.TenantID)
	assert.Equal(t, req.OrganizationID, keyset.OrganizationID)
	assert.Equal(t, kekPath, keyset.KEKPath)
	assert.Equal(t, mount, keyset.KEKMountPath)

	// AEAD slot: single enabled key, primary id set.
	assert.NotZero(t, keyset.KeysetInfo.PrimaryKeyID, "AEAD primary key id must be set")
	require.Len(t, keyset.KeysetInfo.Keys, 1, "envelope-only AEAD keyset must hold exactly one key")
	assert.Equal(t, string(tink.KeyStatusEnabled), keyset.KeysetInfo.Keys[0].Status)

	// PRF slot (stored in the HMAC slot): single enabled key, primary id set.
	assert.NotZero(t, keyset.HMACKeysetInfo.PrimaryKeyID, "PRF primary key id must be set")
	require.Len(t, keyset.HMACKeysetInfo.Keys, 1, "envelope-only PRF keyset must hold exactly one key")
	assert.Equal(t, string(tink.KeyStatusEnabled), keyset.HMACKeysetInfo.Keys[0].Status)
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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.NotZero(t, result.PRFPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Verify keyset was saved
	assert.Equal(t, 1, keysetRepo.saveCalled)
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "tenant-123", savedKeyset.TenantID)
	assert.Equal(t, "org-456", savedKeyset.OrganizationID)
	assert.NotEmpty(t, savedKeyset.WrappedKeyset)
	assert.NotEmpty(t, savedKeyset.WrappedHMACKeyset)

	// The search-token keyset (stored in the HMAC slot) must now be a PRF keyset.
	require.NotEmpty(t, savedKeyset.HMACKeysetInfo.Keys)
	assert.Equal(t, string(tink.KeyTypeHMACPRF), savedKeyset.HMACKeysetInfo.Keys[0].Type,
		"provisioned search-token keyset must be of type HMAC_PRF")

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
		ProvisioningConfig{KEKMountPath: "transit"}, newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.Equal(t, "transit", keysetGenerator.macMountPath, "PRF wrap must use flat base mount for default tenant")

	// The resolved mount IS stored on the keyset record so unwrap is independent
	// of live config; for the "default"/single-tenant case it is the flat base.
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "org-org-456", savedKeyset.KEKPath, "key name stays org-{id}, unchanged by mount resolution")
	assert.Equal(t, "transit", savedKeyset.KEKMountPath, "resolved flat-base mount must be persisted on the keyset")
}

func TestProvisioningService_Provision_MultiTenant_SubMount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator,
		ProvisioningConfig{KEKMountPath: "transit", MultiTenant: true}, newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.Equal(t, want, keysetGenerator.macMountPath, "PRF wrap must use per-tenant sub-mount")

	// Key name remains org-{id}; the resolved per-tenant sub-mount IS persisted on
	// the record so unwrap does not depend on live KMS_VAULT_MOUNT_PATH config.
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "org-org-456", savedKeyset.KEKPath)
	assert.Equal(t, want, savedKeyset.KEKMountPath, "resolved per-tenant sub-mount must be persisted on the keyset")
}

func TestProvisioningService_Provision_MountNotFound_FailsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.aeadErr = fmt.Errorf("wrap aead: %w", vault.ErrMountNotFound)

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator,
		ProvisioningConfig{KEKMountPath: "transit"}, newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

type provisioningInvalidationRegistryRepo struct {
	mu       sync.Mutex
	record   *mmodel.OrganizationRegistryRecord
	saveErr  error
	getCalls int
}

func (p *provisioningInvalidationRegistryRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationRegistryRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.getCalls++
	if p.record == nil {
		return nil, constant.ErrRegistryNotFound
	}

	return p.record, nil
}

func (p *provisioningInvalidationRegistryRepo) Save(_ context.Context, record *mmodel.OrganizationRegistryRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.saveErr != nil {
		return p.saveErr
	}

	p.record = record

	return nil
}

func (p *provisioningInvalidationRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

func (p *provisioningInvalidationRegistryRepo) getCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.getCalls
}

func TestProvisioningService_Provision_InvalidatesProtectionStateOnlyOnSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		configureRegistry    func(*provisioningInvalidationRegistryRepo)
		wantErr              bool
		wantResolvedMode     crypto.EncryptionMode
		wantCanReadLegacy    bool
		wantRegistryGetCalls int
	}{
		{
			name:                 "successful provisioning invalidates protection state",
			wantResolvedMode:     crypto.EncryptionModeEnvelope,
			wantCanReadLegacy:    true,
			wantRegistryGetCalls: 3,
		},
		{
			name: "failed provisioning does not invalidate protection state",
			configureRegistry: func(repo *provisioningInvalidationRegistryRepo) {
				repo.saveErr = errors.New("save failed")
			},
			wantErr:              true,
			wantResolvedMode:     crypto.EncryptionModeLegacy,
			wantCanReadLegacy:    true,
			wantRegistryGetCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registryRepo := &provisioningInvalidationRegistryRepo{}
			if tt.configureRegistry != nil {
				tt.configureRegistry(registryRepo)
			}

			resolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
			service := NewProvisioningService(
				newFakeKeysetRepoForProv(),
				registryRepo,
				newFakeKeysetGenerator(),
				DefaultProvisioningConfig(),
				newSpyAuditWriter(),
				NewProtectionMetrics(nil),
				resolver,
			)

			req := ProvisionInput{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				Actor:          "tester",
				Reason:         "test provisioning invalidation",
			}

			ctx := tmcore.ContextWithTenantID(context.Background(), req.TenantID)
			initial, err := resolver.Resolve(ctx, req.OrganizationID)
			if err != nil {
				t.Fatalf("Resolve() initial call error = %v", err)
			}

			if initial.Mode != crypto.EncryptionModeLegacy {
				t.Fatalf("initial Mode = %v, want legacy", initial.Mode)
			}

			_, err = service.Provision(ctx, req)
			if tt.wantErr && err == nil {
				t.Fatal("Provision() error = nil, want error")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("Provision() unexpected error = %v", err)
			}

			resolved, err := resolver.Resolve(ctx, req.OrganizationID)
			if err != nil {
				t.Fatalf("Resolve() after provisioning error = %v", err)
			}

			if resolved.Mode != tt.wantResolvedMode {
				t.Errorf("resolved Mode = %v, want %v", resolved.Mode, tt.wantResolvedMode)
			}

			if resolved.CanReadLegacy != tt.wantCanReadLegacy {
				t.Errorf("resolved CanReadLegacy = %v, want %v", resolved.CanReadLegacy, tt.wantCanReadLegacy)
			}

			if got := registryRepo.getCallCount(); got != tt.wantRegistryGetCalls {
				t.Errorf("registry Get calls = %d, want %d", got, tt.wantRegistryGetCalls)
			}
		})
	}
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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.Equal(t, uint32(67890), result.PRFPrimaryKeyID)
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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.Equal(t, uint32(67890), result.PRFPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Verify registry was created
	savedRegistry := registryRepo.records["org-456"]
	require.NotNil(t, savedRegistry)
	assert.Equal(t, mmodel.RegistryStatusActive, savedRegistry.Status)
	assert.True(t, savedRegistry.LegacyReadable, "provisioned registry must have LegacyReadable=true")
}

// persistedMixedKeysetInfo builds persisted two-key metadata mirroring what
// T-1.2.2 saves for the manual migration path: a fresh envelope PRIMARY key plus
// an imported legacy ENABLED non-primary key. The legacy key id is set numerically
// LOWER than the primary id on purpose, and placed FIRST in the Keys slice, so any
// helper that derived the primary by position (e.g. Keys[0]) instead of the
// IsPrimary flag / PrimaryKeyID would return the wrong id and fail these regression
// guards. Named distinctly from the tink production composer to avoid confusion.
func persistedMixedKeysetInfo(primaryID, legacyID uint32, primaryType, legacyType string) mmodel.KeysetInfo {
	return mmodel.KeysetInfo{
		PrimaryKeyID: primaryID,
		Keys: []mmodel.KeyInfo{
			{KeyID: legacyID, Status: string(tink.KeyStatusEnabled), Type: legacyType, IsPrimary: false},
			{KeyID: primaryID, Status: string(tink.KeyStatusEnabled), Type: primaryType, IsPrimary: true},
		},
	}
}

// TestProvisioningService_Provision_AlreadyProvisioned_MixedKeyset is the
// T-1.2.3 idempotency regression guard for the new mixed (two-key) metadata.
// When BOTH a mixed keyset and a registry already exist, Provision MUST be
// idempotent: it returns the ENVELOPE PRIMARY AEAD/PRF key IDs from the persisted
// metadata, performs NO keyset rewrite, and triggers NO regeneration or legacy
// re-import (neither the fresh nor the mixed generator methods are called).
func TestProvisioningService_Provision_AlreadyProvisioned_MixedKeyset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	const (
		aeadPrimaryID uint32 = 0x00BBBBBB
		aeadLegacyID  uint32 = 0x000000AA // deliberately lower than primary, non-primary
		prfPrimaryID  uint32 = 0x00DDDDDD
		prfLegacyID   uint32 = 0x000000CC // deliberately lower than primary, non-primary
	)

	// Pre-populate with an existing MIXED keyset (fresh envelope primary + imported
	// legacy non-primary) AND a registry: truly already provisioned.
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     persistedMixedKeysetInfo(aeadPrimaryID, aeadLegacyID, string(tink.KeyTypeAES256GCM), string(tink.KeyTypeLegacyAESGCM)),
		HMACKeysetInfo: persistedMixedKeysetInfo(prfPrimaryID, prfLegacyID, string(tink.KeyTypeHMACPRF), string(tink.KeyTypeLegacyHMACSHA256)),
		Revision:       1,
	}
	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "provision must be idempotent when mixed keyset and registry already exist")

	// Returned primary IDs MUST be the ENVELOPE PRIMARY ids, not the legacy ids and
	// not a positionally-picked first key.
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.Equal(t, aeadPrimaryID, result.AEADPrimaryKeyID, "must return the envelope AEAD PRIMARY id, never the legacy/non-primary id")
	assert.Equal(t, prfPrimaryID, result.PRFPrimaryKeyID, "must return the envelope PRF PRIMARY id, never the legacy/non-primary id")
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// No keyset rewrite on the idempotent path.
	assert.Equal(t, 0, keysetRepo.saveCalled, "idempotent path must not rewrite the keyset")
	assert.Equal(t, 0, registryRepo.saveCalled, "idempotent path must not rewrite the registry")

	// No regeneration and no legacy re-import on the idempotent path.
	assert.Equal(t, 0, keysetGenerator.aeadCalled, "idempotent path must not generate AEAD material")
	assert.Equal(t, 0, keysetGenerator.macCalled, "idempotent path must not generate PRF material")
	assert.Equal(t, 0, keysetGenerator.mixedAEAD, "idempotent path must not re-import legacy AEAD material")
	assert.Equal(t, 0, keysetGenerator.mixedPRF, "idempotent path must not re-import legacy PRF material")
}

// TestProvisioningService_Provision_RecoveryFromPartialFailure_MixedKeyset is the
// T-1.2.3 partial-failure recovery regression guard for mixed (two-key) metadata.
// A mixed keyset exists but the registry is missing (interrupted provisioning).
// Recovery via handleExistingKeyset MUST create the registry and return the
// persisted ENVELOPE PRIMARY ids, with NO keyset regeneration and NO legacy
// re-import.
func TestProvisioningService_Provision_RecoveryFromPartialFailure_MixedKeyset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	const (
		aeadPrimaryID uint32 = 0x00BBBBBB
		aeadLegacyID  uint32 = 0x000000AA // deliberately lower than primary, non-primary
		prfPrimaryID  uint32 = 0x00DDDDDD
		prfLegacyID   uint32 = 0x000000CC // deliberately lower than primary, non-primary
	)

	// Pre-populate with an existing MIXED keyset but NO registry: partial failure.
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     persistedMixedKeysetInfo(aeadPrimaryID, aeadLegacyID, string(tink.KeyTypeAES256GCM), string(tink.KeyTypeLegacyAESGCM)),
		HMACKeysetInfo: persistedMixedKeysetInfo(prfPrimaryID, prfLegacyID, string(tink.KeyTypeHMACPRF), string(tink.KeyTypeLegacyHMACSHA256)),
		Revision:       1,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Recovery from partial failure",
		importLegacy:   true,
	}

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "recovery must complete provisioning from an existing mixed keyset")

	// Recovery returns the persisted ENVELOPE PRIMARY ids.
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.Equal(t, aeadPrimaryID, result.AEADPrimaryKeyID, "recovery must use the persisted envelope AEAD PRIMARY id")
	assert.Equal(t, prfPrimaryID, result.PRFPrimaryKeyID, "recovery must use the persisted envelope PRF PRIMARY id")
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Registry created with active status and legacy-readable set.
	savedRegistry := registryRepo.records["org-456"]
	require.NotNil(t, savedRegistry)
	assert.Equal(t, mmodel.RegistryStatusActive, savedRegistry.Status)
	assert.True(t, savedRegistry.LegacyReadable, "recovered registry must have LegacyReadable=true")

	// The persisted MIXED keyset MUST NOT be overwritten by the recovery path: its
	// two-key metadata (envelope primary + imported legacy) survives unchanged. The
	// recovery flow may regenerate fresh material before detecting the existing
	// keyset (ErrKeysetAlreadyExists), but it MUST discard that material and the
	// stored keyset MUST keep its original primary IDs and two-key shape.
	persisted := keysetRepo.keysets["org-456"]
	require.NotNil(t, persisted)
	assert.Equal(t, aeadPrimaryID, persisted.KeysetInfo.PrimaryKeyID, "stored AEAD primary id must be untouched by recovery")
	assert.Equal(t, prfPrimaryID, persisted.HMACKeysetInfo.PrimaryKeyID, "stored PRF primary id must be untouched by recovery")
	require.Len(t, persisted.KeysetInfo.Keys, 2, "stored AEAD keyset must keep its two-key (mixed) shape")
	require.Len(t, persisted.HMACKeysetInfo.Keys, 2, "stored PRF keyset must keep its two-key (mixed) shape")

	// Pin the "regenerated material is discarded" claim by counter, so a future
	// change that PERSISTS regenerated material fails loudly. Expected call shape on
	// the recovery path:
	//   - Provision detects "not provisioned" (registry missing) and runs the
	//     migration arm, so each mixed generator is invoked exactly once.
	//   - keysetRepo.Save then hits ErrKeysetAlreadyExists; handleExistingKeyset
	//     recovers from the PERSISTED keyset and creates only the registry.
	// The freshly minted (discarded) material must NEVER be written: keyset save is
	// attempted once (and rejected), and the registry is saved exactly once.
	assert.Equal(t, 1, keysetGenerator.mixedAEAD, "recovery regenerates the mixed AEAD keyset exactly once before discovering the existing keyset")
	assert.Equal(t, 1, keysetGenerator.mixedPRF, "recovery regenerates the mixed PRF keyset exactly once before discovering the existing keyset")
	assert.Equal(t, 0, keysetGenerator.aeadCalled-keysetGenerator.mixedAEAD, "recovery must not take the fresh (envelope-only) AEAD path")
	assert.Equal(t, 0, keysetGenerator.macCalled-keysetGenerator.mixedPRF, "recovery must not take the fresh (envelope-only) PRF path")
	assert.Equal(t, 1, keysetRepo.saveCalled, "recovery attempts exactly one keyset save (rejected as already-exists); regenerated material is discarded, never re-persisted")
	assert.Equal(t, 1, registryRepo.saveCalled, "recovery saves only the missing registry exactly once")

	// The persisted primary IDs differ from any fresh material the generator would
	// mint (fake starts at 1000), so the assertions above also prove recovery
	// returns the PERSISTED envelope primary, never a freshly-generated id.
}

func TestProvisioningService_Provision_InvalidRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

func TestProvisioningService_Provision_PRFGenerationFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.macErr = errors.New("KMS unavailable")

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

// TestProvisioningService_Provision_AEADWrapFailureWithContextCause pins the
// pre-refactor error contract (Finding 1a): an AEAD wrap failure whose error
// chain wraps context.Canceled is ALWAYS mapped to the opaque provisioning
// business error, NOT returned verbatim. The presence of a context cause in the
// chain must not leak as an Is-comparable context error.
func TestProvisioningService_Provision_AEADWrapFailureWithContextCause(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.aeadErr = fmt.Errorf("wrap aead: %w", context.Canceled)

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr, "AEAD wrap failure must map to the business error")
	assert.NotErrorIs(t, err, context.Canceled, "wrap failure must not leak as a verbatim context error")
}

// TestProvisioningService_Provision_PRFWrapFailureWithContextCause pins the
// pre-refactor error contract (Finding 1a): a PRF wrap failure whose error chain
// wraps context.Canceled is ALWAYS mapped to the opaque provisioning business
// error, NOT returned verbatim.
func TestProvisioningService_Provision_PRFWrapFailureWithContextCause(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.macErr = fmt.Errorf("wrap prf: %w", context.Canceled)

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr, "PRF wrap failure must map to the business error")
	assert.NotErrorIs(t, err, context.Canceled, "wrap failure must not leak as a verbatim context error")
}

// TestProvisioningService_Provision_ContextCanceledBetweenKeysets pins the
// pre-refactor error contract (Finding 1b): a bare context cancellation observed
// between the AEAD and PRF generations is returned VERBATIM (Is-comparable to
// context.Canceled), not masked as a business error. The cancelling generator
// produces a valid AEAD bundle, then cancels the context, so the bare ctx.Err()
// check between keysets is the path that fires.
func TestProvisioningService_Provision_ContextCanceledBetweenKeysets(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	// cancelAfterAEADGenerator returns a valid AEAD bundle then cancels the context,
	// driving the between-keysets ctx.Err() check.
	keysetGenerator := &cancelAfterAEADGenerator{inner: newFakeKeysetGenerator(), cancel: cancel}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled, "between-keysets cancellation must be returned verbatim")

	var internalErr pkg.InternalServerError
	assert.NotErrorAs(t, err, &internalErr, "verbatim context error must not be masked as a business error")

	// PRF generation must not have been reached.
	assert.Equal(t, 1, keysetGenerator.inner.aeadCalled, "AEAD must be generated once")
	assert.Equal(t, 0, keysetGenerator.inner.macCalled, "PRF must not be generated after cancellation")
}

// cancelAfterAEADGenerator wraps a fakeKeysetGenerator and cancels the test
// context immediately after a successful AEAD generation, so the provisioning
// flow hits the bare between-keysets ctx.Err() check before the PRF step.
type cancelAfterAEADGenerator struct {
	inner  *fakeKeysetGenerator
	cancel context.CancelFunc
}

func (g *cancelAfterAEADGenerator) GenerateAEADKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	bundle, err := g.inner.GenerateAEADKeyset(ctx, mountPath, keyName)
	g.cancel()

	return bundle, err
}

func (g *cancelAfterAEADGenerator) GeneratePRFKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return g.inner.GeneratePRFKeyset(ctx, mountPath, keyName)
}

func (g *cancelAfterAEADGenerator) GenerateMixedAEADKeyset(ctx context.Context, mountPath, keyName, legacyHexKey string) (tink.KeysetBundle, error) {
	bundle, err := g.inner.GenerateMixedAEADKeyset(ctx, mountPath, keyName, legacyHexKey)
	g.cancel()

	return bundle, err
}

func (g *cancelAfterAEADGenerator) GenerateMixedPRFKeyset(ctx context.Context, mountPath, keyName, legacySecret string) (tink.KeysetBundle, error) {
	return g.inner.GenerateMixedPRFKeyset(ctx, mountPath, keyName, legacySecret)
}

// TestProvisioningService_buildProvisioningKeysets_Migration_MixedTwoKeyShape
// pins the contract for the lazy migration arm (importLegacy == true):
// it MUST route to the mixed AEAD/PRF generators and assemble a composite keyset
// holding TWO keys per slot — a fresh envelope PRIMARY key plus the imported
// legacy ENABLED non-primary key — persisted into the existing
// WrappedKeyset/WrappedHMACKeyset and KeysetInfo/HMACKeysetInfo fields. The fresh
// envelope keys remain primary, so the returned primary IDs are the envelope IDs.
func TestProvisioningService_buildProvisioningKeysets_Migration_MixedTwoKeyShape(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(newFakeKeysetRepoForProv(), newFakeRegistryRepoForProv(),
		keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok, "NewProvisioningService must return *provisioningService")

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
		importLegacy:   true,
	}

	mount, err := resolveMount(concreteSvc.kekMountPath, req.TenantID, concreteSvc.multiTenant)
	require.NoError(t, err)

	kekPath := concreteSvc.buildKEKPath(req.OrganizationID)

	keyset, verbatim, err := concreteSvc.buildProvisioningKeysets(ctx, req, mount, kekPath)
	require.NoError(t, err)
	require.False(t, verbatim, "successful build must not flag verbatim")
	require.NotNil(t, keyset)

	// Migration MUST route to the mixed generators, not the fresh-only ones.
	assert.Equal(t, 1, keysetGenerator.mixedAEAD, "migration must call GenerateMixedAEADKeyset exactly once")
	assert.Equal(t, 1, keysetGenerator.mixedPRF, "migration must call GenerateMixedPRFKeyset exactly once")

	assert.Equal(t, req.TenantID, keyset.TenantID)
	assert.Equal(t, req.OrganizationID, keyset.OrganizationID)
	assert.Equal(t, kekPath, keyset.KEKPath)
	assert.Equal(t, mount, keyset.KEKMountPath)

	// Composite data persisted into the EXISTING wrapped-keyset fields.
	assert.NotEmpty(t, keyset.WrappedKeyset, "mixed AEAD wrapped data must be persisted")
	assert.NotEmpty(t, keyset.WrappedHMACKeyset, "mixed PRF wrapped data must be persisted")

	// AEAD slot: two keys — fresh envelope PRIMARY + imported legacy non-primary.
	assert.NotZero(t, keyset.KeysetInfo.PrimaryKeyID, "AEAD primary key id must be set")
	require.Len(t, keyset.KeysetInfo.Keys, 2, "migration AEAD keyset must hold two keys (fresh primary + legacy)")
	assertMixedKeyShape(t, keyset.KeysetInfo, string(tink.KeyTypeAES256GCM), string(tink.KeyTypeLegacyAESGCM))

	// PRF slot (stored in the HMAC slot): two keys — fresh envelope PRIMARY +
	// imported legacy non-primary.
	assert.NotZero(t, keyset.HMACKeysetInfo.PrimaryKeyID, "PRF primary key id must be set")
	require.Len(t, keyset.HMACKeysetInfo.Keys, 2, "migration PRF keyset must hold two keys (fresh primary + legacy)")
	assertMixedKeyShape(t, keyset.HMACKeysetInfo, string(tink.KeyTypeHMACPRF), string(tink.KeyTypeLegacyHMACSHA256))
}

// assertMixedKeyShape verifies a persisted keyset holds exactly one enabled fresh
// PRIMARY key (of freshType, key id == PrimaryKeyID) and one enabled imported
// legacy non-primary key (of legacyType).
func assertMixedKeyShape(t *testing.T, info mmodel.KeysetInfo, freshType, legacyType string) {
	t.Helper()

	var primaries, legacies int

	for _, k := range info.Keys {
		assert.Equal(t, string(tink.KeyStatusEnabled), k.Status, "every key in a mixed keyset must be enabled")

		if k.IsPrimary {
			primaries++

			assert.Equal(t, info.PrimaryKeyID, k.KeyID, "primary key id must match PrimaryKeyID")
			assert.Equal(t, freshType, k.Type, "primary key must be the fresh envelope type")
		} else {
			legacies++

			assert.Equal(t, legacyType, k.Type, "non-primary key must be the imported legacy type")
		}
	}

	assert.Equal(t, 1, primaries, "mixed keyset must hold exactly one primary key")
	assert.Equal(t, 1, legacies, "mixed keyset must hold exactly one imported legacy key")
}

func TestProvisioningService_Provision_KeysetSaveFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	keysetRepo.saveErr = errors.New("database error")
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	status, err := svc.GetProvisioningStatus(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.Nil(t, status)
}

func TestProvisioningService_GetProvisioningStatus_EmptyOrgID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	_, err := svc.GetProvisioningStatus(ctx, "")
	require.Error(t, err)
}

func TestProvisioningService_GetProvisioningStatus_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	active, err := svc.IsActive(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.False(t, active)
}

// ---------------------------------------------------------------------------
// Configuration Tests
// ---------------------------------------------------------------------------

func TestNewProvisioningService_DefaultMountPath(t *testing.T) {
	t.Parallel()

	svc := NewProvisioningService(nil, nil, nil, ProvisioningConfig{}, newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	svc := NewProvisioningService(nil, nil, nil, config, newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.Equal(t, uint32(22222), result.PRFPrimaryKeyID)
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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.Equal(t, savedKeyset.HMACKeysetInfo.PrimaryKeyID, result.PRFPrimaryKeyID)
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
	assert.Equal(t, result.PRFPrimaryKeyID, thirdResult.PRFPrimaryKeyID)
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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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
	assert.NotZero(t, firstResult.PRFPrimaryKeyID)
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
	assert.Equal(t, firstResult.PRFPrimaryKeyID, secondResult.PRFPrimaryKeyID)
	assert.Equal(t, firstResult.RegistryStatus, secondResult.RegistryStatus)

	// Verify no new keyset was generated on second call
	assert.Equal(t, aeadCallsAfterFirst, keysetGenerator.aeadCalled,
		"AEAD keyset generator should NOT be called again for idempotent provision")
	assert.Equal(t, macCallsAfterFirst, keysetGenerator.macCalled,
		"PRF keyset generator should NOT be called again for idempotent provision")

	// STEP 3: Third provision call - should still succeed (idempotent)
	thirdResult, err := svc.Provision(ctx, req)
	require.NoError(t, err, "third provision call should succeed (idempotent behavior)")

	assert.Equal(t, firstResult.AEADPrimaryKeyID, thirdResult.AEADPrimaryKeyID)
	assert.Equal(t, firstResult.PRFPrimaryKeyID, thirdResult.PRFPrimaryKeyID)
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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	// Type assert to access internal method
	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok, "NewProvisioningService must return *provisioningService")

	// Call getExistingProvisionResult (this method does not exist yet - TDD RED)
	result, err := concreteSvc.getExistingProvisionResult(ctx, "org-existing")
	require.NoError(t, err)

	assert.Equal(t, "org-existing", result.OrganizationID)
	assert.Equal(t, "org-org-existing", result.KEKPath)
	assert.Equal(t, uint32(11111), result.AEADPrimaryKeyID)
	assert.Equal(t, uint32(22222), result.PRFPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)
}

func TestProvisioningService_getExistingProvisionResult_EmptyOrgID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(nilKeysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), newSpyAuditWriter(), NewProtectionMetrics(nil), nil)

	concreteSvc, ok := svc.(*provisioningService)
	require.True(t, ok)

	// Call getExistingProvisionResult when registry is nil - should return error
	_, err := concreteSvc.getExistingProvisionResult(ctx, "org-nil-registry")
	require.Error(t, err)
}

// fakeKeysetRepoNilNil implements encryption.KeysetRepository returning (nil, nil) from Get.
type fakeKeysetRepoNilNil struct{}

func (f *fakeKeysetRepoNilNil) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return nil, nil
}

func (f *fakeKeysetRepoNilNil) GetActive(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return nil, nil
}

func (f *fakeKeysetRepoNilNil) GetByVersion(_ context.Context, _ string, _ int) (*mmodel.OrganizationKeyset, error) {
	return nil, nil
}

func (f *fakeKeysetRepoNilNil) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

func (f *fakeKeysetRepoNilNil) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// spyAuditWriter is a controllable AuditWriter test double. It records every
// event passed to Emit/EmitAsync so a test can assert exactly one event was
// emitted with the expected outcome.
type spyAuditWriter struct {
	mu      sync.Mutex
	emitted []*mmodel.ProtectionAuditEvent
}

func newSpyAuditWriter() *spyAuditWriter {
	return &spyAuditWriter{}
}

func (s *spyAuditWriter) Emit(_ context.Context, event *mmodel.ProtectionAuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.emitted = append(s.emitted, event)
}

// EmitAsync records synchronously (no goroutine) so tests observe the event
// deterministically without sleeping. The production writer detaches; this spy
// captures the call to verify the single-point emission contract.
func (s *spyAuditWriter) EmitAsync(_ context.Context, event *mmodel.ProtectionAuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.emitted = append(s.emitted, event)
}

func (s *spyAuditWriter) events() []*mmodel.ProtectionAuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*mmodel.ProtectionAuditEvent, len(s.emitted))
	copy(out, s.emitted)

	return out
}

// requireSingleEvent asserts exactly one event was emitted and returns it.
func requireSingleEvent(t *testing.T, spy *spyAuditWriter) *mmodel.ProtectionAuditEvent {
	t.Helper()

	events := spy.events()
	require.Len(t, events, 1, "exactly one audit event must be emitted per terminal path")
	require.NotNil(t, events[0])

	return events[0]
}

func newProvisionReq() ProvisionInput {
	return ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}
}

func TestProvisioningService_Provision_EmitsSuccessAuditEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditEventTypeProvisioning, event.EventType)
	assert.Equal(t, mmodel.AuditActionProvision, event.Action)
	assert.Equal(t, mmodel.AuditOutcomeSuccess, event.Outcome)
	assert.Equal(t, "org-456", event.OrganizationID)
	assert.Equal(t, "tenant-123", event.TenantID)
	assert.Equal(t, "admin@example.com", event.ActorID)
	assert.Equal(t, "service", event.ActorType)
	require.NotNil(t, event.Details)
	assert.Equal(t, "active", event.Details.NewStatus)
	assert.NotEmpty(t, event.Details.AffectedKeyIDs)
}

// Recognizable sentinel values standing in for the raw legacy key material that
// manual provisioning imports into a keyset (LCRYPTO_ENCRYPT_SECRET_KEY hex AES
// key, LCRYPTO_HASH_SECRET_KEY HMAC secret). These are NOT real secrets; the
// obvious sentinel pattern lets the non-disclosure assertions detect any leak of
// raw legacy material into a persisted keyset or an audit event.
const (
	sentinelProvLegacyAESHex     = "5345435245544145534b45595345435245544145534b4559"
	sentinelProvLegacyHMACSecret = "SENTINEL-LEGACY-HMAC-SECRET-DO-NOT-LEAK"
)

// TestProvisioningService_Provision_NeverExposesRawLegacySecret is a
// non-disclosure regression for the legacy-key import path (E-1.2). It drives a
// full migration provision with RECOGNIZABLE raw legacy secret material flowing
// through the mixed-keyset generators, then asserts that material never appears
// in either persisted/audited surface:
//   - the persisted OrganizationKeyset (only KMS-wrapped opaque bytes are stored)
//   - the emitted provisioning audit event (only primary key IDs + status)
//
// No genuine RED is possible: production only persists KMS-wrapped bytes and the
// audit details carry only key IDs and status, so the raw material is
// structurally absent. This test asserts that ABSENCE and guards against a future
// regression that would persist or audit raw legacy material. The test never
// logs the sentinel secrets; it asserts absence over serialized output.
func TestProvisioningService_Provision_NeverExposesRawLegacySecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	// Inject recognizable legacy material as the process-level secrets the mixed
	// generators substitute when the service passes empty material (mirroring the
	// real keysetGeneratorAdapter behaviour).
	keysetGenerator.defaultLegacyAES = sentinelProvLegacyAESHex
	keysetGenerator.defaultLegacyHMAC = sentinelProvLegacyHMACSecret

	spy := newSpyAuditWriter()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	// importLegacy => lazy migration path => mixed generators import legacy material.
	req := newProvisionReq()
	req.importLegacy = true
	result, err := svc.Provision(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	// Precondition: the raw legacy material genuinely flowed through the import
	// seam, so the absence assertions below are meaningful (not vacuous).
	require.Equal(t, 1, keysetGenerator.mixedAEAD, "migration must import legacy AEAD material")
	require.Equal(t, 1, keysetGenerator.mixedPRF, "migration must import legacy PRF material")
	require.Equal(t, sentinelProvLegacyAESHex, keysetGenerator.gotLegacyAES, "legacy AES material must reach the seam")
	require.Equal(t, sentinelProvLegacyHMACSecret, keysetGenerator.gotLegacyHMAC, "legacy HMAC material must reach the seam")

	// Surface 1: the persisted OrganizationKeyset. Only KMS-wrapped opaque bytes
	// are stored; the raw legacy material must be absent from the full record.
	savedKeyset := keysetRepo.keysets["org-456"]
	require.NotNil(t, savedKeyset)

	keysetSerialized, err := json.Marshal(savedKeyset)
	require.NoError(t, err)

	keysetRendered := string(keysetSerialized) + fmt.Sprintf("%+v", *savedKeyset)
	assert.NotContains(t, keysetRendered, sentinelProvLegacyAESHex, "persisted keyset must never contain raw legacy AES material")
	assert.NotContains(t, keysetRendered, sentinelProvLegacyHMACSecret, "persisted keyset must never contain raw legacy HMAC secret")

	// Surface 2: the emitted provisioning audit event. Details carry only primary
	// key IDs + status, never key material.
	event := requireSingleEvent(t, spy)

	eventSerialized, err := json.Marshal(event)
	require.NoError(t, err)

	eventRendered := string(eventSerialized) + fmt.Sprintf("%+v", *event)
	assert.NotContains(t, eventRendered, sentinelProvLegacyAESHex, "audit event must never contain raw legacy AES material")
	assert.NotContains(t, eventRendered, sentinelProvLegacyHMACSecret, "audit event must never contain raw legacy HMAC secret")

	// Sanity: the audit details still carry the non-secret key IDs + status.
	require.NotNil(t, event.Details)
	assert.Equal(t, "active", event.Details.NewStatus)
	assert.NotEmpty(t, event.Details.AffectedKeyIDs)
}

func TestProvisioningService_Provision_AlreadyProvisioned_EmitsAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

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

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	_, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeAlreadyExists, event.Outcome)
	assert.Equal(t, mmodel.AuditEventTypeProvisioning, event.EventType)
	assert.Equal(t, mmodel.AuditActionProvision, event.Action)
}

func TestProvisioningService_Provision_RegistryRecreatedFromExistingKeyset_EmitsSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	// Keyset exists but registry missing -> handleExistingKeyset recreates registry.
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	result, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeSuccess, event.Outcome)
	require.NotNil(t, event.Details)
	assert.Equal(t, "active", event.Details.NewStatus)
	assert.Contains(t, event.Details.AffectedKeyIDs, uint32(12345))
}

func TestProvisioningService_Provision_RegistrySaveRace_EmitsAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	// Drive the createAndSaveRegistry race deterministically with a registry repo
	// that simulates a concurrent winner: IsProvisioned's first Get sees no
	// registry (not provisioned), the flow generates + saves a fresh keyset, then
	// Save returns ErrRegistryAlreadyExists; the recovery Get then returns the
	// concurrently-written registry so getExistingProvisionResult succeeds.
	registryRepo := &raceRegistryRepo{}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	_, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeAlreadyExists, event.Outcome)
}

// raceRegistryRepo simulates a concurrent provisioning winner. The first Get
// (from IsProvisioned) reports not-provisioned; Save loses the race with
// ErrRegistryAlreadyExists; subsequent Gets return the concurrently-written
// active registry.
type raceRegistryRepo struct {
	getCalls int
}

func (r *raceRegistryRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	r.getCalls++

	if r.getCalls == 1 {
		return nil, mmodel.ErrRegistryNotFound
	}

	return &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: organizationID,
		Status:         mmodel.RegistryStatusActive,
	}, nil
}

func (r *raceRegistryRepo) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return mmodel.ErrRegistryAlreadyExists
}

func (r *raceRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

// TestProvisioningService_Provision_HandleExistingKeyset_BothExist_EmitsAlreadyExists
// drives the handleExistingKeyset "both keyset and registry exist" branch: the
// org is not provisioned at the IsProvisioned check, generation succeeds, the
// keyset Save loses a race with ErrKeysetAlreadyExists, and a concurrently
// written registry is then found -> already_exists.
func TestProvisioningService_Provision_HandleExistingKeyset_BothExist_EmitsAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	keysetRepo := &raceKeysetRepo{}
	registryRepo := &raceRegistryRepoBothExist{}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	_, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeAlreadyExists, event.Outcome)
}

// raceKeysetRepo loses the keyset Save race (ErrKeysetAlreadyExists) but returns
// a stored keyset on Get, simulating a concurrent winner.
type raceKeysetRepo struct{}

func (r *raceKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return mmodel.ErrKeysetAlreadyExists
}

func (r *raceKeysetRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: organizationID,
		Version:        1,
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}, nil
}

func (r *raceKeysetRepo) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return r.Get(ctx, organizationID)
}

func (r *raceKeysetRepo) GetByVersion(ctx context.Context, organizationID string, _ int) (*mmodel.OrganizationKeyset, error) {
	return r.Get(ctx, organizationID)
}

func (r *raceKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// raceRegistryRepoBothExist reports not-provisioned on the first Get (so the
// create path runs) and an active registry afterward (so handleExistingKeyset
// sees an existing registry).
type raceRegistryRepoBothExist struct {
	getCalls int
}

func (r *raceRegistryRepoBothExist) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	r.getCalls++

	if r.getCalls == 1 {
		return nil, mmodel.ErrRegistryNotFound
	}

	return &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: organizationID,
		Status:         mmodel.RegistryStatusActive,
	}, nil
}

func (r *raceRegistryRepoBothExist) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

func (r *raceRegistryRepoBothExist) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

func TestProvisioningService_Provision_Failures_EmitFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(k *fakeKeysetRepoForProv, r *fakeRegistryRepoForProv, g *fakeKeysetGenerator)
		wantErr bool
	}{
		{
			name: "aead generation failed",
			mutate: func(_ *fakeKeysetRepoForProv, _ *fakeRegistryRepoForProv, g *fakeKeysetGenerator) {
				g.aeadErr = errors.New("KMS unavailable")
			},
		},
		{
			name: "prf generation failed",
			mutate: func(_ *fakeKeysetRepoForProv, _ *fakeRegistryRepoForProv, g *fakeKeysetGenerator) {
				g.macErr = errors.New("KMS unavailable")
			},
		},
		{
			name: "keyset save failed",
			mutate: func(k *fakeKeysetRepoForProv, _ *fakeRegistryRepoForProv, _ *fakeKeysetGenerator) {
				k.saveErr = errors.New("database error")
			},
		},
		{
			name: "registry save failed",
			mutate: func(_ *fakeKeysetRepoForProv, r *fakeRegistryRepoForProv, _ *fakeKeysetGenerator) {
				r.saveErr = errors.New("database error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			keysetRepo := newFakeKeysetRepoForProv()
			registryRepo := newFakeRegistryRepoForProv()
			keysetGenerator := newFakeKeysetGenerator()
			spy := newSpyAuditWriter()

			tt.mutate(keysetRepo, registryRepo, keysetGenerator)

			svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

			_, err := svc.Provision(ctx, newProvisionReq())
			require.Error(t, err)

			event := requireSingleEvent(t, spy)
			assert.Equal(t, mmodel.AuditOutcomeFailure, event.Outcome)
			require.NotNil(t, event.Details)
			assert.Equal(t, constant.ErrProvisioningFailed.Error(), event.Details.ErrorCode)
		})
	}
}

// TestProvisioningService_Provision_ResultUnchangedVsWriter proves the audit
// writer never alters the provisioning return value or error path. It uses the
// real repository-backed writer (EmitAsync, panic-safe per T-1.3.1) with a
// repository that errors and one that panics; both must yield the same result
// as the no-op writer.
func TestProvisioningService_Provision_ResultUnchangedVsWriter(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, w AuditWriter) (ProvisionResult, error) {
		t.Helper()

		ctx := context.Background()
		svc := NewProvisioningService(newFakeKeysetRepoForProv(), newFakeRegistryRepoForProv(), newFakeKeysetGenerator(), DefaultProvisioningConfig(), w, NewProtectionMetrics(nil), nil)

		return svc.Provision(ctx, newProvisionReq())
	}

	// Baseline: recording spy writer that never mutates the result.
	baseResult, baseErr := run(t, newSpyAuditWriter())
	require.NoError(t, baseErr)

	// Repository-backed writer whose repo errors: best-effort contract swallows it.
	errRepo := newSpyRepo()
	errRepo.err = errors.New("audit store down")
	errWriter := NewAuditWriter(errRepo, testLogger())

	require.NotPanics(t, func() {
		result, err := run(t, errWriter)
		require.NoError(t, err)
		assert.Equal(t, baseResult.OrganizationID, result.OrganizationID)
		assert.Equal(t, baseResult.RegistryStatus, result.RegistryStatus)
	})

	// Wait for the async write so its (recovered) goroutine finishes before the
	// goleak check in TestMain runs.
	select {
	case <-errRepo.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async write")
	}

	// Repository-backed writer whose repo panics: EmitAsync recovers on its
	// detached goroutine, so Provision is unaffected.
	panicRepo := newSpyRepo()
	panicRepo.panicMsg = "audit store panic"
	panicWriter := NewAuditWriter(panicRepo, testLogger())

	require.NotPanics(t, func() {
		result, err := run(t, panicWriter)
		require.NoError(t, err)
		assert.Equal(t, baseResult.OrganizationID, result.OrganizationID)
	})

	select {
	case <-panicRepo.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async write")
	}
}

// TestProvisioningService_Provision_InvalidRequest_EmitsFailure verifies a
// validation failure still produces exactly one failure audit event.
func TestProvisioningService_Provision_InvalidRequest_EmitsFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spy := newSpyAuditWriter()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	req := newProvisionReq()
	req.TenantID = ""

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	// Validation failure: org id is present so the event can be built.
	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeFailure, event.Outcome)
}

// TestProvisioningService_Provision_EventBuildFails_SkipsEmission verifies that
// when the audit event cannot be built (empty OrganizationID), emission is
// silently skipped and provisioning still returns its validation error.
func TestProvisioningService_Provision_EventBuildFails_SkipsEmission(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spy := newSpyAuditWriter()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), spy, NewProtectionMetrics(nil), nil)

	req := newProvisionReq()
	req.OrganizationID = "" // fails validation AND event build (OrganizationID required)

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	// Event build fails -> emission skipped -> no events recorded.
	assert.Empty(t, spy.events(), "emission must be skipped when the event cannot be built")
}
