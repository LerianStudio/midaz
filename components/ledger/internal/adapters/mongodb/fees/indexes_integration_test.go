//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb_test

import (
	"context"
	"strings"
	"testing"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// expectedPackIndexes are the 7 compound indexes pack.EnsureIndexes creates.
// They are asserted by name so a renamed/dropped index fails loudly instead of
// silently degrading fee-package query performance.
var expectedPackIndexes = []string{
	"idx_org_ledger_enable_deleted",
	"idx_org_ledger_enable_route_deleted",
	"idx_org_ledger_enable_segment_deleted",
	"idx_org_ledger_enable_amounts_deleted",
	"idx_fee_calculation_complete",
	"idx_org_deleted_created",
	"idx_id_org_deleted",
}

// expectedBillingPackageIndexes are the 4 compound indexes
// billing_package.EnsureIndexes creates.
var expectedBillingPackageIndexes = []string{
	"idx_bp_org_ledger_type_enable_deleted",
	"idx_bp_org_ledger_route_enable_deleted",
	"idx_bp_org_ledger_deleted_created",
	"idx_bp_id_org_deleted",
}

// newFeesConnection builds a fees MongoConnection backed by the testcontainer's
// already-connected client (injected via DB so GetDB bypasses lazy dialing).
func newFeesConnection(t *testing.T, container *mongotestutil.ContainerResult) *feesmongo.MongoConnection {
	t.Helper()

	return &feesmongo.MongoConnection{
		ConnectionStringSource: container.URI,
		Database:               container.DBName,
		MaxPoolSize:            1,
		DB:                     container.Client,
	}
}

// listIndexNames returns the set of index names present on the given collection.
func listIndexNames(t *testing.T, ctx context.Context, container *mongotestutil.ContainerResult, collection string) map[string]struct{} {
	t.Helper()

	coll := container.Client.Database(strings.ToLower(container.DBName)).Collection(strings.ToLower(collection))

	cursor, err := coll.Indexes().List(ctx)
	require.NoError(t, err, "listing indexes must succeed")
	defer func() { _ = cursor.Close(ctx) }()

	names := make(map[string]struct{})

	for cursor.Next(ctx) {
		var idx bson.M
		require.NoError(t, cursor.Decode(&idx), "decoding index spec must succeed")

		if name, ok := idx["name"].(string); ok {
			names[name] = struct{}{}
		}
	}

	require.NoError(t, cursor.Err(), "index cursor must not error")

	return names
}

// TestIntegration_FeesEnsureIndexes_PackAndBillingPackage asserts that, after
// EnsureIndexes runs against a real Mongo, ALL 11 named compound indexes exist:
// 7 on the package collection and 4 on the billing_package collection. This is
// the persistence half of the P4 fees collapse (P4-T05): the indexes are
// code-created mongo.IndexModel, not migration files, so this is the only place
// their existence is verified end-to-end.
func TestIntegration_FeesEnsureIndexes_PackAndBillingPackage(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	ctx := context.Background()

	conn := newFeesConnection(t, container)

	require.NoError(t, pack.EnsureIndexes(ctx, conn), "pack.EnsureIndexes must succeed against real Mongo")
	require.NoError(t, billing_package.EnsureIndexes(ctx, conn), "billing_package.EnsureIndexes must succeed against real Mongo")

	packNames := listIndexNames(t, ctx, container, feeconstant.PackageCollection)
	for _, want := range expectedPackIndexes {
		_, ok := packNames[want]
		assert.Truef(t, ok, "package collection must carry index %q after EnsureIndexes; got %v", want, packNames)
	}

	bpNames := listIndexNames(t, ctx, container, feeconstant.BillingPackageCollection)
	for _, want := range expectedBillingPackageIndexes {
		_, ok := bpNames[want]
		assert.Truef(t, ok, "billing_package collection must carry index %q after EnsureIndexes; got %v", want, bpNames)
	}

	// Guard the headline count: 7 + 4 = 11 named compound indexes total. The
	// implicit _id_ index is excluded from the named-set assertions above.
	assert.Len(t, expectedPackIndexes, 7, "pack must declare exactly 7 compound indexes")
	assert.Len(t, expectedBillingPackageIndexes, 4, "billing_package must declare exactly 4 compound indexes")
}

// TestIntegration_FeesEnsureIndexes_Idempotent asserts EnsureIndexes can run
// twice without error — startup re-runs it on every boot, and the fees Mongo is
// per-tenant, so it re-runs per tenant database at first connection rather than
// once globally.
//
// Erroring is not the only failure worth catching: a second run must leave the
// index set unchanged. Comparing the name sets is what makes this a no-op proof
// instead of merely a no-error proof — an EnsureIndexes that added or renamed an
// index on the second pass would show up here.
//
// It does NOT catch a spec edit that reuses an existing index name with different
// keys. Both runs read the same edited spec against the same fresh container, so
// the second run finds exactly what the first created and converges; the case that
// breaks — an already provisioned tenant whose stored keys differ — has no
// counterpart here. Catching it needs the keys pinned against a committed
// expectation, not a self-comparison.
func TestIntegration_FeesEnsureIndexes_Idempotent(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	ctx := context.Background()

	conn := newFeesConnection(t, container)

	snapshots := make([]map[string]struct{}, 0, 4)

	for i := range 2 {
		require.NoErrorf(t, pack.EnsureIndexes(ctx, conn), "pack.EnsureIndexes must be idempotent (run %d)", i+1)
		require.NoErrorf(t, billing_package.EnsureIndexes(ctx, conn), "billing_package.EnsureIndexes must be idempotent (run %d)", i+1)

		snapshots = append(
			snapshots,
			listIndexNames(t, ctx, container, feeconstant.PackageCollection),
			listIndexNames(t, ctx, container, feeconstant.BillingPackageCollection),
		)
	}

	require.NotEmpty(t, snapshots[0], "package collection must carry indexes after the first run")
	require.NotEmpty(t, snapshots[1], "billing_package collection must carry indexes after the first run")

	assert.Equal(t, snapshots[0], snapshots[2], "second pack.EnsureIndexes run must not change the package index set")
	assert.Equal(t, snapshots[1], snapshots[3], "second billing_package.EnsureIndexes run must not change the billing_package index set")
}
