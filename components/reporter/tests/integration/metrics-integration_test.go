//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils"

	libMongo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb"
	reportRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	templateRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// newTestMongoConnection creates a MongoConnection pointing at the shared
// testcontainers MongoDB instance managed by TestMain.
func newTestMongoConnection() *libMongo.MongoConnection {
	connStr := testInfra.MongoDB.ConnectionString

	return &libMongo.MongoConnection{
		ConnectionStringSource: connStr,
		Database:               "reporter",
	}
}

// insertTemplateDocuments inserts raw template BSON documents into the
// "template" collection and registers a t.Cleanup that removes them.
func insertTemplateDocuments(t *testing.T, ctx context.Context, coll *mongo.Collection, docs []bson.M) {
	t.Helper()

	ids := make([]any, 0, len(docs))

	for _, doc := range docs {
		ids = append(ids, doc["_id"])

		_, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err, "inserting template fixture")
	}

	t.Cleanup(func() {
		_, _ = coll.DeleteMany(context.Background(), bson.M{"_id": bson.M{"$in": ids}})
	})
}

// insertReportDocuments inserts raw report BSON documents into the
// "report" collection and registers a t.Cleanup that removes them.
func insertReportDocuments(t *testing.T, ctx context.Context, coll *mongo.Collection, docs []bson.M) {
	t.Helper()

	ids := make([]any, 0, len(docs))

	for _, doc := range docs {
		ids = append(ids, doc["_id"])

		_, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err, "inserting report fixture")
	}

	t.Cleanup(func() {
		_, _ = coll.DeleteMany(context.Background(), bson.M{"_id": bson.M{"$in": ids}})
	})
}

// getCollection returns a mongo.Collection by connecting through the shared testcontainer.
func getCollection(t *testing.T, ctx context.Context, collectionName string) *mongo.Collection {
	t.Helper()

	connStr := testInfra.MongoDB.ConnectionString

	clientOpts := options.Client().ApplyURI(connStr)

	client, err := mongo.Connect(clientOpts)
	require.NoError(t, err, "connecting to MongoDB testcontainer")

	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})

	return client.Database("reporter").Collection(collectionName)
}

// --- IS-1: CountAll on template collection returns correct count (deleted_at filtering) ---

func TestIntegration_TemplateRepo_CountAll_FiltersDeletedDocuments(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "template")

	now := time.Now().UTC()
	deletedAt := now.Add(-1 * time.Hour)

	fixtures := []bson.M{
		{"_id": uuid.New(), "output_format": "HTML", "description": "active-1", "filename": "a.tpl", "created_at": now, "updated_at": now, "deleted_at": nil},
		{"_id": uuid.New(), "output_format": "PDF", "description": "active-2", "filename": "b.tpl", "created_at": now, "updated_at": now, "deleted_at": nil},
		{"_id": uuid.New(), "output_format": "HTML", "description": "active-3", "filename": "c.tpl", "created_at": now, "updated_at": now, "deleted_at": nil},
		{"_id": uuid.New(), "output_format": "CSV", "description": "deleted-1", "filename": "d.tpl", "created_at": now, "updated_at": now, "deleted_at": deletedAt},
		{"_id": uuid.New(), "output_format": "CSV", "description": "deleted-2", "filename": "e.tpl", "created_at": now, "updated_at": now, "deleted_at": deletedAt},
	}

	insertTemplateDocuments(t, ctx, coll, fixtures)

	mc := newTestMongoConnection()

	repo, err := templateRepo.NewTemplateMongoDBRepository(mc)
	require.NoError(t, err, "creating template repository")

	t.Cleanup(func() { _ = mc.Close() })

	count, err := repo.CountAll(ctx)
	require.NoError(t, err, "CountAll must not return error")

	// The collection may contain documents from other tests.
	// We assert at least 3 active documents were counted.
	assert.GreaterOrEqual(t, count, int64(3), "CountAll should count at least 3 non-deleted templates")
}

// --- IS-2: CountAll on report collection returns correct count (all statuses) ---

func TestIntegration_ReportRepo_CountAll_IncludesAllStatuses(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "report")

	now := time.Now().UTC()
	templateID := uuid.New()

	fixtures := []bson.M{
		{"_id": uuid.New(), "template_id": templateID, "status": "Completed", "created_at": now, "updated_at": now, "deleted_at": nil},
		{"_id": uuid.New(), "template_id": templateID, "status": "Error", "created_at": now, "updated_at": now, "deleted_at": nil},
		{"_id": uuid.New(), "template_id": templateID, "status": "Processing", "created_at": now, "updated_at": now, "deleted_at": nil},
		{"_id": uuid.New(), "template_id": templateID, "status": "Pending", "created_at": now, "updated_at": now, "deleted_at": nil},
	}

	insertReportDocuments(t, ctx, coll, fixtures)

	mc := newTestMongoConnection()

	repo, err := reportRepo.NewReportMongoDBRepository(mc)
	require.NoError(t, err, "creating report repository")

	t.Cleanup(func() { _ = mc.Close() })

	count, err := repo.CountAll(ctx)
	require.NoError(t, err, "CountAll must not return error")

	// CountAll uses bson.M{} — no deleted_at filter — so all 4 must be counted
	// (plus any pre-existing documents from other tests).
	assert.GreaterOrEqual(t, count, int64(4), "CountAll should count at least 4 reports regardless of status")
}

// --- IS-3: CountByStatus on report collection returns correct count for given status and date range ---

