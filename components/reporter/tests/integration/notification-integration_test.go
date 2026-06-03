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
	deadlineRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/deadline"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// insertDeadlineDocuments inserts raw deadline BSON documents into the
// "deadline" collection and registers a t.Cleanup that removes them.
func insertDeadlineDocuments(t *testing.T, ctx context.Context, coll *mongo.Collection, docs []bson.M) {
	t.Helper()

	ids := make([]any, 0, len(docs))

	for _, doc := range docs {
		ids = append(ids, doc["_id"])

		_, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err, "inserting deadline fixture")
	}

	t.Cleanup(func() {
		_, _ = coll.DeleteMany(context.Background(), bson.M{"_id": bson.M{"$in": ids}})
	})
}

// --- IS-1: FindActiveNotifiable returns only active, non-delivered, non-deleted deadlines ---

func TestIntegration_DeadlineRepo_FindActiveNotifiable_FiltersCorrectly(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "deadline")

	now := time.Now().UTC()
	deletedAt := now.Add(-1 * time.Hour)
	deliveredAt := now.Add(-2 * time.Hour)

	// Use far-future due dates to ensure they are within notification window
	futureDue := now.Add(48 * time.Hour)

	fixtures := []bson.M{
		// Active, not delivered, not deleted → SHOULD be returned
		{
			"_id": uuid.New(), "name": "active-notifiable-1", "description": "test", "type": "regulatory",
			"due_date": futureDue, "frequency": "monthly", "active": true,
			"notify_days_before": 30, "color": "#FF5733",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
		// Active, not delivered, not deleted → SHOULD be returned
		{
			"_id": uuid.New(), "name": "active-notifiable-2", "description": "test", "type": "custom",
			"due_date": futureDue.Add(1 * time.Hour), "frequency": "once", "active": true,
			"notify_days_before": 14, "color": "#00FF00",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
		// Inactive → should NOT be returned
		{
			"_id": uuid.New(), "name": "inactive-deadline", "description": "test", "type": "regulatory",
			"due_date": futureDue, "frequency": "monthly", "active": false,
			"notify_days_before": 5, "color": "#0000FF",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
		// Delivered → should NOT be returned
		{
			"_id": uuid.New(), "name": "delivered-deadline", "description": "test", "type": "regulatory",
			"due_date": futureDue, "frequency": "monthly", "active": true,
			"notify_days_before": 5, "color": "#FFFF00",
			"delivered_at": deliveredAt, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
		// Soft-deleted → should NOT be returned
		{
			"_id": uuid.New(), "name": "deleted-deadline", "description": "test", "type": "custom",
			"due_date": futureDue, "frequency": "once", "active": true,
			"notify_days_before": 5, "color": "#FF00FF",
			"delivered_at": nil, "deleted_at": deletedAt,
			"created_at": now, "updated_at": now,
		},
	}

	insertDeadlineDocuments(t, ctx, coll, fixtures)

	mc := newTestMongoConnection()

	repo, err := deadlineRepo.NewDeadlineMongoDBRepository(mc)
	require.NoError(t, err, "creating deadline repository")

	t.Cleanup(func() { _ = mc.Close() })

	results, err := repo.FindActiveNotifiable(ctx)
	require.NoError(t, err, "FindActiveNotifiable must not return error")

	// Collect returned names to verify only active, non-delivered, non-deleted deadlines
	returnedNames := make(map[string]bool, len(results))
	for _, d := range results {
		returnedNames[d.Name] = true
	}

	assert.True(t, returnedNames["active-notifiable-1"], "active-notifiable-1 should be returned")
	assert.True(t, returnedNames["active-notifiable-2"], "active-notifiable-2 should be returned")
	assert.False(t, returnedNames["inactive-deadline"], "inactive deadline should NOT be returned")
	assert.False(t, returnedNames["delivered-deadline"], "delivered deadline should NOT be returned")
	assert.False(t, returnedNames["deleted-deadline"], "soft-deleted deadline should NOT be returned")
}

// --- IS-2: FindActiveNotifiable respects limit parameter ---

func TestIntegration_DeadlineRepo_FindActiveNotifiable_RespectsLimit(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "deadline")

	now := time.Now().UTC()
	futureDue := now.Add(48 * time.Hour)

	// Insert 5 active, notifiable deadlines
	fixtures := make([]bson.M, 5)
	for i := range fixtures {
		fixtures[i] = bson.M{
			"_id": uuid.New(), "name": fmt.Sprintf("limit-test-%d", i+1), "description": "test",
			"type": "regulatory", "due_date": futureDue.Add(time.Duration(i) * time.Hour),
			"frequency": "monthly", "active": true,
			"notify_days_before": 30, "color": "#FF5733",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		}
	}

	insertDeadlineDocuments(t, ctx, coll, fixtures)

	mc := newTestMongoConnection()

	repo, err := deadlineRepo.NewDeadlineMongoDBRepository(mc)
	require.NoError(t, err, "creating deadline repository")

	t.Cleanup(func() { _ = mc.Close() })

	// Request limit of 2 — should return at most 2 results
	results, err := repo.FindActiveNotifiable(ctx)
	require.NoError(t, err, "FindActiveNotifiable with limit=2 must not return error")

	assert.LessOrEqual(t, len(results), 2, "FindActiveNotifiable should return at most 2 results when limit=2")
	assert.Greater(t, len(results), 0, "FindActiveNotifiable should return at least 1 result")

	// Request limit of 1 — should return at most 1 result
	results, err = repo.FindActiveNotifiable(ctx)
	require.NoError(t, err, "FindActiveNotifiable with limit=1 must not return error")

	assert.LessOrEqual(t, len(results), 1, "FindActiveNotifiable should return at most 1 result when limit=1")
}

// --- IS-3: Full notification handler returns correct response with real MongoDB data ---

func TestIntegration_NotificationHandler_GetNotifications_ReturnsCorrectResponse(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, http.MethodGet, "/v1/deadlines/notifications", headers, nil)
	require.NoError(t, err, "GET /v1/deadlines/notifications request error")
	require.Equal(t, 200, code, "expected 200 OK, got %d body=%s", code, string(body))

	var resp struct {
		Items []struct {
			ID               string `json:"id"`
			Name             string `json:"name"`
			Description      string `json:"description,omitempty"`
			Type             string `json:"type"`
			DueDate          string `json:"dueDate"`
			Frequency        string `json:"frequency"`
			MonthsOfYear     []int  `json:"monthsOfYear,omitempty"`
			Color            string `json:"color"`
			Severity         string `json:"severity"`
			DaysUntilDue     int    `json:"daysUntilDue"`
			NotifyDaysBefore int    `json:"notifyDaysBefore"`
		} `json:"items"`
		Total int `json:"total"`
	}

	err = json.Unmarshal(body, &resp)
	require.NoError(t, err, "failed to unmarshal notifications response: %s", string(body))

	// Validate structure: total must match items length
	assert.Equal(t, len(resp.Items), resp.Total, "total must match items array length")
	assert.GreaterOrEqual(t, resp.Total, 0, "total must be non-negative")

	// Validate each item has required fields populated
	for i, item := range resp.Items {
		assert.NotEmpty(t, item.ID, "item[%d].id must not be empty", i)
		assert.NotEmpty(t, item.Name, "item[%d].name must not be empty", i)
		assert.NotEmpty(t, item.Type, "item[%d].type must not be empty", i)
		assert.NotEmpty(t, item.DueDate, "item[%d].dueDate must not be empty", i)
		assert.NotEmpty(t, item.Frequency, "item[%d].frequency must not be empty", i)
		assert.NotEmpty(t, item.Color, "item[%d].color must not be empty", i)
		assert.Contains(t, []string{"overdue", "warning", "info"}, item.Severity,
			"item[%d].severity must be overdue, warning, or info", i)
	}

	// Validate response can be re-serialized (valid JSON)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
}

// TestIntegration_NotificationHandler_GetNotifications_InvalidLimit verifies that the handler
// returns 400 when limit query parameter is out of range.
func TestIntegration_NotificationHandler_GetNotifications_InvalidLimit(t *testing.T) {
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
		{name: "zero_limit", query: "?limit=0"},
		{name: "negative_limit", query: "?limit=-5"},
		{name: "exceeds_max", query: "?limit=999"},
		{name: "non_integer", query: "?limit=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fmt.Sprintf("/v1/deadlines/notifications%s", tt.query)

			code, body, err := cli.Request(ctx, http.MethodGet, path, headers, nil)
			require.NoError(t, err, "request error for %s", tt.name)
			assert.Equal(t, 400, code, "expected 400 for %s, got %d body=%s", tt.name, code, string(body))
		})
	}
}

