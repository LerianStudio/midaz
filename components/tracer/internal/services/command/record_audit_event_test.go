// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	commandMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ============================================================================
// resolveActor — the new identity-resolution surface introduced by the Taura
// audit fix. These tests pin the contract that previously did not exist: every
// audit row must reflect the authenticated Principal (when one is present in
// ctx) and fall back to a granular system actor only for background callers.
// ============================================================================

// withCtx builds a context.Context carrying the given Principal (optional) and
// client IP (optional). Pass empty values to omit either.
func withCtx(p *contextutil.Principal, clientIP string) context.Context {
	ctx := context.Background()
	if p != nil {
		ctx = contextutil.WithPrincipal(ctx, *p)
	}

	if clientIP != "" {
		ctx = context.WithValue(ctx, contextutil.ContextKeyClientIP{}, clientIP)
	}

	return ctx
}

// captureActor uses DoAndReturn to grab the Actor that the command stamped onto
// the AuditEvent during InsertWithTx, returning a closure the caller can invoke
// to read it after the command runs.
func captureActor(t *testing.T, mockRepo *commandMocks.MockAuditEventRepository, mockDB *pgdbMocks.MockDB) func() model.Actor {
	t.Helper()

	var captured model.Actor

	mockRepo.EXPECT().InsertWithTx(
		gomock.Any(),
		mockDB,
		gomock.AssignableToTypeOf(&model.AuditEvent{}),
	).DoAndReturn(func(_ context.Context, _ any, event *model.AuditEvent) error {
		captured = event.Actor
		return nil
	}).Times(1)

	return func() model.Actor { return captured }
}

func TestResolveActor_NoPrincipalNoIP_FallsBackToSystem(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	getActor := captureActor(t, mockRepo, mockDB)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordRuleEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventRuleCreated,
		model.AuditActionCreate,
		testutil.MustDeterministicUUID(300),
		nil,
		map[string]any{"name": "r"},
		"",
	)
	require.NoError(t, err)

	a := getActor()
	assert.Equal(t, model.ActorTypeSystem, a.ActorType,
		"empty ctx must fall back to system actor — the Taura finding pattern that motivated this work")
	assert.Equal(t, "svc_tracer", a.ID, "system fallback uses the canonical svc_tracer ID")
	assert.Equal(t, "Tracer Rule Manager", a.Name, "rule events identify as Rule Manager when system-emitted")
	assert.Equal(t, "0.0.0.0", a.IPAddress, "missing IP normalizes to 0.0.0.0")
}

func TestResolveActor_NoPrincipalWithIP_UsesIPInSystemActor(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	getActor := captureActor(t, mockRepo, mockDB)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordLimitEventWithTx(
		withCtx(nil, "203.0.113.45"),
		mockDB,
		model.AuditEventLimitCreated,
		model.AuditActionCreate,
		testutil.MustDeterministicUUID(301),
		nil,
		map[string]any{"maxAmount": "1000"},
		"",
	)
	require.NoError(t, err)

	a := getActor()
	assert.Equal(t, model.ActorTypeSystem, a.ActorType)
	assert.Equal(t, "Tracer Limit Manager", a.Name)
	assert.Equal(t, "203.0.113.45", a.IPAddress, "client IP from ctx must reach the audit row even on the system fallback")
}

func TestResolveActor_UserPrincipal_StampsUserActor(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	getActor := captureActor(t, mockRepo, mockDB)

	p := &contextutil.Principal{Type: "user", ID: "auth0|user-sub-123", Name: "alice"}

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordRuleEventWithTx(
		withCtx(p, "198.51.100.10"),
		mockDB,
		model.AuditEventRuleUpdated,
		model.AuditActionUpdate,
		testutil.MustDeterministicUUID(302),
		map[string]any{"name": "before"},
		map[string]any{"name": "after"},
		"updated via admin UI",
	)
	require.NoError(t, err)

	a := getActor()
	assert.Equal(t, model.ActorTypeUser, a.ActorType, "JWT principal must produce a user actor — closes the Taura attribution gap")
	assert.Equal(t, "auth0|user-sub-123", a.ID, "Principal.ID flows through as Actor.ID")
	assert.Equal(t, "alice", a.Name, "Principal.Name flows through as Actor.Name (preferred_username / email)")
	assert.Equal(t, "198.51.100.10", a.IPAddress)
}

func TestResolveActor_APIKeyPrincipal_StampsAPIKeyActor(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	getActor := captureActor(t, mockRepo, mockDB)

	p := &contextutil.Principal{Type: "api_key", ID: "tracer-prod-eu", Name: ""}

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordLimitEventWithTx(
		withCtx(p, "192.0.2.55"),
		mockDB,
		model.AuditEventLimitUpdated,
		model.AuditActionUpdate,
		testutil.MustDeterministicUUID(303),
		map[string]any{"maxAmount": "500"},
		map[string]any{"maxAmount": "1000"},
		"raised cap via webhook",
	)
	require.NoError(t, err)

	a := getActor()
	assert.Equal(t, model.ActorTypeAPIKey, a.ActorType, "api_key principal must produce an api_key actor — distinct from system fallback")
	assert.Equal(t, "tracer-prod-eu", a.ID, "deployment label flows through as Actor.ID")
	assert.Empty(t, a.Name, "API-key principals carry no human name")
	assert.Equal(t, "192.0.2.55", a.IPAddress)
}

