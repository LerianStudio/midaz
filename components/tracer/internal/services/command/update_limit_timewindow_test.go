// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// TestUpdateLimit_TimeWindow verifies that time window fields are handled correctly during update.
func TestUpdateLimit_TimeWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	clk := testutil.NewDefaultMockClock()

	// auditWriter is nil here: the command still commits the tx, just without audit persistence.
	cmd, err := NewUpdateLimitCommand(mockRepo, clk, nil, txBeginner)
	require.NoError(t, err)

	limitID := testutil.MustDeterministicUUID(1)
	now := testutil.FixedTime().UTC()

	// Existing limit WITHOUT time window
	existingLimit := &model.Limit{
		ID:              limitID,
		Name:            "Test Limit",
		LimitType:       model.LimitTypeDaily,
		MaxAmount:       decimal.RequireFromString("1000.00"),
		Currency:        "BRL",
		Scopes:          []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
		Status:          model.LimitStatusActive,
		ActiveTimeStart: nil, // NO time window initially
		ActiveTimeEnd:   nil,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Mock: GetByID returns existing limit
	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(existingLimit, nil)

	// Prepare time window values before mock setup so we can verify exact values
	startTime, err := model.NewTimeOfDay("06:00")
	require.NoError(t, err)
	endTime, err := model.NewTimeOfDay("20:00")
	require.NoError(t, err)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, limit *model.Limit) error {
				require.NotNil(t, limit.ActiveTimeStart)
				require.NotNil(t, limit.ActiveTimeEnd)
				assert.True(t, limit.ActiveTimeStart.Equal(startTime), "ActiveTimeStart should be 06:00")
				assert.True(t, limit.ActiveTimeEnd.Equal(endTime), "ActiveTimeEnd should be 20:00")
				return nil
			}),
		mockTx.EXPECT().Commit().Return(nil),
	)

	input := &UpdateLimitInput{
		ActiveTimeStart: &startTime,
		ActiveTimeEnd:   &endTime,
	}

	updatedLimit, err := cmd.Execute(context.Background(), limitID, input)
	require.NoError(t, err)

	require.NotNil(t, updatedLimit.ActiveTimeStart, "ActiveTimeStart should be updated")
	require.NotNil(t, updatedLimit.ActiveTimeEnd, "ActiveTimeEnd should be updated")
	assert.Equal(t, startTime, *updatedLimit.ActiveTimeStart, "ActiveTimeStart should match input")
	assert.Equal(t, endTime, *updatedLimit.ActiveTimeEnd, "ActiveTimeEnd should match input")
}

// TestUpdateLimit_CustomPeriod verifies that custom period fields are handled correctly during update.
func TestUpdateLimit_CustomPeriod(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := NewMockLimitRepository(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	clk := testutil.NewDefaultMockClock()

	cmd, err := NewUpdateLimitCommand(mockRepo, clk, nil, txBeginner)
	require.NoError(t, err)

	limitID := testutil.MustDeterministicUUID(1)
	now := testutil.FixedTime().UTC()

	// Initial dates: Nov 27-28, 2026 (Black Friday)
	initialStart := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	initialEnd := time.Date(2026, 11, 28, 23, 59, 59, 0, time.UTC)

	existingLimit := &model.Limit{
		ID:              limitID,
		Name:            "Black Friday Limit",
		LimitType:       model.LimitTypeCustom,
		MaxAmount:       decimal.RequireFromString("10000.00"),
		Currency:        "BRL",
		Scopes:          []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
		Status:          model.LimitStatusActive,
		CustomStartDate: &initialStart,
		CustomEndDate:   &initialEnd,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Mock: GetByID returns existing limit
	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		Return(existingLimit, nil)

	// Mock: UpdateWithTx should be called with new custom dates
	newStart := "2026-12-01T00:00:00Z"
	newEnd := "2026-12-02T23:59:59Z"
	expectedStart, parseErr := time.Parse(time.RFC3339, newStart)
	require.NoError(t, parseErr)
	expectedEnd, parseErr := time.Parse(time.RFC3339, newEnd)
	require.NoError(t, parseErr)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, limit *model.Limit) error {
				require.NotNil(t, limit.CustomStartDate, "Update should have CustomStartDate set")
				require.NotNil(t, limit.CustomEndDate, "Update should have CustomEndDate set")
				assert.Equal(t, expectedStart.UTC(), limit.CustomStartDate.UTC(), "CustomStartDate should match input")
				assert.Equal(t, expectedEnd.UTC(), limit.CustomEndDate.UTC(), "CustomEndDate should match input")
				return nil
			}),
		mockTx.EXPECT().Commit().Return(nil),
	)

	input := &UpdateLimitInput{
		CustomStartDate: &newStart,
		CustomEndDate:   &newEnd,
	}

	updatedLimit, err := cmd.Execute(context.Background(), limitID, input)
	require.NoError(t, err)

	require.NotNil(t, updatedLimit.CustomStartDate, "CustomStartDate should be updated")
	require.NotNil(t, updatedLimit.CustomEndDate, "CustomEndDate should be updated")
	assert.Equal(t, expectedStart.UTC(), updatedLimit.CustomStartDate.UTC(), "CustomStartDate should match input")
	assert.Equal(t, expectedEnd.UTC(), updatedLimit.CustomEndDate.UTC(), "CustomEndDate should match input")
}
