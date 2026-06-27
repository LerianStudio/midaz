// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"reflect"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/crypto"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// fakeRegistryRepoForProtection is a test double for mongoEncryption.RegistryRepository.
type fakeRegistryRepoForProtection struct {
	record *mmodel.OrganizationRegistryRecord
	err    error
}

func (f *fakeRegistryRepoForProtection) Get(_ context.Context, _ string) (*mmodel.OrganizationRegistryRecord, error) {
	return f.record, f.err
}

func (f *fakeRegistryRepoForProtection) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

func (f *fakeRegistryRepoForProtection) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

func TestProtectionStateResolver_Resolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		organizationID           string
		readerRecord             *mmodel.OrganizationRegistryRecord
		readerErr                error
		wantMode                 crypto.EncryptionMode
		wantCanReadLegacy        bool
		wantCurrentKeysetVersion int
		wantTenantID             string
		wantOrganizationID       string
		wantErr                  error
	}{
		{
			name:                     "registry not found returns legacy mode",
			organizationID:           "org-123",
			readerRecord:             nil,
			readerErr:                constant.ErrRegistryNotFound,
			wantMode:                 crypto.EncryptionModeLegacy,
			wantCanReadLegacy:        true,
			wantCurrentKeysetVersion: 0,
			wantTenantID:             "",
			wantOrganizationID:       "org-123",
			wantErr:                  nil,
		},
		{
			name:           "status active returns envelope mode",
			organizationID: "org-active",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:        "tenant-ghi",
				OrganizationID:  "org-active",
				Status:          mmodel.RegistryStatusActive,
				CurrentVersion:  2,
				LegacyReadable:  false,
				ProtectionModel: mmodel.ProtectionModelEnvelope,
			},
			readerErr:                nil,
			wantMode:                 crypto.EncryptionModeEnvelope,
			wantCanReadLegacy:        false,
			wantCurrentKeysetVersion: 2,
			wantTenantID:             "tenant-ghi",
			wantOrganizationID:       "org-active",
			wantErr:                  nil,
		},
		{
			name:           "status active with legacy readable",
			organizationID: "org-active-legacy",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:        "tenant-jkl",
				OrganizationID:  "org-active-legacy",
				Status:          mmodel.RegistryStatusActive,
				CurrentVersion:  1,
				LegacyReadable:  true,
				ProtectionModel: mmodel.ProtectionModelEnvelope,
			},
			readerErr:                nil,
			wantMode:                 crypto.EncryptionModeEnvelope,
			wantCanReadLegacy:        true,
			wantCurrentKeysetVersion: 1,
			wantTenantID:             "tenant-jkl",
			wantOrganizationID:       "org-active-legacy",
			wantErr:                  nil,
		},
		{
			name:           "unknown status returns error",
			organizationID: "org-unknown",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:       "tenant-xyz",
				OrganizationID: "org-unknown",
				Status:         "unknown_status",
			},
			readerErr: nil,
			wantErr:   errors.New("unknown registry status"),
		},
		{
			name:           "repository error is propagated",
			organizationID: "org-error",
			readerRecord:   nil,
			readerErr:      errors.New("database connection failed"),
			wantErr:        errors.New("database connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := &fakeRegistryRepoForProtection{
				record: tt.readerRecord,
				err:    tt.readerErr,
			}

			resolver := NewProtectionStateResolver(reader, NewProtectionMetrics(nil))
			state, err := resolver.Resolve(context.Background(), tt.organizationID)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Resolve() expected error %v, got nil", tt.wantErr)
					return
				}

				// For non-sentinel errors, just check that we got an error
				return
			}

			if err != nil {
				t.Errorf("Resolve() unexpected error = %v", err)
				return
			}

			if state.Mode != tt.wantMode {
				t.Errorf("Resolve() Mode = %v, want %v", state.Mode, tt.wantMode)
			}

			if state.CanReadLegacy != tt.wantCanReadLegacy {
				t.Errorf("Resolve() CanReadLegacy = %v, want %v", state.CanReadLegacy, tt.wantCanReadLegacy)
			}

			if state.CurrentKeysetVersion != tt.wantCurrentKeysetVersion {
				t.Errorf("Resolve() CurrentKeysetVersion = %v, want %v", state.CurrentKeysetVersion, tt.wantCurrentKeysetVersion)
			}

			if state.TenantID != tt.wantTenantID {
				t.Errorf("Resolve() TenantID = %q, want %q", state.TenantID, tt.wantTenantID)
			}

			if state.OrganizationID != tt.wantOrganizationID {
				t.Errorf("Resolve() OrganizationID = %q, want %q", state.OrganizationID, tt.wantOrganizationID)
			}
		})
	}
}

