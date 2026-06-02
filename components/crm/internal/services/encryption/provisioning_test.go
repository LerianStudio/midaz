// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fakes and Helpers
// ---------------------------------------------------------------------------

// fakeKeysetWriter implements KeysetWriter for tests.
type fakeKeysetWriter struct {
	keysets    map[string]*mmodel.OrganizationKeyset
	saveErr    error
	saveCalled int
}

func newFakeKeysetWriter() *fakeKeysetWriter {
	return &fakeKeysetWriter{
		keysets: make(map[string]*mmodel.OrganizationKeyset),
	}
}

func (f *fakeKeysetWriter) Save(_ context.Context, keyset *mmodel.OrganizationKeyset) error {
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

func (f *fakeKeysetWriter) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	keyset, ok := f.keysets[organizationID]
	if !ok {
		return nil, mmodel.ErrKeysetNotFound
	}

	return keyset, nil
}

// fakeRegistryWriter implements RegistryWriter for tests.
type fakeRegistryWriter struct {
	records      map[string]*mmodel.OrganizationRegistryRecord
	saveErr      error
	getErr       error
	updateErr    error
	saveCalled   int
	returnNilNil bool // When true, Get returns (nil, nil) to simulate edge case
}

func newFakeRegistryWriter() *fakeRegistryWriter {
	return &fakeRegistryWriter{
		records: make(map[string]*mmodel.OrganizationRegistryRecord),
	}
}

func (f *fakeRegistryWriter) Save(_ context.Context, record *mmodel.OrganizationRegistryRecord) error {
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

func (f *fakeRegistryWriter) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
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

func (f *fakeRegistryWriter) Update(_ context.Context, record *mmodel.OrganizationRegistryRecord, expectedRevision int64) error {
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
	nextKeyID     uint32
	callSequencer int
}

func newFakeKeysetGenerator() *fakeKeysetGenerator {
	return &fakeKeysetGenerator{
		nextKeyID: 1000,
	}
}

func (f *fakeKeysetGenerator) GenerateAEADKeyset(_ context.Context, _ string) (tink.KeysetBundle, error) {
	f.aeadCalled++
	f.callSequencer++

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

func (f *fakeKeysetGenerator) GenerateMACKeyset(_ context.Context, _ string) (tink.KeysetBundle, error) {
	f.macCalled++
	f.callSequencer++

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
// ActivateRequest Validation Tests
// ---------------------------------------------------------------------------

func TestActivateRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     ActivateInput
		wantErr bool
	}{
		{
			name: "valid request",
			req: ActivateInput{
				OrganizationID: "org-456",
				Actor:          "admin@example.com",
				Reason:         "Activation after migration",
			},
			wantErr: false,
		},
		{
			name: "missing organization_id",
			req: ActivateInput{
				OrganizationID: "",
				Actor:          "admin@example.com",
				Reason:         "Activation after migration",
			},
			wantErr: true,
		},
		{
			name: "missing actor",
			req: ActivateInput{
				OrganizationID: "org-456",
				Actor:          "",
				Reason:         "Activation after migration",
			},
			wantErr: true,
		},
		{
			name: "missing reason",
			req: ActivateInput{
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
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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
	assert.Equal(t, mmodel.RegistryStatusPendingMigration, result.RegistryStatus)

	// Verify keyset was saved
	assert.Equal(t, 1, keysetWriter.saveCalled)
	savedKeyset := keysetWriter.keysets["org-456"]
	require.NotNil(t, savedKeyset)
	assert.Equal(t, "tenant-123", savedKeyset.TenantID)
	assert.Equal(t, "org-456", savedKeyset.OrganizationID)
	assert.NotEmpty(t, savedKeyset.WrappedKeyset)
	assert.NotEmpty(t, savedKeyset.WrappedHMACKeyset)

	// Verify registry was saved
	assert.Equal(t, 1, registryWriter.saveCalled)
	savedRegistry := registryWriter.records["org-456"]
	require.NotNil(t, savedRegistry)
	assert.Equal(t, "tenant-123", savedRegistry.TenantID)
	assert.Equal(t, "org-456", savedRegistry.OrganizationID)
	assert.Equal(t, mmodel.RegistryStatusPendingMigration, savedRegistry.Status)

	// Verify generators were called
	assert.Equal(t, 1, keysetGenerator.aeadCalled)
	assert.Equal(t, 1, keysetGenerator.macCalled)
}

func TestProvisioningService_Provision_AlreadyProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset AND registry (truly already provisioned)
	keysetWriter.keysets["org-456"] = &mmodel.OrganizationKeyset{
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}
	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusPendingMigration,
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var conflictErr pkg.EntityConflictError
	assert.ErrorAs(t, err, &conflictErr)
}

func TestProvisioningService_Provision_RecoveryFromPartialFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset but NO registry (partial failure scenario)
	keysetWriter.keysets["org-456"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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
	assert.Equal(t, mmodel.RegistryStatusPendingMigration, result.RegistryStatus)

	// Verify registry was created
	savedRegistry := registryWriter.records["org-456"]
	require.NotNil(t, savedRegistry)
	assert.Equal(t, mmodel.RegistryStatusPendingMigration, savedRegistry.Status)
}

func TestProvisioningService_Provision_InvalidRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, nil, DefaultProvisioningConfig())

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
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.aeadErr = errors.New("KMS unavailable")

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()
	keysetGenerator.macErr = errors.New("KMS unavailable")

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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
	keysetWriter := newFakeKeysetWriter()
	keysetWriter.saveErr = errors.New("database error")
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	registryWriter.saveErr = errors.New("database error")
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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

	svc := NewProvisioningService(nil, nil, nil, nil, DefaultProvisioningConfig())

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
// Activate Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_Activate_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// First provision the organization
	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	provisionReq := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, provisionReq)
	require.NoError(t, err)

	// Now activate
	activateReq := ActivateInput{
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Migration complete",
	}

	err = svc.Activate(ctx, activateReq)
	require.NoError(t, err)

	// Verify registry was updated
	registry := registryWriter.records["org-456"]
	require.NotNil(t, registry)
	assert.Equal(t, mmodel.RegistryStatusActive, registry.Status)
	assert.Equal(t, mmodel.ProtectionModelEnvelope, registry.ProtectionModel)
}

