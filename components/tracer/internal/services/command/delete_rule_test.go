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

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestNewDeleteRuleService_NilRepository(t *testing.T) {
	service, err := NewDeleteRuleService(nil, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilDeleteRuleRepository)
	assert.Nil(t, service)
}

func TestNewDeleteRuleService_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)

	service, err := NewDeleteRuleService(mockRepo, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilAuditWriter)
	assert.Nil(t, service)
}

func TestDeleteRule_Success_FromInactive(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)

	// Transactional chain: BeginTx -> DeleteWithTx -> RecordRuleEventWithTx -> Commit.
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDeleted,
				model.AuditActionDelete,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule deleted via API",
			).
			DoAndReturn(func(_ any, _ any, _ any, _ any, _ any, beforeState map[string]any, afterState map[string]any, _ any) error {
				// Verify beforeState contains INACTIVE status
				assert.Equal(t, model.RuleStatusInactive, beforeState["status"], "beforeState should have INACTIVE status")
				assert.Equal(t, rule.Name, beforeState["name"], "beforeState should have rule name")
				// Verify afterState is nil for delete
				assert.Nil(t, afterState, "afterState should be nil for delete")
				return nil
			}),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.NoError(t, err)
}

func TestDeleteRule_Success_FromDraft(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Draft Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 500",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDeleted,
				model.AuditActionDelete,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule deleted via API",
			).
			DoAndReturn(func(_ any, _ any, _ any, _ any, _ any, beforeState map[string]any, afterState map[string]any, _ any) error {
				// Verify beforeState contains DRAFT status
				assert.Equal(t, model.RuleStatusDraft, beforeState["status"], "beforeState should have DRAFT status")
				assert.Equal(t, rule.Name, beforeState["name"], "beforeState should have rule name")
				// Verify afterState is nil for delete
				assert.Nil(t, afterState, "afterState should be nil for delete")
				return nil
			}),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.NoError(t, err)
}

func TestDeleteRule_RuleNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestDeleteRule_NilRuleFromRepo exercises the defensive `if rule == nil` guard
// added after GetByID. Returning (nil, nil) from the repository is a contract
// violation, but the guard treats it as ErrRuleNotFound rather than panicking
// on the subsequent rule.<field> dereference.
func TestDeleteRule_NilRuleFromRepo(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(203)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	// Contract violation: repo returns (nil, nil). Only the `if rule == nil`
	// guard catches this — without it, the subsequent rule.Status access
	// would panic.
	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, nil)

	// No downstream side effects must fire: no tx, no audit write.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assertBusinessCode(t, err, constant.ErrRuleNotFound.Error())
}

func TestDeleteRule_AlreadyDeleted_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDeleted,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)

	// Idempotent path: no tx, no audit.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.NoError(t, err)
}

func TestDeleteRule_InvalidTransition(t *testing.T) {
	// Only ACTIVE → DELETED is invalid (ACTIVE must go to INACTIVE first)
	tests := []struct {
		name       string
		fromStatus model.RuleStatus
	}{
		{"FromActive", model.RuleStatusActive},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			ctx := context.Background()
			ruleID := testutil.MustDeterministicUUID(1)

			rule := &model.Rule{
				ID:         ruleID,
				Name:       "Test Rule",
				Status:     tc.fromStatus,
				Expression: "amount > 1000",
			}

			// Record original state before Execute
			originalStatus := rule.Status
			originalName := rule.Name
			originalExpression := rule.Expression

			mockRepo := NewMockRuleRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			mockRepo.EXPECT().
				GetByID(gomock.Any(), ruleID).
				Return(rule, nil)

			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			auditWriter.EXPECT().
				RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
			require.NoError(t, err)

			err = service.Execute(ctx, ruleID)

			require.Error(t, err)
			var transitionErr *model.InvalidTransitionError
			assert.True(t, errors.As(err, &transitionErr), "should be an InvalidTransitionError")

			// Verify rule was not mutated on error
			assert.Equal(t, originalStatus, rule.Status, "rule status should not be mutated on error")
			assert.Equal(t, originalName, rule.Name, "rule name should not be mutated on error")
			assert.Equal(t, originalExpression, rule.Expression, "rule expression should not be mutated on error")
		})
	}
}

func TestDeleteRule_GetByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, errors.New("database error"))

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestDeleteRule_DeleteError verifies that when DeleteWithTx fails inside the
// transactional callback, the tx is rolled back, Commit is never called, and
// the audit writer is never invoked.
func TestDeleteRule_DeleteError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	originalStatus := rule.Status
	originalName := rule.Name
	originalExpression := rule.Expression

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")

	// Verify rule was not mutated on error (delete command never calls SetStatus)
	assert.Equal(t, originalStatus, rule.Status, "rule status should not be mutated on error")
	assert.Equal(t, originalName, rule.Name, "rule name should not be mutated on error")
	assert.Equal(t, originalExpression, rule.Expression, "rule expression should not be mutated on error")
}

// TestDeleteRule_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer
// transactional methods.
func TestDeleteRule_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(2)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		DeleteWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
}

// TestDeleteRule_AuditError_Rollback verifies that when the audit writer call
// inside the transaction fails, the transaction is rolled back, Commit is
// never invoked, and the command returns the audit error.
func TestDeleteRule_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(3)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit insert failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDeleted,
				model.AuditActionDelete,
				ruleID,
				gomock.Any(),
				gomock.Nil(),
				"Rule deleted via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
}

// TestDeleteRule_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error and a defer-driven rollback fires.
func TestDeleteRule_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(4)

	rule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(rule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			DeleteWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), ruleID).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDeleted,
				model.AuditActionDelete,
				ruleID,
				gomock.Any(),
				gomock.Nil(),
				"Rule deleted via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	service, err := NewDeleteRuleService(mockRepo, auditWriter, testutil.NewDefaultMockClock(), txBeginner)
	require.NoError(t, err)

	err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
}
