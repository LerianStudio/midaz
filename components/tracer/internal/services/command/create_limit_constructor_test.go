// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "tracer/internal/adapters/postgres/db"
	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
	"tracer/internal/testhelper"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// TestNewCreateLimitCommand_NilAuditWriter exercises the auditWriter-nil
// branch of the validating constructor.
func TestNewCreateLimitCommand_NilAuditWriter(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockLimitRepository(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)

	cmd, err := NewCreateLimitCommand(repo, testutil.NewDefaultMockClock(), nil, tx)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateLimitAuditWriter)
	assert.Nil(t, cmd)
}

// TestNewCreateLimitCommand_NilTxBeginner exercises the txBeginner-nil branch
// of the validating constructor. The tx beginner is required because limit
// creation and audit recording must be persisted atomically — deferring the
// nil check to first execution would conceal a DI bug behind a runtime error.
func TestNewCreateLimitCommand_NilTxBeginner(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockLimitRepository(ctrl)
	audit := NewMockAuditWriter(ctrl)

	cmd, err := NewCreateLimitCommand(repo, testutil.NewDefaultMockClock(), audit, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilCreateLimitTxBeginner)
	assert.Nil(t, cmd)
}

// TestCreateLimit_Success_TimeWindow drives the time-window-only branch
// (NewLimitWithTimeWindow) end-to-end through the atomic flow.
func TestCreateLimit_Success_TimeWindow(t *testing.T) {
	ctrl := gomock.NewController(t)

	startTime := testhelper.MustNewTimeOfDay("09:00")
	endTime := testhelper.MustNewTimeOfDay("18:00")
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
			DoAndReturn(func(_ context.Context, _ pgdb.DB, l *model.Limit) error {
				require.NotNil(t, l.ActiveTimeStart)
				require.NotNil(t, l.ActiveTimeEnd)
				assert.Equal(t, "09:00", l.ActiveTimeStart.String())
				assert.Equal(t, "18:00", l.ActiveTimeEnd.String())

				return nil
			}),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(), mockTx,
				model.AuditEventLimitCreated, model.AuditActionCreate,
				gomock.Any(), gomock.Nil(), gomock.Not(gomock.Nil()),
				"Limit created via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	mockTx.EXPECT().Rollback().Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:            "Daytime Card Limit",
		LimitType:       model.LimitTypeDaily,
		MaxAmount:       decimal.RequireFromString("500"),
		Currency:        "USD",
		Scopes:          []model.Scope{validScope},
		ActiveTimeStart: &startTime,
		ActiveTimeEnd:   &endTime,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.ID)
	require.NotNil(t, result.ActiveTimeStart)
	require.NotNil(t, result.ActiveTimeEnd)
}

// TestCreateLimit_Success_CustomPeriod drives the custom-period-only branch
// (NewLimitWithCustomPeriod) end-to-end through the atomic flow. The clock
// returns FixedTime() (2024-01-15) which is before the customStart so the
// period is in the future.
func TestCreateLimit_Success_CustomPeriod(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	startDate := "2026-11-27T00:00:00Z"
	endDate := "2026-12-03T23:59:59Z"

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, l *model.Limit) error {
				require.NotNil(t, l.CustomStartDate)
				require.NotNil(t, l.CustomEndDate)

				return nil
			}),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(), mockTx,
				model.AuditEventLimitCreated, model.AuditActionCreate,
				gomock.Any(), gomock.Nil(), gomock.Not(gomock.Nil()),
				"Limit created via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	mockTx.EXPECT().Rollback().Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:            "Black Friday Limit",
		LimitType:       model.LimitTypeCustom,
		MaxAmount:       decimal.RequireFromString("10000"),
		Currency:        "USD",
		Scopes:          []model.Scope{validScope},
		CustomStartDate: &startDate,
		CustomEndDate:   &endDate,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.ID)
}

// TestCreateLimit_Success_CustomPeriodAndTimeWindow drives the AC-09 branch
// (NewLimitWithCustomPeriodAndTimeWindow) end-to-end through the atomic flow.
func TestCreateLimit_Success_CustomPeriodAndTimeWindow(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	startDate := "2026-11-27T00:00:00Z"
	endDate := "2026-12-03T23:59:59Z"
	startTime := testhelper.MustNewTimeOfDay("09:00")
	endTime := testhelper.MustNewTimeOfDay("18:00")

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, l *model.Limit) error {
				require.NotNil(t, l.CustomStartDate)
				require.NotNil(t, l.CustomEndDate)
				require.NotNil(t, l.ActiveTimeStart)
				require.NotNil(t, l.ActiveTimeEnd)

				return nil
			}),
		auditWriter.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(), mockTx,
				model.AuditEventLimitCreated, model.AuditActionCreate,
				gomock.Any(), gomock.Nil(), gomock.Not(gomock.Nil()),
				"Limit created via API",
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	mockTx.EXPECT().Rollback().Times(0)

	cmd, err := NewCreateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	input := &CreateLimitInput{
		Name:            "Black Friday Daytime Limit",
		LimitType:       model.LimitTypeCustom,
		MaxAmount:       decimal.RequireFromString("10000"),
		Currency:        "USD",
		Scopes:          []model.Scope{validScope},
		CustomStartDate: &startDate,
		CustomEndDate:   &endDate,
		ActiveTimeStart: &startTime,
		ActiveTimeEnd:   &endTime,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.ID)
}

// TestCreateLimit_InvalidCustomStartFormat_NoTx exercises the parse failure
// path on customStartDate (RFC3339). The transaction must not begin —
// pre-tx invariant.
func TestCreateLimit_InvalidCustomStartFormat_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	badStart := "not a date"
	endDate := "2026-12-03T23:59:59Z"

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

	input := &CreateLimitInput{
		Name:            "Bad Start Limit",
		LimitType:       model.LimitTypeCustom,
		MaxAmount:       decimal.RequireFromString("100"),
		Currency:        "USD",
		Scopes:          []model.Scope{validScope},
		CustomStartDate: &badStart,
		CustomEndDate:   &endDate,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidCustomStartFormat)
	assert.Nil(t, result)
}

// TestCreateLimit_InvalidCustomEndFormat_NoTx exercises the parse failure
// path on customEndDate (RFC3339).
func TestCreateLimit_InvalidCustomEndFormat_NoTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	validScope := model.Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
	}

	startDate := "2026-11-27T00:00:00Z"
	badEnd := "definitely not RFC3339"

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

	input := &CreateLimitInput{
		Name:            "Bad End Limit",
		LimitType:       model.LimitTypeCustom,
		MaxAmount:       decimal.RequireFromString("100"),
		Currency:        "USD",
		Scopes:          []model.Scope{validScope},
		CustomStartDate: &startDate,
		CustomEndDate:   &badEnd,
	}

	result, err := cmd.Execute(context.Background(), input)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidCustomEndFormat)
	assert.Nil(t, result)
}
