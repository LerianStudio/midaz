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

func TestNewActivateRuleService_NilRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)

	service, err := NewActivateRuleService(nil, mockExprCompiler, testutil.NewDefaultMockClock(), nil, nil, nil)

	require.Nil(t, service)
	require.ErrorIs(t, err, ErrActivateNilRepository)
}

func TestNewActivateRuleService_NilExpressionCompiler(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)

	service, err := NewActivateRuleService(mockRepo, nil, testutil.NewDefaultMockClock(), nil, nil, nil)

	require.Nil(t, service)
	require.ErrorIs(t, err, ErrActivateNilExpressionCompiler)
}

func TestNewActivateRuleService_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, nil, nil, nil, nil)

	require.Nil(t, service)
	require.ErrorIs(t, err, ErrActivateNilClock)
}

func TestActivateRule_Success(t *testing.T) {
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
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	compiledProgram := struct{ name string }{name: "stub-program"}

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(compiledProgram, nil)

	// Transactional chain: BeginTx -> UpdateWithTx -> RecordRuleEventWithTx -> Commit.
	// Then the cache writer MUST be invoked AFTER Commit with the compiled program.
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, rule *model.Rule) error {
				assert.Equal(t, ruleID, rule.ID)
				assert.Equal(t, model.RuleStatusActive, rule.Status)
				assert.NotNil(t, rule.ActivatedAt, "activatedAt should be set")
				assert.Nil(t, rule.DeactivatedAt, "deactivatedAt should be nil for activate")
				return nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleActivated,
				model.AuditActionActivate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule activated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
		cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), compiledProgram),
		cacheWriter.EXPECT().MarkReady(gomock.Any()),
	)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, model.RuleStatusActive, result.Status)
	assert.False(t, result.UpdatedAt.IsZero(), "UpdatedAt should be set")
}

// TestActivateRule_Success_NilCacheWriter verifies that the command completes
// successfully when no RuleCacheWriter is wired: the transactional chain still
// runs and no cache operation is invoked.
func TestActivateRule_Success_NilCacheWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(100)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, nil)

	expectRuleUpdateTxSuccess(t, txBeginner, mockTx, mockRepo, auditWriter, ruleID, model.AuditEventRuleActivated, model.AuditActionActivate, "Rule activated via API")

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, nil, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.RuleStatusActive, result.Status)
}

// TestActivateRule_Success_NilAuditWriter verifies the auditWriter == nil
// short-circuit branch inside the transactional callback: the status update
// runs and Commit is invoked without any audit call, then the cache is
// updated post-commit.
func TestActivateRule_Success_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(110)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	compiledProgram := struct{ name string }{name: "stub-program-nil-audit"}

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(compiledProgram, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
		cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), compiledProgram),
		cacheWriter.EXPECT().MarkReady(gomock.Any()),
	)
	mockTx.EXPECT().Rollback().Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), nil, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.RuleStatusActive, result.Status)
}

func TestActivateRule_RuleNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)
	// No tx / audit / cache calls expected - operation failed before any mutation
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestActivateRule_NilRuleFromRepo exercises the defensive `if rule == nil` guard
// added after GetByID. Returning (nil, nil) from the repository is a contract
// violation, but the guard treats it as ErrRuleNotFound rather than panicking
// on the subsequent rule.<field> dereference.
func TestActivateRule_NilRuleFromRepo(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(200)

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	// Contract violation: repo returns (nil, nil). Only the `if rule == nil`
	// guard catches this — without it, the subsequent rule.Expression access
	// would panic.
	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, nil)

	// No downstream side effects must fire: no expression compile, no tx,
	// no audit write, no cache mutation.
	mockExprCompiler.EXPECT().Compile(gomock.Any(), gomock.Any()).Times(0)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.Error(t, err)
	require.Nil(t, result, "rule pointer must be nil when repo returns (nil, nil)")
	assert.ErrorIs(t, err, constant.ErrRuleNotFound, "guard must surface ErrRuleNotFound, wrapped via ValidateBusinessError")
}

func TestActivateRule_AlreadyActive_Idempotent(t *testing.T) {
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
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)

	// Idempotent path: no compile, no tx, no audit, no cache mutation.
	mockExprCompiler.EXPECT().Compile(gomock.Any(), gomock.Any()).Times(0)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	result, err := service.Execute(ctx, ruleID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, model.RuleStatusActive, result.Status, "Status should remain ACTIVE")
}

func TestActivateRule_InvalidTransition(t *testing.T) {
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
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, nil)

	// No tx / audit / cache expected - invalid transition short-circuits before executeInTx.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	// Verify the error is the typed InvalidTransitionError
	var transitionErr *model.InvalidTransitionError
	assert.True(t, errors.As(err, &transitionErr), "should be an InvalidTransitionError")
}

func TestActivateRule_EmptyExpression(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	// No compile / tx / audit / cache expected - empty expression short-circuits earlier.
	mockExprCompiler.EXPECT().Compile(gomock.Any(), gomock.Any()).Times(0)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

func TestActivateRule_ExpressionCompilationFailed(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "invalid expression syntax",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, errors.New("syntax error"))
	// No tx / audit / cache expected - compilation failure short-circuits before executeInTx.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

func TestActivateRule_GetByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, errors.New("database error"))
	// No compile / tx / audit / cache expected - GetByID failed.
	mockExprCompiler.EXPECT().Compile(gomock.Any(), gomock.Any()).Times(0)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
}

// TestActivateRule_UpdateError verifies that when UpdateWithTx fails inside the
// transactional callback, the tx is rolled back, Commit is never called, the
// audit writer is never invoked, and the cache is not updated.
func TestActivateRule_UpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(1)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	originalName := inputRule.Name
	originalExpression := inputRule.Expression

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Commit MUST NOT be called when UpdateWithTx fails.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")

	// Note: inputRule in memory IS mutated by SetStatus() before persistence fails.
	// This is current behavior - domain method mutates object, then persistence may fail.
	assert.Equal(t, model.RuleStatusActive, inputRule.Status, "Status is mutated in memory by SetStatus()")
	assert.Equal(t, originalName, inputRule.Name, "Name should not change")
	assert.Equal(t, originalExpression, inputRule.Expression, "Expression should not change")
}

// TestActivateRule_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer /
// cache writer transactional methods.
func TestActivateRule_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(2)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	// No WithTx / audit / cache calls expected when BeginTx fails.
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
}

// TestActivateRule_AuditError_Rollback verifies that when the audit writer call
// inside the transaction fails, the transaction is rolled back, Commit is never
// invoked, the command returns the audit error, and the cache is NOT updated.
func TestActivateRule_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(3)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit insert failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleActivated,
				model.AuditActionActivate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule activated via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Commit / cache MUST NOT fire on audit failure.
	mockTx.EXPECT().Commit().Times(0)
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
}

// TestActivateRule_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error, a defer-driven rollback fires, and the cache is NOT updated.
func TestActivateRule_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	ruleID := testutil.MustDeterministicUUID(4)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "Test Rule",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	cacheWriter := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventRuleActivated,
				model.AuditActionActivate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule activated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		// Commit failure: defer-based cleanup must Rollback to release locks.
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Cache must not update when the tx fails to commit.
	cacheWriter.EXPECT().UpsertRule(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	cacheWriter.EXPECT().MarkReady(gomock.Any()).Times(0)

	service, err := NewActivateRuleService(mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), auditWriter, cacheWriter, txBeginner)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
}