func TestProtectionState_MustUseEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode crypto.EncryptionMode
		want bool
	}{
		{
			name: "legacy mode returns false",
			mode: crypto.EncryptionModeLegacy,
			want: false,
		},
		{
			name: "envelope mode returns true",
			mode: crypto.EncryptionModeEnvelope,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ps := ProtectionState{Mode: tt.mode}

			if got := ps.MustUseEnvelope(); got != tt.want {
				t.Errorf("MustUseEnvelope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewProtectionStateResolver(t *testing.T) {
	t.Parallel()

	reader := &fakeRegistryRepoForProtection{}
	resolver := NewProtectionStateResolver(reader, NewProtectionMetrics(nil))

	if resolver == nil {
		t.Error("NewProtectionStateResolver() returned nil")
	}
}

func TestProtectionStateResolver_Resolve_NilRegistryRepoReturnsLegacyReadable(t *testing.T) {
	t.Parallel()

	resolver := NewProtectionStateResolver(nil, NewProtectionMetrics(nil))
	state, err := resolver.Resolve(context.Background(), "org-123")
	if err != nil {
		t.Errorf("Resolve() with nil registry repo unexpected error = %v", err)
		return
	}

	if state.Mode != crypto.EncryptionModeLegacy {
		t.Errorf("Resolve() Mode = %v, want %v", state.Mode, crypto.EncryptionModeLegacy)
	}

	if !state.CanReadLegacy {
		t.Error("Resolve() CanReadLegacy = false, want true")
	}

	if state.OrganizationID != "org-123" {
		t.Errorf("Resolve() OrganizationID = %q, want %q", state.OrganizationID, "org-123")
	}
}

func TestProtectionStateResolver_Resolve_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &fakeRegistryRepoForProtection{
		record: nil,
		err:    context.Canceled,
	}

	resolver := NewProtectionStateResolver(reader, NewProtectionMetrics(nil))
	_, err := resolver.Resolve(ctx, "org-123")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Resolve() with cancelled context error = %v, want %v", err, context.Canceled)
	}
}

// countingRegistryRepo counts Get calls to verify the resolver's short-TTL cache
// avoids repeated live registry lookups within the cache window.
type countingRegistryRepo struct {
	record   *mmodel.OrganizationRegistryRecord
	err      error
	getCalls int
}

func (c *countingRegistryRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationRegistryRecord, error) {
	c.getCalls++

	return c.record, c.err
}

func (c *countingRegistryRepo) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

func (c *countingRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

func TestProtectionStateResolver_Resolve_CachesWithinTTL(t *testing.T) {
	t.Parallel()

	reader := &countingRegistryRepo{
		record: &mmodel.OrganizationRegistryRecord{
			TenantID:        "tenant-cache",
			OrganizationID:  "org-cache",
			Status:          mmodel.RegistryStatusActive,
			CurrentVersion:  3,
			LegacyReadable:  true,
			ProtectionModel: mmodel.ProtectionModelEnvelope,
		},
	}

	resolver := NewProtectionStateResolver(reader, NewProtectionMetrics(nil))

	first, err := resolver.Resolve(context.Background(), "org-cache")
	if err != nil {
		t.Fatalf("Resolve() first call error = %v", err)
	}

	second, err := resolver.Resolve(context.Background(), "org-cache")
	if err != nil {
		t.Fatalf("Resolve() second call error = %v", err)
	}

	if reader.getCalls != 1 {
		t.Errorf("registry Get calls = %d, want 1 (second resolve must hit the cache)", reader.getCalls)
	}

	if !reflect.DeepEqual(first, second) {
		t.Errorf("cached resolve returned different state: first=%+v second=%+v", first, second)
	}

	if second.Mode != crypto.EncryptionModeEnvelope {
		t.Errorf("cached state Mode = %v, want envelope", second.Mode)
	}
}

func TestProtectionStateResolver_Resolve_CacheKeyedByTenant(t *testing.T) {
	t.Parallel()

	reader := &countingRegistryRepo{
		record: &mmodel.OrganizationRegistryRecord{
			OrganizationID: "shared-org",
			Status:         mmodel.RegistryStatusActive,
		},
	}

	resolver := NewProtectionStateResolver(reader, NewProtectionMetrics(nil))

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	if _, err := resolver.Resolve(ctxA, "shared-org"); err != nil {
		t.Fatalf("Resolve(tenant-a) error = %v", err)
	}

	if _, err := resolver.Resolve(ctxB, "shared-org"); err != nil {
		t.Fatalf("Resolve(tenant-b) error = %v", err)
	}

	if reader.getCalls != 2 {
		t.Errorf("registry Get calls = %d, want 2 (different tenants must not share a cache entry)", reader.getCalls)
	}
}

func TestProtectionStateResolver_Invalidate_RemovesCachedState(t *testing.T) {
	t.Parallel()

	reader := &countingRegistryRepo{
		err: constant.ErrRegistryNotFound,
	}

	resolver := NewProtectionStateResolver(reader, NewProtectionMetrics(nil))
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-cache")

	initial, err := resolver.Resolve(ctx, "org-cache")
	if err != nil {
		t.Fatalf("Resolve() initial call error = %v", err)
	}

	if initial.Mode != crypto.EncryptionModeLegacy {
		t.Fatalf("initial Mode = %v, want legacy", initial.Mode)
	}

	reader.err = nil
	reader.record = &mmodel.OrganizationRegistryRecord{
		TenantID:        "tenant-cache",
		OrganizationID:  "org-cache",
		Status:          mmodel.RegistryStatusActive,
		CurrentVersion:  1,
		LegacyReadable:  true,
		ProtectionModel: mmodel.ProtectionModelEnvelope,
	}

	cached, err := resolver.Resolve(ctx, "org-cache")
	if err != nil {
		t.Fatalf("Resolve() cached call error = %v", err)
	}

	if cached.Mode != crypto.EncryptionModeLegacy {
		t.Fatalf("cached Mode = %v, want legacy before invalidation", cached.Mode)
	}

	resolver.Invalidate("tenant-cache", "org-cache")

	refreshed, err := resolver.Resolve(ctx, "org-cache")
	if err != nil {
		t.Fatalf("Resolve() refreshed call error = %v", err)
	}

	if refreshed.Mode != crypto.EncryptionModeEnvelope {
		t.Errorf("refreshed Mode = %v, want envelope", refreshed.Mode)
	}

	if !refreshed.CanReadLegacy {
		t.Error("refreshed CanReadLegacy = false, want true")
	}

	if reader.getCalls != 2 {
		t.Errorf("registry Get calls = %d, want 2 after invalidation", reader.getCalls)
	}
}
