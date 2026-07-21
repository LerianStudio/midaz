// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/stretchr/testify/assert"
)

// TestResolveTenantID_MultiTenantPopulated verifies that when a tenant ID is
// present in the context via lib-commons multi-tenancy middleware, the helper
// returns that exact tenant ID.
func TestResolveTenantID_MultiTenantPopulated(t *testing.T) {
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

	got := pkgStreaming.ResolveTenantID(ctx)

	assert.Equal(t, "tenant-abc", got)
}

// TestResolveTenantID_EmptyContextReturnsDefault verifies that when no
// tenant ID is in the context the helper falls back to DefaultTenantID
// so every emission still carries a valid ce-tenantid header.
func TestResolveTenantID_EmptyContextReturnsDefault(t *testing.T) {
	got := pkgStreaming.ResolveTenantID(context.Background())

	assert.Equal(t, pkgStreaming.DefaultTenantID, got)
	assert.Equal(t, "default", got)
}

// TestResolveTenantID_NilContextReturnsDefault guards against a nil
// context (defense in depth — should never happen in real call sites
// but must not panic).
func TestResolveTenantID_NilContextReturnsDefault(t *testing.T) {
	//nolint:staticcheck // SA1012: intentional nil-context test
	got := pkgStreaming.ResolveTenantID(nil)

	assert.Equal(t, pkgStreaming.DefaultTenantID, got)
}
