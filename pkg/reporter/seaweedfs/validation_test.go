// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seaweedfs

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateKeyForTenant_MultiTenant_ValidKey(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "org-01abc")
	err := ValidateKeyForTenant(ctx, "org-01abc/reports/myreport.pdf")

	require.NoError(t, err)
}

func TestValidateKeyForTenant_MultiTenant_InvalidKey_WrongPrefix(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "org-01abc")
	err := ValidateKeyForTenant(ctx, "org-OTHER/reports/myreport.pdf")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant key prefix mismatch")
}

func TestValidateKeyForTenant_MultiTenant_InvalidKey_NoPrefix(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "org-01abc")
	err := ValidateKeyForTenant(ctx, "reports/myreport.pdf")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant key prefix mismatch")
}

func TestValidateKeyForTenant_SingleTenant_NoValidation(t *testing.T) {
	t.Parallel()

	// No tenant in context (single-tenant mode) -- should pass regardless of key
	ctx := context.Background()
	err := ValidateKeyForTenant(ctx, "reports/myreport.pdf")

	require.NoError(t, err)
}

func TestValidateKeyForTenant_SingleTenant_AnyKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := ValidateKeyForTenant(ctx, "anything/goes/here")

	require.NoError(t, err)
}

func TestValidateKeyForTenant_NilContext(t *testing.T) {
	t.Parallel()

	// nil context should not panic and should pass (single-tenant fallback)
	err := ValidateKeyForTenant(nil, "reports/myreport.pdf")

	require.NoError(t, err)
}

func TestValidateKeyForTenant_EmptyKey(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "org-01abc")
	err := ValidateKeyForTenant(ctx, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant key prefix mismatch")
}

func TestValidateKeyForTenant_TraversalAttempt(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "org-01abc")
	// Attempt to use path traversal to escape tenant prefix
	err := ValidateKeyForTenant(ctx, "org-01abc/../other-tenant/reports/secret.pdf")

	// The key technically starts with "org-01abc/" so basic prefix check passes.
	// Path traversal prevention is handled by S3/object storage itself, not by this function.
	// This test documents the boundary: ValidateKeyForTenant checks prefix only.
	require.NoError(t, err)
}
