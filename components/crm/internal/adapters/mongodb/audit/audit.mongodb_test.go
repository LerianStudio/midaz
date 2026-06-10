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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// fixedID is a deterministic UUID used across conversion tests so the round-trip
// does not depend on time.Now() or random generation.
var fixedID = uuid.MustParse("0190b3f0-1234-7abc-8def-0123456789ab")

// fixedTimestamp is a deterministic event timestamp.
var fixedTimestamp = time.Date(2026, time.June, 10, 12, 30, 0, 0, time.UTC)

func newFullEvent() *mmodel.ProtectionAuditEvent {
	return &mmodel.ProtectionAuditEvent{
		ID:             fixedID,
		TenantID:       "tenant-a",
		OrganizationID: "org-a",
		EventType:      mmodel.AuditEventTypeProvisioning,
		Action:         mmodel.AuditActionProvision,
		Outcome:        mmodel.AuditOutcomeSuccess,
		ActorID:        "actor-1",
		ActorType:      "service",
		Reason:         "initial provisioning",
		Timestamp:      fixedTimestamp,
		RequestID:      "req-123",
		Details: &mmodel.AuditDetails{
			PreviousStatus:    "PENDING",
			NewStatus:         "ACTIVE",
			AffectedKeyIDs:    []uint32{1, 2, 3},
			ProviderReference: "ref-xyz",
			ErrorCode:         "",
		},
	}
}

// encodeCursor is a test helper that produces an opaque libHTTP cursor token for
// the given _id and direction, mirroring what the repository emits.
func encodeCursor(t *testing.T, id, direction string) string {
	t.Helper()

	token, err := libHTTP.EncodeCursor(libHTTP.Cursor{ID: id, Direction: direction})
	require.NoError(t, err)

	return token
}

func TestFromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	model := FromEntity(nil)

	assert.Nil(t, model)
}

func TestToEntity_NilModel(t *testing.T) {
	t.Parallel()

	var model *AuditEventModel

	entity := model.ToEntity()

	assert.Nil(t, entity)
}

func TestFromEntity_LeavesExpiresAtNil(t *testing.T) {
	t.Parallel()

	model := FromEntity(newFullEvent())

	require.NotNil(t, model)
	assert.Nil(t, model.ExpiresAt, "FromEntity must leave expires_at nil (TTL disabled)")
}

func TestRoundTrip_FullEvent_PreservesAllFields(t *testing.T) {
	t.Parallel()

	original := newFullEvent()

	model := FromEntity(original)
	require.NotNil(t, model)
	assert.Equal(t, original.ID.String(), model.ID, "_id must be the UUID string")

	got := model.ToEntity()
	require.NotNil(t, got)

	assert.Equal(t, original.ID, got.ID)
	assert.Equal(t, original.TenantID, got.TenantID)
	assert.Equal(t, original.OrganizationID, got.OrganizationID)
	assert.Equal(t, original.EventType, got.EventType)
	assert.Equal(t, original.Action, got.Action)
	assert.Equal(t, original.Outcome, got.Outcome)
	assert.Equal(t, original.ActorID, got.ActorID)
	assert.Equal(t, original.ActorType, got.ActorType)
	assert.Equal(t, original.Reason, got.Reason)
	assert.Equal(t, original.Timestamp, got.Timestamp)
	assert.Equal(t, original.RequestID, got.RequestID)

	require.NotNil(t, got.Details)
	assert.Equal(t, original.Details.PreviousStatus, got.Details.PreviousStatus)
	assert.Equal(t, original.Details.NewStatus, got.Details.NewStatus)
	assert.Equal(t, original.Details.AffectedKeyIDs, got.Details.AffectedKeyIDs)
	assert.Equal(t, original.Details.ProviderReference, got.Details.ProviderReference)
	assert.Equal(t, original.Details.ErrorCode, got.Details.ErrorCode)
}

func TestRoundTrip_NilDetails_PreservedAsNil(t *testing.T) {
	t.Parallel()

	original := newFullEvent()
	original.Details = nil

	got := FromEntity(original).ToEntity()

	require.NotNil(t, got)
	assert.Nil(t, got.Details, "nil Details must round-trip as nil")
}

func TestRoundTrip_AffectedKeyIDs_EmptyVsNilPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ids  []uint32
	}{
		{name: "nil_slice", ids: nil},
		{name: "empty_slice", ids: []uint32{}},
		{name: "populated_slice", ids: []uint32{10, 20}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := newFullEvent()
			original.Details.AffectedKeyIDs = tc.ids

			got := FromEntity(original).ToEntity()

			require.NotNil(t, got)
			require.NotNil(t, got.Details)
			assert.Equal(t, tc.ids, got.Details.AffectedKeyIDs)

			if tc.ids == nil {
				assert.Nil(t, got.Details.AffectedKeyIDs)
			} else {
				assert.NotNil(t, got.Details.AffectedKeyIDs)
			}
		})
	}
}

