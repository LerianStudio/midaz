// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libStreaming "github.com/LerianStudio/lib-streaming"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// limitFixture builds a limit in the given pre-transition status with fixed
// times, usable as the repository GetByID return for the status-transition and
// delete streaming tests. Times come from the fixed test clock, never
// time.Now().
func limitFixture(id, scopeSeed int64, status model.LimitStatus) *model.Limit {
	return &model.Limit{
		ID:        testutil.MustDeterministicUUID(id),
		Name:      "Streaming Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Status:    status,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(scopeSeed))},
		},
		CreatedAt: testutil.FixedTime(),
		UpdatedAt: testutil.FixedTime(),
	}
}

// assertLimitFenceClean fails if any forbidden free-text / financial key appears
// at the top level of the wire payload.
func assertLimitFenceClean(t *testing.T, raw []byte) {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	for _, forbidden := range []string{"name", "description", "maxAmount"} {
		_, present := m[forbidden]
		assert.Falsef(t, present, "forbidden key %q must never appear on the wire", forbidden)
	}
}

// ── limit.created ─────────────────────────────────────────────────────────────

func TestCreateLimit_EmitsLimitCreated(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	expectLimitCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventLimitCreated, model.AuditActionCreate, "Limit created via API")

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	input := &CreateLimitInput{
		Name:        "Daily Card Limit",
		Description: testutil.StringPtr("Daily spending limit"),
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes:      []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
	}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "limit.created", emitted[0].DefinitionKey)
	assert.Equal(t, result.ID.String(), emitted[0].Subject)

	var payload events.LimitCreatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, result.ID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, "DAILY", payload.LimitType)
	assert.Equal(t, "USD", payload.Currency)
	require.Len(t, payload.Scopes, 1)

	assertLimitFenceClean(t, emitted[0].Payload)
}

func TestCreateLimit_NilEmitter_NoEmit_NoPanic(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	expectLimitCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventLimitCreated, model.AuditActionCreate, "Limit created via API")

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	// Streaming left nil.

	input := &CreateLimitInput{
		Name: "Daily Card Limit", LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"), Currency: "USD",
		Scopes: []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
	}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCreateLimit_NoopEmitter_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	expectLimitCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventLimitCreated, model.AuditActionCreate, "Limit created via API")

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = libStreaming.NewNoopEmitter()

	input := &CreateLimitInput{
		Name: "Daily Card Limit", LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"), Currency: "USD",
		Scopes: []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
	}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCreateLimit_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	expectLimitCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventLimitCreated, model.AuditActionCreate, "Limit created via API")

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	input := &CreateLimitInput{
		Name: "Daily Card Limit", LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"), Currency: "USD",
		Scopes: []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
	}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── limit.updated ─────────────────────────────────────────────────────────────

func TestUpdateLimit_EmitsLimitUpdated(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(10)
	existing := limitFixture(10, 11, model.LimitStatusDraft)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(), gomock.AssignableToTypeOf(mockTx),
			model.AuditEventLimitUpdated, model.AuditActionUpdate, limitID,
			gomock.Any(), gomock.Any(), "Limit updated via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, err := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID, &UpdateLimitInput{Name: testutil.StringPtr("Updated Name")})
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "limit.updated", emitted[0].DefinitionKey)
	assert.Equal(t, limitID.String(), emitted[0].Subject)

	var payload events.LimitUpdatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, limitID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, testutil.FixedTime().Format("2006-01-02T15:04:05Z07:00"), payload.UpdatedAt)
	assertLimitFenceClean(t, emitted[0].Payload)
}

func TestUpdateLimit_NoChange_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(10)
	existing := limitFixture(10, 11, model.LimitStatusDraft)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)
	// No fields to change → no tx, no emit.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	cmd, err := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID, &UpdateLimitInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "no-change update must not emit")
}

func TestUpdateLimit_NilEmitter_NoPanic(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(10)
	existing := limitFixture(10, 11, model.LimitStatusDraft)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(), gomock.AssignableToTypeOf(mockTx),
			model.AuditEventLimitUpdated, model.AuditActionUpdate, limitID,
			gomock.Any(), gomock.Any(), "Limit updated via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, err := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(context.Background(), limitID, &UpdateLimitInput{Name: testutil.StringPtr("Updated Name")})
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestUpdateLimit_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(10)
	existing := limitFixture(10, 11, model.LimitStatusDraft)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(), gomock.AssignableToTypeOf(mockTx),
			model.AuditEventLimitUpdated, model.AuditActionUpdate, limitID,
			gomock.Any(), gomock.Any(), "Limit updated via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, err := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID, &UpdateLimitInput{Name: testutil.StringPtr("Updated Name")})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── limit.activated ───────────────────────────────────────────────────────────

func TestActivateLimit_EmitsLimitActivated(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(20)
	inputLimit := limitFixture(20, 21, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusActive, model.AuditEventLimitActivated, model.AuditActionActivate,
		"Limit activated via API", gomock.Any())

	cmd, err := NewActivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "limit.activated", emitted[0].DefinitionKey)
	assert.Equal(t, limitID.String(), emitted[0].Subject)

	var payload events.LimitActivatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, limitID.String(), payload.ID)
	assert.Equal(t, "ACTIVE", payload.Status)
	assert.NotEmpty(t, payload.UpdatedAt)
}

