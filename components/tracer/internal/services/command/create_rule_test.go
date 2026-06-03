// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MiNhA REGRA", "minha regra"},
		{"  spaces around  ", "spaces around"},
		{"UPPERCASE", "uppercase"},
		{"lowercase", "lowercase"},
		{"  Mixed CASE with Spaces  ", "mixed case with spaces"},
		{"", ""},
		{"   ", ""},
		{"  mInha    regra  xpto ", "minha regra xpto"},
		{"minha reGra xpto", "minha regra xpto"},
		{"a   b    c     d", "a b c d"},
		{"tab\there", "tab here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNewCreateRuleCommand_NilRepository asserts the constructor rejects a nil
// repository. After the atomicity fix, NewCreateRuleCommand returns
// (*CreateRuleCommand, error) following the same pattern as the limit
// constructors and the activate/delete/update lifecycle services.
//
// The other deps are passed as non-nil so only the repo-nil branch can fire
// (otherwise the constructor's first nil check would return ErrNilCreateRuleRepository
// even if the repo were non-nil and a different dep were nil — the assertion
// below pins the specific branch under test).
func TestNewCreateRuleCommand_NilRepository(t *testing.T) {
	ctrl := gomock.NewController(t)

	cel := NewMockExpressionCompiler(ctrl)
	audit := NewMockAuditWriter(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)

	cmd, err := NewCreateRuleCommand(nil, cel, testutil.NewDefaultMockClock(), audit, tx)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateRuleRepository)
	assert.Nil(t, cmd)
}

// TestCreateRule_Success_Atomic exercises the happy path of the atomic
// CreateRule command: BeginTx → CreateWithTx → RecordRuleEventWithTx → Commit
// (no Rollback). Drives the helper expectRuleCreateTxSuccess which pins the
// strict in-order chain.
func TestCreateRule_Success_Atomic(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 1000000").
		Return(nil, nil)

	expectRuleCreateTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		model.AuditEventRuleCreated,
		model.AuditActionCreate,
		"Rule created via API",
	)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:        "High Value Transaction Rule",
		Description: "Blocks transactions over $10,000",
		Expression:  "amount > 1000000",
		Action:      model.DecisionDeny,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))},
		},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Equal(t, NormalizeName(input.Name), result.Name)
	assert.Equal(t, input.Action, result.Action)
	assert.Equal(t, model.RuleStatusDraft, result.Status)
}

// TestCreateRule_Success_NoScopes drives the happy path with an empty scopes
// slice ([]model.Scope{}, not nil) — model.NewRule treats both empty and nil
// as "global" rules and normalizes nil to empty before persistence. This test
// pins the empty-slice branch end-to-end through the atomic flow and asserts
// that the persisted rule has an empty Scopes slice (length 0). The nil
// branch is exercised implicitly elsewhere; we standardize on the explicit
// empty form here for clarity.
func TestCreateRule_Success_NoScopes(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 0").
		Return(nil, nil)

	expectRuleCreateTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		model.AuditEventRuleCreated,
		model.AuditActionCreate,
		"Rule created via API",
	)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "Global Rule",
		Expression: "amount > 0",
		Action:     model.DecisionAllow,
		Scopes:     []model.Scope{},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Equal(t, "global rule", result.Name)
	assert.Empty(t, result.Scopes,
		"global rule must have an empty (non-nil) Scopes slice after normalization")
}

// TestCreateRule_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer
// transactional methods.
func TestCreateRule_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 1000000").
		Return(nil, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "Test Rule",
		Expression: "amount > 1000000",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
	assert.Nil(t, result)
}

// TestCreateRule_RepoError_Rollback verifies that when CreateWithTx fails
// inside the transactional callback, the tx is rolled back, Commit is never
// called, and the audit writer is never invoked.
func TestCreateRule_RepoError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 1000000").
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(nil, dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit and audit must NOT fire on the rollback path.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "Test Rule",
		Expression: "amount > 1000000",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	assert.Nil(t, result)
}