func TestProvisioningService_Activate_NotProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ActivateInput{
		OrganizationID: "org-not-provisioned",
		Actor:          "admin@example.com",
		Reason:         "Activation attempt",
	}

	err := svc.Activate(ctx, req)
	require.Error(t, err)

	var notFoundErr pkg.EntityNotFoundError
	assert.ErrorAs(t, err, &notFoundErr)
}

func TestProvisioningService_Activate_InvalidRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, nil, DefaultProvisioningConfig())

	req := ActivateInput{
		OrganizationID: "",
		Actor:          "admin@example.com",
		Reason:         "Activation attempt",
	}

	err := svc.Activate(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid activate request")
}

func TestProvisioningService_Activate_AlreadyActive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with already active registry
	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID:  "org-456",
		Status:          mmodel.RegistryStatusActive,
		ProtectionModel: mmodel.ProtectionModelEnvelope,
		Revision:        2,
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ActivateInput{
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Re-activation attempt",
	}

	err := svc.Activate(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_Activate_ConcurrentModification(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	registryWriter.updateErr = mmodel.ErrRegistryRevisionConflict
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with pending registry
	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID:  "org-456",
		Status:          mmodel.RegistryStatusPendingMigration,
		ProtectionModel: mmodel.ProtectionModelLegacy,
		Revision:        1,
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ActivateInput{
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Activation attempt",
	}

	err := svc.Activate(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_Activate_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewProvisioningService(nil, nil, nil, nil, DefaultProvisioningConfig())

	req := ActivateInput{
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Activation attempt",
	}

	err := svc.Activate(ctx, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// GetProvisioningStatus Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_GetProvisioningStatus_Provisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with registry
	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	status, err := svc.GetProvisioningStatus(ctx, "org-456")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, mmodel.RegistryStatusActive, *status)
}

func TestProvisioningService_GetProvisioningStatus_NotProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	status, err := svc.GetProvisioningStatus(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.Nil(t, status)
}

func TestProvisioningService_GetProvisioningStatus_EmptyOrgID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewProvisioningService(nil, nil, nil, nil, DefaultProvisioningConfig())

	_, err := svc.GetProvisioningStatus(ctx, "")
	require.Error(t, err)
}

func TestProvisioningService_GetProvisioningStatus_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewProvisioningService(nil, nil, nil, nil, DefaultProvisioningConfig())

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
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusPendingMigration,
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	provisioned, err := svc.IsProvisioned(ctx, "org-456")
	require.NoError(t, err)
	assert.True(t, provisioned)
}

func TestProvisioningService_IsProvisioned_False(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	provisioned, err := svc.IsProvisioned(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.False(t, provisioned)
}

// ---------------------------------------------------------------------------
// IsActive Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_IsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   mmodel.RegistryStatus
		wantTrue bool
	}{
		{
			name:     "active status",
			status:   mmodel.RegistryStatusActive,
			wantTrue: true,
		},
		{
			name:     "partially migrated status",
			status:   mmodel.RegistryStatusPartiallyMigrated,
			wantTrue: true,
		},
		{
			name:     "migration complete status",
			status:   mmodel.RegistryStatusMigrationComplete,
			wantTrue: true,
		},
		{
			name:     "pending migration status",
			status:   mmodel.RegistryStatusPendingMigration,
			wantTrue: false,
		},
		{
			name:     "legacy status",
			status:   mmodel.RegistryStatusLegacy,
			wantTrue: false,
		},
		{
			name:     "failed status",
			status:   mmodel.RegistryStatusFailed,
			wantTrue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			keysetWriter := newFakeKeysetWriter()
			registryWriter := newFakeRegistryWriter()
			keysetGenerator := newFakeKeysetGenerator()

			registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
				OrganizationID: "org-456",
				Status:         tt.status,
			}

			svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

			active, err := svc.IsActive(ctx, "org-456")
			require.NoError(t, err)
			assert.Equal(t, tt.wantTrue, active)
		})
	}
}

