// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
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
// 10 and any sub-unitary amount (0 < x < 1) collapsed to 0 — silently under-holding,
// or wholly dropping, held capacity. Amounts are compared with decimal.Equal so the
// check is exact and insensitive to scale / trailing zeros.
//
// Tests here stay sequential (no t.Parallel): newResolverForTest wires
// SetupTestTracing, which installs a process-global tracer provider under a mutex —
// a parallel-gate exception, not an omission.
func TestResolveReservations_PreservesFractionalAmount(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(9100)
	limitID := testutil.MustDeterministicUUID(9101)

	tests := []struct {
		name   string
		amount string
	}{
		{name: "fractional above one preserves the cents", amount: "10.5"},
		{name: "sub-unitary preserves the whole fraction", amount: "0.99"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			checker := newResolverForTest(t, []model.Limit{
				{
					ID:        limitID,
					Name:      "Daily fractional cap",
					LimitType: model.LimitTypeDaily,
					MaxAmount: decimal.RequireFromString("20.75"),
					Currency:  "USD",
					Scopes:    []model.Scope{{AccountID: &accountID}},
					Status:    model.LimitStatusActive,
				},
			})

			want := decimal.RequireFromString(tt.amount)
			wantMax := decimal.RequireFromString("20.75")

			input, err := model.NewCheckLimitsInput(
				want,
				"USD",
				accountID,
				nil, nil, nil, nil, nil,
				testutil.DefaultTestTime,
			)
			require.NoError(t, err)

			specs, denied, err := checker.ResolveReservations(context.Background(), input)
			require.NoError(t, err)
			require.False(t, denied, "%s against a 20.75 cap must not be denied", tt.amount)
			require.Len(t, specs, 1)

			require.True(t, want.Equal(specs[0].Amount),
				"reservation amount must preserve the fraction, not truncate %s -> integer; got %s",
				tt.amount, specs[0].Amount)

			require.True(t, wantMax.Equal(specs[0].MaxAmount),
				"reservation cap must preserve the fraction, not truncate 20.75 -> integer; got %s",
				specs[0].MaxAmount)
		})
	}
}
