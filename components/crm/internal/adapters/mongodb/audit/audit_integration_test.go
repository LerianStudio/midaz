//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package audit

import (
	"context"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// createAuditRepository creates a MongoDBRepository for integration testing.
// Resets the global index tracker for this database to ensure a fresh state,
// since each test runs with a new MongoDB container.
func createAuditRepository(t *testing.T, container *mongotestutil.ContainerResult) *MongoDBRepository {
	t.Helper()

	globalIndexTracker.reset(container.DBName + ":" + auditCollection)

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	repo, err := NewMongoDBRepository(conn)
	require.NoError(t, err)

	return repo
}

// newIntegrationEvent builds a clean, valid protection audit event.
func newIntegrationEvent(t *testing.T, organizationID string) *mmodel.ProtectionAuditEvent {
	t.Helper()

	event, err := mmodel.NewProtectionAuditEvent(mmodel.ProtectionAuditEventInput{
		TenantID:       "tenant-" + uuid.New().String()[:8],
		OrganizationID: organizationID,
		EventType:      mmodel.AuditEventTypeProvisioning,
		Action:         mmodel.AuditActionProvision,
		Outcome:        mmodel.AuditOutcomeSuccess,
		ActorID:        "actor-1",
		ActorType:      "service",
		Reason:         "integration test provisioning",
		RequestID:      "req-" + uuid.New().String()[:8],
		Details: &mmodel.AuditDetails{
			PreviousStatus: "PENDING",
			NewStatus:      "ACTIVE",
			AffectedKeyIDs: []uint32{1, 2},
		},
	})
	require.NoError(t, err)

	return event
}

func TestIntegration_AuditRepo_Create_CleanEventInserts(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-" + uuid.New().String()[:8]
	event := newIntegrationEvent(t, organizationID)

	err := repo.Create(ctx, event)
	require.NoError(t, err, "clean event should insert without error")

	count := mongotestutil.CountDocuments(t, container.Database, auditCollection, bson.M{"organization_id": organizationID})
	assert.Equal(t, int64(1), count, "exactly one audit document should be persisted")
}

func TestIntegration_AuditRepo_Create_BuildsAllSixIndexes(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-idx-" + uuid.New().String()[:8]
	event := newIntegrationEvent(t, organizationID)

	err := repo.Create(ctx, event)
	require.NoError(t, err)

	cursor, err := container.Database.Collection(auditCollection).Indexes().List(ctx)
	require.NoError(t, err)

	var indexes []bson.M
	require.NoError(t, cursor.All(ctx, &indexes))

	// 6 created indexes + the implicit _id_ index = 7 total.
	assert.Len(t, indexes, 7, "expected 6 created indexes plus the default _id_ index")

	byKey := make(map[string]bson.M, len(indexes))

	for _, idx := range indexes {
		keys, ok := idx["key"].(bson.M)
		require.True(t, ok, "index key must be a document")

		byKey[keyString(keys)] = idx
	}

	// New org_id-led / _id-tailed query indexes (tenant_id dropped — physical
	// DB-per-tenant isolation makes it redundant; _id tail serves cursor+sort).
	require.Contains(t, byKey, "organization_id=1,_id=-1")
	require.Contains(t, byKey, "organization_id=1,action=1,_id=-1")
	require.Contains(t, byKey, "organization_id=1,actor_id=1,_id=-1")
	require.Contains(t, byKey, "organization_id=1,outcome=1,_id=-1")

	// The old tenant_id-led / timestamp-tailed indexes must no longer exist.
	require.NotContains(t, byKey, "tenant_id=1,organization_id=1,timestamp=-1")

	requestIndex := byKey["request_id=1"]
	require.NotNil(t, requestIndex, "request_id index must exist")
	assert.Equal(t, true, requestIndex["sparse"], "request_id index must be sparse")

	expiresIndex := byKey["expires_at=1"]
	require.NotNil(t, expiresIndex, "expires_at TTL index must exist")
	require.Contains(t, expiresIndex, "expireAfterSeconds", "expires_at must be a TTL index")
	assert.EqualValues(t, 0, toInt64(expiresIndex["expireAfterSeconds"]), "expireAfterSeconds must be exactly 0")
}

func TestIntegration_AuditRepo_Create_ForbiddenContentNotInserted(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	tests := []struct {
		name   string
		reason string
	}{
		{name: "vault_token", reason: "leaked hvs.CAESIabc123_-token must be dropped"},
		{name: "tink_envelope_marker", reason: "value tink:v1:Q0FFU0lKOHhfYWJj must be dropped"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			organizationID := "org-forbidden-" + uuid.New().String()[:8]
			event := newIntegrationEvent(t, organizationID)
			event.Reason = tc.reason

			err := repo.Create(ctx, event)
			require.Error(t, err, "forbidden-content event must be rejected")
			assert.ErrorIs(t, err, constant.ErrAuditWriteFailed)

			count := mongotestutil.CountDocuments(t, container.Database, auditCollection, bson.M{"organization_id": organizationID})
			assert.Equal(t, int64(0), count, "forbidden-content event must NOT be persisted")
		})
	}
}

