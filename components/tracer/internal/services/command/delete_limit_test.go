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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestNewDeleteLimitCommand_NilRepository(t *testing.T) {
	cmd, err := NewDeleteLimitCommand(nil, testutil.NewDefaultMockClock(), nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilLimitRepository)
	assert.Nil(t, cmd)
}

func TestNewDeleteLimitCommand_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	cmd, err := NewDeleteLimitCommand(mockRepo, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilClock)
	assert.Nil(t, cmd)
}

func TestNewDeleteLimitCommand(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	cmd, err := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)

	require.NoError(t, err)
	assert.NotNil(t, cmd)
	assert.Equal(t, mockRepo, cmd.repo)
	assert.Equal(t, auditWriter, cmd.auditWriter)
}

func TestDeleteLimitCommand_Execute_InvalidTransition_FromActive(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(1)
	now := testutil.FixedTime()

	activeLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(activeLimit, nil)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(),
		).
		Times(0)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	// ACTIVE → DELETED is not a valid transition (ACTIVE can only go to INACTIVE)
	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidStatusChange)
}

func TestDeleteLimitCommand_Execute_Success_FromInactive(t *testing.T) {
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
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(inactiveLimit, nil)

	// delete intentionally emits nil as the "after" state.
	expectLimitStatusTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		limitID,
		model.LimitStatusDeleted,
		model.AuditEventLimitDeleted,
		model.AuditActionDelete,
		"Limit deleted via API",
		gomock.Nil(),
	)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.NoError(t, err)
}

func TestDeleteLimitCommand_Execute_AlreadyDeleted_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(20)
	now := testutil.FixedTime()

	deletedLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(21))}},
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

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.NoError(t, err)
}

func TestDeleteLimitCommand_Execute_LimitNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(30)

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

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitNotFound)
}

func TestDeleteLimitCommand_Execute_GetByIDError(t *testing.T) {
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

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	// Verify the original error is wrapped and preserved
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
}

func TestDeleteLimitCommand_Execute_UpdateStatusError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(50)
	now := testutil.FixedTime()

	// Use INACTIVE status since ACTIVE → DELETED is not allowed
	inactiveLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(51))}},
		Status:    model.LimitStatusInactive,
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
		Return(inactiveLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusDeleted, gomock.AssignableToTypeOf(time.Time{})).
			Return(dbErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	// Commit MUST NOT be called when UpdateStatusWithTx fails.
	mockTx.EXPECT().Commit().Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	// Verify the original error is wrapped and preserved
	assert.ErrorIs(t, err, dbErr, "should wrap the original database error")
}

func TestDeleteLimitCommand_Execute_NilUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(context.Background(), uuid.Nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidID)
}

func TestDeleteLimitCommand_Execute_ContextCancellation(t *testing.T) {
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

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)
	err := cmd.Execute(ctx, testutil.MustDeterministicUUID(60))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestDeleteLimitCommand_Execute_BeginTxError verifies BeginTx error handling.
func TestDeleteLimitCommand_Execute_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(70)
	now := testutil.FixedTime()

	inactiveLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(71))}},
		Status:    model.LimitStatusInactive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(inactiveLimit, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().
		UpdateStatusWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	auditWriter.EXPECT().
		RecordLimitEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr)
}

// TestDeleteLimitCommand_Execute_AuditError_Rollback verifies audit failure triggers rollback.
func TestDeleteLimitCommand_Execute_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(80)
	now := testutil.FixedTime()

	inactiveLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(81))}},
		Status:    model.LimitStatusInactive,
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
		Return(inactiveLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusDeleted, gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventLimitDeleted,
				model.AuditActionDelete,
				limitID,
				gomock.Any(),
				gomock.Nil(),
				"Limit deleted via API",
			).
			Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr)
}

// TestDeleteLimitCommand_Execute_CommitError verifies Commit error handling.
func TestDeleteLimitCommand_Execute_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(90)
	now := testutil.FixedTime()

	inactiveLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(91))}},
		Status:    model.LimitStatusInactive,
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
		Return(inactiveLimit, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), limitID, model.LimitStatusDeleted, gomock.Any()).
			Return(nil),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),
				gomock.AssignableToTypeOf(mockTx),
				model.AuditEventLimitDeleted,
				model.AuditActionDelete,
				limitID,
				gomock.Any(),
				gomock.Nil(),
				"Limit deleted via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr)
}

// TestDeleteLimitCommand_Execute_Success_NilAuditWriter verifies that the
// transactional callback commits successfully when auditWriter is nil: the
// status update runs and Commit is invoked without any audit call.
// This pins the `auditWriter == nil` short-circuit branch inside executeInTx.
// The transition exercised is INACTIVE -> DELETED (valid per validStatusTransitions;
// ACTIVE -> DELETED is not permitted).
func TestDeleteLimitCommand_Execute_Success_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(280)
	now := testutil.FixedTime()

	inactiveLimit := &model.Limit{
		ID:        limitID,
		Name:      "Test Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(281))}},
		Status:    model.LimitStatusInactive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(inactiveLimit, nil)

	// Nil audit branch: BeginTx -> UpdateStatusWithTx -> Commit. No audit call.
	expectLimitStatusTxSuccessNoAudit(t, txBeginner, mockTx, mockRepo, limitID, model.LimitStatusDeleted)

	cmd, cmdErr := NewDeleteLimitCommand(mockRepo, testutil.NewDefaultMockClock(), nil, txBeginner)
	require.NoError(t, cmdErr)

	err := cmd.Execute(ctx, limitID)

	require.NoError(t, err)
}
