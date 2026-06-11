// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"fmt"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// spyQueryRepo is a controllable audit.Repository test double for the read path.
// It captures the organizationID and query handed to FindByOrganization and
// returns a configurable fixed result, so tests can assert the use case passes
// everything through and returns the repository output verbatim. Create is
// present only to satisfy the audit.Repository interface; the query use case
// never calls it.
type spyQueryRepo struct {
	gotOrganizationID string
	gotQuery          audit.AuditQuery
	calls             int

	events     []*mmodel.ProtectionAuditEvent
	pagination libHTTP.CursorPagination
	err        error
}

func (s *spyQueryRepo) Create(_ context.Context, _ *mmodel.ProtectionAuditEvent) error {
	return nil
}

func (s *spyQueryRepo) FindByOrganization(_ context.Context, organizationID string, query audit.AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error) {
	s.calls++
	s.gotOrganizationID = organizationID
	s.gotQuery = query

	return s.events, s.pagination, s.err
}

// contextWithRecorder returns a context carrying a recording tracer plus the
// SpanRecorder that captures the spans the use case ends. This lets the test
// assert on the exact attributes set on the span without any real exporter.
func contextWithRecorder(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracer := provider.Tracer("test")

	return libCommons.ContextWithTracer(context.Background(), tracer), recorder
}

func sampleAuditEvents() []*mmodel.ProtectionAuditEvent {
	return []*mmodel.ProtectionAuditEvent{
		{ID: uuid.New(), OrganizationID: "org-1", EventType: mmodel.AuditEventTypeProvisioning},
		{ID: uuid.New(), OrganizationID: "org-1", EventType: mmodel.AuditEventTypeProvisioning},
	}
}

func TestAuditQueryService_GetAuditEvents_ReturnsRepositoryOutputVerbatim(t *testing.T) {
	t.Parallel()

	ctx, _ := contextWithRecorder(t)

	want := sampleAuditEvents()
	repo := &spyQueryRepo{
		events:     want,
		pagination: libHTTP.CursorPagination{Next: "n", Prev: "p"},
	}

	svc := NewAuditQueryService(repo)

	query := audit.AuditQuery{Limit: 50, SortOrder: "asc", Cursor: "c0"}

	got, pagination, err := svc.GetAuditEvents(ctx, "org-1", query)

	require.NoError(t, err)
	assert.Equal(t, want, got, "events must be returned verbatim")
	assert.Equal(t, libHTTP.CursorPagination{Next: "n", Prev: "p"}, pagination, "pagination must be returned verbatim")

	assert.Equal(t, 1, repo.calls)
	assert.Equal(t, "org-1", repo.gotOrganizationID)
	assert.Equal(t, query, repo.gotQuery, "query must be passed straight through, not re-clamped or re-defaulted")
}

func TestAuditQueryService_GetAuditEvents_EmptyResult(t *testing.T) {
	t.Parallel()

	ctx, _ := contextWithRecorder(t)

	repo := &spyQueryRepo{
		events:     []*mmodel.ProtectionAuditEvent{},
		pagination: libHTTP.CursorPagination{},
	}

	svc := NewAuditQueryService(repo)

	got, pagination, err := svc.GetAuditEvents(ctx, "org-1", audit.AuditQuery{})

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, libHTTP.CursorPagination{}, pagination)
}

func TestAuditQueryService_GetAuditEvents_InvalidCursorPropagatedUnchanged(t *testing.T) {
	t.Parallel()

	ctx, _ := contextWithRecorder(t)

	repo := &spyQueryRepo{err: libHTTP.ErrInvalidCursor}

	svc := NewAuditQueryService(repo)

	got, pagination, err := svc.GetAuditEvents(ctx, "org-1", audit.AuditQuery{Cursor: "bad"})

	require.Error(t, err)
	assert.ErrorIs(t, err, libHTTP.ErrInvalidCursor, "invalid-cursor error must be propagated unchanged")
	assert.Nil(t, got)
	assert.Equal(t, libHTTP.CursorPagination{}, pagination)
}

func TestAuditQueryService_GetAuditEvents_WrappedRepoErrorPropagatedUnchanged(t *testing.T) {
	t.Parallel()

	ctx, _ := contextWithRecorder(t)

	sentinel := fmt.Errorf("mongo unavailable")
	repo := &spyQueryRepo{err: sentinel}

	svc := NewAuditQueryService(repo)

	_, _, err := svc.GetAuditEvents(ctx, "org-1", audit.AuditQuery{})

	require.Error(t, err)
	assert.Same(t, sentinel, err, "repository error must be returned unchanged (handler maps to HTTP status)")
}

func TestAuditQueryService_GetAuditEvents_SpanAttributesNeverLeakFilterValues(t *testing.T) {
	t.Parallel()

	ctx, recorder := contextWithRecorder(t)

	repo := &spyQueryRepo{pagination: libHTTP.CursorPagination{}}

	svc := NewAuditQueryService(repo)

	// Distinctive, easily searchable filter VALUES that MUST NOT appear in any
	// span attribute value.
	const (
		secretAction  = "SECRET_ACTION_VALUE"
		secretActor   = "SECRET_ACTOR_VALUE"
		secretOutcome = "SECRET_OUTCOME_VALUE"
	)

	query := audit.AuditQuery{
		Limit:     25,
		SortOrder: "desc",
		Action:    secretAction,
		Actor:     secretActor,
		Outcome:   secretOutcome,
	}

	_, _, err := svc.GetAuditEvents(ctx, "org-1", query)
	require.NoError(t, err)

	ended := recorder.Ended()
	require.Len(t, ended, 1, "exactly one span must be recorded")

	span := ended[0]
	assert.Equal(t, "service.protection.get_audit_events", span.Name())

	attrs := map[string]string{}
	presence := map[string]bool{}

	for _, kv := range span.Attributes() {
		key := string(kv.Key)
		// No attribute value may equal a filter VALUE.
		strVal := kv.Value.Emit()
		assert.NotContains(t, strVal, secretAction, "span attr %q leaked the action filter value", key)
		assert.NotContains(t, strVal, secretActor, "span attr %q leaked the actor filter value", key)
		assert.NotContains(t, strVal, secretOutcome, "span attr %q leaked the outcome filter value", key)

		attrs[key] = strVal

		if kv.Value.Type().String() == "BOOL" {
			presence[key] = kv.Value.AsBool()
		}
	}

	// Safe attributes MUST be present.
	assert.Equal(t, "org-1", attrs["app.request.organization_id"])
	assert.Equal(t, "25", attrs["app.request.limit"])
	assert.Equal(t, "desc", attrs["app.request.sort_order"])

	// Presence booleans MUST reflect that the filters are set, without the value.
	assert.True(t, presence["app.request.filter_action"])
	assert.True(t, presence["app.request.filter_actor"])
	assert.True(t, presence["app.request.filter_outcome"])
}

// Compile-time assertions that the spy satisfies the read contract and that the
// implementation satisfies the exported AuditQueryService interface.
var (
	_ audit.Repository  = (*spyQueryRepo)(nil)
	_ AuditQueryService = (*auditQueryService)(nil)
)
