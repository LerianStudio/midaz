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

	"github.com/shopspring/decimal"

	pgdb "tracer/internal/adapters/postgres/db"
	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
	"tracer/internal/testhelper"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

func TestNewCreateLimitCommand(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	// No audit expected - constructor only
	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)

	require.NoError(t, err)
	assert.NotNil(t, cmd)
}

func TestNewCreateLimitCommand_NilRepository(t *testing.T) {
	cmd, err := NewCreateLimitCommand(nil, testutil.NewDefaultMockClock(), nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilLimitRepository)
	assert.Nil(t, cmd)
}

func TestNewCreateLimitCommand_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	cmd, err := NewCreateLimitCommand(mockRepo, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilClock)
	assert.Nil(t, cmd)
}

// TestCreateLimit_Success_Atomic exercises the happy path of the atomic
// CreateLimit command: BeginTx → CreateWithTx → RecordLimitEventWithTx →
// Commit (no Rollback). Drives the helper expectLimitCreateTxSuccess which
// pins the strict in-order chain.
//
// Field-level assertions on the returned *model.Limit cover LimitType,
// MaxAmount, Currency, ResetAt, and Scopes — mirroring the depth of the
// gomock.Cond-based field checks in TestCreateLimitCommand_Execute_Normalization.
func TestCreateLimit_Success_Atomic(t *testing.T) {
	ctrl := gomock.NewController(t)

	scopeAccountID := testutil.MustDeterministicUUID(1)
	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(scopeAccountID),
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	expectLimitCreateTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		model.AuditEventLimitCreated,
		model.AuditActionCreate,
		"Limit created via API",
	)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	maxAmount := decimal.RequireFromString("1000")
	input := &CreateLimitInput{
		Name:        "Daily Card Limit",
		Description: testutil.StringPtr("Daily spending limit"),
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   maxAmount,
		Currency:    "USD",
		Scopes:      []model.Scope{validScope},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Equal(t, "Daily Card Limit", result.Name)
	assert.Equal(t, model.LimitStatusDraft, result.Status)
	assert.Equal(t, model.LimitTypeDaily, result.LimitType)
	assert.True(t, maxAmount.Equal(result.MaxAmount),
		"MaxAmount must equal input: expected %s got %s", maxAmount, result.MaxAmount)
	assert.Equal(t, "USD", result.Currency)
	assert.NotNil(t, result.ResetAt,
		"ResetAt must be set by the DAILY constructor")
	require.Len(t, result.Scopes, 1)
	require.NotNil(t, result.Scopes[0].AccountID)
	assert.Equal(t, scopeAccountID, *result.Scopes[0].AccountID)
}

// TestCreateLimit_BeginTxError verifies that when BeginTx fails the command
// returns a wrapped error and never invokes the repository / audit writer
// transactional methods.
func TestCreateLimit_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:      "BeginTx Failure Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{validScope},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be surfaced to caller")
	assert.Nil(t, result)
}

// TestCreateLimit_RepoError_Rollback verifies that when CreateWithTx fails
// inside the transactional callback, the tx is rolled back, Commit is never
// called, and the audit writer is never invoked.
func TestCreateLimit_RepoError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit and audit must NOT fire on the rollback path.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:      "Repo Failure Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{validScope},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	assert.Nil(t, result)
}

// TestCreateLimit_RepoError_NameAlreadyExists ensures a unique-violation
// (sentinel constant.ErrLimitNameAlreadyExists) returned from CreateWithTx
// triggers rollback and surfaces as the same sentinel for the caller.
// Mirrors TestCreateRule_RepoError_NameAlreadyExists on the limit side.
func TestCreateLimit_RepoError_NameAlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(constant.ErrLimitNameAlreadyExists),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit and audit must NOT fire on the rollback path.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:      "Existing Limit Name",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{validScope},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitNameAlreadyExists)
	assert.Nil(t, result)
}

// TestCreateLimit_AuditError_Rollback inverts the previous best-effort
// behavior: when the audit writer call inside the transaction fails, the
// transaction is rolled back, Commit is never invoked, and the command
// returns a wrapped audit error. The limit insert MUST NOT be committed
// without an audit trail.
func TestCreateLimit_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit write failed")

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.Limit) error {
				return nil
			}),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventLimitCreated,
				model.AuditActionCreate,
				gomock.Any(),
				gomock.Nil(),
				gomock.Not(gomock.Nil()),
				"Limit created via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Negative expectations: commit must NOT fire on the audit-rollback path.
	mockTx.EXPECT().Commit().Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:      "Audit Failure Test",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{validScope},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr, "audit error must surface to caller")
	assert.Nil(t, result)
}