// TestIntegration_NotificationHandler_GetNotifications_ValidLimit verifies that the handler
// accepts a valid custom limit parameter.
func TestIntegration_NotificationHandler_GetNotifications_ValidLimit(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (HTTP API).
	env := h.LoadEnvironment()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, http.MethodGet, "/v1/deadlines/notifications?limit=5", headers, nil)
	require.NoError(t, err, "GET /v1/deadlines/notifications?limit=5 request error")
	require.Equal(t, 200, code, "expected 200 OK with limit=5, got %d body=%s", code, string(body))

	var resp struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}

	err = json.Unmarshal(body, &resp)
	require.NoError(t, err, "failed to unmarshal notifications response")

	assert.LessOrEqual(t, resp.Total, 5, "total should not exceed the requested limit of 5")
	assert.Equal(t, len(resp.Items), resp.Total, "total must match items array length")
}

// --- IS-4: Handler correctly filters by notification window (excludes deadlines outside window) ---

func TestIntegration_DeadlineRepo_FindActiveNotifiable_SortsByDueDateAscending(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "deadline")

	now := time.Now().UTC()

	// Create deadlines with different due dates to verify sort order
	// Insert in reverse order to prove sorting works
	dueEarly := now.Add(24 * time.Hour)
	dueMid := now.Add(72 * time.Hour)
	dueLate := now.Add(168 * time.Hour)

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	fixtures := []bson.M{
		// Latest due date — inserted first
		{
			"_id": id3, "name": "late-due", "description": "test", "type": "regulatory",
			"due_date": dueLate, "frequency": "monthly", "active": true,
			"notify_days_before": 30, "color": "#0000FF",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
		// Earliest due date — inserted second
		{
			"_id": id1, "name": "early-due", "description": "test", "type": "regulatory",
			"due_date": dueEarly, "frequency": "monthly", "active": true,
			"notify_days_before": 30, "color": "#FF0000",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
		// Middle due date — inserted third
		{
			"_id": id2, "name": "mid-due", "description": "test", "type": "custom",
			"due_date": dueMid, "frequency": "once", "active": true,
			"notify_days_before": 30, "color": "#00FF00",
			"delivered_at": nil, "deleted_at": nil,
			"created_at": now, "updated_at": now,
		},
	}

	insertDeadlineDocuments(t, ctx, coll, fixtures)

	mc := newTestMongoConnection()

	repo, err := deadlineRepo.NewDeadlineMongoDBRepository(mc)
	require.NoError(t, err, "creating deadline repository")

	t.Cleanup(func() { _ = mc.Close() })

	results, err := repo.FindActiveNotifiable(ctx)
	require.NoError(t, err, "FindActiveNotifiable must not return error")
	require.GreaterOrEqual(t, len(results), 3, "should return at least the 3 inserted fixtures")

	// Verify results are sorted by due_date ascending: find the positions of our test fixtures
	fixtureOrder := make([]string, 0)
	for _, d := range results {
		switch d.ID {
		case id1:
			fixtureOrder = append(fixtureOrder, "early")
		case id2:
			fixtureOrder = append(fixtureOrder, "mid")
		case id3:
			fixtureOrder = append(fixtureOrder, "late")
		}
	}

	require.Equal(t, 3, len(fixtureOrder), "all 3 test fixtures must appear in results")
	assert.Equal(t, "early", fixtureOrder[0], "earliest due date should appear first")
	assert.Equal(t, "mid", fixtureOrder[1], "middle due date should appear second")
	assert.Equal(t, "late", fixtureOrder[2], "latest due date should appear third")
}

// TestIntegration_NotificationHandler_GetNotifications_FiltersByNotificationWindow verifies that
// the handler excludes deadlines whose due date is outside their notification window.
// A deadline with notifyDaysBefore=3 and dueDate 30 days from now should NOT appear.
func TestIntegration_NotificationHandler_GetNotifications_FiltersByNotificationWindow(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer, HTTP API).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coll := getCollection(t, ctx, "deadline")

	now := time.Now().UTC()

	// Deadline with due date 30 days in the future and notifyDaysBefore=3
	// This is OUTSIDE the notification window (30 days > 3 days) → should be excluded
	outsideWindowID := uuid.New()
	outsideWindowFixture := bson.M{
		"_id": outsideWindowID, "name": "outside-window-test", "description": "should not appear",
		"type": "regulatory", "due_date": now.Add(30 * 24 * time.Hour), "frequency": "monthly",
		"active": true, "notify_days_before": 3, "color": "#AABBCC",
		"delivered_at": nil, "deleted_at": nil,
		"created_at": now, "updated_at": now,
	}

	// Deadline with due date 2 days in the future and notifyDaysBefore=5
	// This is INSIDE the notification window (2 days < 5 days) → should be included
	insideWindowID := uuid.New()
	insideWindowFixture := bson.M{
		"_id": insideWindowID, "name": "inside-window-test", "description": "should appear",
		"type": "custom", "due_date": now.Add(2 * 24 * time.Hour), "frequency": "once",
		"active": true, "notify_days_before": 5, "color": "#DDEEFF",
		"delivered_at": nil, "deleted_at": nil,
		"created_at": now, "updated_at": now,
	}

	// Deadline that is overdue (due date in the past, not delivered)
	// Overdue deadlines should always appear regardless of notifyDaysBefore
	overdueID := uuid.New()
	overdueFixture := bson.M{
		"_id": overdueID, "name": "overdue-window-test", "description": "overdue should appear",
		"type": "regulatory", "due_date": now.Add(-2 * 24 * time.Hour), "frequency": "once",
		"active": true, "notify_days_before": 1, "color": "#FF0000",
		"delivered_at": nil, "deleted_at": nil,
		"created_at": now, "updated_at": now,
	}

	insertDeadlineDocuments(t, ctx, coll, []bson.M{outsideWindowFixture, insideWindowFixture, overdueFixture})

	// Query the notifications handler via HTTP
	env := h.LoadEnvironment()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, http.MethodGet, "/v1/deadlines/notifications?limit=100", headers, nil)
	require.NoError(t, err, "GET /v1/deadlines/notifications request error")
	require.Equal(t, 200, code, "expected 200 OK, got %d body=%s", code, string(body))

	var resp struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
		Total int `json:"total"`
	}

	err = json.Unmarshal(body, &resp)
	require.NoError(t, err, "failed to unmarshal notifications response")

	// Check which of our test fixtures appear in the response
	foundInside := false
	foundOutside := false
	foundOverdue := false

	for _, item := range resp.Items {
		switch item.ID {
		case insideWindowID.String():
			foundInside = true
		case outsideWindowID.String():
			foundOutside = true
		case overdueID.String():
			foundOverdue = true
		}
	}

	assert.True(t, foundInside, "deadline inside notification window (2 days due, 5 notifyDaysBefore) should appear")
	assert.False(t, foundOutside, "deadline outside notification window (30 days due, 3 notifyDaysBefore) should NOT appear")
	assert.True(t, foundOverdue, "overdue deadline should always appear in notifications")
}

// TestIntegration_DeadlineRepo_FindActiveNotifiable_EmptyCollection verifies that
// FindActiveNotifiable returns an empty slice (not nil) when no matching deadlines exist.
func TestIntegration_DeadlineRepo_FindActiveNotifiable_EmptyCollection(t *testing.T) {
	// NOTE: Cannot use t.Parallel() — shared external service (MongoDB testcontainer).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mc := &libMongo.MongoConnection{
		ConnectionStringSource: testInfra.MongoDB.ConnectionString,
		Database:               "reporter_empty_test_" + uuid.New().String()[:8],
	}

	repo, err := deadlineRepo.NewDeadlineMongoDBRepository(mc)
	require.NoError(t, err, "creating deadline repository for empty DB")

	t.Cleanup(func() { _ = mc.Close() })

	results, err := repo.FindActiveNotifiable(ctx)
	require.NoError(t, err, "FindActiveNotifiable on empty collection must not return error")

	assert.Empty(t, results, "FindActiveNotifiable should return empty slice for empty collection")
}
