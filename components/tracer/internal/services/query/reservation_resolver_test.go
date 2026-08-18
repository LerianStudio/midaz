// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// newResolverForTest wires a LimitCheckerService whose limit repository returns the
// supplied limits, so ResolveReservations can be driven without a real database.
func newResolverForTest(t *testing.T, limits []model.Limit) *LimitCheckerService {
	t.Helper()

	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	limitRepo := NewMockLimitRepository(ctrl)
	usageRepo := NewMockUsageCounterRepository(ctrl)

	limitRepo.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(&model.ListLimitsResult{Limits: limits, HasMore: false}, nil).
		AnyTimes()

	checker, err := NewLimitChecker(limitRepo, usageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	return checker
}

// TestResolveReservations_PreservesFractionalAmount is the money-path regression
// guard for the reservation seam: a fractional transaction amount must reach the
// reservation spec intact. Under the pre-fix int64 IntPart() hop, 10.50 collapsed to
// 10 — silently under-holding capacity. The assertion compares the canonical decimal
// string form so it is exact (no float equality).
func TestResolveReservations_PreservesFractionalAmount(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(9100)
	limitID := testutil.MustDeterministicUUID(9101)

	checker := newResolverForTest(t, []model.Limit{
		{
			ID:        limitID,
			Name:      "Daily fractional cap",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("20"),
			Currency:  "USD",
			Scopes:    []model.Scope{{AccountID: &accountID}},
			Status:    model.LimitStatusActive,
		},
	})

	input, err := model.NewCheckLimitsInput(
		decimal.RequireFromString("10.5"),
		"USD",
		accountID,
		nil, nil, nil, nil, nil,
		testutil.DefaultTestTime,
	)
	require.NoError(t, err)

	specs, denied, err := checker.ResolveReservations(context.Background(), input)
	require.NoError(t, err)
	require.False(t, denied, "10.50 against a 20.00 cap must not be denied")
	require.Len(t, specs, 1)

	require.Equal(t, "10.5", fmt.Sprintf("%v", specs[0].Amount),
		"reservation amount must preserve the fractional part, not truncate 10.50 -> 10")
}
