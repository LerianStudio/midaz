// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrganizationRegistryRecord(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")

	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "tenant-a", record.TenantID)
	assert.Equal(t, "org-a", record.OrganizationID)
	assert.Equal(t, RegistryStatusPendingMigration, record.Status)
	assert.Equal(t, ProtectionModelLegacy, record.ProtectionModel)
	assert.Equal(t, int64(1), record.Revision)
	assert.True(t, record.LegacyReadable)
	assert.Equal(t, "system", record.CreatedBy)
	assert.Equal(t, "system", record.UpdatedBy)
	assert.Equal(t, "initial setup", record.LastTransitionReason)
}

func TestNewOrganizationRegistryRecord_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tenantID       string
		organizationID string
		actor          string
		reason         string
		wantErrMsg     string
	}{
		{
			name:           "empty tenant",
			tenantID:       "",
			organizationID: "org-a",
			actor:          "system",
			reason:         "test",
			wantErrMsg:     "tenant_id is required",
		},
		{
			name:           "empty organization",
			tenantID:       "tenant-a",
			organizationID: "",
			actor:          "system",
			reason:         "test",
			wantErrMsg:     "organization_id is required",
		},
		{
			name:           "empty actor",
			tenantID:       "tenant-a",
			organizationID: "org-a",
			actor:          "",
			reason:         "test",
			wantErrMsg:     "actor is required",
		},
		{
			name:           "empty reason",
			tenantID:       "tenant-a",
			organizationID: "org-a",
			actor:          "system",
			reason:         "",
			wantErrMsg:     "reason is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			record, err := NewOrganizationRegistryRecord(tt.tenantID, tt.organizationID, tt.actor, tt.reason)

			require.Error(t, err)
			assert.Nil(t, record)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestOrganizationRegistryRecord_Activate(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	err = record.Activate(1, "admin", "keyset provisioned")

	require.NoError(t, err)
	assert.Equal(t, RegistryStatusActive, record.Status)
	assert.Equal(t, ProtectionModelEnvelope, record.ProtectionModel)
	assert.Equal(t, 1, record.CurrentVersion)
	assert.Equal(t, []int{1}, record.ReadableVersions)
	assert.Equal(t, int64(2), record.Revision)
	assert.Equal(t, "admin", record.UpdatedBy)
	assert.Equal(t, "keyset provisioned", record.LastTransitionReason)
}

func TestOrganizationRegistryRecord_ActivateRevisionConflict(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	// Try to activate with wrong expected revision
	err = record.Activate(99, "admin", "keyset provisioned")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRegistryRevisionConflict)
	// Status should not change
	assert.Equal(t, RegistryStatusPendingMigration, record.Status)
}

func TestOrganizationRegistryRecord_ActivateInvalidTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status RegistryStatus
	}{
		{name: "from active", status: RegistryStatusActive},
		{name: "from legacy", status: RegistryStatusLegacy},
		{name: "from partially_migrated", status: RegistryStatusPartiallyMigrated},
		{name: "from migration_complete", status: RegistryStatusMigrationComplete},
		{name: "from failed", status: RegistryStatusFailed},
		{name: "from blocked", status: RegistryStatusBlocked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			record := &OrganizationRegistryRecord{
				TenantID:       "tenant-a",
				OrganizationID: "org-a",
				Status:         tt.status,
				Revision:       1,
			}

			err := record.Activate(1, "admin", "keyset provisioned")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot activate from status")
			assert.Contains(t, err.Error(), string(tt.status))
			// Status should not change
			assert.Equal(t, tt.status, record.Status)
		})
	}
}

func TestOrganizationRegistryRecord_UsesEnvelopeMode(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	// Initially uses legacy mode
	assert.False(t, record.UsesEnvelopeMode())

	// After activation uses envelope mode
	err = record.Activate(1, "admin", "activated")
	require.NoError(t, err)
	assert.True(t, record.UsesEnvelopeMode())
}

func TestOrganizationRegistryRecord_CanReadLegacy(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	// Initially can read legacy
	assert.True(t, record.CanReadLegacy())

	// After activation can still read legacy (during migration)
	err = record.Activate(1, "admin", "activated")
	require.NoError(t, err)
	assert.True(t, record.CanReadLegacy())
}

func TestOrganizationRegistryRecord_ReadableKeysetVersions(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	err = record.Activate(1, "admin", "activated")
	require.NoError(t, err)

	versions := record.ReadableKeysetVersions()

	assert.Equal(t, []int{1}, versions)

	// Verify returned slice is a copy
	versions[0] = 999
	assert.Equal(t, []int{1}, record.ReadableVersions)
}
