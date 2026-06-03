// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package report

import (
	"context"
	"fmt"
	"testing"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmS3 "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3Key_WithTenant_HasTenantPrefix verifies that GetObjectStorageKeyForTenant
// prepends the tenant ID to the base key when a tenant is present in context.
// The format enforced by lib-commons is "{tenantID}/{key}".
func TestS3Key_WithTenant_HasTenantPrefix(t *testing.T) {
	t.Parallel()

	tenantID := "org-01abc"
	objectName := "myreport.pdf"
	baseKey := fmt.Sprintf("reports/%s", objectName)

	ctx := tmCore.ContextWithTenantID(context.Background(), tenantID)
	key, err := tmS3.GetS3KeyStorageContext(ctx, baseKey)
	require.NoError(t, err)

	expected := tenantID + "/" + baseKey
	assert.Equal(t, expected, key,
		"S3 key with tenant must be {tenantID}/{baseKey}")
	assert.Equal(t, "org-01abc/reports/myreport.pdf", key)
}

// TestS3Key_WithoutTenant_NoPrefix verifies that GetObjectStorageKeyForTenant returns
// the base key unchanged (with no prefix) when no tenant ID is in context.
// This is the single-tenant backward-compatible path.
func TestS3Key_WithoutTenant_NoPrefix(t *testing.T) {
	t.Parallel()

	objectName := "myreport.pdf"
	baseKey := fmt.Sprintf("reports/%s", objectName)

	ctx := context.Background() // no tenant set
	key, err := tmS3.GetS3KeyStorageContext(ctx, baseKey)
	require.NoError(t, err)

	assert.Equal(t, baseKey, key,
		"S3 key without tenant must equal the base key unchanged (single-tenant backward compat)")
	assert.Equal(t, "reports/myreport.pdf", key)
}

// TestS3Key_NilContext_NoPrefix verifies that passing a nil context does not panic
// and returns the base key unchanged (normalised, leading slashes stripped).
func TestS3Key_NilContext_NoPrefix(t *testing.T) {
	t.Parallel()

	baseKey := "reports/niltest.pdf"
	key, err := tmS3.GetS3KeyStorageContext(nil, baseKey)
	require.NoError(t, err)

	assert.Equal(t, baseKey, key,
		"nil context must not panic and must return the key unchanged")
}

// TestS3Key_TwoTenants_ProduceDifferentKeys verifies that two tenants with the same
// object name produce distinct S3 keys, preventing cross-tenant object access.
func TestS3Key_TwoTenants_ProduceDifferentKeys(t *testing.T) {
	t.Parallel()

	objectName := "annual-report.csv"
	baseKey := fmt.Sprintf("reports/%s", objectName)

	ctxA := tmCore.ContextWithTenantID(context.Background(), "tenant-alpha")
	ctxB := tmCore.ContextWithTenantID(context.Background(), "tenant-beta")

	keyA, err := tmS3.GetS3KeyStorageContext(ctxA, baseKey)
	require.NoError(t, err)
	keyB, err := tmS3.GetS3KeyStorageContext(ctxB, baseKey)
	require.NoError(t, err)

	assert.NotEqual(t, keyA, keyB,
		"the same object name for different tenants must produce different S3 keys; "+
			"tenant-alpha and tenant-beta must not share storage paths")
	assert.Equal(t, "tenant-alpha/reports/annual-report.csv", keyA)
	assert.Equal(t, "tenant-beta/reports/annual-report.csv", keyB)
}

// TestS3Key_ReportsPrefix_IsPreservedInTenantKey verifies that the "reports/" prefix
// that the repository always adds is preserved after tenant prefixing. This guards
// against a regression where the prefix is accidentally dropped or duplicated.
func TestS3Key_ReportsPrefix_IsPreservedInTenantKey(t *testing.T) {
	t.Parallel()

	tenantID := "acme-corp"
	objectName := "q4-report.html"
	baseKey := fmt.Sprintf("reports/%s", objectName)

	ctx := tmCore.ContextWithTenantID(context.Background(), tenantID)
	key, err := tmS3.GetS3KeyStorageContext(ctx, baseKey)
	require.NoError(t, err)

	assert.Contains(t, key, "reports/",
		"the reports/ path prefix must be preserved in the tenant-scoped key")
	assert.Contains(t, key, objectName,
		"the object name must be preserved in the tenant-scoped key")
	assert.Contains(t, key, tenantID,
		"the tenant ID must appear as a prefix in the key")
}

func TestS3Key_WithInvalidTenant_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant/bad")
	key, err := tmS3.GetS3KeyStorageContext(ctx, "reports/myreport.pdf")
	require.Error(t, err)
	assert.Empty(t, key)
}