func TestToEntity_InvalidID_ReturnsNilUUID(t *testing.T) {
	t.Parallel()

	model := &AuditEventModel{ID: "not-a-uuid"}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, uuid.Nil, entity.ID, "an unparseable _id must map to uuid.Nil")
}

func TestNewMongoDBRepository_ImplementsRepository(t *testing.T) {
	t.Parallel()

	repo, err := NewMongoDBRepository(nil)

	require.NoError(t, err)
	require.Implements(t, (*Repository)(nil), repo)
}

// ============================================================================
// Forbidden-content guard tests (pure, unit-testable)
// ============================================================================

func TestContainsForbiddenContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(e *mmodel.ProtectionAuditEvent)
		want  bool
	}{
		{
			name:  "clean_event_passes",
			setup: func(_ *mmodel.ProtectionAuditEvent) {},
			want:  false,
		},
		{
			name: "nil_details_is_safe",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Details = nil
			},
			want: false,
		},
		{
			name: "vault_token_in_reason_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Reason = "leaked hvs.CAESIJ8x_abc-123 token"
			},
			want: true,
		},
		{
			name: "vault_token_in_provider_reference_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Details.ProviderReference = "hvs.AbCdEf012_-"
			},
			want: true,
		},
		{
			name: "tink_keyset_marker_in_reason_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Reason = `{"primaryKeyId": 123456789, "key": []}`
			},
			want: true,
		},
		{
			name: "tink_keyset_marker_spaced_in_error_code_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Details.ErrorCode = `"primaryKeyId"   :   42`
			},
			want: true,
		},
		{
			name: "tink_marker_in_previous_status_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Details.PreviousStatus = `"primaryKeyId":7`
			},
			want: true,
		},
		{
			name: "tink_marker_in_new_status_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Details.NewStatus = `"primaryKeyId":8`
			},
			want: true,
		},
		{
			name: "tink_envelope_marker_in_reason_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Reason = "value tink:v1:Q0FFU0lKOHhfYWJj is leaking"
			},
			want: true,
		},
		{
			name: "tink_envelope_marker_in_provider_reference_rejected",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Details.ProviderReference = "tink:v42:YWJjZGVm-_=="
			},
			want: true,
		},
		{
			name: "plain_reason_with_hvs_word_passes",
			setup: func(e *mmodel.ProtectionAuditEvent) {
				e.Reason = "rotation harvest completed"
			},
			want: false,
		},
		{
			name:  "nil_event_is_safe",
			setup: nil,
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var event *mmodel.ProtectionAuditEvent
			if tc.setup != nil {
				event = newFullEvent()
				tc.setup(event)
			}

			got := containsForbiddenContent(event)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMongoDBRepository_Create_RejectsForbiddenContent(t *testing.T) {
	t.Parallel()

	repo, err := NewMongoDBRepository(nil)
	require.NoError(t, err)

	event := newFullEvent()
	event.Reason = "secret hvs.CAESIabc123_-token here"

	err = repo.Create(context.Background(), event)

	require.Error(t, err, "forbidden content must be rejected without insert")
	assert.ErrorIs(t, err, constant.ErrAuditWriteFailed)
}

// ============================================================================
// FindByOrganization pure-helper tests (no Mongo I/O)
// ============================================================================

func TestClampLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int
		want int64
	}{
		{name: "zero_defaults_to_20", in: 0, want: 20},
		{name: "negative_defaults_to_20", in: -5, want: 20},
		{name: "one_is_kept", in: 1, want: 1},
		{name: "in_range_is_kept", in: 50, want: 50},
		{name: "exactly_100_is_kept", in: 100, want: 100},
		{name: "over_100_clamped_to_100", in: 101, want: 100},
		{name: "way_over_clamped_to_100", in: 100000, want: 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := clampLimit(tc.in)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFindOptions_SortDirectionAndLimitPlusOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		limit       int64
		ascending   bool
		wantLimit   int64
		wantSortAsc bool
	}{
		{name: "ascending_sorts_by_id_asc", limit: 10, ascending: true, wantLimit: 11, wantSortAsc: true},
		{name: "descending_sorts_by_id_desc", limit: 10, ascending: false, wantLimit: 11, wantSortAsc: false},
		{name: "limit_plus_one_default", limit: 20, ascending: false, wantLimit: 21, wantSortAsc: false},
		{name: "limit_plus_one_max", limit: 100, ascending: true, wantLimit: 101, wantSortAsc: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := findOptions(tc.limit, tc.ascending)

			require.NotNil(t, opts)
			require.NotNil(t, opts.Limit)
			assert.Equal(t, tc.wantLimit, *opts.Limit, "limit must be the over-fetch value + 1")

			sort, ok := opts.Sort.(bson.D)
			require.True(t, ok, "sort must be a bson.D")
			require.Len(t, sort, 1)
			assert.Equal(t, "_id", sort[0].Key)

			if tc.wantSortAsc {
				assert.Equal(t, 1, sort[0].Value)
			} else {
				assert.Equal(t, -1, sort[0].Value)
			}
		})
	}
}