// seedEvents creates n clean events for organizationID via Create, applying an
// optional per-event mutator before insert. Because Create persists the model's
// UUIDv7 _id, insertion order matches chronological _id order. It returns the
// events in insertion order.
func seedEvents(t *testing.T, repo *MongoDBRepository, organizationID string, n int, mutate func(i int, e *mmodel.ProtectionAuditEvent)) []*mmodel.ProtectionAuditEvent {
	t.Helper()

	ctx := context.Background()
	out := make([]*mmodel.ProtectionAuditEvent, 0, n)

	for i := 0; i < n; i++ {
		event := newIntegrationEvent(t, organizationID)

		if mutate != nil {
			mutate(i, event)
		}

		require.NoError(t, repo.Create(ctx, event))

		out = append(out, event)
	}

	return out
}

func TestIntegration_AuditRepo_FindByOrganization_EmptyResult(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-empty-" + uuid.New().String()[:8]

	events, pagination, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10})

	require.NoError(t, err)
	assert.Empty(t, events, "no events should be returned for an empty organization")
	assert.Empty(t, pagination.Next, "no next cursor when there are no results")
	assert.Empty(t, pagination.Prev, "no prev cursor when there are no results")
}

func TestIntegration_AuditRepo_FindByOrganization_CursorPaginationRoundTrips(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-page-" + uuid.New().String()[:8]
	seedEvents(t, repo, organizationID, 5, nil)

	// Page 1 (desc/newest-first): 2 items + a next cursor.
	page1, p1, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, p1.Next, "a next cursor is expected with more pages remaining")

	// Page 2.
	page2, p2, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 2, Cursor: p1.Next})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	require.NotEmpty(t, p2.Next)
	require.NotEmpty(t, p2.Prev, "a prev cursor is expected on a middle page")

	// Page 3 (last): 1 item, no next cursor.
	page3, p3, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 2, Cursor: p2.Next})
	require.NoError(t, err)
	require.Len(t, page3, 1)
	assert.Empty(t, p3.Next, "last page must not return a next cursor")

	// All five IDs must be distinct and span every seeded event.
	seen := map[string]struct{}{}
	for _, e := range append(append(append([]*mmodel.ProtectionAuditEvent{}, page1...), page2...), page3...) {
		seen[e.ID.String()] = struct{}{}
	}

	assert.Len(t, seen, 5, "pagination must traverse all 5 events without duplicates or gaps")
}

func TestIntegration_AuditRepo_FindByOrganization_SortOrder(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-sort-" + uuid.New().String()[:8]
	seeded := seedEvents(t, repo, organizationID, 3, nil)

	oldest := seeded[0].ID.String()
	newest := seeded[2].ID.String()

	desc, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, SortOrder: "desc"})
	require.NoError(t, err)
	require.Len(t, desc, 3)
	assert.Equal(t, newest, desc[0].ID.String(), "desc must return newest first")
	assert.Equal(t, oldest, desc[2].ID.String(), "desc must return oldest last")

	asc, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, SortOrder: "asc"})
	require.NoError(t, err)
	require.Len(t, asc, 3)
	assert.Equal(t, oldest, asc[0].ID.String(), "asc must return oldest first")
	assert.Equal(t, newest, asc[2].ID.String(), "asc must return newest last")
}

func TestIntegration_AuditRepo_FindByOrganization_EqualityFilters(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filter-" + uuid.New().String()[:8]

	// Distribute each discriminating attribute across a DIFFERENT seeded doc so
	// each equality filter is independently discriminating (not co-mutated on a
	// single doc). 5 docs total: index 1 -> action, 2 -> actor, 3 -> outcome,
	// 0 and 4 remain the baseline.
	seedEvents(t, repo, organizationID, 5, func(i int, e *mmodel.ProtectionAuditEvent) {
		switch i {
		case 1:
			e.Action = "rotate"
		case 2:
			e.ActorID = "actor-special"
		case 3:
			e.Outcome = mmodel.AuditOutcomeFailure
		}
	})

	byAction, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, Action: "rotate"})
	require.NoError(t, err)
	require.Len(t, byAction, 1, "action filter must match exactly one event")
	assert.Equal(t, mmodel.AuditAction("rotate"), byAction[0].Action)

	byActor, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, Actor: "actor-special"})
	require.NoError(t, err)
	require.Len(t, byActor, 1, "actor filter must match exactly one event")
	assert.Equal(t, "actor-special", byActor[0].ActorID)

	byOutcome, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, Outcome: string(mmodel.AuditOutcomeFailure)})
	require.NoError(t, err)
	require.Len(t, byOutcome, 1, "outcome filter must match exactly one event")
	assert.Equal(t, mmodel.AuditOutcomeFailure, byOutcome[0].Outcome)
}

