// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ============================================================================
// RULE AUDIT EVENTS
// ============================================================================

// TestAuditEventRecording_CreateRule validates CREATE audit event.
// The rule insert and the audit event are persisted atomically via executeInTx,
// so the test wires MockTxBeginner/MockTx and asserts the WithTx method family.
func TestAuditEventRecording_CreateRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().Compile(gomock.Any(), gomock.Any()).Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, r *model.Rule) (*model.Rule, error) {
				return r, nil
			}),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventRuleCreated,
			model.AuditActionCreate,
			gomock.Any(),
			gomock.Nil(),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	_, err = cmd.Execute(context.Background(), &CreateRuleInput{
		Name: "Test", Expression: "true", Action: model.DecisionAllow,
	})

	require.NoError(t, err)
}

// TestAuditEventRecording_ActivateRule validates ACTIVATE audit event.
// The rule update and audit event are persisted atomically via executeInTx,
// so the test wires MockTxBeginner/MockTx and asserts the WithTx method family.
func TestAuditEventRecording_ActivateRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	ruleID := testutil.MustDeterministicUUID(10)
	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(&model.Rule{
		ID: ruleID, Expression: "true", Status: model.RuleStatusDraft,
	}, nil)
	mockCEL.EXPECT().Compile(gomock.Any(), gomock.Any()).Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventRuleActivated,
			model.AuditActionActivate,
			ruleID,
			gomock.Not(gomock.Nil()),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewActivateRuleService(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	_, err = service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
}

// TestAuditEventRecording_DeactivateRule validates DEACTIVATE audit event.
// The rule update and audit event are persisted atomically via executeInTx.
func TestAuditEventRecording_DeactivateRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	ruleID := testutil.MustDeterministicUUID(20)
	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(&model.Rule{
		ID: ruleID, Status: model.RuleStatusActive,
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventRuleDeactivated,
			model.AuditActionDeactivate,
			ruleID,
			gomock.Not(gomock.Nil()),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)
	_, err = service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
}

// TestAuditEventRecording_UpdateRule validates UPDATE audit event.
// The rule update and the audit event are persisted atomically via executeInTx,
// so the test wires MockTxBeginner/MockTx and asserts the WithTx method family.
func TestAuditEventRecording_UpdateRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	ruleID := testutil.MustDeterministicUUID(30)
	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(&model.Rule{
		ID: ruleID, Name: "Old", Expression: "true",
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventRuleUpdated,
			model.AuditActionUpdate,
			ruleID,
			gomock.Not(gomock.Nil()),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	_, err = cmd.Execute(context.Background(), ruleID, &UpdateRuleInput{
		Name: testutil.StringPtr("New"),
	})
	require.NoError(t, err)
}

// TestAuditEventRecording_DeleteRule validates DELETE audit event.
// The soft-delete and audit event are persisted atomically via executeInTx.
func TestAuditEventRecording_DeleteRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	ruleID := testutil.MustDeterministicUUID(40)
	mockRepo.EXPECT().GetByID(gomock.Any(), ruleID).Return(&model.Rule{
		ID: ruleID, Status: model.RuleStatusInactive,
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().DeleteWithTx(gomock.Any(), mockTx, ruleID).Return(nil),
		auditWriter.EXPECT().RecordRuleEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventRuleDeleted,
			model.AuditActionDelete,
			ruleID,
			gomock.Not(gomock.Nil()),
			gomock.Nil(),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, txBeginner)
	require.NoError(t, err)
	err = service.Execute(context.Background(), ruleID)
	require.NoError(t, err)
}

// ============================================================================
// LIMIT AUDIT EVENTS
// ============================================================================

// TestAuditEventRecording_CreateLimit validates limit CREATE audit.
// The limit insert and the audit event are persisted atomically via executeInTx,
// so the test wires MockTxBeginner/MockTx and asserts the WithTx method family.
func TestAuditEventRecording_CreateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.Limit) error {
				return nil
			}),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventLimitCreated,
			model.AuditActionCreate,
			gomock.Any(),
			gomock.Nil(),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)
	_, err = cmd.Execute(context.Background(), &CreateLimitInput{
		Name: "Test", LimitType: model.LimitTypeDaily, MaxAmount: decimal.RequireFromString("1000"),
		Currency: "BRL", Scopes: []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(50))}},
	})
	require.NoError(t, err)
}