// TestCreateRule_RepoError_NameAlreadyExists ensures a unique-violation
// (sentinel constant.ErrRuleNameAlreadyExistsInCtx) returned from CreateWithTx
// triggers rollback and surfaces as the same sentinel for the caller.
func TestCreateRule_RepoError_NameAlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 100").
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(nil, constant.ErrRuleNameAlreadyExistsInCtx),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit and audit must NOT fire on the rollback path.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "  EXISTING Rule  ",
		Expression: "amount > 100",
		Action:     model.DecisionAllow,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleNameAlreadyExistsInCtx)
	assert.Nil(t, result)
}

// TestCreateRule_AuditError_Rollback verifies that when the audit writer call
// inside the transaction fails, the transaction is rolled back, Commit is
// never invoked, and the command returns a wrapped audit error. This is the
// inversion of the previous best-effort behavior — audit failure now
// propagates as a hard error so the rule insert is not committed without an
// audit trail.
func TestCreateRule_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit insert failed")

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 1000000").
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, r *model.Rule) (*model.Rule, error) {
				return r, nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventRuleCreated,
				model.AuditActionCreate,
				gomock.Any(),
				gomock.Nil(),
				gomock.Not(gomock.Nil()),
				"Rule created via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit must NOT fire on the audit-rollback path.
	mockTx.EXPECT().Commit().Times(0)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "Audit Failure Rule",
		Expression: "amount > 1000000",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
	assert.Nil(t, result)
}

// TestCreateRule_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error and a defer-driven rollback fires.
func TestCreateRule_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 1000000").
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, r *model.Rule) (*model.Rule, error) {
				return r, nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventRuleCreated,
				model.AuditActionCreate,
				gomock.Any(),
				gomock.Nil(),
				gomock.Not(gomock.Nil()),
				"Rule created via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "Commit Failure Rule",
		Expression: "amount > 1000000",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
	assert.Nil(t, result)
}

// TestCreateRule_NilInput_NoTx ensures the nil-input pre-tx guard returns
// before BeginTx is called.
func TestCreateRule_NilInput_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(context.Background(), nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleNilInput)
	assert.Nil(t, result)
}

// TestCreateRule_InvalidCEL_NoTx ensures that CEL compilation failure (a
// pre-tx validation step) returns before BeginTx is called.
func TestCreateRule_InvalidCEL_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockCEL.EXPECT().
		Compile(gomock.Any(), "invalid cel >>>").
		Return(nil, constant.ErrExpressionSyntax)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateRuleInput{
		Name:       "Bad Expression Rule",
		Expression: "invalid cel >>>",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrExpressionSyntax)
	assert.Nil(t, result)
}

// TestCreateRuleCommand_Execute_SetsCorrectFields verifies that the rule
// passed into CreateWithTx has all fields populated as expected. This guards
// against a refactor accidentally dropping a field.
func TestCreateRuleCommand_Execute_SetsCorrectFields(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	testAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	input := &CreateRuleInput{
		Name:        "Test Rule",
		Description: "Test Description",
		Expression:  "amount > 100",
		Action:      model.DecisionReview,
		Scopes: []model.Scope{
			{
				AccountID:       testutil.UUIDPtr(testAccountID),
				TransactionType: testutil.Ptr(model.TransactionTypeCard),
			},
		},
	}

	normalizedName := NormalizeName(input.Name)

	mockCEL.EXPECT().
		Compile(gomock.Any(), input.Expression).
		Return(nil, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, rule *model.Rule) (*model.Rule, error) {
				assert.NotEqual(t, uuid.Nil, rule.ID)
				assert.Equal(t, normalizedName, rule.Name)
				assert.Equal(t, input.Description, *rule.Description)
				assert.Equal(t, input.Expression, rule.Expression)
				assert.Equal(t, input.Action, rule.Action)
				assert.Equal(t, model.RuleStatusDraft, rule.Status)
				assert.Len(t, rule.Scopes, 1)
				assert.Equal(t, testAccountID, *rule.Scopes[0].AccountID)
				assert.Equal(t, testutil.FixedTime(), rule.CreatedAt)
				assert.Equal(t, testutil.FixedTime(), rule.UpdatedAt)
				assert.Nil(t, rule.DeletedAt)
				return rule, nil
			}),
		auditWriter.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventRuleCreated,
				model.AuditActionCreate,
				gomock.Any(),
				gomock.Nil(),
				gomock.Not(gomock.Nil()),
				"Rule created via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
}
