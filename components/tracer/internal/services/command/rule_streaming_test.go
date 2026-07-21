// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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

// draftRuleFixture builds an INACTIVE rule usable as the pre-transition state
// for activate/deactivate/draft/delete streaming tests. Times come from the
// fixed test clock, never time.Now().
func inactiveRuleFixture(id, seed int64) *model.Rule {
	ruleID := testutil.MustDeterministicUUID(id)
	return &model.Rule{
		ID:         ruleID,
		Name:       "Streaming Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(seed))},
		},
		CreatedAt: testutil.FixedTime(),
		UpdatedAt: testutil.FixedTime(),
	}
}

// ── rule.created ────────────────────────────────────────────────────────────

func TestCreateRule_EmitsRuleCreated(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockCEL.EXPECT().Compile(gomock.Any(), "amount > 1000000").Return(nil, nil)
	expectRuleCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventRuleCreated, model.AuditActionCreate, "Rule created via API")

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	input := &CreateRuleInput{
		Name:        "High Value Transaction Rule",
		Description: "Blocks transactions over $10,000",
		Expression:  "amount > 1000000",
		Action:      model.DecisionDeny,
		Scopes:      []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
	}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "rule.created", emitted[0].DefinitionKey)
	assert.Equal(t, result.ID.String(), emitted[0].Subject)

	var payload events.RuleCreatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, result.ID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, "DENY", payload.Action)
	require.Len(t, payload.Scopes, 1)

	// Fence: free-text / rule-logic keys must never appear on the wire.
	assertRuleFenceClean(t, emitted[0].Payload)
}

func TestCreateRule_NilEmitter_NoEmit_NoPanic(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().Compile(gomock.Any(), "amount > 1000000").Return(nil, nil)
	expectRuleCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventRuleCreated, model.AuditActionCreate, "Rule created via API")

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	// Streaming left nil.

	input := &CreateRuleInput{
		Name:       "Global Rule",
		Expression: "amount > 1000000",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCreateRule_NoopEmitter_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().Compile(gomock.Any(), "amount > 1000000").Return(nil, nil)
	expectRuleCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventRuleCreated, model.AuditActionCreate, "Rule created via API")

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = libStreaming.NewNoopEmitter()

	input := &CreateRuleInput{Name: "Global Rule", Expression: "amount > 1000000", Action: model.DecisionDeny}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCreateRule_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockCEL.EXPECT().Compile(gomock.Any(), "amount > 1000000").Return(nil, nil)
	expectRuleCreateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		model.AuditEventRuleCreated, model.AuditActionCreate, "Rule created via API")

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	input := &CreateRuleInput{Name: "Global Rule", Expression: "amount > 1000000", Action: model.DecisionDeny}

	result, err := cmd.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── rule.updated ────────────────────────────────────────────────────────────

func TestUpdateRule_EmitsRuleUpdated(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(10)
	existing := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  testutil.FixedTime(),
		UpdatedAt:  testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(existing, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleUpdated, model.AuditActionUpdate, "Rule updated via API")

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), ruleID, &UpdateRuleInput{Name: testutil.StringPtr("Updated Name")})
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "rule.updated", emitted[0].DefinitionKey)
	assert.Equal(t, ruleID.String(), emitted[0].Subject)

	var payload events.RuleUpdatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, ruleID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, testutil.FixedTime().Format("2006-01-02T15:04:05Z07:00"), payload.UpdatedAt)
	assertRuleFenceClean(t, emitted[0].Payload)
}

func TestUpdateRule_NilEmitter_NoPanic(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(10)
	existing := &model.Rule{
		ID: ruleID, Name: "existing rule", Expression: "amount > 1000",
		Action: model.DecisionDeny, Status: model.RuleStatusDraft,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(existing, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleUpdated, model.AuditActionUpdate, "Rule updated via API")

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(context.Background(), ruleID, &UpdateRuleInput{Name: testutil.StringPtr("Updated Name")})
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestUpdateRule_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(10)
	existing := &model.Rule{
		ID: ruleID, Name: "existing rule", Expression: "amount > 1000",
		Action: model.DecisionDeny, Status: model.RuleStatusDraft,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(existing, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleUpdated, model.AuditActionUpdate, "Rule updated via API")

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	cmd.Streaming = emitter

	result, err := cmd.Execute(context.Background(), ruleID, &UpdateRuleInput{Name: testutil.StringPtr("Updated Name")})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── rule.activated ──────────────────────────────────────────────────────────

func TestActivateRule_EmitsRuleActivated(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(20)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusDraft,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	mockExprCompiler.EXPECT().Compile(gomock.Any(), inputRule.Expression).Return(struct{}{}, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleActivated, model.AuditActionActivate, "Rule activated via API")

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "rule.activated", emitted[0].DefinitionKey)
	assert.Equal(t, ruleID.String(), emitted[0].Subject)

	var payload events.RuleActivatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, ruleID.String(), payload.ID)
	assert.Equal(t, "ACTIVE", payload.Status)
	require.NotNil(t, payload.ActivatedAt, "activatedAt populated after a real transition")
}

func TestActivateRule_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(20)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusActive,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "idempotent no-op must not emit")
}