func TestIntegration_AuditRepo_FindByOrganization_FilterWithPaginationAcrossPages(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filterpage-" + uuid.New().String()[:8]

	// Seed 10 docs: 5 with action "rotate" (the filtered set) interleaved with 5
	// baseline "provision" docs, so the equality filter must independently select
	// only the rotate docs while paginating across multiple pages.
	const matching = 5

	seedEvents(t, repo, organizationID, 2*matching, func(i int, e *mmodel.ProtectionAuditEvent) {
		if i%2 == 0 {
			e.Action = "rotate"
		}
	})

	query := AuditQuery{Limit: 2, Action: "rotate"}

	seen := map[string]struct{}{}
	cursor := ""
	pages := 0

	for {
		page := query
		page.Cursor = cursor

		events, pagination, err := repo.FindByOrganization(ctx, organizationID, page)
		require.NoError(t, err)

		for _, e := range events {
			assert.Equal(t, mmodel.AuditAction("rotate"), e.Action, "filtered+paged set must contain only matching docs")

			_, dup := seen[e.ID.String()]
			require.False(t, dup, "no duplicates across pages")
			seen[e.ID.String()] = struct{}{}
		}

		pages++
		require.LessOrEqual(t, pages, matching+2, "pagination must terminate")

		if pagination.Next == "" {
			break
		}

		cursor = pagination.Next
	}

	assert.Len(t, seen, matching, "filtered+paged traversal must cover every matching doc with no gaps or duplicates")
}

func TestIntegration_AuditRepo_FindByOrganization_TimeRangeFilter(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-time-" + uuid.New().String()[:8]

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	// Three events on Jan 1, Jan 2, Jan 3. _id stays UUIDv7-chronological from
	// Create; we override only the queried timestamp field.
	seedEvents(t, repo, organizationID, 3, func(i int, e *mmodel.ProtectionAuditEvent) {
		e.Timestamp = base.AddDate(0, 0, i)
	})

	start := base.AddDate(0, 0, 1) // Jan 2
	end := base.AddDate(0, 0, 2)   // Jan 3
	mid, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{
		Limit:     10,
		StartTime: start,
		EndTime:   end,
	})
	require.NoError(t, err)
	require.Len(t, mid, 2, "time range [Jan 2, Jan 3] must include exactly two events")

	for _, e := range mid {
		assert.False(t, e.Timestamp.Before(start), "result must not predate StartTime")
		assert.False(t, e.Timestamp.After(end), "result must not exceed EndTime")
	}

	// Lower bound only: Jan 2 and Jan 3.
	fromJan2, _, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, StartTime: start})
	require.NoError(t, err)
	assert.Len(t, fromJan2, 2, "StartTime-only bound must include Jan 2 and Jan 3")
}

func TestIntegration_AuditRepo_FindByOrganization_InvalidCursorRejected(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := createAuditRepository(t, container)
	ctx := context.Background()

	organizationID := "org-badcursor-" + uuid.New().String()[:8]
	seedEvents(t, repo, organizationID, 1, nil)

	events, pagination, err := repo.FindByOrganization(ctx, organizationID, AuditQuery{Limit: 10, Cursor: "not-a-valid-token"})

	require.Error(t, err, "an invalid cursor must surface a validation error, not be ignored")
	assert.ErrorIs(t, err, libHTTP.ErrInvalidCursor)
	assert.Nil(t, events)
	assert.Equal(t, libHTTP.CursorPagination{}, pagination)
}

// keyString renders an index key document as a stable comparable string.
func keyString(keys bson.M) string {
	// Field set covering both the new org_id-led indexes and the legacy keys, so
	// the test can assert presence of the new set and absence of the old.
	order := []string{"tenant_id", "organization_id", "action", "actor_id", "outcome", "_id", "timestamp", "request_id", "expires_at"}

	out := ""

	for _, field := range order {
		v, ok := keys[field]
		if !ok {
			continue
		}

		if out != "" {
			out += ","
		}

		out += field + "=" + numToString(v)
	}

	return out
}

// numToString normalizes a numeric index direction (int32/int64/float64) to its string form.
func numToString(v any) string {
	switch n := v.(type) {
	case int32:
		if n < 0 {
			return "-1"
		}

		return "1"
	case int64:
		if n < 0 {
			return "-1"
		}

		return "1"
	case float64:
		if n < 0 {
			return "-1"
		}

		return "1"
	default:
		return "?"
	}
}

// toInt64 normalizes a numeric BSON value (int32/int64/float64) to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return -1
	}
}
