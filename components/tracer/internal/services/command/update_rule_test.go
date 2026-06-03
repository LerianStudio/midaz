// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "tracer/internal/adapters/postgres/db"
	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// Constructor nil-dependency cases (NilRepository, NilCEL, NilClock,
// NilAuditWriter, NilTxBeginner) are consolidated as a single table-driven
// test in update_rule_constructor_test.go (TestNewUpdateRuleCommand_NilDependency).

// TestUpdateRule_Success_Atomic exercises the happy path of the atomic
// UpdateRule command: GetByID (pre-tx) → BeginTx → UpdateWithTx →
// RecordRuleEventWithTx → Commit (no Rollback). Drives the helper
// expectRuleUpdateTxSuccess which pins the strict in-order chain.
func TestUpdateRule_Success_Atomic(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(1)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	expectRuleUpdateTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		ruleID,
		model.AuditEventRuleUpdated,
		model.AuditActionUpdate,
		"Rule updated via API",
	)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Name: testutil.StringPtr("Updated Rule Name"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	assert.Equal(t, "updated rule name", result.Name)
}

// TestUpdateRule_Success_UpdateScopes verifies that the Scopes field can be
// updated end-to-end through the atomic flow. The test uses a non-nil pointer
// to a non-empty []model.Scope and asserts the rule's Scopes are persisted
// with the new value and the existing happy-path tx chain is exercised.
func TestUpdateRule_Success_UpdateScopes(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(30)
	originalAccountID := testutil.MustDeterministicUUID(31)
	newAccountID := testutil.MustDeterministicUUID(32)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "scopes update rule",
		Expression: "amount > 100",
		Action:     model.DecisionAllow,
		Status:     model.RuleStatusDraft,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(originalAccountID)},
		},
		CreatedAt: baseTime,
		UpdatedAt: baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	expectRuleUpdateTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		ruleID,
		model.AuditEventRuleUpdated,
		model.AuditActionUpdate,
		"Rule updated via API",
	)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	newScopes := []model.Scope{
		{AccountID: testutil.UUIDPtr(newAccountID)},
	}
	input := &UpdateRuleInput{
		Scopes: &newScopes,
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ruleID, result.ID)
	require.Len(t, result.Scopes, 1)
	require.NotNil(t, result.Scopes[0].AccountID)
	assert.Equal(t, newAccountID, *result.Scopes[0].AccountID,
		"updated rule must reflect the new scope account id")
}

// TestUpdateRule_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer
// transactional methods.
func TestUpdateRule_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(2)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Name: testutil.StringPtr("New Name"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
	assert.Nil(t, result)
}

// TestUpdateRule_RepoError_Rollback verifies that when UpdateWithTx fails
// inside the transactional callback, the tx is rolled back, Commit is never
// called, and the audit writer is never invoked.
func TestUpdateRule_RepoError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(3)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit and audit must NOT fire on the rollback path.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Action: testutil.Ptr(model.DecisionReview),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	assert.Nil(t, result)
}

// TestUpdateRule_RepoError_NameAlreadyExists ensures a unique-violation
// returned from UpdateWithTx triggers rollback and surfaces as the same
// sentinel for the caller.
func TestUpdateRule_RepoError_NameAlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(4)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(constant.ErrRuleNameAlreadyExistsInCtx),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit and audit must NOT fire on the rollback path.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Name: testutil.StringPtr("Another Rule"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleNameAlreadyExistsInCtx)
	assert.Nil(t, result)
}

// TestUpdateRule_AuditError_Rollback verifies that when the audit writer call
// inside the transaction fails, the transaction is rolled back, Commit is
// never invoked, and the command returns a wrapped audit error. This is the
// inversion of the previous best-effort behavior — audit failure now
// propagates as a hard error so the rule update is not committed without an
// audit trail.
func TestUpdateRule_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(5)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit insert failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.Rule) error {
				return nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventRuleUpdated,
				model.AuditActionUpdate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule updated via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit must NOT fire on the audit-rollback path.
	mockTx.EXPECT().Commit().Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Name: testutil.StringPtr("New Name"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
	assert.Nil(t, result)
}

// TestUpdateRule_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error and a defer-driven rollback fires.
func TestUpdateRule_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(6)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.Rule) error {
				return nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventRuleUpdated,
				model.AuditActionUpdate,
				ruleID,
				gomock.Any(),
				gomock.Any(),
				"Rule updated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Name: testutil.StringPtr("New Name"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
	assert.Nil(t, result)
}

// TestUpdateRule_NotFound_NoTx ensures that ErrRuleNotFound from GetByID
// (a pre-tx step) returns before BeginTx is called.
func TestUpdateRule_NotFound_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(7)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Name: testutil.StringPtr("New Name"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleNotFound)
	assert.Nil(t, result)
}

// TestUpdateRule_InvalidCEL_NoTx ensures CEL compile failure (a pre-tx
// validation step) returns before BeginTx is called.
func TestUpdateRule_InvalidCEL_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(8)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)
	mockCEL.EXPECT().
		Compile(gomock.Any(), "invalid cel >>>").
		Return(nil, constant.ErrExpressionSyntax)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Expression: testutil.StringPtr("invalid cel >>>"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrExpressionSyntax)
	assert.Nil(t, result)
}

// TestUpdateRule_InvalidAction_NoTx ensures invalid action (a pre-tx
// validation step on the domain entity) returns before BeginTx is called.
func TestUpdateRule_InvalidAction_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(9)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "existing rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(copyRule(existingRule), nil)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &UpdateRuleInput{
		Action: testutil.Ptr(model.Decision("INVALID")),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleInvalidAction)
	assert.Nil(t, result)
}

// copyRule creates a copy of a rule to avoid mutation issues in tests.
// copyRule returns an independent deep copy of r so tests can mutate the
// original without leaking changes into stored snapshots (and vice versa).
// The shallow `*r` copies the struct fields but the Scopes slice and every
// pointer inside each Scope must be copied explicitly to break aliasing
// between the original and the copy.
func copyRule(r *model.Rule) *model.Rule {
	if r == nil {
		return nil
	}

	dup := *r

	if r.Scopes != nil {
		dup.Scopes = make([]model.Scope, len(r.Scopes))
		for i, scope := range r.Scopes {
			dup.Scopes[i] = scope

			if scope.SegmentID != nil {
				v := *scope.SegmentID
				dup.Scopes[i].SegmentID = &v
			}

			if scope.PortfolioID != nil {
				v := *scope.PortfolioID
				dup.Scopes[i].PortfolioID = &v
			}

			if scope.AccountID != nil {
				v := *scope.AccountID
				dup.Scopes[i].AccountID = &v
			}

			if scope.MerchantID != nil {
				v := *scope.MerchantID
				dup.Scopes[i].MerchantID = &v
			}

			if scope.TransactionType != nil {
				v := *scope.TransactionType
				dup.Scopes[i].TransactionType = &v
			}

			if scope.SubType != nil {
				v := *scope.SubType
				dup.Scopes[i].SubType = &v
			}
		}
	}

	return &dup
}