func TestResolveActor_PrincipalWithInvalidType_FallsBackToSystem(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	getActor := captureActor(t, mockRepo, mockDB)

	// A Principal with a Type that does not map to a valid model.ActorType must
	// NOT be coerced — coercing only the type while keeping the ID/Name would
	// silently misattribute (e.g. ActorType=system with id=<user-sub>). The
	// command drops the principal entirely and emits a system fallback.
	p := &contextutil.Principal{Type: "totally-bogus-type", ID: "ignored", Name: "ignored"}

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordRuleEventWithTx(
		withCtx(p, "203.0.113.99"),
		mockDB,
		model.AuditEventRuleActivated,
		model.AuditActionActivate,
		testutil.MustDeterministicUUID(304),
		map[string]any{"status": "INACTIVE"},
		map[string]any{"status": "ACTIVE"},
		"",
	)
	require.NoError(t, err)

	a := getActor()
	assert.Equal(t, model.ActorTypeSystem, a.ActorType,
		"invalid principal type must not coerce — fall back to system actor to avoid misattribution")
	assert.Equal(t, "svc_tracer", a.ID, "must NOT leak the bogus principal id into the audit row")
	assert.Equal(t, "Tracer Rule Manager", a.Name)
}

func TestResolveActor_PrincipalWithEmptyID_FallsBackToSystem(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	getActor := captureActor(t, mockRepo, mockDB)

	// A zero-ID principal is degenerate (no identity to attribute) — fall back
	// to system rather than recording an empty actor_id.
	p := &contextutil.Principal{Type: "user", ID: "", Name: "ghost"}

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordLimitEventWithTx(
		withCtx(p, ""),
		mockDB,
		model.AuditEventLimitDeleted,
		model.AuditActionDelete,
		testutil.MustDeterministicUUID(305),
		map[string]any{"status": "INACTIVE"},
		nil,
		"",
	)
	require.NoError(t, err)

	a := getActor()
	assert.Equal(t, model.ActorTypeSystem, a.ActorType, "empty Principal.ID must drop to system fallback")
	assert.Equal(t, "svc_tracer", a.ID)
}

// ============================================================================
// RecordRuleEventWithTx — preserved smoke tests (event shape + error propagation)
// ============================================================================

func TestRecordRuleEventWithTx_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	ruleID := testutil.MustDeterministicUUID(200)
	before := map[string]any{"name": "old"}
	after := map[string]any{"name": "new"}

	mockRepo.EXPECT().InsertWithTx(
		gomock.Any(),
		mockDB,
		gomock.AssignableToTypeOf(&model.AuditEvent{}),
	).DoAndReturn(func(_ context.Context, _ any, event *model.AuditEvent) error {
		assert.Equal(t, model.AuditEventRuleUpdated, event.EventType)
		assert.Equal(t, model.AuditActionUpdate, event.Action)
		assert.Equal(t, ruleID.String(), event.ResourceID)
		assert.Equal(t, model.ResourceTypeRule, event.ResourceType)
		return nil
	}).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordRuleEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventRuleUpdated,
		model.AuditActionUpdate,
		ruleID,
		before,
		after,
		"updated via admin API",
	)

	require.NoError(t, err)
}

func TestRecordRuleEventWithTx_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	ruleID := testutil.MustDeterministicUUID(201)
	dbErr := errors.New("insert failed")

	mockRepo.EXPECT().InsertWithTx(gomock.Any(), mockDB, gomock.Any()).Return(dbErr).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordRuleEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventRuleCreated,
		model.AuditActionCreate,
		ruleID,
		nil,
		map[string]any{"name": "r"},
		"",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "repository error must propagate")
}

// ============================================================================
// RecordLimitEventWithTx — preserved smoke tests
// ============================================================================

func TestRecordLimitEventWithTx_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	limitID := testutil.MustDeterministicUUID(210)
	before := map[string]any{"maxAmount": "500"}
	after := map[string]any{"maxAmount": "1000"}

	mockRepo.EXPECT().InsertWithTx(
		gomock.Any(),
		mockDB,
		gomock.AssignableToTypeOf(&model.AuditEvent{}),
	).DoAndReturn(func(_ context.Context, _ any, event *model.AuditEvent) error {
		assert.Equal(t, model.AuditEventLimitUpdated, event.EventType)
		assert.Equal(t, model.AuditActionUpdate, event.Action)
		assert.Equal(t, limitID.String(), event.ResourceID)
		assert.Equal(t, model.ResourceTypeLimit, event.ResourceType)
		return nil
	}).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordLimitEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventLimitUpdated,
		model.AuditActionUpdate,
		limitID,
		before,
		after,
		"raised daily cap",
	)

	require.NoError(t, err)
}

func TestRecordLimitEventWithTx_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	limitID := testutil.MustDeterministicUUID(211)
	dbErr := errors.New("tx insert failed")

	mockRepo.EXPECT().InsertWithTx(gomock.Any(), mockDB, gomock.Any()).Return(dbErr).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordLimitEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventLimitDeleted,
		model.AuditActionDelete,
		limitID,
		map[string]any{"status": "INACTIVE"},
		nil,
		"cleanup",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "repository error must propagate")
}
