//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"strings"
	"testing"
	"time"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// scopePlanSeedCount is large enough that a scan would be unmistakable in the
// examined-document counts asserted below.
const scopePlanSeedCount = 500

// explainScopeFilter runs the given filter under explain and reports the index
// the planner chose plus how much of the collection it had to touch.
func explainScopeFilter(t *testing.T, ctx context.Context, coll *mongo.Collection, filter bson.M) (indexName string, keysExamined, docsExamined int64, plan string) {
	t.Helper()

	cmd := bson.D{
		{Key: "explain", Value: bson.D{
			{Key: "find", Value: coll.Name()},
			{Key: "filter", Value: filter},
		}},
		{Key: "verbosity", Value: "executionStats"},
	}

	var raw bson.Raw
	require.NoError(t, coll.Database().RunCommand(ctx, cmd).Decode(&raw), "explain must succeed")

	plan = raw.Lookup("queryPlanner", "winningPlan").String()

	if v, err := raw.LookupErr("queryPlanner", "winningPlan", "inputStage", "indexName"); err == nil {
		indexName = v.StringValue()
	}

	keys, err := raw.LookupErr("executionStats", "totalKeysExamined")
	require.NoError(t, err, "explain must report totalKeysExamined")
	keysExamined = keys.AsInt64()

	docs, err := raw.LookupErr("executionStats", "totalDocsExamined")
	require.NoError(t, err, "explain must report totalDocsExamined")
	docsExamined = docs.AsInt64()

	return indexName, keysExamined, docsExamined, plan
}

// TestIntegration_BillingPackageScopeFilter_ResolvesBySingleDocumentFetch pins
// the access path behind every by-ID billing package read and write.
//
// billingPackageScopeFilter always pins _id, and _id is backed by the unique
// index MongoDB creates on every collection. That makes the lookup resolve to
// at most one document before organization_id, ledger_id and deleted_at are
// applied as residual predicates, under BOTH scope shapes the helper emits. The
// counts are asserted rather than the index name so the test states the
// guarantee that matters — a by-ID lookup on the money path never scans —
// instead of pinning one planner's choice.
//
// This is what makes a ledger-scoped compound index unnecessary: there is no
// scan for it to remove. If a future filter stops pinning _id, these counts
// climb with the collection and this test fails.
func TestIntegration_BillingPackageScopeFilter_ResolvesBySingleDocumentFetch(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	ctx := context.Background()

	conn := &feesmongo.MongoConnection{
		ConnectionStringSource: container.URI,
		Database:               container.DBName,
		MaxPoolSize:            1,
		DB:                     container.Client,
	}
	require.NoError(t, EnsureIndexes(ctx, conn), "EnsureIndexes must succeed")

	coll := container.Client.
		Database(strings.ToLower(container.DBName)).
		Collection(strings.ToLower(feeconstant.BillingPackageCollection))

	organizationID := uuid.NewString()
	ownerLedgerID := uuid.NewString()
	otherLedgerID := uuid.NewString()
	targetID := uuid.NewString()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	docs := make([]any, 0, scopePlanSeedCount)

	// Both ledgers get roughly half the documents so that dropping _id from the
	// filter would leave a large scan under EVERY scope shape, including the
	// one that matches the owning ledger.
	for i := range scopePlanSeedCount {
		id, ledgerID := uuid.NewString(), otherLedgerID
		if i%2 == 0 {
			ledgerID = ownerLedgerID
		}

		if i == scopePlanSeedCount/2 {
			id, ledgerID = targetID, ownerLedgerID
		}

		docs = append(docs, bson.M{
			"_id":             id,
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"deleted_at":      nil,
			"created_at":      fixedTime.Add(time.Duration(i) * time.Second),
		})
	}

	_, err := coll.InsertMany(ctx, docs)
	require.NoError(t, err, "seeding the billing package collection must succeed")

	tests := []struct {
		name        string
		ledgerID    string
		wantMatches int64
	}{
		{
			name:        "organization scope omits the ledger clause",
			ledgerID:    AnyLedger,
			wantMatches: 1,
		},
		{
			name:        "ledger scope matches the owning ledger",
			ledgerID:    ownerLedgerID,
			wantMatches: 1,
		},
		{
			name:        "ledger scope rejects a package owned by another ledger",
			ledgerID:    otherLedgerID,
			wantMatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := billingPackageScopeFilter(targetID, organizationID, tt.ledgerID)

			indexName, keysExamined, docsExamined, plan := explainScopeFilter(t, ctx, coll, filter)

			assert.LessOrEqualf(t, docsExamined, int64(1),
				"by-ID lookup must fetch at most one document, not scan %d seeded; index=%q plan=%s",
				scopePlanSeedCount, indexName, plan)
			assert.LessOrEqualf(t, keysExamined, int64(1),
				"by-ID lookup must examine at most one index key; index=%q plan=%s", indexName, plan)

			matched, err := coll.CountDocuments(ctx, filter)
			require.NoError(t, err, "counting matches must succeed")
			assert.Equal(t, tt.wantMatches, matched, "scope filter must match the expected document count")
		})
	}
}
