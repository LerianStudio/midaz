// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrganizationRegistryRecord_ReturnsActiveStatus(t *testing.T) {
	t.Parallel()

	// When creating a new registry record
	record, err := NewOrganizationRegistryRecord(
		"tenant-123",
		"org-456",
		"provisioning-service",
		"initial provisioning",
	)

	// Then it should succeed
	require.NoError(t, err)
	require.NotNil(t, record)

	// And the status should be active (not pending_migration)
	assert.Equal(t, RegistryStatusActive, record.Status,
		"newly created registry records should have status 'active', not 'pending_migration'")
}

func TestNewOrganizationRegistryRecord_ReturnsEnvelopeProtectionModel(t *testing.T) {
	t.Parallel()

	// When creating a new registry record
	record, err := NewOrganizationRegistryRecord(
		"tenant-123",
		"org-456",
		"provisioning-service",
		"initial provisioning",
	)

	// Then it should succeed
	require.NoError(t, err)
	require.NotNil(t, record)

	// And the protection model should be envelope (not legacy)
	assert.Equal(t, ProtectionModelEnvelope, record.ProtectionModel,
		"newly created registry records should use 'envelope' protection model, not 'legacy'")
}

func TestNewOrganizationRegistryRecord_SetsVersionFields(t *testing.T) {
	t.Parallel()

	// When creating a new registry record
	record, err := NewOrganizationRegistryRecord(
		"tenant-123",
		"org-456",
		"provisioning-service",
		"initial provisioning",
	)

	// Then it should succeed
	require.NoError(t, err)
	require.NotNil(t, record)

	// And CurrentVersion should be set to 1
	assert.Equal(t, 1, record.CurrentVersion,
		"newly created registry records should have CurrentVersion = 1")

	// And ReadableVersions should contain [1]
	assert.Equal(t, []int{1}, record.ReadableVersions,
		"newly created registry records should have ReadableVersions = [1]")
}

func TestNewOrganizationRegistryRecord_SetsLegacyReadableTrue(t *testing.T) {
	t.Parallel()

	record, err := NewOrganizationRegistryRecord("tenant-123", "org-456", "provisioning-service", "initial provisioning")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.True(t, record.LegacyReadable,
		"newly created registry records should have LegacyReadable = true to allow reading existing legacy data")
}