func TestProvisioningService_IsActive_NotProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	active, err := svc.IsActive(ctx, "org-not-provisioned")
	require.NoError(t, err)
	assert.False(t, active)
}

// ---------------------------------------------------------------------------
// Configuration Tests
// ---------------------------------------------------------------------------

func TestNewProvisioningService_DefaultMountPath(t *testing.T) {
	t.Parallel()

	svc := NewProvisioningService(nil, nil, nil, nil, ProvisioningConfig{})

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
	svc := NewProvisioningService(nil, nil, nil, nil, config)

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

func TestProvisioningService_Provision_RegistryAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate registry only (keyset doesn't exist)
	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var conflictErr pkg.EntityConflictError
	assert.ErrorAs(t, err, &conflictErr)
}

func TestProvisioningService_Activate_GetRegistryFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	registryWriter.getErr = errors.New("database error")
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ActivateInput{
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Activation attempt",
	}

	err := svc.Activate(ctx, req)
	require.Error(t, err)

	var internalErr pkg.InternalServerError
	assert.ErrorAs(t, err, &internalErr)
}

func TestProvisioningService_GetProvisioningStatus_DatabaseError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	registryWriter.getErr = errors.New("database error")
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	_, err := svc.GetProvisioningStatus(ctx, "org-456")
	require.Error(t, err)
}

func TestProvisioningService_GetProvisioningStatus_NilNilFromRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	registryWriter.returnNilNil = true // Simulate edge case: repository returns (nil, nil)
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

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
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	// Pre-populate with existing keyset AND registry to test constant.ErrKeysetAlreadyExists handling
	keysetWriter.keysets["org-456"] = &mmodel.OrganizationKeyset{
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
	}
	registryWriter.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusPendingMigration,
	}

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	var conflictErr pkg.EntityConflictError
	assert.ErrorAs(t, err, &conflictErr)
}

// ---------------------------------------------------------------------------
// Registry Save Failure + Retry Recovery Tests
// ---------------------------------------------------------------------------

func TestProvisioningService_Provision_RegistrySaveFailure_ThenRetryRecovers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetWriter := newFakeKeysetWriter()
	registryWriter := newFakeRegistryWriter()
	keysetGenerator := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetWriter, keysetWriter, registryWriter, keysetGenerator, DefaultProvisioningConfig())

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	// STEP 1: First attempt - keyset saves successfully, registry save fails
	registryWriter.saveErr = errors.New("transient database error")

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	// Verify keyset was saved (this is the partial failure state)
	assert.Equal(t, 1, keysetWriter.saveCalled)
	savedKeyset := keysetWriter.keysets["org-456"]
	require.NotNil(t, savedKeyset, "keyset should be persisted despite registry failure")
	assert.Equal(t, "tenant-123", savedKeyset.TenantID)
	assert.Equal(t, "org-456", savedKeyset.OrganizationID)

	// Verify registry was NOT saved
	assert.Equal(t, 1, registryWriter.saveCalled)
	_, registryExists := registryWriter.records["org-456"]
	assert.False(t, registryExists, "registry should NOT exist after failed save")

	// STEP 2: Second attempt (retry) - should recover by detecting existing keyset
	registryWriter.saveErr = nil // Clear the error for retry

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err, "retry should succeed by recovering from partial failure")

	// Verify result uses existing keyset's key IDs
	assert.Equal(t, "org-456", result.OrganizationID)
	assert.Equal(t, "org-org-456", result.KEKPath)
	assert.Equal(t, savedKeyset.KeysetInfo.PrimaryKeyID, result.AEADPrimaryKeyID)
	assert.Equal(t, savedKeyset.HMACKeysetInfo.PrimaryKeyID, result.MACPrimaryKeyID)
	assert.Equal(t, mmodel.RegistryStatusPendingMigration, result.RegistryStatus)

	// Verify keyset was NOT regenerated (save called only once more for the existing check)
	assert.Equal(t, 2, keysetWriter.saveCalled, "keyset save should be called again (hitting already exists)")

	// Verify registry was created on retry
	assert.Equal(t, 2, registryWriter.saveCalled)
	savedRegistry := registryWriter.records["org-456"]
	require.NotNil(t, savedRegistry, "registry should be created on retry")
	assert.Equal(t, mmodel.RegistryStatusPendingMigration, savedRegistry.Status)

	// STEP 3: Third attempt should fail with conflict (fully provisioned)
	_, err = svc.Provision(ctx, req)
	require.Error(t, err, "third attempt should fail - already fully provisioned")

	var conflictErr pkg.EntityConflictError
	assert.ErrorAs(t, err, &conflictErr)
}
