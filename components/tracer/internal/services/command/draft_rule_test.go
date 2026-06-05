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
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestNewDraftRuleService_NilRepository(t *testing.T) {
	svc, err := NewDraftRuleService(nil, testutil.NewDefaultMockClock(), nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilRuleRepository)
	assert.Nil(t, svc)
}

func TestNewDraftRuleService_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	svc, err := NewDraftRuleService(mockRepo, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilClock)
	assert.Nil(t, svc)
}

func TestDraftRule_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
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
		Return(inputRule, nil)

	// Transactional chain: BeginTx -> UpdateWithTx -> RecordRuleEventWithTx -> Commit.
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, rule *model.Rule) error {
				assert.Equal(t, ruleID, rule.ID)
				assert.Equal(t, model.RuleStatusDraft, rule.Status)
				assert.Nil(t, rule.ActivatedAt, "activatedAt should be nil")
				assert.Nil(t, rule.DeactivatedAt, "deactivatedAt should be nil after draft")
				return nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDrafted,
				model.AuditActionDraft,
				ruleID,
				gomock.AssignableToTypeOf(map[string]any{}),
				gomock.AssignableToTypeOf(map[string]any{}),
				"Rule transitioned to draft via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, model.RuleStatusDraft, result.Status)
	assert.False(t, result.UpdatedAt.IsZero(), "UpdatedAt should be set")
	assert.Nil(t, result.ActivatedAt, "ActivatedAt should be nil after draft")
	assert.Nil(t, result.DeactivatedAt, "DeactivatedAt should be nil after draft")
}

// TestDraftRule_Success_NilAuditWriter verifies the auditWriter == nil
// short-circuit branch inside the transactional callback.
func TestDraftRule_Success_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(110)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	expectRuleUpdateTxSuccessNoAudit(t, txBeginner, mockTx, mockRepo)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), nil, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.RuleStatusDraft, result.Status)
}

func TestDraftRule_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name       string
		fromStatus model.RuleStatus
	}{
		{"from ACTIVE", model.RuleStatusActive},
		{"from DELETED", model.RuleStatusDeleted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			ruleID := testutil.MustDeterministicUUID(1)

			mockRepo := NewMockRuleRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			mockRepo.EXPECT().
				GetByID(gomock.Any(), ruleID).
				Return(&model.Rule{
					ID:         ruleID,
					Name:       "Test Rule",
					Status:     tc.fromStatus,
					Expression: "amount > 1000",
				}, nil)

			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			auditWriter.EXPECT().
				RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, err)

			_, err = service.Execute(context.Background(), ruleID)

			require.Error(t, err)
			var transitionErr *model.InvalidTransitionError
			require.True(t, errors.As(err, &transitionErr), "should be an InvalidTransitionError")
			assert.Equal(t, tc.fromStatus, transitionErr.From)
			assert.Equal(t, model.RuleStatusDraft, transitionErr.To)
		})
	}
}

func TestDraftRule_RuleNotFound(t *testing.T) {
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

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestDraftRule_NilRuleFromRepo exercises the defensive `if rule == nil` guard
// added after GetByID. Returning (nil, nil) from the repository is a contract
// violation, but the guard treats it as ErrRuleNotFound rather than panicking
// on the subsequent rule.<field> dereference.
func TestDraftRule_NilRuleFromRepo(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(202)

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

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.Error(t, err)
	require.Nil(t, result, "rule pointer must be nil when repo returns (nil, nil)")
	assert.ErrorIs(t, err, constant.ErrRuleNotFound, "guard must surface ErrRuleNotFound, wrapped via ValidateBusinessError")
}

func TestDraftRule_AlreadyDraft_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, model.RuleStatusDraft, result.Status, "Status should remain DRAFT")
}

func TestDraftRule_GetByIDError(t *testing.T) {
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

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestDraftRule_UpdateError verifies that when UpdateWithTx fails inside the
// transactional callback, the tx is rolled back, Commit is never called and
// audit writer is never invoked.
func TestDraftRule_UpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusInactive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	// Note: inputRule in memory IS mutated by SetStatus() before persistence fails.
	assert.Equal(t, model.RuleStatusDraft, inputRule.Status, "Status is mutated in memory by SetStatus()")
}

// TestDraftRule_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer
// transactional methods.
func TestDraftRule_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(2)

	inputRule := &model.Rule{
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
		Return(inputRule, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
}

// TestDraftRule_AuditError_Rollback verifies that when the audit writer call
// inside the transaction fails, the transaction is rolled back, Commit is
// never invoked, and the command returns the audit error.
func TestDraftRule_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(3)

	inputRule := &model.Rule{
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
		Return(inputRule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDrafted,
				model.AuditActionDraft,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule transitioned to draft via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
}

// TestDraftRule_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error and a defer-driven rollback fires.
func TestDraftRule_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(4)

	inputRule := &model.Rule{
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
		Return(inputRule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDrafted,
				model.AuditActionDraft,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule transitioned to draft via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	service, err := NewDraftRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
}