// TestCreateLimit_CommitError verifies that when Commit fails after all
// in-transaction operations succeed, the caller receives a wrapped commit
// error and a defer-driven rollback fires.
func TestCreateLimit_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.Limit) error {
				return nil
			}),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				mockTx,
				model.AuditEventLimitCreated,
				model.AuditActionCreate,
				gomock.Any(),
				gomock.Nil(),
				gomock.Not(gomock.Nil()),
				"Limit created via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:      "Commit Failure Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{validScope},
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "commit error must surface to caller")
	assert.Nil(t, result)
}

// TestCreateLimit_NilInput_NoTx ensures the nil-input pre-tx guard returns
// before BeginTx is called.
func TestCreateLimit_NilInput_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(context.Background(), nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitNilInput)
	assert.Nil(t, result)
}

// TestCreateLimit_DomainValidation_NoTx exercises pre-tx domain validation
// failures (invalid name, type, maxAmount, currency, scopes). All must
// short-circuit before BeginTx is called.
func TestCreateLimit_DomainValidation_NoTx(t *testing.T) {
	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	tests := []struct {
		name    string
		input   *CreateLimitInput
		errorIs error
	}{
		{
			name: "whitespace-only name",
			input: &CreateLimitInput{
				Name:      "   ",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitNameRequired,
		},
		{
			name: "empty name",
			input: &CreateLimitInput{
				Name:      "",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitNameRequired,
		},
		{
			name: "invalid limit type",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitType("INVALID"),
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitInvalidType,
		},
		{
			name: "zero maxAmount",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("0"),
				Currency:  "USD",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitInvalidMaxAmount,
		},
		{
			name: "negative maxAmount",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("-1"),
				Currency:  "USD",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitInvalidMaxAmount,
		},
		{
			name: "invalid currency (contains number)",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "US1",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitInvalidCurrency,
		},
		{
			name: "currency too short",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "US",
				Scopes:    []model.Scope{validScope},
			},
			errorIs: constant.ErrLimitInvalidCurrency,
		},
		{
			name: "empty scopes",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{},
			},
			errorIs: constant.ErrLimitInvalidScope,
		},
		{
			name: "nil scopes",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    nil,
			},
			errorIs: constant.ErrLimitInvalidScope,
		},
		{
			name: "empty scope in array",
			input: &CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{}},
			},
			errorIs: constant.ErrLimitInvalidScope,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			mockRepo.EXPECT().
				CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)
			auditWriter.EXPECT().
				RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, err)

			result, err := cmd.Execute(context.Background(), tc.input)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.errorIs)
			assert.Nil(t, result)
		})
	}
}

// TestCreateLimitCommand_Execute_Normalization verifies that the limit
// passed into CreateWithTx has its name and currency normalized.
func TestCreateLimitCommand_Execute_Normalization(t *testing.T) {
	tests := []struct {
		name             string
		inputName        string
		inputCurrency    string
		expectedName     string
		expectedCurrency string
	}{
		{
			name:             "trims whitespace from name",
			inputName:        "  Whitespace Name  ",
			inputCurrency:    "USD",
			expectedName:     "Whitespace Name",
			expectedCurrency: "USD",
		},
		{
			name:             "uppercases lowercase currency",
			inputName:        "Lowercase Currency Test",
			inputCurrency:    "usd",
			expectedName:     "Lowercase Currency Test",
			expectedCurrency: "USD",
		},
		{
			name:             "trims and normalizes both",
			inputName:        "  Foo  ",
			inputCurrency:    " usd ",
			expectedName:     "Foo",
			expectedCurrency: "USD",
		},
	}

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
			mockTx := pgdbMocks.NewMockTx(ctrl)

			gomock.InOrder(
				txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
				mockRepo.EXPECT().
					CreateWithTx(gomock.Any(), mockTx, gomock.Cond(func(x any) bool {
						limit, ok := x.(*model.Limit)
						return ok && limit.Name == tc.expectedName && limit.Currency == tc.expectedCurrency
					})).
					Return(nil),
				auditWriter.EXPECT().
					RecordLimitEventWithTx(
						gomock.Any(),
						mockTx,
						model.AuditEventLimitCreated,
						model.AuditActionCreate,
						gomock.Any(),
						gomock.Nil(),
						gomock.Not(gomock.Nil()),
						"Limit created via API",
					).
					Return(nil),
				mockTx.EXPECT().Commit().Return(nil),
			)

			cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, err)

			input := &CreateLimitInput{
				Name:      tc.inputName,
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  tc.inputCurrency,
				Scopes:    []model.Scope{validScope},
			}

			result, err := cmd.Execute(context.Background(), input)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tc.expectedName, result.Name)
			assert.Equal(t, tc.expectedCurrency, result.Currency)
		})
	}
}

