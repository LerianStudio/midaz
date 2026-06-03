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
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestReportRepo_TenantContext_FlowsToMongo verifies the branching logic documented
// in getCollection: when a real *mongo.Database is stored via ContextWithMB,
// GetMBContext must return it so the repository can use the tenant-scoped connection
// instead of the static one.
func TestReportRepo_TenantContext_FlowsToMongo(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect()
	require.NoError(t, err, "mongo.NewClient must not error in unit test setup")

	db := client.Database("tenant_test_db")

	ctx := tmCore.ContextWithMB(context.Background(), db)

	got := tmCore.GetMBContext(ctx)
	assert.NotNil(t, got, "returned *mongo.Database must not be nil")
	assert.Equal(t, db, got, "returned DB must be the same instance stored in context")
}

// TestReportRepo_NoTenantContext_FallsBackToStaticConnection verifies that when no
// tenant context is set, GetMBContext returns nil.
// The repository getCollection function checks for nil to fall back
// to the static MongoConnection — the backward-compatible single-tenant path.
func TestReportRepo_NoTenantContext_FallsBackToStaticConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	got := tmCore.GetMBContext(ctx)
	assert.Nil(t, got,
		"GetMBContext must return nil when no tenant context is set; "+
			"this triggers the static-connection fallback in getCollection")
}

// TestReportRepo_TenantContextIsolation verifies that two independent contexts with
// different tenant databases do not leak into one another.
func TestReportRepo_TenantContextIsolation(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect()
	require.NoError(t, err)

	dbA := client.Database("tenant_a_db")
	dbB := client.Database("tenant_b_db")

	ctxA := tmCore.ContextWithMB(context.Background(), dbA)
	ctxB := tmCore.ContextWithMB(context.Background(), dbB)

	gotA := tmCore.GetMBContext(ctxA)
	gotB := tmCore.GetMBContext(ctxB)

	assert.Equal(t, dbA, gotA, "context A must return tenant A database")
	assert.Equal(t, dbB, gotB, "context B must return tenant B database")
	assert.NotEqual(t, gotA, gotB,
		"tenant A and tenant B must resolve to distinct database instances")
}

// TestReportRepo_TenantContext_NilDB_FallsBack verifies that storing a nil
// *mongo.Database in context does not satisfy GetMBContext — the repository
// must still fall back to the static connection rather than panicking downstream.
func TestReportRepo_TenantContext_NilDB_FallsBack(t *testing.T) {
	t.Parallel()

	ctx := tmCore.ContextWithMB(context.Background(), nil)

	got := tmCore.GetMBContext(ctx)
	assert.Nil(t, got,
		"a nil *mongo.Database stored in context must not satisfy GetMBContext; "+
			"fallback to static connection must be preserved")
}