func TestBuildFindFilter_OrganizationAlwaysPresent(t *testing.T) {
	t.Parallel()

	filter := buildFindFilter("org-a", AuditQuery{})

	assert.Equal(t, "org-a", filter["organization_id"])
}

func TestBuildFindFilter_OptionalEqualityFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       AuditQuery
		field       string
		wantPresent bool
		wantValue   any
	}{
		{name: "action_absent", query: AuditQuery{}, field: "action", wantPresent: false},
		{name: "action_present", query: AuditQuery{Action: "PROVISION"}, field: "action", wantPresent: true, wantValue: "PROVISION"},
		{name: "actor_absent", query: AuditQuery{}, field: "actor_id", wantPresent: false},
		{name: "actor_present_maps_to_actor_id", query: AuditQuery{Actor: "actor-7"}, field: "actor_id", wantPresent: true, wantValue: "actor-7"},
		{name: "outcome_absent", query: AuditQuery{}, field: "outcome", wantPresent: false},
		{name: "outcome_present", query: AuditQuery{Outcome: "SUCCESS"}, field: "outcome", wantPresent: true, wantValue: "SUCCESS"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter := buildFindFilter("org-a", tc.query)

			value, present := filter[tc.field]
			assert.Equal(t, tc.wantPresent, present)

			if tc.wantPresent {
				assert.Equal(t, tc.wantValue, value)
			}
		})
	}
}

func TestBuildFindFilter_TimeRangeBounds(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		query    AuditQuery
		wantTime bool
		wantGte  bool
		wantLte  bool
	}{
		{name: "no_bounds", query: AuditQuery{}, wantTime: false},
		{name: "only_start", query: AuditQuery{StartTime: start}, wantTime: true, wantGte: true, wantLte: false},
		{name: "only_end", query: AuditQuery{EndTime: end}, wantTime: true, wantGte: false, wantLte: true},
		{name: "both_bounds", query: AuditQuery{StartTime: start, EndTime: end}, wantTime: true, wantGte: true, wantLte: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter := buildFindFilter("org-a", tc.query)

			raw, present := filter["timestamp"]
			require.Equal(t, tc.wantTime, present)

			if !tc.wantTime {
				return
			}

			ts, ok := raw.(bson.M)
			require.True(t, ok, "timestamp filter must be a bson.M range")

			gte, hasGte := ts["$gte"]
			assert.Equal(t, tc.wantGte, hasGte)

			if tc.wantGte {
				assert.Equal(t, start, gte)
			}

			lte, hasLte := ts["$lte"]
			assert.Equal(t, tc.wantLte, hasLte)

			if tc.wantLte {
				assert.Equal(t, end, lte)
			}
		})
	}
}

func TestApplyCursorComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mongoOp  string
		cursorID string
	}{
		{name: "gt_for_forward", mongoOp: "$gt", cursorID: fixedID.String()},
		{name: "lt_for_backward", mongoOp: "$lt", cursorID: fixedID.String()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter := bson.M{"organization_id": "org-a"}
			applyCursorComparison(filter, tc.mongoOp, tc.cursorID)

			raw, present := filter["_id"]
			require.True(t, present, "_id comparison must be present")

			idFilter, ok := raw.(bson.M)
			require.True(t, ok)

			value, hasOp := idFilter[tc.mongoOp]
			assert.True(t, hasOp, "expected %s operator", tc.mongoOp)
			assert.Equal(t, tc.cursorID, value)
		})
	}
}

func TestMongoOperatorFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		operator string
		want     string
	}{
		{name: "gt_maps_to_mongo_gt", operator: ">", want: "$gt"},
		{name: "lt_maps_to_mongo_lt", operator: "<", want: "$lt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, mongoOperatorFor(tc.operator))
		})
	}
}

func TestMongoDBRepository_FindByOrganization_InvalidCursorRejected(t *testing.T) {
	t.Parallel()

	repo, err := NewMongoDBRepository(nil)
	require.NoError(t, err)

	events, pagination, err := repo.FindByOrganization(context.Background(), "org-a", AuditQuery{Cursor: "not-a-valid-token"})

	require.Error(t, err, "invalid cursor must surface a validation error")
	assert.Nil(t, events)
	assert.Equal(t, libHTTP.CursorPagination{}, pagination)
	assert.ErrorIs(t, err, libHTTP.ErrInvalidCursor)
}