// TestCreateLimitCommand_Execute_ContextCancellation verifies that when
// context is cancelled before Execute, no transaction is started, no repo
// call happens, and no audit event is recorded.
func TestCreateLimitCommand_Execute_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	input := &CreateLimitInput{
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{validScope},
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().
		CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before Execute

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	result, err := cmd.Execute(ctx, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

// TestCreateLimitCommand_Execute_PartialCustomPeriod ensures partial CUSTOM
// period configuration is rejected pre-tx (no BeginTx).
func TestCreateLimitCommand_Execute_PartialCustomPeriod(t *testing.T) {
	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	startDate := "2026-11-27T00:00:00Z"

	tests := []struct {
		name            string
		limitType       model.LimitType
		customStartDate *string
		customEndDate   *string
		errorIs         error
	}{
		{
			name:            "CUSTOM with only customStartDate",
			limitType:       model.LimitTypeCustom,
			customStartDate: &startDate,
			customEndDate:   nil,
			errorIs:         constant.ErrLimitCustomDatesRequired,
		},
		{
			name:            "CUSTOM with only customEndDate",
			limitType:       model.LimitTypeCustom,
			customStartDate: nil,
			customEndDate:   &startDate,
			errorIs:         constant.ErrLimitCustomDatesRequired,
		},
		{
			name:            "DAILY with only customStartDate rejected",
			limitType:       model.LimitTypeDaily,
			customStartDate: &startDate,
			customEndDate:   nil,
			errorIs:         constant.ErrLimitCustomDatesRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			mockRepo.EXPECT().
				CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)
			auditWriter.EXPECT().
				RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			cmd, cmdErr := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, cmdErr)

			input := &CreateLimitInput{
				Name:            "Test Partial Custom",
				LimitType:       tc.limitType,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{validScope},
				CustomStartDate: tc.customStartDate,
				CustomEndDate:   tc.customEndDate,
			}

			result, err := cmd.Execute(context.Background(), input)

			require.Error(t, err, "Partial custom period should be rejected")
			assert.ErrorIs(t, err, tc.errorIs)
			assert.Nil(t, result)
		})
	}
}

// TestCreateLimitCommand_Execute_PartialTimeWindow ensures partial time
// window configuration is rejected pre-tx (no BeginTx).
func TestCreateLimitCommand_Execute_PartialTimeWindow(t *testing.T) {
	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	startTime := testhelper.MustNewTimeOfDay("09:00")

	tests := []struct {
		name            string
		activeTimeStart *model.TimeOfDay
		activeTimeEnd   *model.TimeOfDay
	}{
		{
			name:            "only activeTimeStart provided",
			activeTimeStart: &startTime,
			activeTimeEnd:   nil,
		},
		{
			name:            "only activeTimeEnd provided",
			activeTimeStart: nil,
			activeTimeEnd:   &startTime,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			mockRepo.EXPECT().
				CreateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)
			auditWriter.EXPECT().
				RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			cmd, cmdErr := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, cmdErr)

			input := &CreateLimitInput{
				Name:            "Test Partial TimeWindow",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{validScope},
				ActiveTimeStart: tc.activeTimeStart,
				ActiveTimeEnd:   tc.activeTimeEnd,
			}

			result, err := cmd.Execute(context.Background(), input)

			require.Error(t, err, "Partial time window should be rejected")
			assert.ErrorIs(t, err, constant.ErrLimitTimeWindowMismatch)
			assert.Nil(t, result)
		})
	}
}
