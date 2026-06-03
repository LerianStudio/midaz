// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package report_test

import (
	"context"
	"testing"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
)

// TestReportRepo_BackwardCompat_NoTenantContext verifies that the tenant-manager core
// package is correctly imported and that GetMBContext returns nil
// when no tenant connection is set in context (single-tenant / no-middleware mode).
func TestReportRepo_BackwardCompat_NoTenantContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// GetMBContext must return nil when no tenant is in context.
	// The repository fallback relies on this nil check to switch to the static connection.
	got := tmCore.GetMBContext(ctx)
	assert.Nil(t, got,
		"expected nil when no tenant context is set; this triggers fallback to static connection")
}

// TestReportRepo_TenantContext_MongoSet verifies that GetMBContext returns nil when
// a nil *mongo.Database is stored in context via ContextWithMB.
func TestReportRepo_TenantContext_MongoSet(t *testing.T) {
	t.Parallel()

	// We pass nil here to verify the nil case does NOT succeed (nil is not stored).
	ctx := context.Background()
	ctx = tmCore.ContextWithMB(ctx, nil)

	// Storing nil does not satisfy the check — GetMBContext returns nil for nil.
	got := tmCore.GetMBContext(ctx)
	assert.Nil(t, got,
		"storing nil mongo DB must not satisfy tenant context check; fallback must trigger")
}