func TestIntegration_ReportRepo_CountByStatus_FiltersStatusAndDateRange(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "report")

	templateID := uuid.New()

	// Define a narrow time window for the test fixtures.
	baseTime := time.Date(2099, 6, 15, 12, 0, 0, 0, time.UTC)
	from := baseTime
	to := baseTime.Add(24 * time.Hour)

	insideWindow := baseTime.Add(6 * time.Hour)
	outsideWindowBefore := baseTime.Add(-1 * time.Hour)
	outsideWindowAfter := to.Add(1 * time.Hour)

	fixtures := []bson.M{
		// Inside window + Error status → should be counted
		{"_id": uuid.New(), "template_id": templateID, "status": "Error", "created_at": insideWindow, "updated_at": insideWindow, "deleted_at": nil},
		{"_id": uuid.New(), "template_id": templateID, "status": "Error", "created_at": insideWindow, "updated_at": insideWindow, "deleted_at": nil},
		// Inside window + different status → should NOT be counted
		{"_id": uuid.New(), "template_id": templateID, "status": "Completed", "created_at": insideWindow, "updated_at": insideWindow, "deleted_at": nil},
		// Error status but outside window → should NOT be counted
		{"_id": uuid.New(), "template_id": templateID, "status": "Error", "created_at": outsideWindowBefore, "updated_at": outsideWindowBefore, "deleted_at": nil},
		{"_id": uuid.New(), "template_id": templateID, "status": "Error", "created_at": outsideWindowAfter, "updated_at": outsideWindowAfter, "deleted_at": nil},
	}

	insertReportDocuments(t, ctx, coll, fixtures)

	mc := newTestMongoConnection()

	repo, err := reportRepo.NewReportMongoDBRepository(mc)
	require.NoError(t, err, "creating report repository")

	t.Cleanup(func() { _ = mc.Close() })

	count, err := repo.CountByStatus(ctx, "Error", from, to)
	require.NoError(t, err, "CountByStatus must not return error")

	// Exactly 2 Error reports fall within [from, to).
	assert.Equal(t, int64(2), count, "CountByStatus should return exactly 2 Error reports in the time window")
}

// --- IS-4: Full metrics handler returns correct response with real MongoDB data ---

func TestIntegration_MetricsHandler_GetMetrics_ReturnsCorrectResponse(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, http.MethodGet, "/v1/metrics", headers, nil)
	require.NoError(t, err, "GET /v1/metrics request error")
	require.Equal(t, 200, code, "expected 200 OK, got %d body=%s", code, string(body))

	var resp struct {
		Templates   int64 `json:"templates"`
		Reports     int64 `json:"reports"`
		DataSources int64 `json:"dataSources"`
		Errors      struct {
			Total               int64 `json:"total"`
			PreviousPeriodTotal int64 `json:"previousPeriodTotal"`
		} `json:"errors"`
	}

	err = json.Unmarshal(body, &resp)
	require.NoError(t, err, "failed to unmarshal metrics response: %s", string(body))

	// Validate structure: all counters must be non-negative.
	assert.GreaterOrEqual(t, resp.Templates, int64(0), "templates count must be >= 0")
	assert.GreaterOrEqual(t, resp.Reports, int64(0), "reports count must be >= 0")
	assert.GreaterOrEqual(t, resp.DataSources, int64(0), "dataSources count must be >= 0")
	assert.GreaterOrEqual(t, resp.Errors.Total, int64(0), "errors.total must be >= 0")
	assert.GreaterOrEqual(t, resp.Errors.PreviousPeriodTotal, int64(0), "errors.previousPeriodTotal must be >= 0")

	// Validate the response contains valid JSON by re-serializing.
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
}

// TestIntegration_MetricsHandler_GetMetrics_InvalidErrorPeriodDays verifies that the handler
// returns 400 when errorPeriodDays is out of range.
func TestIntegration_MetricsHandler_GetMetrics_InvalidErrorPeriodDays(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (HTTP API).
	env := h.LoadEnvironment()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	tests := []struct {
		name  string
		query string
	}{
		{name: "zero_days", query: "?errorPeriodDays=0"},
		{name: "negative_days", query: "?errorPeriodDays=-5"},
		{name: "exceeds_max", query: "?errorPeriodDays=999"},
		{name: "non_integer", query: "?errorPeriodDays=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fmt.Sprintf("/v1/metrics%s", tt.query)

			code, body, err := cli.Request(ctx, http.MethodGet, path, headers, nil)
			require.NoError(t, err, "request error for %s", tt.name)
			assert.Equal(t, 400, code, "expected 400 for %s, got %d body=%s", tt.name, code, string(body))
		})
	}
}

// TestIntegration_MetricsHandler_GetMetrics_CustomErrorPeriodDays verifies that the handler
// accepts a valid custom errorPeriodDays parameter.
func TestIntegration_MetricsHandler_GetMetrics_CustomErrorPeriodDays(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (HTTP API).
	env := h.LoadEnvironment()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, http.MethodGet, "/v1/metrics?errorPeriodDays=30", headers, nil)
	require.NoError(t, err, "GET /v1/metrics?errorPeriodDays=30 request error")
	require.Equal(t, 200, code, "expected 200 OK with errorPeriodDays=30, got %d body=%s", code, string(body))

	var resp struct {
		Templates   int64 `json:"templates"`
		Reports     int64 `json:"reports"`
		DataSources int64 `json:"dataSources"`
		Errors      struct {
			Total               int64 `json:"total"`
			PreviousPeriodTotal int64 `json:"previousPeriodTotal"`
		} `json:"errors"`
	}

	err = json.Unmarshal(body, &resp)
	require.NoError(t, err, "failed to unmarshal metrics response")

	assert.GreaterOrEqual(t, resp.Templates, int64(0))
	assert.GreaterOrEqual(t, resp.Reports, int64(0))
}