func TestActivateRule_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(20)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusDraft,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	mockExprCompiler.EXPECT().Compile(gomock.Any(), inputRule.Expression).Return(struct{}{}, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleActivated, model.AuditActionActivate, "Rule activated via API")

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── rule.deactivated ────────────────────────────────────────────────────────

func TestDeactivateRule_EmitsRuleDeactivated(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(30)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusActive,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleDeactivated, model.AuditActionDeactivate, "Rule deactivated via API")

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "rule.deactivated", emitted[0].DefinitionKey)
	assert.Equal(t, ruleID.String(), emitted[0].Subject)

	var payload events.RuleDeactivatedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, "INACTIVE", payload.Status)
	require.NotNil(t, payload.DeactivatedAt)
}

func TestDeactivateRule_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(30)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusInactive,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events())
}

func TestDeactivateRule_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(30)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusActive,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleDeactivated, model.AuditActionDeactivate, "Rule deactivated via API")

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── rule.drafted ────────────────────────────────────────────────────────────

func TestDraftRule_EmitsRuleDrafted(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(40)
	inputRule := inactiveRuleFixture(40, 41)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleDrafted, model.AuditActionDraft, "Rule transitioned to draft via API")

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "rule.drafted", emitted[0].DefinitionKey)
	assert.Equal(t, ruleID.String(), emitted[0].Subject)

	var payload events.RuleDraftedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, "DRAFT", payload.Status)
}

func TestDraftRule_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(40)
	inputRule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusDraft,
		Expression: "amount > 1000", Action: model.DecisionDeny,
		CreatedAt: testutil.FixedTime(), UpdatedAt: testutil.FixedTime(),
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events())
}

func TestDraftRule_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(40)
	inputRule := inactiveRuleFixture(40, 41)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(inputRule, nil)
	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter,
		ruleID, model.AuditEventRuleDrafted, model.AuditActionDraft, "Rule transitioned to draft via API")

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	result, err := service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// ── rule.deleted ────────────────────────────────────────────────────────────

func TestDeleteRule_EmitsRuleDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(50)
	rule := &model.Rule{
		ID: ruleID, Name: "Test Rule", Status: model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(rule, nil)
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).Return(nil),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(), gomock.AssignableToTypeOf(mockTx),
			model.AuditEventRuleDeleted, model.AuditActionDelete, ruleID,
			gomock.Any(), gomock.Nil(), "Rule deleted via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	err = service.Execute(context.Background(), ruleID)
	require.NoError(t, err)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, "rule.deleted", emitted[0].DefinitionKey)
	assert.Equal(t, ruleID.String(), emitted[0].Subject)

	var payload events.RuleDeletedPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, ruleID.String(), payload.ID)
	assert.Equal(t, testutil.FixedTime().Format("2006-01-02T15:04:05Z07:00"), payload.DeletedAt)
}

func TestDeleteRule_Idempotent_EmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(50)
	rule := &model.Rule{ID: ruleID, Name: "Test Rule", Status: model.RuleStatusDeleted, Expression: "amount > 1000"}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(rule, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	err = service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	assert.Empty(t, emitter.Events())
}

func TestDeleteRule_EmitFailure_RequestStillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(50)
	rule := &model.Rule{ID: ruleID, Name: "Test Rule", Status: model.RuleStatusInactive, Expression: "amount > 1000"}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker down"))

	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(rule, nil)
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).Return(nil),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(), gomock.AssignableToTypeOf(mockTx),
			model.AuditEventRuleDeleted, model.AuditActionDelete, ruleID,
			gomock.Any(), gomock.Nil(), "Rule deleted via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)
	service.Streaming = emitter

	err = service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
	assert.Empty(t, emitter.Events(), "failed emits are not captured")
}

// assertRuleFenceClean fails if any forbidden free-text / rule-logic key
// appears at the top level of the wire payload.
func assertRuleFenceClean(t *testing.T, raw []byte) {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	for _, forbidden := range []string{"name", "description", "expression", "compiledProgram"} {
		_, present := m[forbidden]
		assert.Falsef(t, present, "forbidden key %q must never appear on the wire", forbidden)
	}
}
