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

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestNewUpdateRuleCommand_NilDependency exercises every nil-dependency
// branch of the validating constructor. Each subtest passes a single nil
// dependency (with all others valid) and asserts the matching sentinel
// error. The atomicity contract requires every dependency at boot time, so
// any nil arg must surface a typed error rather than defer the failure to
// first execution.
func TestNewUpdateRuleCommand_NilDependency(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockRuleRepository(ctrl)
	cel := NewMockExpressionCompiler(ctrl)
	audit := NewMockAuditWriter(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)
	clk := testutil.NewDefaultMockClock()

	cases := []struct {
		name        string
		repo        RuleRepository
		cel         ExpressionCompiler
		clk         clock.Clock
		audit       AuditWriter
		tx          pgdb.TxBeginner
		expectedErr error
	}{
		{"nil repository", nil, cel, clk, audit, tx, ErrNilUpdateRuleRepository},
		{"nil cel", repo, nil, clk, audit, tx, ErrNilUpdateRuleCEL},
		{"nil clock", repo, cel, nil, audit, tx, ErrNilUpdateRuleClock},
		{"nil audit writer", repo, cel, clk, nil, tx, ErrNilUpdateRuleAuditWriter},
		{"nil tx beginner", repo, cel, clk, audit, nil, ErrNilUpdateRuleTxBeginner},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := NewUpdateRuleCommand(tc.repo, tc.cel, tc.clk, tc.audit, tc.tx)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedErr)
			assert.Nil(t, cmd)
		})
	}
}

// TestUpdateRule_GetByIDError_NoTx exercises the non-NotFound error branch
// from GetByID. Distinct from TestUpdateRule_NotFound_NoTx, this case asserts
// that an unexpected repository error is wrapped (with "failed to get rule"
// prefix) and never opens a transaction.
// TestUpdateRule_NilInput_NoTx asserts that calling Execute with a nil
// *UpdateRuleInput is rejected pre-tx with ErrRuleNilInput. Without this
// guard the first dereference of input (e.g. input.Expression) would panic.
func TestUpdateRule_NilInput_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(19)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	// No repository, transaction, or audit call must occur for a nil input.
	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(context.Background(), ruleID, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleNilInput)
	assert.Nil(t, result)
}

func TestUpdateRule_GetByIDError_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(20)
	dbErr := errors.New("connection reset by peer")

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(nil, dbErr)

	// No transaction must start when GetByID fails.
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
	assert.ErrorIs(t, err, dbErr, "unexpected repo error must surface to caller")
	assert.Contains(t, err.Error(), "failed to get rule",
		"unexpected repo error must be wrapped with explanatory prefix")
	assert.Nil(t, result)
}

// TestUpdateRule_ExpressionOnNonDraft_NoTx asserts that an Expression update
// on a rule whose status is not DRAFT is rejected pre-tx with
// ErrExpressionNotModifiable. Expression mutations on ACTIVE/INACTIVE rules
// would silently bypass governance because activation snapshots the
// expression — the guard in update_rule.go:134 enforces this.
func TestUpdateRule_ExpressionOnNonDraft_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(21)
	baseTime := testutil.FixedTime()
	activeRule := &model.Rule{
		ID:         ruleID,
		Name:       "active rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(activeRule, nil)

	// Expression rejection happens before CEL.Compile because the status
	// gate runs first; CEL must NOT be called.
	mockCEL.EXPECT().
		Compile(gomock.Any(), gomock.Any()).
		Times(0)

	// No transaction must start.
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
		Expression: testutil.StringPtr("amount > 9999"),
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrExpressionNotModifiable)
	assert.Nil(t, result)
}

// TestUpdateRule_DomainUpdateValidationError_NoTx asserts that when
// rule.Update() rejects the input (e.g. name too long), the command surfaces
// the error pre-tx and does not begin a transaction.
func TestUpdateRule_DomainUpdateValidationError_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	ruleID := testutil.MustDeterministicUUID(22)
	baseTime := testutil.FixedTime()
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
		Return(existingRule, nil)

	// Build a name that exceeds MaxRuleNameLength to trigger rule.Update
	// validation failure. The exact constant is enforced by domain validation;
	// we generate something obviously over-the-limit.
	overlong := make([]byte, model.MaxRuleNameLength+10)
	for i := range overlong {
		overlong[i] = 'a'
	}

	// No transaction must start when domain validation fails.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewUpdateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	overlongName := string(overlong)
	input := &UpdateRuleInput{
		Name: &overlongName,
	}

	result, err := cmd.Execute(context.Background(), ruleID, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleNameTooLong)
	assert.Nil(t, result)
}
