// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestGetLimitUsage_ResetAtAndCountersShareOneInstant pins that the usage
// snapshot answers "how much is spent" and "when does the cap refresh" from the
// SAME moment.
//
// The counters are selected for the period the service clock is in, but the
// boundary arrives already resolved on the limit the repository handed back,
// against the repository's own separate read of the wall clock. Two reads means
// two instants, and a request that straddles midnight, a Monday, or the first of
// a month gets one of each: the counters for the period that just ended next to
// the reset moment of the one that just began — a boundary the caller is told to
// wait for although it has already passed.
//
// The stored boundary here is deliberately far away, standing in for whatever
// the repository resolved it to.
func TestGetLimitUsage_ResetAtAndCountersShareOneInstant(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	limitID := testutil.MustDeterministicUUID(11)
	repositoryResolved := time.Date(2030, 9, 2, 0, 0, 0, 0, time.UTC)

	limitRepo := query.NewMockLimitRepository(ctrl)
	limitRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(&model.Limit{
		ID:        limitID,
		Name:      "daily cap",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.NewFromInt(1000),
		Asset:     "USD",
		Status:    model.LimitStatusActive,
		ResetAt:   &repositoryResolved,
	}, nil)

	// The period key pins the other half of the pair: the counters summed below
	// belong to the day the service clock is in.
	counterRepo := query.NewMockUsageCounterRepository(ctrl)
	counterRepo.EXPECT().GetByLimitID(gomock.Any(), limitID, "2026-03-15").
		Return([]model.UsageCounter{{CurrentUsage: decimal.NewFromInt(300)}}, nil)

	svc := NewLimitService(
		nil, nil, nil, nil, nil, nil,
		query.NewGetLimitQuery(limitRepo), nil, counterRepo, clock.NewFixedClock(now),
	)

	snapshot, err := svc.GetLimitUsage(context.Background(), limitID)
	require.NoError(t, err, "GetLimitUsage must succeed")
	require.NotNil(t, snapshot.ResetAt, "a daily limit MUST report a boundary")

	assert.Equal(t, time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC), snapshot.ResetAt.UTC(),
		"resetAt MUST be the boundary of the period the counters came from; reporting the boundary the repository resolved separately pairs one period's usage with another period's reset moment")
	assert.Equal(t, "300", snapshot.CurrentUsage.String(), "the counters themselves are unchanged")
}

// TestGetLimitUsage_CustomBoundaryIsNotRecomputed pins the other side: a CUSTOM
// limit does not recur, so its boundary is the end date the operator set and
// re-resolving against the clock must leave it alone.
func TestGetLimitUsage_CustomBoundaryIsNotRecomputed(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	limitID := testutil.MustDeterministicUUID(12)
	operatorEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	limitRepo := query.NewMockLimitRepository(ctrl)
	limitRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(&model.Limit{
		ID:        limitID,
		Name:      "campaign cap",
		LimitType: model.LimitTypeCustom,
		MaxAmount: decimal.NewFromInt(1000),
		Asset:     "USD",
		Status:    model.LimitStatusActive,
		ResetAt:   &operatorEnd,
	}, nil)

	counterRepo := query.NewMockUsageCounterRepository(ctrl)
	counterRepo.EXPECT().GetByLimitID(gomock.Any(), limitID, "custom").Return(nil, nil)

	svc := NewLimitService(
		nil, nil, nil, nil, nil, nil,
		query.NewGetLimitQuery(limitRepo), nil, counterRepo, clock.NewFixedClock(now),
	)

	snapshot, err := svc.GetLimitUsage(context.Background(), limitID)
	require.NoError(t, err)
	require.NotNil(t, snapshot.ResetAt)

	assert.Equal(t, operatorEnd, snapshot.ResetAt.UTC(),
		"a CUSTOM limit's boundary is the end date the operator set; recomputing it erases their own configuration")
}
