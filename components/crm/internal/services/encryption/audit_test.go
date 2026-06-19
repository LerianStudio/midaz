// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestMain verifies no goroutines leak across the whole package test run. The
// async audit path is fire-and-forget, so a leaking writer goroutine would be a
// real defect this guards against.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// spyRepo is a controllable audit.Repository test double. It records Create
// calls, can be configured to return an error or to panic, and signals each
// call over a channel so async tests synchronize deterministically (no sleep).
type spyRepo struct {
	mu       sync.Mutex
	calls    []*mmodel.ProtectionAuditEvent
	err      error
	panicMsg string
	called   chan struct{}
}

func newSpyRepo() *spyRepo {
	return &spyRepo{called: make(chan struct{}, 1)}
}

func (s *spyRepo) Create(_ context.Context, event *mmodel.ProtectionAuditEvent) error {
	s.mu.Lock()
	s.calls = append(s.calls, event)
	s.mu.Unlock()

	// Signal after recording so a waiting test observes a recorded call.
	select {
	case s.called <- struct{}{}:
	default:
	}

	if s.panicMsg != "" {
		panic(s.panicMsg)
	}

	return s.err
}

// FindByOrganization is unused by the writer; present only to satisfy the
// audit.Repository interface.
func (s *spyRepo) FindByOrganization(_ context.Context, _ string, _ audit.AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *spyRepo) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.calls)
}

func newTestEvent() *mmodel.ProtectionAuditEvent {
	event, err := mmodel.NewProtectionAuditEvent(mmodel.ProtectionAuditEventInput{
		OrganizationID: "org-1",
		EventType:      mmodel.AuditEventTypeProvisioning,
		Action:         mmodel.AuditActionProvision,
		Outcome:        mmodel.AuditOutcomeSuccess,
	})
	if err != nil {
		panic(err)
	}

	return event
}

// testLogger is a minimal libLog.Logger that discards output. The writer must
// never block or fail on logging.
func testLogger() libLog.Logger {
	return &libLog.GoLogger{Level: libLog.LevelDebug}
}

func TestAuditWriter_Emit_Success(t *testing.T) {
	t.Parallel()

	repo := newSpyRepo()
	w := NewAuditWriter(repo, testLogger())

	w.Emit(context.Background(), newTestEvent())

	assert.Equal(t, 1, repo.callCount())
}

func TestAuditWriter_Emit_RepoError_Swallowed(t *testing.T) {
	t.Parallel()

	repo := newSpyRepo()
	repo.err = errors.New("mongo down")
	w := NewAuditWriter(repo, testLogger())

	// Must not panic and must not propagate; method returns nothing.
	w.Emit(context.Background(), newTestEvent())

	assert.Equal(t, 1, repo.callCount())
}

func TestAuditWriter_Emit_NilEvent_NoCreate(t *testing.T) {
	t.Parallel()

	repo := newSpyRepo()
	w := NewAuditWriter(repo, testLogger())

	w.Emit(context.Background(), nil)

	assert.Equal(t, 0, repo.callCount(), "nil event must not reach the repository")
}

func TestAuditWriter_EmitAsync_Success(t *testing.T) {
	t.Parallel()

	repo := newSpyRepo()
	w := NewAuditWriter(repo, testLogger())

	w.EmitAsync(context.Background(), newTestEvent())

	<-repo.called // deterministic wait, no time.Sleep
	assert.Equal(t, 1, repo.callCount())
}

func TestAuditWriter_EmitAsync_ParentCancelled_StillWrites(t *testing.T) {
	t.Parallel()

	repo := newSpyRepo()
	w := NewAuditWriter(repo, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // parent already cancelled before the async write

	w.EmitAsync(ctx, newTestEvent())

	<-repo.called
	assert.Equal(t, 1, repo.callCount(), "WithoutCancel must detach the write from parent cancellation")
}

func TestAuditWriter_EmitAsync_RepoPanic_Recovered(t *testing.T) {
	t.Parallel()

	repo := newSpyRepo()
	repo.panicMsg = "boom"
	w := NewAuditWriter(repo, testLogger())

	// The goroutine panics inside Create; SafeGo's RecoverWithPolicy
	// (KeepRunning) must swallow the panic and keep the process alive. We
	// synchronize on the call signal; reaching the assertions below — plus a
	// clean goleak verification in TestMain — proves the goroutine recovered
	// and completed rather than crashing the process.
	w.EmitAsync(context.Background(), newTestEvent())

	<-repo.called
	assert.Equal(t, 1, repo.callCount())
}

func TestSafeAuditLogFields_NilEvent(t *testing.T) {
	t.Parallel()

	assert.Nil(t, safeAuditLogFields(nil))
}

func TestSafeAuditLogFields_PopulatedEvent(t *testing.T) {
	t.Parallel()

	fields := safeAuditLogFields(newTestEvent())

	assert.NotEmpty(t, fields)

	// The adapted fields must carry the safe, non-sensitive event descriptors
	// from SafeLogFields, not merely be non-empty.
	keys := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		keys[f.Key] = struct{}{}
	}

	assert.Contains(t, keys, "event_type")
	assert.Contains(t, keys, "outcome")
}

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

func sampleAuditEvents() []*mmodel.ProtectionAuditEvent {
	return []*mmodel.ProtectionAuditEvent{
		{ID: uuid.New(), OrganizationID: "org-1", EventType: mmodel.AuditEventTypeProvisioning},
		{ID: uuid.New(), OrganizationID: "org-1", EventType: mmodel.AuditEventTypeProvisioning},
	}
}

func TestAuditQueryService_GetAuditEvents_ReturnsRepositoryOutputVerbatim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

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

	ctx := context.Background()

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

	ctx := context.Background()

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

	ctx := context.Background()

	sentinel := fmt.Errorf("mongo unavailable")
	repo := &spyQueryRepo{err: sentinel}

	svc := NewAuditQueryService(repo)

	_, _, err := svc.GetAuditEvents(ctx, "org-1", audit.AuditQuery{})

	require.Error(t, err)
	assert.Same(t, sentinel, err, "repository error must be returned unchanged (handler maps to HTTP status)")
}

// Compile-time assertions that the spy satisfies the read contract and that the
// implementation satisfies the exported AuditQueryService interface.
var (
	_ audit.Repository  = (*spyQueryRepo)(nil)
	_ AuditQueryService = (*auditQueryService)(nil)
)