func TestActivateLimit_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(20)
	inputLimit := limitFixture(20, 21, model.LimitStatusActive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	cmd, err := NewActivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "idempotent no-op must not emit")
}

func TestActivateLimit_NilEmitter_NoPanic(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(20)
	inputLimit := limitFixture(20, 21, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusActive, model.AuditEventLimitActivated, model.AuditActionActivate,
		"Limit activated via API", gomock.Any())

	cmd, err := NewActivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestActivateLimit_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(20)
	inputLimit := limitFixture(20, 21, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusActive, model.AuditEventLimitActivated, model.AuditActionActivate,
		"Limit activated via API", gomock.Any())

	cmd, err := NewActivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── limit.deactivated ─────────────────────────────────────────────────────────

func TestDeactivateLimit_EmitsLimitDeactivated(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(30)
	inputLimit := limitFixture(30, 31, model.LimitStatusActive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusInactive, model.AuditEventLimitDeactivated, model.AuditActionDeactivate,
		"Limit deactivated via API", gomock.Any())

	cmd, err := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "limit.deactivated", emitted[0].DefinitionKey)
	assert.Equal(t, limitID.String(), emitted[0].Subject)

	var payload events.LimitDeactivatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, "INACTIVE", payload.Status)
	assert.NotEmpty(t, payload.UpdatedAt)
}

func TestDeactivateLimit_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(30)
	inputLimit := limitFixture(30, 31, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	cmd, err := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events())
}

func TestDeactivateLimit_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(30)
	inputLimit := limitFixture(30, 31, model.LimitStatusActive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusInactive, model.AuditEventLimitDeactivated, model.AuditActionDeactivate,
		"Limit deactivated via API", gomock.Any())

	cmd, err := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── limit.drafted ─────────────────────────────────────────────────────────────

func TestDraftLimit_EmitsLimitDrafted(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(40)
	inputLimit := limitFixture(40, 41, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusDraft, model.AuditEventLimitDrafted, model.AuditActionDraft,
		"Limit transitioned to draft via API", gomock.Any())

	cmd, err := NewDraftLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "limit.drafted", emitted[0].DefinitionKey)
	assert.Equal(t, limitID.String(), emitted[0].Subject)

	var payload events.LimitDraftedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, "DRAFT", payload.Status)
	assert.NotEmpty(t, payload.UpdatedAt)
}

func TestDraftLimit_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(40)
	inputLimit := limitFixture(40, 41, model.LimitStatusDraft)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	cmd, err := NewDraftLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events())
}

func TestDraftLimit_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(40)
	inputLimit := limitFixture(40, 41, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusDraft, model.AuditEventLimitDrafted, model.AuditActionDraft,
		"Limit transitioned to draft via API", gomock.Any())

	cmd, err := NewDraftLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── limit.deleted ─────────────────────────────────────────────────────────────

func TestDeleteLimit_EmitsLimitDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(50)
	inputLimit := limitFixture(50, 51, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusDeleted, model.AuditEventLimitDeleted, model.AuditActionDelete,
		"Limit deleted via API", gomock.Nil())

	cmd, err := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	err = cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "limit.deleted", emitted[0].DefinitionKey)
	assert.Equal(t, limitID.String(), emitted[0].Subject)

	var payload events.LimitDeletedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, limitID.String(), payload.ID)
	// Fixed clock: SetStatus(DELETED) stamps DeletedAt at the clock time.
	assert.Equal(t, testutil.FixedTime().Format("2006-01-02T15:04:05Z07:00"), payload.DeletedAt)
}

func TestDeleteLimit_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(50)
	inputLimit := limitFixture(50, 51, model.LimitStatusDeleted)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	cmd, err := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	err = cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	assert.Empty(t, emitter.Events())
}

func TestDeleteLimit_NilEmitter_NoPanic(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(50)
	inputLimit := limitFixture(50, 51, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusDeleted, model.AuditEventLimitDeleted, model.AuditActionDelete,
		"Limit deleted via API", gomock.Nil())

	cmd, err := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	err = cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
}

func TestDeleteLimit_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(50)
	inputLimit := limitFixture(50, 51, model.LimitStatusInactive)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(inputLimit, nil)
	expectLimitStatusTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		limitID, model.LimitStatusDeleted, model.AuditEventLimitDeleted, model.AuditActionDelete,
		"Limit deleted via API", gomock.Nil())

	cmd, err := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	err = cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}
