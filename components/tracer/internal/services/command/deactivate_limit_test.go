// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/shopspring/decimal"

	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestNewDeactivateLimitCommand_NilRepository(t *testing.T) {
	cmd, err := NewDeactivateLimitCommand(nil, testutil.NewDefaultMockClock(), nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilLimitRepository)
	assert.Nil(t, cmd)
}

func TestNewDeactivateLimitCommand_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	cmd, err := NewDeactivateLimitCommand(mockRepo, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilClock)
	assert.Nil(t, cmd)
}

func TestNewDeactivateLimitCommand(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	// No audit expected - constructor only
	cmd, err := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)

	require.NoError(t, err)
	assert.NotNil(t, cmd)
	assert.Equal(t, mockRepo, cmd.repo)
	assert.Equal(t, auditWriter, cmd.auditWriter)
}

func TestDeactivateLimitCommand_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(1)
	testStartTime := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
		Status:    model.LimitStatusActive,
		CreatedAt: testStartTime.Add(-time.Hour), // Created an hour ago
		UpdatedAt: testStartTime.Add(-time.Hour), // Last updated an hour ago
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// Capture the timestamp passed to UpdateStatusWithTx to verify it's sensible
	var capturedTimestamp time.Time

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusInactive, gomock.AssignableToTypeOf(time.Time{})).
			Do(func(_ context.Context, _ any, _ uuid.UUID, _ model.LimitStatus, ts time.Time) {
				capturedTimestamp = ts
			}).
			Return(nil),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventLimitDeactivated,
				model.AuditActionDeactivate,
				limitID,
				gomock.Any(),
				gomock.Any(),
				"Limit deactivated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, limitID, result.ID)
	assert.Equal(t, model.LimitStatusInactive, result.Status)
	assert.False(t, result.UpdatedAt.IsZero(), "UpdatedAt should be set")

	// Verify the timestamp passed to repository is sensible (>= test start time)
	require.False(t, capturedTimestamp.IsZero(), "Captured timestamp should not be zero")
	require.True(t, !capturedTimestamp.Before(testStartTime),
		"Timestamp passed to UpdateStatusWithTx should be >= test start time, got %v (test started at %v)",
		capturedTimestamp, testStartTime)

	// Verify result.UpdatedAt matches what was passed to repository
	assert.Equal(t, capturedTimestamp, result.UpdatedAt,
		"result.UpdatedAt should match timestamp passed to repository")
}

func TestDeactivateLimitCommand_Execute_AlreadyInactive_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(10)
	now := testutil.FixedTime()

	inactiveLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(11))}},
		Status:    model.LimitStatusInactive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(inactiveLimit, nil)

	// Idempotent: no tx, no audit.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, limitID, result.ID)
	assert.Equal(t, model.LimitStatusInactive, result.Status, "Status should remain INACTIVE")
}

func TestDeactivateLimitCommand_Execute_LimitNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(20)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(nil, constant.ErrLimitNotFound)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitNotFound)
	assert.Nil(t, result)
}

func TestDeactivateLimitCommand_Execute_InvalidTransition_FromDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(30)
	now := testutil.FixedTime()

	deletedLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(31))}},
		Status:    model.LimitStatusDeleted,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(deletedLimit, nil)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidStatusChange)
	assert.Nil(t, result)
}

func TestDeactivateLimitCommand_Execute_GetByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(40)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	dbErr := errors.New("database error")
	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(nil, dbErr)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	// Verify the original error is wrapped and preserved
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	assert.Nil(t, result)
}

func TestDeactivateLimitCommand_Execute_UpdateStatusError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(50)
	now := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(51))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	dbErr := errors.New("database error")
	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusInactive, gomock.AssignableToTypeOf(time.Time{})).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Commit MUST NOT be called when UpdateStatusWithTx fails.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	// Verify the original error is wrapped and preserved
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
	assert.Nil(t, result)
}

func TestDeactivateLimitCommand_Execute_NilUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(context.Background(), uuid.Nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidID)
	assert.Nil(t, result)
}

func TestDeactivateLimitCommand_Execute_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)
	result, err := cmd.Execute(ctx, testutil.MustDeterministicUUID(60))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

// TestDeactivateLimitCommand_Execute_BeginTxError verifies that when BeginTx fails,
// the command returns a wrapped error and never invokes the WithTx methods.
func TestDeactivateLimitCommand_Execute_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(70)
	now := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(71))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		UpdateStatusWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr)
	assert.Nil(t, result)
}

// TestDeactivateLimitCommand_Execute_AuditError_Rollback verifies that when
// the audit writer fails inside the tx, the tx rolls back and the command
// returns the wrapped audit error.
func TestDeactivateLimitCommand_Execute_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(80)
	now := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(81))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit insert failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusInactive, gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventLimitDeactivated,
				model.AuditActionDeactivate,
				limitID,
				gomock.Any(),
				gomock.Any(),
				"Limit deactivated via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr)
	assert.Nil(t, result)
}

// TestDeactivateLimitCommand_Execute_CommitError verifies that when Commit
// fails after the transactional work succeeds, the caller receives the wrapped
// commit error.
func TestDeactivateLimitCommand_Execute_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(90)
	now := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(91))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusInactive, gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventLimitDeactivated,
				model.AuditActionDeactivate,
				limitID,
				gomock.Any(),
				gomock.Any(),
				"Limit deactivated via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr)
	assert.Nil(t, result)
}

// TestDeactivateLimitCommand_Execute_Success_NilAuditWriter verifies that the
// transactional callback commits successfully when auditWriter is nil: the
// status update runs and Commit is invoked without any audit call.
// This pins the `auditWriter == nil` short-circuit branch inside executeInTx.
func TestDeactivateLimitCommand_Execute_Success_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(260)
	now := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(261))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)

	// Nil audit branch: BeginTx -> UpdateStatusWithTx -> Commit. No audit call.
	expectLimitStatusTxSuccessNoAudit(t, txBeginner, mockTx, mockRepo, limitID, model.LimitStatusInactive)

	cmd, cmdErr := NewDeactivateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), nil, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, limitID, result.ID)
	assert.Equal(t, model.LimitStatusInactive, result.Status)
	assert.False(t, result.UpdatedAt.IsZero(), "UpdatedAt should be set")
}
