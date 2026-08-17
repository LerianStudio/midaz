// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestResolveReservations_CarriesCounterRetentionExpiry(t *testing.T) {
	serverNow := testutil.FixedTime()
	accountID := testutil.MustDeterministicUUID(7391)
	customStart := serverNow.Add(-time.Hour)
	customEnd := serverNow.Add(24 * time.Hour)

	tests := []struct {
		name           string
		limitType      model.LimitType
		customStart    *time.Time
		customEnd      *time.Time
		expectedExpiry time.Time
	}{
		{
			name:           "daily counter outlives its reset by the retention window",
			limitType:      model.LimitTypeDaily,
			expectedExpiry: model.CalculateResetAt(model.LimitTypeDaily, serverNow).AddDate(0, 0, trcConstant.CounterRetentionDays),
		},
		{
			name:           "weekly counter outlives its reset by the retention window",
			limitType:      model.LimitTypeWeekly,
			expectedExpiry: model.CalculateResetAt(model.LimitTypeWeekly, serverNow).AddDate(0, 0, trcConstant.CounterRetentionDays),
		},
		{
			name:           "monthly counter outlives its reset by the retention window",
			limitType:      model.LimitTypeMonthly,
			expectedExpiry: model.CalculateResetAt(model.LimitTypeMonthly, serverNow).AddDate(0, 0, trcConstant.CounterRetentionDays),
		},
		{
			name:           "custom counter outlives the custom end by the retention window",
			limitType:      model.LimitTypeCustom,
			customStart:    &customStart,
			customEnd:      &customEnd,
			expectedExpiry: customEnd.AddDate(0, 0, trcConstant.CounterRetentionDays),
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			limitRepo := NewMockLimitRepository(ctrl)
			counterRepo := NewMockUsageCounterRepository(ctrl)
			service, err := NewLimitChecker(limitRepo, counterRepo, testutil.NewMockClock(serverNow))
			require.NoError(t, err)

			status := model.LimitStatusActive
			currency := "USD"
			limitID := testutil.MustDeterministicUUID(int64(7392 + i))
			limitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
				Status:   &status,
				Currency: &currency,
				Limit:    trcConstant.MaxPaginationLimit,
			}).Return(&model.ListLimitsResult{
				Limits: []model.Limit{{
					ID:              limitID,
					Name:            "Reservation retention proof",
					LimitType:       tc.limitType,
					MaxAmount:       decimal.NewFromInt(1000),
					Currency:        currency,
					Scopes:          []model.Scope{{AccountID: &accountID}},
					Status:          status,
					CustomStartDate: tc.customStart,
					CustomEndDate:   tc.customEnd,
				}},
			}, nil)

			input := &model.CheckLimitsInput{
				Amount:               decimal.NewFromInt(100),
				Currency:             currency,
				AccountID:            accountID,
				TransactionTimestamp: serverNow,
			}
			specs, denied, err := service.ResolveReservations(context.Background(), input)
			require.NoError(t, err)
			assert.False(t, denied)
			require.Len(t, specs, 1)
			require.NotNil(t, specs[0].CounterExpiresAt)
			assert.Equal(t, tc.expectedExpiry, *specs[0].CounterExpiresAt)
		})
	}
}
