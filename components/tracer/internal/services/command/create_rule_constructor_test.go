// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestNewCreateRuleCommand_NilCEL exercises the cel-nil branch of the
// validating constructor. Passing valid repo + nil cel must surface
// ErrNilCreateRuleCEL.
func TestNewCreateRuleCommand_NilCEL(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockRuleRepository(ctrl)
	audit := NewMockAuditWriter(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)

	cmd, err := NewCreateRuleCommand(repo, nil, testutil.NewDefaultMockClock(), audit, tx)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateRuleCEL)
	assert.Nil(t, cmd)
}

// TestNewCreateRuleCommand_NilClock exercises the clk-nil branch of the
// validating constructor.
func TestNewCreateRuleCommand_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockRuleRepository(ctrl)
	cel := NewMockExpressionCompiler(ctrl)
	audit := NewMockAuditWriter(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)

	var nilClk clock.Clock
	cmd, err := NewCreateRuleCommand(repo, cel, nilClk, audit, tx)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateRuleClock)
	assert.Nil(t, cmd)
}

// TestNewCreateRuleCommand_NilAuditWriter exercises the auditWriter-nil
// branch of the validating constructor.
func TestNewCreateRuleCommand_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockRuleRepository(ctrl)
	cel := NewMockExpressionCompiler(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)

	cmd, err := NewCreateRuleCommand(repo, cel, testutil.NewDefaultMockClock(), nil, tx)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateRuleAuditWriter)
	assert.Nil(t, cmd)
}

// TestNewCreateRuleCommand_NilTxBeginner exercises the txBeginner-nil branch
// of the validating constructor. The tx beginner is required because rule
// creation and audit recording must be persisted atomically — deferring the
// nil check to first execution would conceal a DI bug behind a runtime error.
func TestNewCreateRuleCommand_NilTxBeginner(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockRuleRepository(ctrl)
	cel := NewMockExpressionCompiler(ctrl)
	audit := NewMockAuditWriter(ctrl)

	cmd, err := NewCreateRuleCommand(repo, cel, testutil.NewDefaultMockClock(), audit, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateRuleTxBeginner)
	assert.Nil(t, cmd)
}

// TestCreateRule_DomainValidation_NoTx exercises the model.NewRule failure
// path inside Execute. The CEL mock returns nil so compilation succeeds, but
// model.NewRule then rejects the empty/invalid input. The transaction must
// not begin: any persistence call after a domain-validation failure would
// violate the pre-tx invariant.
func TestCreateRule_DomainValidation_NoTx(t *testing.T) {
	tests := []struct {
		name      string
		input     *CreateRuleInput
		expectErr error
	}{
		{
			name: "empty expression rejected by NewRule",
			input: &CreateRuleInput{
				Name:       "Some Rule",
				Expression: "",
				Action:     model.DecisionDeny,
			},
			expectErr: constant.ErrRuleExpressionRequired,
		},
		{
			name: "empty name rejected by NewRule",
			input: &CreateRuleInput{
				Name:       "   ",
				Expression: "amount > 1000",
				Action:     model.DecisionDeny,
			},
			expectErr: constant.ErrRuleNameRequired,
		},
		{
			name: "invalid action rejected by NewRule",
			input: &CreateRuleInput{
				Name:       "Some Rule",
				Expression: "amount > 1000",
				Action:     model.Decision("BOGUS"),
			},
			expectErr: constant.ErrRuleInvalidAction,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockRuleRepository(ctrl)
			mockCEL := NewMockExpressionCompiler(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			// CEL is called before model.NewRule with the raw expression and
			// returns nil; the rejection happens inside model.NewRule.
			mockCEL.EXPECT().
				Compile(gomock.Any(), tc.input.Expression).
				Return(nil, nil)

			// No transaction must start when NewRule fails — atomic-flow guard.
			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			mockRepo.EXPECT().
				CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)
			auditWriter.EXPECT().
				RecordRuleEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, err)

			result, err := cmd.Execute(context.Background(), tc.input)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expectErr)
			assert.Nil(t, result)
		})
	}
}