// TestAuditEventRecording_ActivateLimit validates limit ACTIVATE audit.
// The status update and audit event are persisted atomically via executeInTx,
// so the test wires MockTxBeginner/MockTx and asserts the WithTx method family.
func TestAuditEventRecording_ActivateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	limitID := testutil.MustDeterministicUUID(60)
	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(&model.Limit{
		ID: limitID, Status: model.LimitStatusInactive,
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateStatusWithTx(gomock.Any(), mockTx, limitID, model.LimitStatusActive, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventLimitActivated,
			model.AuditActionActivate,
			limitID,
			gomock.Not(gomock.Nil()),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, cmdErr := NewActivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)
	_, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
}

// TestAuditEventRecording_DeactivateLimit validates limit DEACTIVATE audit.
// The status update and audit event are persisted atomically via executeInTx.
func TestAuditEventRecording_DeactivateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	limitID := testutil.MustDeterministicUUID(70)
	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(&model.Limit{
		ID: limitID, Status: model.LimitStatusActive,
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateStatusWithTx(gomock.Any(), mockTx, limitID, model.LimitStatusInactive, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventLimitDeactivated,
			model.AuditActionDeactivate,
			limitID,
			gomock.Not(gomock.Nil()),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)
	_, err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
}

// TestAuditEventRecording_UpdateLimit validates limit UPDATE audit.
// The full update and audit event are persisted atomically via executeInTx.
func TestAuditEventRecording_UpdateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	limitID := testutil.MustDeterministicUUID(80)
	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(&model.Limit{
		ID: limitID, MaxAmount: decimal.RequireFromString("500"),
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventLimitUpdated,
			model.AuditActionUpdate,
			limitID,
			gomock.Not(gomock.Nil()),
			gomock.Not(gomock.Nil()),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)
	_, err := cmd.Execute(context.Background(), limitID, &UpdateLimitInput{
		MaxAmount: testutil.Ptr(decimal.RequireFromString("1000")),
	})
	require.NoError(t, err)
}

// TestAuditEventRecording_DeleteLimit validates limit DELETE audit.
// The status update and audit event are persisted atomically via executeInTx.
func TestAuditEventRecording_DeleteLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	limitID := testutil.MustDeterministicUUID(90)
	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(&model.Limit{
		ID: limitID, Status: model.LimitStatusInactive,
	}, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateStatusWithTx(gomock.Any(), mockTx, limitID, model.LimitStatusDeleted, gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			mockTx,
			model.AuditEventLimitDeleted,
			model.AuditActionDelete,
			limitID,
			gomock.Not(gomock.Nil()),
			gomock.Nil(),
			gomock.Any(),
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)
	err := cmd.Execute(context.Background(), limitID)
	require.NoError(t, err)
}

// ============================================================================
// VALIDATION AUDIT EVENTS
// ============================================================================

// TestAuditEventRecording_ValidationEvent validates validation event recording.
// Note: This test validates the RecordValidationEvent interface contract.
// End-to-end testing is done in integration tests (tests/integration/11_audit_events_test.go).
func TestAuditEventRecording_ValidationEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	auditWriter := NewMockAuditWriter(ctrl)

	validationID := testutil.MustDeterministicUUID(100)
	accountID := testutil.MustDeterministicUUID(101)

	// Mock request data
	request := map[string]any{
		"requestId":       testutil.MustDeterministicUUID(102).String(),
		"transactionType": "PIX",
		"amount":          decimal.RequireFromString("100"),
		"currency":        "BRL",
		"timestamp":       testutil.FixedTime(),
		"account": map[string]any{
			"id":       accountID.String(),
			"type":     "CHECKING",
			"status":   "ACTIVE",
			"metadata": map[string]any{},
		},
		"metadata": map[string]any{},
	}

	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{},
		[]uuid.UUID{},
		"All rules passed",
	)
	require.NoError(t, err)

	responseContext := model.ValidationResponseContext{
		ProcessingTimeMs:  50,
		LimitUsageDetails: []model.LimitUsageDetail{},
	}

	// VALIDATE: capture the request snapshot and evaluation result that flow into
	// the audit writer. Client IP attribution is no longer asserted here — it is
	// resolved from ctx by resolveActor (see record_audit_event_test.go for the
	// Principal/IP capture assertions).
	var capturedRequest map[string]any
	var capturedEvalResult model.EvaluationResult

	auditWriter.EXPECT().RecordValidationEvent(
		gomock.Any(),
		validationID,
		gomock.Not(gomock.Nil()),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, valID uuid.UUID, req map[string]any,
		evalRes model.EvaluationResult, respCtx model.ValidationResponseContext) error {
		capturedRequest = req
		capturedEvalResult = evalRes
		return nil
	}).Times(1)

	// Execute
	err = auditWriter.RecordValidationEvent(
		context.Background(),
		validationID,
		request,
		*evalResult,
		responseContext,
	)

	require.NoError(t, err)

	// VALIDATE CAPTURED DATA
	assert.NotNil(t, capturedRequest, "request snapshot must be captured")
	assert.Equal(t, request["requestId"], capturedRequest["requestId"], "requestId must be in snapshot")
	assert.Equal(t, "PIX", capturedRequest["transactionType"], "transaction type must be in snapshot")
	assert.Equal(t, decimal.RequireFromString("100"), capturedRequest["amount"], "amount must be in snapshot")
	assert.Equal(t, "BRL", capturedRequest["currency"], "currency must be in snapshot")
	assert.NotNil(t, capturedRequest["account"], "account must be in snapshot")

	assert.Equal(t, model.DecisionAllow, capturedEvalResult.Decision, "decision must match")
	assert.Equal(t, "All rules passed", capturedEvalResult.Reason, "reason must match")
}
