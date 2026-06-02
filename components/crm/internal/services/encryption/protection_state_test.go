// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// fakeRegistryReader is a test double for RegistryReader.
type fakeRegistryReader struct {
	record *mmodel.OrganizationRegistryRecord
	err    error
}

func (f *fakeRegistryReader) Get(_ context.Context, _ string) (*mmodel.OrganizationRegistryRecord, error) {
	return f.record, f.err
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
			name:           "status legacy returns legacy mode",
			organizationID: "org-456",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:        "tenant-abc",
				OrganizationID:  "org-456",
				Status:          mmodel.RegistryStatusLegacy,
				CurrentVersion:  0,
				LegacyReadable:  true,
				ProtectionModel: mmodel.ProtectionModelLegacy,
			},
			readerErr:                nil,
			wantMode:                 crypto.EncryptionModeLegacy,
			wantCanReadLegacy:        true,
			wantCurrentKeysetVersion: 0,
			wantTenantID:             "tenant-abc",
			wantOrganizationID:       "org-456",
			wantErr:                  nil,
		},
		{
			name:           "status pending_migration returns legacy mode",
			organizationID: "org-789",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:        "tenant-def",
				OrganizationID:  "org-789",
				Status:          mmodel.RegistryStatusPendingMigration,
				CurrentVersion:  0,
				LegacyReadable:  true,
				ProtectionModel: mmodel.ProtectionModelLegacy,
			},
			readerErr:                nil,
			wantMode:                 crypto.EncryptionModeLegacy,
			wantCanReadLegacy:        true,
			wantCurrentKeysetVersion: 0,
			wantTenantID:             "tenant-def",
			wantOrganizationID:       "org-789",
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
			name:           "status partially_migrated returns envelope mode with legacy readable",
			organizationID: "org-partial",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:        "tenant-mno",
				OrganizationID:  "org-partial",
				Status:          mmodel.RegistryStatusPartiallyMigrated,
				CurrentVersion:  3,
				LegacyReadable:  true,
				ProtectionModel: mmodel.ProtectionModelEnvelope,
			},
			readerErr:                nil,
			wantMode:                 crypto.EncryptionModeEnvelope,
			wantCanReadLegacy:        true,
			wantCurrentKeysetVersion: 3,
			wantTenantID:             "tenant-mno",
			wantOrganizationID:       "org-partial",
			wantErr:                  nil,
		},
		{
			name:           "status migration_complete returns envelope mode",
			organizationID: "org-complete",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:        "tenant-pqr",
				OrganizationID:  "org-complete",
				Status:          mmodel.RegistryStatusMigrationComplete,
				CurrentVersion:  5,
				LegacyReadable:  false,
				ProtectionModel: mmodel.ProtectionModelEnvelope,
			},
			readerErr:                nil,
			wantMode:                 crypto.EncryptionModeEnvelope,
			wantCanReadLegacy:        false,
			wantCurrentKeysetVersion: 5,
			wantTenantID:             "tenant-pqr",
			wantOrganizationID:       "org-complete",
			wantErr:                  nil,
		},
		{
			name:           "status failed returns error",
			organizationID: "org-failed",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:       "tenant-stu",
				OrganizationID: "org-failed",
				Status:         mmodel.RegistryStatusFailed,
			},
			readerErr: nil,
			wantErr:   constant.ErrOrganizationEncryptionFailed,
		},
		{
			name:           "status blocked returns error",
			organizationID: "org-blocked",
			readerRecord: &mmodel.OrganizationRegistryRecord{
				TenantID:       "tenant-vwx",
				OrganizationID: "org-blocked",
				Status:         mmodel.RegistryStatusBlocked,
			},
			readerErr: nil,
			wantErr:   constant.ErrOrganizationEncryptionBlocked,
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

			reader := &fakeRegistryReader{
				record: tt.readerRecord,
				err:    tt.readerErr,
			}

			resolver := NewProtectionStateResolver(reader)
			state, err := resolver.Resolve(context.Background(), tt.organizationID)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Resolve() expected error %v, got nil", tt.wantErr)
					return
				}

				// Check for sentinel errors using errors.Is
				if errors.Is(tt.wantErr, constant.ErrOrganizationEncryptionFailed) ||
					errors.Is(tt.wantErr, constant.ErrOrganizationEncryptionBlocked) ||
					errors.Is(tt.wantErr, constant.ErrRegistryNotFound) {
					if !errors.Is(err, tt.wantErr) {
						t.Errorf("Resolve() error = %v, want %v", err, tt.wantErr)
					}

					return
				}

				// For non-sentinel errors, check that we got an error
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

	reader := &fakeRegistryReader{}
	resolver := NewProtectionStateResolver(reader)

	if resolver == nil {
		t.Error("NewProtectionStateResolver() returned nil")
	}
}

func TestProtectionStateResolver_Resolve_NilReader(t *testing.T) {
	t.Parallel()

	resolver := NewProtectionStateResolver(nil)
	_, err := resolver.Resolve(context.Background(), "org-123")

	if err == nil {
		t.Error("Resolve() with nil reader expected error, got nil")
	}
}

func TestProtectionStateResolver_Resolve_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &fakeRegistryReader{
		record: nil,
		err:    context.Canceled,
	}

	resolver := NewProtectionStateResolver(reader)
	_, err := resolver.Resolve(ctx, "org-123")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Resolve() with cancelled context error = %v, want %v", err, context.Canceled)
	}
}
