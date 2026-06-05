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

func TestNewDeactivateRuleService_NilRepository(t *testing.T) {
	service, err := NewDeactivateRuleService(nil, testutil.NewDefaultMockClock(), nil, nil, nil)

	require.Nil(t, service)
	require.ErrorIs(t, err, ErrDeactivateNilRepository)
}

func TestNewDeactivateRuleService_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)

	service, err := NewDeactivateRuleService(mockRepo, nil, nil, nil, nil)

	require.Nil(t, service)
	require.ErrorIs(t, err, ErrDeactivateNilClock)
}

func TestDeactivateRule_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	// Transactional chain: BeginTx -> UpdateWithTx -> RecordRuleEventWithTx -> Commit -> cacheWriter.RemoveRule.
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, rule *model.Rule) error {
				assert.Equal(t, ruleID, rule.ID)
				assert.Equal(t, model.RuleStatusInactive, rule.Status)
				assert.NotNil(t, rule.DeactivatedAt, "deactivatedAt should be set")
				return nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleDeactivated,
				model.AuditActionDeactivate,
				ruleID,
				gomock.AssignableToTypeOf(map[string]any{}),
				gomock.AssignableToTypeOf(map[string]any{}),
				"Rule deactivated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
		cacheWriter.EXPECT().RemoveRule(gomock.Any(), ruleID),
	)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, model.RuleStatusInactive, result.Status)
	assert.False(t, result.UpdatedAt.IsZero(), "UpdatedAt should be set")
	assert.NotNil(t, result.DeactivatedAt, "DeactivatedAt should be set after deactivation")
	assert.Nil(t, result.ActivatedAt, "ActivatedAt should be nil after deactivation from ACTIVE")
}

// TestDeactivateRule_Success_NilCacheWriter verifies the command completes
// successfully when no RuleCacheWriter is wired.
func TestDeactivateRule_Success_NilCacheWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(100)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter, ruleID, model.AuditEventRuleDeactivated, model.AuditActionDeactivate, "Rule deactivated via API")

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.RuleStatusInactive, result.Status)
}

// TestDeactivateRule_Success_NilAuditWriter verifies the auditWriter == nil
// short-circuit branch inside the transactional callback.
func TestDeactivateRule_Success_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(110)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
		cacheWriter.EXPECT().RemoveRule(gomock.Any(), ruleID),
	)
	mockTx.EXPECT().Rollback().Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), nil, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.RuleStatusInactive, result.Status)
}

func TestDeactivateRule_FromDraft_InvalidTransition(t *testing.T) {
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
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	// No tx / audit / cache expected - invalid transition short-circuits before executeInTx.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	// DRAFT → INACTIVE is not a valid transition (DRAFT can only go to ACTIVE or DELETED)
	require.Error(t, err)
	var transitionErr *model.InvalidTransitionError
	require.True(t, errors.As(err, &transitionErr), "should be an InvalidTransitionError")
	assert.Equal(t, model.RuleStatusDraft, transitionErr.From)
	assert.Equal(t, model.RuleStatusInactive, transitionErr.To)
}

func TestDeactivateRule_RuleNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestDeactivateRule_NilRuleFromRepo exercises the defensive `if rule == nil` guard
// added after GetByID. Returning (nil, nil) from the repository is a contract
// violation, but the guard treats it as ErrRuleNotFound rather than panicking
// on the subsequent rule.<field> dereference.
func TestDeactivateRule_NilRuleFromRepo(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(201)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	// Contract violation: repo returns (nil, nil). Only the `if rule == nil`
	// guard catches this — without it, the subsequent rule.Status access
	// would panic.
	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, nil)

	// No downstream side effects must fire: no tx, no audit write, no cache mutation.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.Error(t, err)
	require.Nil(t, result, "rule pointer must be nil when repo returns (nil, nil)")
	assert.ErrorIs(t, err, constant.ErrRuleNotFound, "guard must surface ErrRuleNotFound, wrapped via ValidateBusinessError")
}

func TestDeactivateRule_AlreadyInactive_Idempotent(t *testing.T) {
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
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	// Idempotent path: no tx, no audit, no cache.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, model.RuleStatusInactive, result.Status, "Status should remain INACTIVE")
}

func TestDeactivateRule_InvalidTransition(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDeleted,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	// Verify the error is the typed InvalidTransitionError
	var transitionErr *model.InvalidTransitionError
	assert.True(t, errors.As(err, &transitionErr), "should be an InvalidTransitionError")
}

func TestDeactivateRule_GetByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, errors.New("database error"))

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestDeactivateRule_UpdateError verifies that when UpdateWithTx fails inside
// the transactional callback, the tx is rolled back, Commit is never called,
// the audit writer is never invoked, and the cache is not updated.
func TestDeactivateRule_UpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
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
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	// Note: inputRule in memory IS mutated by SetStatus() before persistence fails.
	assert.Equal(t, model.RuleStatusInactive, inputRule.Status, "Status is mutated in memory by SetStatus()")
}

// TestDeactivateRule_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer /
// cache writer transactional methods.
func TestDeactivateRule_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(2)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
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
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
}

// TestDeactivateRule_AuditError_Rollback verifies that when the audit writer
// call inside the transaction fails, the transaction is rolled back, Commit is
// never invoked, the command returns the audit error, and the cache is NOT
// mutated.
func TestDeactivateRule_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(3)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
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
				model.AuditEventRuleDeactivated,
				model.AuditActionDeactivate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule deactivated via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
}

// TestDeactivateRule_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error, a defer-driven rollback fires, and the cache is NOT mutated.
func TestDeactivateRule_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(4)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusActive,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
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
				model.AuditEventRuleDeactivated,
				model.AuditActionDeactivate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule deactivated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	cacheWriter.EXPECT().RemoveRule(gomock.Any(), gomock.Any()).Times(0)

	service, err := NewDeactivateRuleService(mockRepo, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
}
