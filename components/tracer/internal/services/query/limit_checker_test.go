// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testhelper"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// errDatabase is a sentinel error for testing database error paths.
var errDatabase = errors.New("database error")

// serverPeriodKeyDaily and serverPeriodKeyMonthly are derived from the
// deterministic mock clock so period-key expectations stay in sync.
var (
	serverPeriodKeyDaily   = testutil.DefaultTestTime.Format("2006-01-02")
	serverPeriodKeyMonthly = testutil.DefaultTestTime.Format("2006-01")
)

// setupTest creates test context with tracing setup.
func setupTest(t *testing.T) context.Context {
	t.Helper()
	testutil.SetupTestTracing(t)

	return context.Background()
}

func TestNewLimitChecker(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)

	tests := []struct {
		name      string
		limitRepo LimitRepository
		usageRepo UsageCounterRepository
		clock     clock.Clock
		wantErr   bool
		wantErrIs error
	}{
		{
			name:      "valid repositories",
			limitRepo: mockLimitRepo,
			usageRepo: mockUsageRepo,
			clock:     testutil.NewDefaultMockClock(),
			wantErr:   false,
		},
		{
			name:      "nil limit repository",
			limitRepo: nil,
			usageRepo: mockUsageRepo,
			clock:     testutil.NewDefaultMockClock(),
			wantErr:   true,
			wantErrIs: constant.ErrLimitCheckerNilLimitRepo,
		},
		{
			name:      "nil usage counter repository",
			limitRepo: mockLimitRepo,
			usageRepo: nil,
			clock:     testutil.NewDefaultMockClock(),
			wantErr:   true,
			wantErrIs: constant.ErrLimitCheckerNilUsageCounterRepo,
		},
		{
			name:      "nil clock",
			limitRepo: mockLimitRepo,
			usageRepo: mockUsageRepo,
			clock:     nil,
			wantErr:   true,
			wantErrIs: constant.ErrLimitCheckerNilClock,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checker, err := NewLimitChecker(tc.limitRepo, tc.usageRepo, tc.clock)

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)
				assert.Nil(t, checker)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, checker)
			}
		})
	}
}

func TestLimitCheckerService_CheckLimits(t *testing.T) {
	// Test UUIDs
	limitID1 := testutil.MustDeterministicUUID(1)
	limitID2 := testutil.MustDeterministicUUID(2)
	limitID3 := testutil.MustDeterministicUUID(3)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	tests := []struct {
		name         string
		input        *model.CheckLimitsInput
		setupMocks   func(*MockLimitRepository, *MockUsageCounterRepository, pgdb.DB)
		wantAllowed  bool
		wantExceeded []uuid.UUID
		wantDetails  int
		wantErr      bool
		wantErrIs    error
	}{
		{
			name: "no active limits - allowed",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits:  []model.Limit{},
					HasMore: false,
				}, nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  0,
			wantErr:      false,
		},
		{
			name: "single DAILY limit - not exceeded",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Daily Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				scopeKey := "acct:" + accountID.String()
				// Atomic upsert: returns new usage (500 + 50 = 550)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("1000"), gomock.Any()).
					Return(decimal.RequireFromString("550"), nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "single DAILY limit - exceeded",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("600"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Daily Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				scopeKey := "acct:" + accountID.String()
				// Atomic upsert: returns ErrUsageCounterExceedsLimit when 500 + 600 > 1000
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("600"), decimal.RequireFromString("1000"), gomock.Any()).
					Return(decimal.RequireFromString("500"), constant.ErrUsageCounterExceedsLimit)
			},
			wantAllowed:  false,
			wantExceeded: []uuid.UUID{limitID1},
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "PER_TRANSACTION limit - not exceeded",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Per Transaction Limit",
							LimitType: model.LimitTypePerTransaction,
							MaxAmount: decimal.RequireFromString("100"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)
				// No usage counter calls for PER_TRANSACTION
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "PER_TRANSACTION limit - exceeded",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("150"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Per Transaction Limit",
							LimitType: model.LimitTypePerTransaction,
							MaxAmount: decimal.RequireFromString("100"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)
			},
			wantAllowed:  false,
			wantExceeded: []uuid.UUID{limitID1},
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "multiple limits - one exceeded",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("80"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Daily Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
						{
							ID:        limitID2,
							Name:      "Per Transaction Limit",
							LimitType: model.LimitTypePerTransaction,
							MaxAmount: decimal.RequireFromString("50"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				scopeKey := "acct:" + accountID.String()
				// First limit (DAILY) is atomically incremented (succeeds: 500 + 80 <= 1000)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("80"), decimal.RequireFromString("1000"), gomock.Any()).
					Return(decimal.RequireFromString("580"), nil)

				// Second limit (PER_TRANSACTION) is checked directly: 80 > 50 → exceeded
				// No rollback expectations - in transactional mode, caller does tx.Rollback()
			},
			wantAllowed:  false,
			wantExceeded: []uuid.UUID{limitID2},
			wantDetails:  2,
			wantErr:      false,
		},
		{
			name: "currency filter - no limits for currency (DB-level filtering)",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				// With DB-level currency filtering, query for USD returns empty list
				// (BRL limits are filtered out at database level)
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits:  []model.Limit{},
					HasMore: false,
				}, nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  0,
			wantErr:      false,
		},
		{
			name: "scope mismatch - limit not applicable",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				otherAccountID := testutil.MustDeterministicUUID(999)
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Other Account Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &otherAccountID}}, // Different account
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  0,
			wantErr:      false,
		},
		{
			name: "global limit - matches all scopes",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Global Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{}, // Empty scopes = global
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				// Global limit uses "global" scope key, not account-specific key.
				// This ensures all transactions aggregate under a single counter.
				scopeKey := "global"
				// Atomic upsert: returns new usage (0 + 50 = 50)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("1000"), gomock.Any()).
					Return(decimal.RequireFromString("50"), nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "invalid input - zero amount",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("0"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				// No mocks - validation fails before repository calls
			},
			wantAllowed: false,
			wantErr:     true,
			wantErrIs:   constant.ErrCheckLimitsInvalidAmount,
		},
		{
			name: "invalid input - nil accountID",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            uuid.Nil,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				// No mocks - validation fails before repository calls
			},
			wantAllowed: false,
			wantErr:     true,
			wantErrIs:   constant.ErrCheckLimitsInvalidAccountID,
		},
		{
			name: "MONTHLY limit period key",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID3,
							Name:      "Monthly Limit",
							LimitType: model.LimitTypeMonthly,
							MaxAmount: decimal.RequireFromString("5000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				scopeKey := "acct:" + accountID.String()
				periodKeyMonthly := serverPeriodKeyMonthly
				// Atomic upsert: returns new usage (1000 + 50 = 1050)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID3, scopeKey, periodKeyMonthly, decimal.RequireFromString("50"), decimal.RequireFromString("5000"), gomock.Any()).
					Return(decimal.RequireFromString("1050"), nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "boundary - amount equals remaining capacity",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("500"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Daily Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				scopeKey := "acct:" + accountID.String()
				// Atomic upsert: CurrentUsage 500 + Amount 500 == MaxAmount 1000 (exactly at limit, allowed)
				// Returns new usage (1000)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("500"), decimal.RequireFromString("1000"), gomock.Any()).
					Return(decimal.RequireFromString("1000"), nil)
			},
			wantAllowed:  true,
			wantExceeded: nil,
			wantDetails:  1,
			wantErr:      false,
		},
		{
			name: "error - LimitRepository.List returns error",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(nil, errDatabase)
			},
			wantAllowed: false,
			wantErr:     true,
			wantErrIs:   errDatabase,
		},
		{
			name: "error - UsageCounterRepository.UpsertAndIncrementAtomic returns error",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				status := model.LimitStatusActive
				currency := "USD"
				lr.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
					Status:   &status,
					Currency: &currency,
					Limit:    constant.MaxPaginationLimit,
				}).Return(&model.ListLimitsResult{
					Limits: []model.Limit{
						{
							ID:        limitID1,
							Name:      "Daily Limit",
							LimitType: model.LimitTypeDaily,
							MaxAmount: decimal.RequireFromString("1000"),
							Currency:  "USD",
							Scopes:    []model.Scope{{AccountID: &accountID}},
							Status:    model.LimitStatusActive,
						},
					},
					HasMore: false,
				}, nil)

				scopeKey := "acct:" + accountID.String()
				// Atomic upsert returns database error
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("1000"), gomock.Any()).
					Return(decimal.Zero, errDatabase)
			},
			wantAllowed: false,
			wantErr:     true,
			wantErrIs:   errDatabase,
		},
		{
			name: "invalid input - negative amount",
			input: &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("-1"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			},
			setupMocks: func(lr *MockLimitRepository, ucr *MockUsageCounterRepository, db pgdb.DB) {
				// No mocks - validation fails before repository calls
			},
			wantAllowed: false,
			wantErr:     true,
			wantErrIs:   constant.ErrCheckLimitsInvalidAmount,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockLimitRepo := NewMockLimitRepository(ctrl)
			mockUsageRepo := NewMockUsageCounterRepository(ctrl)
			mockDB := dbmocks.NewMockDB(ctrl)

			tc.setupMocks(mockLimitRepo, mockUsageRepo, mockDB)

			ctx := setupTest(t)

			checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
			require.NoError(t, err)

			output, err := checker.CheckLimits(ctx, mockDB, tc.input)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				}
				assert.Nil(t, output)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
				assert.Equal(t, tc.wantAllowed, output.Allowed)
				assert.Len(t, output.LimitUsageDetails, tc.wantDetails)

				if tc.wantExceeded != nil {
					assert.Equal(t, tc.wantExceeded, output.ExceededLimitIDs)
				} else {
					assert.Empty(t, output.ExceededLimitIDs)
				}
			}
		})
	}
}

func TestLimitCheckerService_CheckLimits_ConcurrentAccess(t *testing.T) {
	// This test verifies that CheckLimits handles concurrent requests correctly.
	// The key behaviors tested:
	// 1. Multiple goroutines can call CheckLimits simultaneously
	// 2. Each call uses atomic UpsertAndIncrementAtomic
	// 3. Increments happen atomically in DB

	limitID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	const numGoroutines = 10
	amountPerRequest := decimal.RequireFromString("10")

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	// Setup mock expectations for concurrent calls
	// Each goroutine will call List + UpsertAndIncrementAtomic
	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"), // High enough to not be exceeded
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil).Times(numGoroutines)

	scopeKey := "acct:" + accountID.String()

	// Each concurrent call uses atomic upsert (returns incremental usage values)
	// Use counter with mutex to simulate real atomic increment behavior
	var mu sync.Mutex
	callCounter := 0

	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKeyDaily,
		amountPerRequest,
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).DoAndReturn(func(
		ctx context.Context,
		db pgdb.DB,
		limitID uuid.UUID,
		scopeKey string,
		periodKey string,
		amount decimal.Decimal,
		maxAmount decimal.Decimal,
		expiresAt *time.Time,
	) (decimal.Decimal, error) {
		mu.Lock()
		callCounter++
		currentValue := callCounter * 10 // Each call increments by 10
		mu.Unlock()
		return decimal.RequireFromString(fmt.Sprintf("%d", currentValue)), nil
	}).Times(numGoroutines)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	// Run concurrent CheckLimits calls
	var wg sync.WaitGroup

	errors := make(chan error, numGoroutines)
	results := make(chan *model.CheckLimitsOutput, numGoroutines)

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := &model.CheckLimitsInput{
				Amount:               amountPerRequest,
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			}

			output, err := checker.CheckLimits(ctx, mockDB, input)
			if err != nil {
				errors <- err
				return
			}

			results <- output
		}()
	}

	wg.Wait()
	close(errors)
	close(results)

	// Verify no errors occurred
	for err := range errors {
		t.Errorf("Concurrent CheckLimits failed: %v", err)
	}

	// Verify all calls succeeded and were allowed
	successCount := 0
	for output := range results {
		assert.True(t, output.Allowed, "All concurrent requests should be allowed")
		successCount++
	}

	assert.Equal(t, numGoroutines, successCount, "All goroutines should complete successfully")
}

func TestLimitCheckerService_CheckLimits_TwoPhaseNoPartialIncrement(t *testing.T) {
	// This test verifies that when multiple limits are checked and one exceeds,
	// in transactional mode the caller is responsible for tx.Rollback() to undo any changes.

	limitID1 := testutil.MustDeterministicUUID(1)
	limitID2 := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	// Two limits: first one passes, second one exceeds
	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID1,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"), // Will NOT exceed
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
			{
				ID:        limitID2,
				Name:      "Per Transaction Limit",
				LimitType: model.LimitTypePerTransaction,
				MaxAmount: decimal.RequireFromString("50"), // Will exceed (amount is 80)
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	scopeKey := "acct:" + accountID.String()

	// First limit (DAILY) is atomically incremented (succeeds: 500 + 80 <= 1000)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("80"), decimal.RequireFromString("1000"), gomock.Any()).
		Return(decimal.RequireFromString("580"), nil)

	// Second limit (PER_TRANSACTION) is checked directly: 80 > 50 → exceeded
	// In transactional mode, caller does tx.Rollback() to undo counter increment

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("80"), // Exceeds PER_TRANSACTION limit of 50
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.False(t, output.Allowed, "Should be denied due to PER_TRANSACTION limit")
	assert.Contains(t, output.ExceededLimitIDs, limitID2, "PER_TRANSACTION limit should be exceeded")
	assert.Len(t, output.LimitUsageDetails, 2, "Should have details for both limits")
}

func TestLimitCheckerService_CheckLimits_LargeAmountNearInt64Max(t *testing.T) {
	// This test verifies behavior with large decimal amounts.
	// Tests that projected usage is compared correctly against maxAmount.

	limitID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	tests := []struct {
		name        string
		amount      decimal.Decimal
		maxAmount   decimal.Decimal
		wantAllowed bool
		wantErr     bool
		wantErrIs   error
		setupUpsert func(*MockUsageCounterRepository, string, pgdb.DB)
	}{
		{
			name:        "large amount within limits - allowed",
			amount:      decimal.RequireFromString("10000000000000"), // 10 trillion
			maxAmount:   decimal.RequireFromString("50000000000000"),
			wantAllowed: true,
			wantErr:     false,
			setupUpsert: func(ucr *MockUsageCounterRepository, scopeKey string, db pgdb.DB) {
				// Atomic upsert succeeds, returns new usage (10 trillion + 10 trillion = 20 trillion)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID, scopeKey, periodKeyDaily, decimal.RequireFromString("10000000000000"), decimal.RequireFromString("50000000000000"), gomock.Any()).
					Return(decimal.RequireFromString("20000000000000"), nil)
			},
		},
		{
			name:        "large amount exceeds limit - projected usage > maxAmount",
			amount:      decimal.RequireFromString("10000000000000000"),    // Very large amount (10 quadrillion)
			maxAmount:   decimal.RequireFromString("92233720368547758.07"), // MaxInt64 / 100 (~92 quadrillion)
			wantAllowed: false,                                             // DB returns ErrUsageCounterExceedsLimit
			wantErr:     false,                                             // No error, just exceeds limit
			wantErrIs:   nil,
			setupUpsert: func(ucr *MockUsageCounterRepository, scopeKey string, db pgdb.DB) {
				// Atomic upsert returns ErrUsageCounterExceedsLimit when currentUsage + amount > maxAmount
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID, scopeKey, periodKeyDaily, decimal.RequireFromString("10000000000000000"), decimal.RequireFromString("92233720368547758.07"), gomock.Any()).
					Return(decimal.RequireFromString("90000000000000000"), constant.ErrUsageCounterExceedsLimit)
			},
		},
		{
			name:        "amount exactly at remaining capacity - allowed",
			amount:      decimal.RequireFromString("10000000000"),
			maxAmount:   decimal.RequireFromString("100000000000"), // currentUsage + amount == maxAmount
			wantAllowed: true,
			wantErr:     false,
			setupUpsert: func(ucr *MockUsageCounterRepository, scopeKey string, db pgdb.DB) {
				// Atomic upsert succeeds at boundary (returns exactly maxAmount)
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID, scopeKey, periodKeyDaily, decimal.RequireFromString("10000000000"), decimal.RequireFromString("100000000000"), gomock.Any()).
					Return(decimal.RequireFromString("100000000000"), nil)
			},
		},
		{
			name:        "amount exceeds limit - not allowed but no error",
			amount:      decimal.RequireFromString("20000000000"),
			maxAmount:   decimal.RequireFromString("100000000000"), // currentUsage + amount > maxAmount
			wantAllowed: false,
			wantErr:     false,
			setupUpsert: func(ucr *MockUsageCounterRepository, scopeKey string, db pgdb.DB) {
				// Atomic upsert returns ErrUsageCounterExceedsLimit
				ucr.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), db, limitID, scopeKey, periodKeyDaily, decimal.RequireFromString("20000000000"), decimal.RequireFromString("100000000000"), gomock.Any()).
					Return(decimal.RequireFromString("90000000000"), constant.ErrUsageCounterExceedsLimit)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockLimitRepo := NewMockLimitRepository(ctrl)
			mockUsageRepo := NewMockUsageCounterRepository(ctrl)
			mockDB := dbmocks.NewMockDB(ctrl)

			status := model.LimitStatusActive
			currency := "USD"

			mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
				Status:   &status,
				Currency: &currency,
				Limit:    constant.MaxPaginationLimit,
				Cursor:   "",
			}).Return(&model.ListLimitsResult{
				Limits: []model.Limit{
					{
						ID:        limitID,
						Name:      "High Value Limit",
						LimitType: model.LimitTypeDaily,
						MaxAmount: tc.maxAmount,
						Currency:  "USD",
						Scopes:    []model.Scope{{AccountID: &accountID}},
						Status:    model.LimitStatusActive,
					},
				},
				HasMore: false,
			}, nil)

			scopeKey := "acct:" + accountID.String()
			tc.setupUpsert(mockUsageRepo, scopeKey, mockDB)

			ctx := setupTest(t)

			checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
			require.NoError(t, err)

			input := &model.CheckLimitsInput{
				Amount:               tc.amount,
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: timestamp,
			}

			output, err := checker.CheckLimits(ctx, mockDB, input)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
				assert.Equal(t, tc.wantAllowed, output.Allowed)
			}
		})
	}
}

func TestLimitUsageDetail_RemainingAmount_LargeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		limitAmount  decimal.Decimal
		currentUsage decimal.Decimal
		expected     decimal.Decimal
	}{
		{
			name:         "large limit with zero usage",
			limitAmount:  decimal.RequireFromString("92233720368547758.07"), // MaxInt64 / 100
			currentUsage: decimal.RequireFromString("0"),
			expected:     decimal.RequireFromString("92233720368547758.07"),
		},
		{
			name:         "large limit with large usage",
			limitAmount:  decimal.RequireFromString("92233720368547758.07"),
			currentUsage: decimal.RequireFromString("92233720368547758"),
			expected:     decimal.RequireFromString("0.07"), // MaxInt64/100 - 92233720368547758
		},
		{
			name:         "large limit exactly at max",
			limitAmount:  decimal.RequireFromString("92233720368547758.07"),
			currentUsage: decimal.RequireFromString("92233720368547758.07"),
			expected:     decimal.RequireFromString("0"),
		},
		{
			name:         "quadrillion scale values",
			limitAmount:  decimal.RequireFromString("50000000000000"),
			currentUsage: decimal.RequireFromString("30000000000000"),
			expected:     decimal.RequireFromString("20000000000000"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			detail := model.LimitUsageDetail{
				LimitAmount:  tc.limitAmount,
				CurrentUsage: tc.currentUsage,
			}

			result := detail.RemainingAmount()
			assert.True(t, tc.expected.Equal(result), "expected %s, got %s", tc.expected.String(), result.String())
		})
	}
}

func TestLimitCheckerService_CheckLimits_PaginationLoop(t *testing.T) {
	// This test verifies that getApplicableLimits correctly handles pagination
	// when HasMore=true, fetching multiple pages of results.

	limitID1 := testutil.MustDeterministicUUID(1)
	limitID2 := testutil.MustDeterministicUUID(2)
	limitID3 := testutil.MustDeterministicUUID(3)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	// First page returns 2 limits with HasMore=true
	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID1,
				Name:      "Daily Limit 1",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
			{
				ID:        limitID2,
				Name:      "Daily Limit 2",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("2000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore:    true,
		NextCursor: "cursor-page-2",
	}, nil)

	// Second page returns 1 limit with HasMore=false
	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "cursor-page-2",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID3,
				Name:      "Daily Limit 3",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("3000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore:    false,
		NextCursor: "",
	}, nil)

	// Expect UpsertAndIncrementAtomic for all 3 limits (none exceeded)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("1000"), gomock.Any()).
		Return(decimal.RequireFromString("50"), nil)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID2, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("2000"), gomock.Any()).
		Return(decimal.RequireFromString("50"), nil)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID3, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("3000"), gomock.Any()).
		Return(decimal.RequireFromString("50"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("50"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "All limits should pass")
	assert.Len(t, output.LimitUsageDetails, 3, "Should have details for all 3 limits from both pages")
	assert.Empty(t, output.ExceededLimitIDs)
}

func TestLimitCheckerService_CheckLimits_NilInput(t *testing.T) {
	// This test verifies that CheckLimits returns the appropriate error
	// when called with a nil input.

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	// Call with nil input
	output, err := checker.CheckLimits(ctx, mockDB, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsNilInput)
	assert.Nil(t, output)
}

func TestLimitCheckerService_CheckLimits_LargeDecimalValues(t *testing.T) {
	// This test verifies that large decimal values are handled correctly.
	// With decimal.Decimal, there is no overflow - arithmetic works normally.

	limitID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID,
				Name:      "High Value Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("92233720368547758.07"), // MaxInt64 / 100
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	scopeKey := "acct:" + accountID.String()

	// Atomic upsert returns ErrUsageCounterExceedsLimit when current (92233720368547758) + 1 > max (92233720368547758.07)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID, scopeKey, periodKeyDaily, decimal.RequireFromString("1"), decimal.RequireFromString("92233720368547758.07"), gomock.Any()).
		Return(decimal.RequireFromString("92233720368547758"), constant.ErrUsageCounterExceedsLimit)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("1"), // Small amount but would exceed limit
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.False(t, output.Allowed, "Should be denied because projected usage exceeds limit")
	assert.Contains(t, output.ExceededLimitIDs, limitID, "Limit should be marked as exceeded when projected > max")
	require.Len(t, output.LimitUsageDetails, 1)

	// CurrentUsage is projected usage (current + amount) when exceeded
	assert.True(t, decimal.RequireFromString("92233720368547759").Equal(output.LimitUsageDetails[0].CurrentUsage),
		"CurrentUsage should reflect projected usage (current + amount)")
}

// TestLimitChecker_MerchantScope verifies that merchant-scoped limits are correctly
// enforced when the CheckLimitsInput carries a MerchantID.
//
// This guards the Limits path against a class of defect where limits with a
// merchant scope were silently NOT enforced because CheckLimitsInput did not
// expose MerchantID and buildTransactionScope did not map it. The Rules path
// already handles MerchantID correctly (see ValidationRequest.ToTransactionScope);
// this test mirrors that expectation for the Limits path.
//
// Scenarios covered:
//  1. Limit scope MerchantID=M1, transaction MerchantID=M1 → scope MATCHES (limit applies).
//  2. Limit scope MerchantID=M1, transaction MerchantID=M2 → scope does NOT match.
//  3. Limit scope MerchantID=M1, transaction MerchantID=nil → scope does NOT match
//     (limit requires a merchant, transaction has none).
//  4. Limit scope with all nil fields → empty scope (global); matches regardless of merchant.
//
// The test also asserts that formatScopeString renders "merchant:<uuid>" when a
// merchant is present in the limit scope, so operators can read scope strings in
// the UsageDetails output.
func TestLimitChecker_MerchantScope(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	merchantID1 := testutil.MustDeterministicUUID(10)
	merchantID2 := testutil.MustDeterministicUUID(11)

	fixedTime := testutil.FixedTime()

	tests := []struct {
		name             string
		limitScope       model.Scope
		inputMerchantID  *uuid.UUID
		wantLimitMatches bool
	}{
		{
			name:             "merchant match: limit M1, tx M1 → scope applies",
			limitScope:       model.Scope{MerchantID: &merchantID1},
			inputMerchantID:  &merchantID1,
			wantLimitMatches: true,
		},
		{
			name:             "merchant mismatch: limit M1, tx M2 → scope does not apply",
			limitScope:       model.Scope{MerchantID: &merchantID1},
			inputMerchantID:  &merchantID2,
			wantLimitMatches: false,
		},
		{
			name:             "merchant missing on input: limit M1, tx nil does not match",
			limitScope:       model.Scope{MerchantID: &merchantID1},
			inputMerchantID:  nil,
			wantLimitMatches: false,
		},
		{
			name:             "all-nil limit scope (global) matches any transaction",
			limitScope:       model.Scope{},
			inputMerchantID:  &merchantID1,
			wantLimitMatches: true,
		},
		{
			// Exercises ptrMatches(pattern=nil, value=&M1) → TRUE via per-field
			// wildcard in a NON-empty scope (account is set, merchant is nil).
			// This is the most common real-world limit shape: account-scoped,
			// merchant-agnostic. A regression making MerchantID-wildcard behave
			// as "required" would pass the all-nil case (short-circuits on
			// len(scopes)==0 semantics) but fail here because the per-field
			// wildcard path is actually exercised.
			name:             "limit has account but no merchant (wildcard merchant), tx with merchant matches",
			limitScope:       model.Scope{AccountID: &accountID, MerchantID: nil},
			inputMerchantID:  &merchantID1,
			wantLimitMatches: true,
		},
		{
			// Composite scope: account matches but merchant does not → limit must NOT apply.
			// Guards against short-circuit bugs where Scope.Matches returns true after
			// account match without checking remaining fields. This is the canonical
			// false-positive shape for merchant-scoped limits.
			name:             "limit {account:A, merchant:M1}, tx {account:A, merchant:M2} → no match",
			limitScope:       model.Scope{AccountID: &accountID, MerchantID: &merchantID1},
			inputMerchantID:  &merchantID2,
			wantLimitMatches: false,
		},
		{
			// Composite positive: both account AND merchant match → limit applies.
			// Paired with the composite-negative case above so the scope-key
			// assertions below can exercise the "both tokens present" path and
			// catch regressions where `acct:` is silently dropped from the
			// composite key (which would merge counters across accounts that
			// happen to share a merchant).
			name:             "limit {account:A, merchant:M1}, tx {account:A, merchant:M1} → match",
			limitScope:       model.Scope{AccountID: &accountID, MerchantID: &merchantID1},
			inputMerchantID:  &merchantID1,
			wantLimitMatches: true,
		},
	}

	for _, tc := range tests {
		tc := tc // defensive shadow for pre-Go-1.22 semantics and t.Parallel safety

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Derive expectation inline from the scope itself — eliminates the
			// drift risk of maintaining a redundant boolean on each case.
			wantScopeHasMerchID := tc.limitScope.MerchantID != nil

			// Build the CheckLimitsInput the way ValidationRequest.ToCheckLimitsInput
			// builds it in production, threading MerchantID through to the scope
			// builder so the assertions below exercise the full propagation path.
			input := &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				AccountID:            accountID,
				MerchantID:           tc.inputMerchantID,
				TransactionTimestamp: fixedTime,
			}

			// buildTransactionScope MUST propagate MerchantID into the resulting Scope.
			txScope := buildTransactionScope(input)
			require.NotNil(t, txScope, "buildTransactionScope must not return nil for non-nil input")

			if tc.inputMerchantID == nil {
				assert.Nil(t, txScope.MerchantID, "txScope.MerchantID should be nil when input.MerchantID is nil")
			} else {
				require.NotNil(t, txScope.MerchantID, "txScope.MerchantID must be set when input.MerchantID is non-nil")
				require.Equal(t, *tc.inputMerchantID, *txScope.MerchantID, "txScope.MerchantID must equal input.MerchantID")
			}

			// Scope match expectation: limit scope matches transaction scope according
			// to Scope.Matches semantics (nil in limit = wildcard; non-nil in limit must
			// equal the corresponding field on the transaction, not nil).
			gotMatches := scopeMatchesLimit([]model.Scope{tc.limitScope}, txScope)
			require.Equal(t, tc.wantLimitMatches, gotMatches,
				"scopeMatchesLimit for limit %+v vs tx merchant %v", tc.limitScope, tc.inputMerchantID)

			// calculateScopeKeyFromScopes builds the counter-lookup key from the
			// LIMIT scope (not the transaction scope). The key MUST encode every
			// non-nil field in the limit scope so counters for M1 cannot collide
			// with counters for M2 when the limit is merchant-scoped. A bug that
			// forgets the merchant token in the key would silently cross-count
			// usage across merchants — the same class of defect as missing
			// MerchantID propagation, just moved one layer down.
			//
			// Only meaningful when the limit matches the transaction (otherwise
			// calculateScopeKeyFromScopes falls through to the defensive branch
			// using txScope, which isn't what we're pinning here).
			if tc.wantLimitMatches {
				scopeKey := calculateScopeKeyFromScopes([]model.Scope{tc.limitScope}, txScope)
				require.NotEmpty(t, scopeKey, "calculateScopeKeyFromScopes must return a non-empty key when limit matches")

				switch {
				case tc.limitScope.MerchantID != nil:
					assert.Contains(t, scopeKey, "merch:"+tc.limitScope.MerchantID.String(),
						"scope key must include merchant token to avoid counter collision across merchants")
					// Composite-key guard: when both AccountID and MerchantID are
					// set on the limit scope, the key MUST carry BOTH tokens. A
					// regression that drops `acct:` from {account, merchant} keys
					// would silently merge counters across different accounts that
					// share the same merchant. Asserting the merchant token alone
					// would not catch that failure mode.
					if tc.limitScope.AccountID != nil {
						assert.Contains(t, scopeKey, "acct:"+tc.limitScope.AccountID.String(),
							"composite scope key must include account token alongside merchant token")
					}
				case tc.limitScope.AccountID != nil:
					// Wildcard-partial case: account is set, merchant is not.
					// Key must pin the account and MUST NOT include a merchant
					// token (a spurious merch: in the key would fragment counters
					// per merchant when the limit was intended to be merchant-agnostic).
					assert.Contains(t, scopeKey, "acct:"+tc.limitScope.AccountID.String(),
						"scope key must include account token when limit scope has AccountID")
					assert.NotContains(t, scopeKey, "merch:",
						"scope key must not include merchant token when limit scope has no MerchantID")
				default:
					// All-nil (global) scope: canonical key is GlobalScopeKey.
					require.Equal(t, constant.GlobalScopeKey, scopeKey,
						"calculateScopeKeyFromScopes must return canonical GlobalScopeKey for all-nil scope")
				}
			}

			// formatScopeString must surface the merchant token for audit/debug output.
			scopeStr := formatScopeString([]model.Scope{tc.limitScope})
			if wantScopeHasMerchID {
				require.NotNil(t, tc.limitScope.MerchantID)
				assert.Contains(t, scopeStr, "merchant:"+tc.limitScope.MerchantID.String(),
					"formatScopeString must include merchant token when limit scope has MerchantID")
			} else {
				assert.NotContains(t, scopeStr, "merchant:",
					"formatScopeString must not include merchant token when limit scope has no MerchantID")
				assert.NotEmpty(t, scopeStr, "formatScopeString must return non-empty string")

				// Tighter assertion for the all-nil (global) scope: canonical
				// rendering is constant.GlobalScopeKey ("global"). A bug
				// returning "" would be caught by NotEmpty above, but pinning
				// the exact canonical value catches subtler regressions (e.g.,
				// returning "()" or "(global)") that NotContains/NotEmpty miss.
				if tc.limitScope.AccountID == nil && tc.limitScope.SegmentID == nil &&
					tc.limitScope.PortfolioID == nil && tc.limitScope.MerchantID == nil &&
					tc.limitScope.TransactionType == nil && tc.limitScope.SubType == nil {
					require.Equal(t, constant.GlobalScopeKey, scopeStr,
						"formatScopeString must return canonical GlobalScopeKey for all-nil scope")
				}
			}
		})
	}
}

func TestFormatScopeString(t *testing.T) {
	accountID1 := testutil.MustDeterministicUUID(1)
	accountID2 := testutil.MustDeterministicUUID(2)
	segmentID := testutil.MustDeterministicUUID(3)
	portfolioID := testutil.MustDeterministicUUID(4)
	merchantID := testutil.MustDeterministicUUID(5)
	transactionType := model.TransactionTypeCard
	subType := "online"

	tests := []struct {
		name     string
		scopes   []model.Scope
		expected string
	}{
		{
			name:     "empty scopes returns global",
			scopes:   []model.Scope{},
			expected: "global",
		},
		{
			name:     "nil scopes returns global",
			scopes:   nil,
			expected: "global",
		},
		{
			name: "single scope with one field",
			scopes: []model.Scope{
				{AccountID: &accountID1},
			},
			expected: "(account:" + accountID1.String() + ")",
		},
		{
			name: "single scope with multiple fields",
			scopes: []model.Scope{
				{
					AccountID: &accountID1,
					SegmentID: &segmentID,
				},
			},
			expected: "(account:" + accountID1.String() + ",segment:" + segmentID.String() + ")",
		},
		{
			name: "multiple scopes each with one field",
			scopes: []model.Scope{
				{AccountID: &accountID1},
				{AccountID: &accountID2},
			},
			expected: "(account:" + accountID1.String() + ") OR (account:" + accountID2.String() + ")",
		},
		{
			name: "multiple scopes with different fields",
			scopes: []model.Scope{
				{
					AccountID: &accountID1,
					SegmentID: &segmentID,
				},
				{AccountID: &accountID2},
			},
			expected: "(account:" + accountID1.String() + ",segment:" + segmentID.String() + ") OR (account:" + accountID2.String() + ")",
		},
		{
			name: "scope with all fields",
			scopes: []model.Scope{
				{
					AccountID:       &accountID1,
					SegmentID:       &segmentID,
					PortfolioID:     &portfolioID,
					MerchantID:      &merchantID,
					TransactionType: &transactionType,
					SubType:         &subType,
				},
			},
			expected: "(account:" + accountID1.String() + ",segment:" + segmentID.String() + ",portfolio:" + portfolioID.String() + ",merchant:" + merchantID.String() + ",transactionType:CARD,subType:online)",
		},
		{
			name: "scope with empty fields returns global",
			scopes: []model.Scope{
				{}, // all nil fields
			},
			expected: "global",
		},
		{
			name: "mixed: one scope with fields, one empty",
			scopes: []model.Scope{
				{AccountID: &accountID1},
				{}, // empty scope
			},
			expected: "(account:" + accountID1.String() + ")",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatScopeString(tc.scopes)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildTransactionScope(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	segmentID := testutil.MustDeterministicUUID(2)
	portfolioID := testutil.MustDeterministicUUID(3)
	transactionType := model.TransactionTypeCard
	subType := "online"

	tests := []struct {
		name     string
		input    *model.CheckLimitsInput
		validate func(*testing.T, *model.Scope)
	}{
		{
			name:  "nil input returns nil scope",
			input: nil,
			validate: func(t *testing.T, scope *model.Scope) {
				assert.Nil(t, scope)
			},
		},
		{
			name: "full input builds complete scope",
			input: &model.CheckLimitsInput{
				AccountID:            accountID,
				SegmentID:            &segmentID,
				PortfolioID:          &portfolioID,
				TransactionType:      &transactionType,
				SubType:              &subType,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				TransactionTimestamp: testutil.FixedTime(),
			},
			validate: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope)
				assert.Equal(t, accountID, *scope.AccountID)
				assert.Equal(t, segmentID, *scope.SegmentID)
				assert.Equal(t, portfolioID, *scope.PortfolioID)
				assert.Equal(t, transactionType, *scope.TransactionType)
				assert.Equal(t, subType, *scope.SubType)
			},
		},
		{
			name: "minimal input builds scope with account only",
			input: &model.CheckLimitsInput{
				AccountID:            accountID,
				Amount:               decimal.RequireFromString("50"),
				Currency:             "USD",
				TransactionTimestamp: testutil.FixedTime(),
			},
			validate: func(t *testing.T, scope *model.Scope) {
				require.NotNil(t, scope)
				assert.Equal(t, accountID, *scope.AccountID)
				assert.Nil(t, scope.SegmentID)
				assert.Nil(t, scope.PortfolioID)
				assert.Nil(t, scope.TransactionType)
				assert.Nil(t, scope.SubType)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := buildTransactionScope(tc.input)
			tc.validate(t, result)
		})
	}
}

func TestScopeMatchesLimit(t *testing.T) {
	t.Parallel()

	accountID1 := testutil.MustDeterministicUUID(1)
	accountID2 := testutil.MustDeterministicUUID(2)
	segmentID := testutil.MustDeterministicUUID(3)

	tests := []struct {
		name        string
		limitScopes []model.Scope
		txScope     *model.Scope
		expected    bool
	}{
		{
			name:        "empty limit scopes (global) matches any transaction",
			limitScopes: []model.Scope{},
			txScope:     &model.Scope{AccountID: &accountID1},
			expected:    true,
		},
		{
			name:        "nil limit scopes (global) matches any transaction",
			limitScopes: nil,
			txScope:     &model.Scope{AccountID: &accountID1},
			expected:    true,
		},
		{
			name:        "nil transaction scope doesn't match non-global limit",
			limitScopes: []model.Scope{{AccountID: &accountID1}},
			txScope:     nil,
			expected:    false,
		},
		{
			name:        "matching account scope",
			limitScopes: []model.Scope{{AccountID: &accountID1}},
			txScope:     &model.Scope{AccountID: &accountID1},
			expected:    true,
		},
		{
			name:        "non-matching account scope",
			limitScopes: []model.Scope{{AccountID: &accountID1}},
			txScope:     &model.Scope{AccountID: &accountID2},
			expected:    false,
		},
		{
			name:        "multiple limit scopes - one matches",
			limitScopes: []model.Scope{{AccountID: &accountID1}, {AccountID: &accountID2}},
			txScope:     &model.Scope{AccountID: &accountID2},
			expected:    true,
		},
		{
			name:        "limit scope requires segment but transaction missing segment",
			limitScopes: []model.Scope{{AccountID: &accountID1, SegmentID: &segmentID}},
			txScope:     &model.Scope{AccountID: &accountID1}, // Missing segment
			expected:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := scopeMatchesLimit(tc.limitScopes, tc.txScope)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCheckLimits_ServerTimestamp(t *testing.T) {
	t.Parallel()

	// Seed 8152-8159
	limitID := testutil.MustDeterministicUUID(8152)
	accountID := testutil.MustDeterministicUUID(8153)

	// Server clock: 2024-01-15 10:30:00 UTC
	serverTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	// Client timestamp: 2024-01-14 08:00:00 UTC (yesterday - the attacker's manipulated date)
	clientTimestamp := time.Date(2024, 1, 14, 8, 0, 0, 0, time.UTC)

	// The expected period key MUST be based on server date, NOT client date
	expectedPeriodKeyDaily := "2024-01-15"
	// Client would compute period key "2024-01-14" — must never be used

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: UpsertAndIncrementAtomic must be called with server-date period key "2024-01-15",
	// NOT the client-supplied "2024-01-14". This is the core security verification.
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID, scopeKey, expectedPeriodKeyDaily, decimal.RequireFromString("100"), decimal.RequireFromString("1000"), gomock.Any()).
		Return(decimal.RequireFromString("600"), nil) // Returns new usage (500 + 100)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: clientTimestamp, // Attacker-controlled: yesterday
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "Transaction should be allowed (500 + 100 <= 1000)")
	assert.Len(t, output.LimitUsageDetails, 1)

	// If the mock for UpsertAndIncrementAtomic with "2024-01-15" was NOT called,
	// gomock will fail the test -- proving the server timestamp was used.
}

func TestCheckLimits_ServerTimestamp_Monthly(t *testing.T) {
	t.Parallel()

	// Seed 8160-8167
	limitID := testutil.MustDeterministicUUID(8160)
	accountID := testutil.MustDeterministicUUID(8161)

	// Server clock: 2024-02-01 00:05:00 UTC (first day of February)
	serverTime := time.Date(2024, 2, 1, 0, 5, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	// Client timestamp: 2024-01-31 23:55:00 UTC (last day of January - boundary attack)
	clientTimestamp := time.Date(2024, 1, 31, 23, 55, 0, 0, time.UTC)

	// The expected period key MUST be based on server date (February), NOT client date (January)
	expectedPeriodKeyMonthly := "2024-02"

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID,
				Name:      "Monthly Limit",
				LimitType: model.LimitTypeMonthly,
				MaxAmount: decimal.RequireFromString("5000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: period key must be "2024-02" (server month), not "2024-01" (client month)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID, scopeKey, expectedPeriodKeyMonthly, decimal.RequireFromString("200"), decimal.RequireFromString("5000"), gomock.Any()).
		Return(decimal.RequireFromString("2200"), nil) // Returns new usage (2000 + 200)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("200"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: clientTimestamp, // Attacker-controlled: last day of previous month
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "Transaction should be allowed (2000 + 200 <= 5000)")
	assert.Len(t, output.LimitUsageDetails, 1)
}

func TestCheckLimits_PerTransactionUnaffectedByClock(t *testing.T) {
	t.Parallel()

	// Seed 8170-8177
	limitID := testutil.MustDeterministicUUID(8170)
	accountID := testutil.MustDeterministicUUID(8171)

	// Server clock: some arbitrary date - should NOT matter for PER_TRANSACTION
	serverTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	// Client timestamp: completely different date - also should NOT matter
	clientTimestamp := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID,
				Name:      "Per Transaction Limit",
				LimitType: model.LimitTypePerTransaction,
				MaxAmount: decimal.RequireFromString("500"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	// No UsageCounterRepository calls expected for PER_TRANSACTION limits.
	// PER_TRANSACTION checks amount directly against maxAmount with no counters.

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("200"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: clientTimestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "200 <= 500 should be allowed for PER_TRANSACTION")
	assert.Empty(t, output.ExceededLimitIDs)
	assert.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.Equal(t, limitID, detail.LimitID)
	assert.True(t, decimal.Zero.Equal(detail.CurrentUsage), "PER_TRANSACTION has no persistent usage, should be 0")
	assert.True(t, decimal.RequireFromString("200").Equal(detail.AttemptedAmount))
	assert.False(t, detail.Exceeded)

	// Verify that gomock saw NO calls to GetOrCreateForUpdate or IncrementAtomic.
	// This confirms PER_TRANSACTION does not touch usage counters at all,
	// regardless of what clock is injected.
}

func TestLimitCheckerService_CheckLimits_PreCheckGetUsageError(t *testing.T) {
	// This test verifies that when amount > maxAmount (pre-check path),
	// and GetUsageForLimits fails, the error is propagated instead of being silently ignored.

	limitID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(100)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	// Mock limit with maxAmount=100, but we'll try to transact 500 (triggers pre-check)
	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("100"), // Amount=500 > 100 -> pre-check
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	scopeKey := "acct:" + accountID.String()

	// GetUsageForLimits fails (DB error)
	expectedErr := errors.New("database connection lost")
	mockUsageRepo.EXPECT().GetUsageForLimits(gomock.Any(), mockDB, []uuid.UUID{limitID}, scopeKey, periodKeyDaily).
		Return(nil, expectedErr)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("500"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	// Should return error instead of silently using zero usage
	require.Error(t, err, "Should return error when GetUsageForLimits fails in pre-check path")
	assert.Contains(t, err.Error(), "failed to get existing usage for pre-check")
	assert.Nil(t, output, "Output should be nil on error")
}

func TestLimitCheckerService_CheckLimits_ScopeKeyPerLimit(t *testing.T) {
	// This test verifies that scopeKey is calculated per limit based on limit's scope,
	// not once for all limits based on transaction scope.
	// This prevents counter fragmentation when limits have different granularities.

	limitID1 := testutil.MustDeterministicUUID(1) // Account-only limit
	limitID2 := testutil.MustDeterministicUUID(2) // Account+Segment limit
	accountID := testutil.MustDeterministicUUID(100)
	segmentID := testutil.MustDeterministicUUID(200)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID1,
				Name:      "Account Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}}, // Account-only scope
				Status:    model.LimitStatusActive,
			},
			{
				ID:        limitID2,
				Name:      "Account+Segment Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("500"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID, SegmentID: &segmentID}}, // Account+Segment scope
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	// Limit 1: Account-only scope → should use "acct:X" key (NOT "acct:X:seg:Y")
	scopeKey1 := "acct:" + accountID.String()
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID1,
		scopeKey1, // Account-only key
		periodKeyDaily,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	// Limit 2: Account+Segment scope → should use "acct:X|seg:Y" key (pipe separator)
	scopeKey2 := "acct:" + accountID.String() + "|seg:" + segmentID.String()
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID2,
		scopeKey2, // Account+Segment key
		periodKeyDaily,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("500"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	// Transaction has both AccountID and SegmentID
	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		SegmentID:            &segmentID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	assert.True(t, output.Allowed, "Both limits should allow the transaction")
	assert.Len(t, output.LimitUsageDetails, 2, "Should have 2 limit details")

	// Verify both limits were checked
	limitIDs := []uuid.UUID{output.LimitUsageDetails[0].LimitID, output.LimitUsageDetails[1].LimitID}
	assert.Contains(t, limitIDs, limitID1, "Should include account-only limit")
	assert.Contains(t, limitIDs, limitID2, "Should include account+segment limit")
}

// =============================================================================
// Time Window Skip Logic Tests
// =============================================================================
// These tests verify that limits with time windows (activeTimeStart/activeTimeEnd)
// are correctly skipped when the server timestamp is outside the configured window.
// Seeds: 9000-9099 range.

// TestCheckLimits_TimeWindow_OutsideWindow_Skipped verifies that limits with time windows
// are skipped when the transaction timestamp (server clock) is outside the window.
// The limit should appear in LimitUsageDetails with Skipped=true and SkipReason="outside_time_window".
// Counter should NOT be incremented.
// Seeds: 9000-9009
func TestCheckLimits_TimeWindow_OutsideWindow_Skipped(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9000)
	accountID := testutil.MustDeterministicUUID(9001)

	// Server clock at 14:00 UTC - OUTSIDE the 20:00-06:00 overnight window
	serverTime := time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	// Create limit with overnight time window (20:00 to 06:00)
	activeTimeStart := testhelper.MustNewTimeOfDay("20:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("06:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Overnight Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart,
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: NO counter operations should be called when limit is skipped
	// If the implementation calls UpsertAndIncrementAtomic, this test fails.

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)

	// Transaction should be allowed (skipped limits don't block)
	assert.True(t, output.Allowed, "Transaction should be allowed when limit is skipped")

	// Limit should appear in usage details
	require.Len(t, output.LimitUsageDetails, 1, "Should have 1 limit detail even when skipped")

	detail := output.LimitUsageDetails[0]
	assert.Equal(t, limitID, detail.LimitID)

	// KEY ASSERTIONS: These fields should exist and be populated for skipped limits
	// If Skipped field doesn't exist, test fails at compile time
	// If implementation doesn't set these fields, test fails at runtime
	assert.True(t, detail.Skipped, "Limit should be marked as skipped")
	assert.Equal(t, "outside_time_window", detail.SkipReason, "Skip reason should be 'outside_time_window'")

	// Skipped limits should have zero current usage and not be exceeded
	assert.True(t, detail.CurrentUsage.Equal(decimal.Zero), "Skipped limit should have zero current usage")
	assert.False(t, detail.Exceeded, "Skipped limit should not be marked as exceeded")
}

// TestCheckLimits_TimeWindow_InsideWindow_Evaluated verifies that limits with time windows
// are evaluated normally when the transaction timestamp (server clock) is inside the window.
// Seeds: 9010-9019
func TestCheckLimits_TimeWindow_InsideWindow_Evaluated(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9010)
	accountID := testutil.MustDeterministicUUID(9011)

	// Server clock at 21:30 UTC - INSIDE the 20:00-06:00 overnight window
	serverTime := time.Date(2024, 1, 15, 21, 30, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)
	periodKeyDaily := "2024-01-15"

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	// Create limit with overnight time window (20:00 to 06:00)
	activeTimeStart := testhelper.MustNewTimeOfDay("20:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("06:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Overnight Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart,
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// Counter SHOULD be called when inside time window
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKeyDaily,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("600"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.Equal(t, limitID, detail.LimitID)

	// Limit should NOT be skipped when inside time window
	assert.False(t, detail.Skipped, "Limit should not be skipped when inside time window")
	assert.Equal(t, "", detail.SkipReason, "Skip reason should be empty when limit is evaluated")

	// Should have normal usage tracking
	assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("600")), "Should have updated current usage")
	assert.False(t, detail.Exceeded)
}

// TestCheckLimits_TimeWindow_OvernightWindow_EarlyMorning_Evaluated verifies that
// overnight windows correctly include early morning hours (e.g., 03:00 is inside 20:00-06:00).
// Seeds: 9020-9029
func TestCheckLimits_TimeWindow_OvernightWindow_EarlyMorning_Evaluated(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9020)
	accountID := testutil.MustDeterministicUUID(9021)

	// Server clock at 03:00 UTC - INSIDE the 20:00-06:00 overnight window (early morning)
	serverTime := time.Date(2024, 1, 15, 3, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)
	periodKeyDaily := "2024-01-15"

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	// Create limit with overnight time window (20:00 to 06:00)
	activeTimeStart := testhelper.MustNewTimeOfDay("20:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("06:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Overnight Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart,
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// Counter SHOULD be called when inside time window (3:00 is inside 20:00-06:00)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKeyDaily,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.False(t, detail.Skipped, "03:00 is inside 20:00-06:00 window, should not be skipped")
}

// TestCheckLimits_TimeWindow_BusinessHours_Boundary_Inclusive verifies that
// business hours windows are inclusive at start time (09:00 is inside 09:00-17:00).
// Seeds: 9030-9039
func TestCheckLimits_TimeWindow_BusinessHours_Boundary_Inclusive(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9030)
	accountID := testutil.MustDeterministicUUID(9031)

	// Server clock at 09:00 UTC - exactly at start of 09:00-17:00 window (inclusive)
	serverTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)
	periodKeyDaily := "2024-01-15"

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	// Create limit with business hours window (09:00 to 17:00)
	activeTimeStart := testhelper.MustNewTimeOfDay("09:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("17:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Business Hours Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("500"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart,
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// Counter SHOULD be called - 09:00 is inclusive start
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKeyDaily,
		decimal.RequireFromString("50"),
		decimal.RequireFromString("500"),
		gomock.Any(),
	).Return(decimal.RequireFromString("50"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("50"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.False(t, detail.Skipped, "09:00 is at start of 09:00-17:00 window (inclusive), should not be skipped")
}

// TestCheckLimits_TimeWindow_BusinessHours_Boundary_Exclusive verifies that
// business hours windows are exclusive at end time (17:00 is OUTSIDE 09:00-17:00).
// Seeds: 9040-9049
func TestCheckLimits_TimeWindow_BusinessHours_Boundary_Exclusive(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9040)
	accountID := testutil.MustDeterministicUUID(9041)

	// Server clock at 17:00 UTC - exactly at end of 09:00-17:00 window (exclusive)
	serverTime := time.Date(2024, 1, 15, 17, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	// Create limit with business hours window (09:00 to 17:00)
	activeTimeStart := testhelper.MustNewTimeOfDay("09:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("17:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Business Hours Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("500"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart,
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: NO counter operations should be called when limit is skipped (17:00 is exclusive)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("50"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "Transaction should be allowed when limit is skipped")
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.True(t, detail.Skipped, "17:00 is at end of 09:00-17:00 window (exclusive), should be skipped")
	assert.Equal(t, "outside_time_window", detail.SkipReason)
}

// TestCheckLimits_TimeWindow_NoTimeWindow_AlwaysEvaluated verifies that limits
// without time windows are always evaluated regardless of time.
// Seeds: 9050-9059
func TestCheckLimits_TimeWindow_NoTimeWindow_AlwaysEvaluated(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9050)
	accountID := testutil.MustDeterministicUUID(9051)

	// Test at various times throughout the day - should all be evaluated
	testTimes := []time.Time{
		time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),   // Midnight
		time.Date(2024, 1, 15, 3, 30, 0, 0, time.UTC),  // Early morning
		time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),  // Noon
		time.Date(2024, 1, 15, 23, 59, 0, 0, time.UTC), // Late night
	}

	for _, serverTime := range testTimes {
		t.Run(serverTime.Format("15:04"), func(t *testing.T) {
			mockClock := testutil.NewMockClock(serverTime)
			periodKeyDaily := serverTime.Format("2006-01-02")

			ctrl := gomock.NewController(t)

			mockLimitRepo := NewMockLimitRepository(ctrl)
			mockUsageRepo := NewMockUsageCounterRepository(ctrl)
			mockDB := dbmocks.NewMockDB(ctrl)

			status := model.LimitStatusActive
			currency := "USD"
			scopeKey := "acct:" + accountID.String()

			// Limit WITHOUT time window (ActiveTimeStart and ActiveTimeEnd are nil)
			mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
				Status:   &status,
				Currency: &currency,
				Limit:    constant.MaxPaginationLimit,
				Cursor:   "",
			}).Return(&model.ListLimitsResult{
				Limits: []model.Limit{
					{
						ID:              limitID,
						Name:            "Always Active Limit",
						LimitType:       model.LimitTypeDaily,
						MaxAmount:       decimal.RequireFromString("1000"),
						Currency:        "USD",
						Scopes:          []model.Scope{{AccountID: &accountID}},
						Status:          model.LimitStatusActive,
						ActiveTimeStart: nil, // No time window
						ActiveTimeEnd:   nil, // No time window
					},
				},
				HasMore: false,
			}, nil)

			// Counter SHOULD always be called when no time window is configured
			mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
				gomock.Any(),
				mockDB,
				limitID,
				scopeKey,
				periodKeyDaily,
				decimal.RequireFromString("100"),
				decimal.RequireFromString("1000"),
				gomock.Any(),
			).Return(decimal.RequireFromString("100"), nil)

			ctx := setupTest(t)

			checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
			require.NoError(t, err)

			input := &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: serverTime,
			}

			output, err := checker.CheckLimits(ctx, mockDB, input)

			require.NoError(t, err)
			require.NotNil(t, output)
			assert.True(t, output.Allowed)
			require.Len(t, output.LimitUsageDetails, 1)

			detail := output.LimitUsageDetails[0]
			assert.False(t, detail.Skipped, "Limit without time window should never be skipped")
		})
	}
}

// TestCheckLimits_TimeWindow_MixedLimits_SomeSkipped verifies behavior when multiple limits
// exist and some have time windows that exclude the current time while others don't.
// Seeds: 9060-9069
func TestCheckLimits_TimeWindow_MixedLimits_SomeSkipped(t *testing.T) {
	t.Parallel()

	limitID1 := testutil.MustDeterministicUUID(9060) // No time window - always evaluated
	limitID2 := testutil.MustDeterministicUUID(9061) // 09:00-17:00 - will be skipped at 20:00
	limitID3 := testutil.MustDeterministicUUID(9062) // 18:00-23:00 - will be evaluated at 20:00
	accountID := testutil.MustDeterministicUUID(9063)

	// Server clock at 20:00 UTC
	serverTime := time.Date(2024, 1, 15, 20, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)
	periodKeyDaily := "2024-01-15"

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()

	// Time windows
	businessStart := testhelper.MustNewTimeOfDay("09:00")
	businessEnd := testhelper.MustNewTimeOfDay("17:00")
	eveningStart := testhelper.MustNewTimeOfDay("18:00")
	eveningEnd := testhelper.MustNewTimeOfDay("23:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID1,
				Name:            "Always Active Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("5000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: nil, // No time window
				ActiveTimeEnd:   nil,
			},
			{
				ID:              limitID2,
				Name:            "Business Hours Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("500"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &businessStart, // 09:00-17:00
				ActiveTimeEnd:   &businessEnd,
			},
			{
				ID:              limitID3,
				Name:            "Evening Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &eveningStart, // 18:00-23:00
				ActiveTimeEnd:   &eveningEnd,
			},
		},
		HasMore: false,
	}, nil)

	// Limit 1 (no time window) - SHOULD be evaluated
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID1,
		scopeKey,
		periodKeyDaily,
		decimal.RequireFromString("200"),
		decimal.RequireFromString("5000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("200"), nil)

	// Limit 2 (09:00-17:00) - should be SKIPPED at 20:00, NO counter call

	// Limit 3 (18:00-23:00) - SHOULD be evaluated at 20:00
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID3,
		scopeKey,
		periodKeyDaily,
		decimal.RequireFromString("200"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("200"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("200"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)

	// Should have details for all 3 limits
	require.Len(t, output.LimitUsageDetails, 3, "Should have details for all 3 limits including skipped ones")

	// Find each limit detail
	var detail1, detail2, detail3 *model.LimitUsageDetail
	for i := range output.LimitUsageDetails {
		d := &output.LimitUsageDetails[i]
		switch d.LimitID {
		case limitID1:
			detail1 = d
		case limitID2:
			detail2 = d
		case limitID3:
			detail3 = d
		}
	}

	require.NotNil(t, detail1, "Should have detail for limit 1")
	require.NotNil(t, detail2, "Should have detail for limit 2")
	require.NotNil(t, detail3, "Should have detail for limit 3")

	// Limit 1: No time window - evaluated
	assert.False(t, detail1.Skipped, "Limit 1 (no time window) should not be skipped")
	assert.True(t, detail1.CurrentUsage.Equal(decimal.RequireFromString("200")))

	// Limit 2: 09:00-17:00 - skipped at 20:00
	assert.True(t, detail2.Skipped, "Limit 2 (09:00-17:00) should be skipped at 20:00")
	assert.Equal(t, "outside_time_window", detail2.SkipReason)
	assert.True(t, detail2.CurrentUsage.Equal(decimal.Zero), "Skipped limit should have zero usage")
	assert.False(t, detail2.Exceeded, "Skipped limit should not be exceeded")

	// Limit 3: 18:00-23:00 - evaluated at 20:00
	assert.False(t, detail3.Skipped, "Limit 3 (18:00-23:00) should not be skipped at 20:00")
	assert.True(t, detail3.CurrentUsage.Equal(decimal.RequireFromString("200")))
}

// TestCheckLimits_TimeWindow_SkippedLimit_NoExceededFlag verifies that a limit that would
// have been exceeded is NOT marked as exceeded when it's skipped due to time window.
// Seeds: 9070-9079
func TestCheckLimits_TimeWindow_SkippedLimit_NoExceededFlag(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9070)
	accountID := testutil.MustDeterministicUUID(9071)

	// Server clock at 14:00 UTC - OUTSIDE the 09:00-12:00 window
	serverTime := time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	// Small limit that would be exceeded if evaluated
	activeTimeStart := testhelper.MustNewTimeOfDay("09:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("12:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Morning Limit",
				LimitType:       model.LimitTypeDaily,
				MaxAmount:       decimal.RequireFromString("50"), // Small limit
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart, // 09:00-12:00
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// NO counter operations when skipped

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"), // Would exceed 50 limit if evaluated
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)

	// Transaction should be allowed because limit is skipped (not because it passed)
	assert.True(t, output.Allowed, "Transaction should be allowed when limit is skipped")
	assert.Empty(t, output.ExceededLimitIDs, "Should have no exceeded limits")

	require.Len(t, output.LimitUsageDetails, 1)
	detail := output.LimitUsageDetails[0]

	// KEY ASSERTION: Even though amount (100) > maxAmount (50), the limit should be
	// skipped, not exceeded, because we're outside the time window
	assert.True(t, detail.Skipped, "Limit should be skipped at 14:00 (outside 09:00-12:00)")
	assert.Equal(t, "outside_time_window", detail.SkipReason)
	assert.False(t, detail.Exceeded, "Skipped limit should NOT be marked as exceeded")
}

// TestCheckLimits_TimeWindow_PerTransaction_Skipped verifies that PER_TRANSACTION limits
// with time windows are also skipped when outside the window.
// Seeds: 9080-9089
func TestCheckLimits_TimeWindow_PerTransaction_Skipped(t *testing.T) {
	t.Parallel()

	limitID := testutil.MustDeterministicUUID(9080)
	accountID := testutil.MustDeterministicUUID(9081)

	// Server clock at 22:00 UTC - OUTSIDE the 08:00-18:00 window
	serverTime := time.Date(2024, 1, 15, 22, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	activeTimeStart := testhelper.MustNewTimeOfDay("08:00")
	activeTimeEnd := testhelper.MustNewTimeOfDay("18:00")

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Business Hours Per-Tx Limit",
				LimitType:       model.LimitTypePerTransaction, // PER_TRANSACTION type
				MaxAmount:       decimal.RequireFromString("200"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				ActiveTimeStart: &activeTimeStart, // 08:00-18:00
				ActiveTimeEnd:   &activeTimeEnd,
			},
		},
		HasMore: false,
	}, nil)

	// NO counter operations for PER_TRANSACTION, but also should skip due to time window

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("300"), // Would exceed 200 if evaluated
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)

	// Transaction allowed because limit is skipped
	assert.True(t, output.Allowed)
	assert.Empty(t, output.ExceededLimitIDs)

	require.Len(t, output.LimitUsageDetails, 1)
	detail := output.LimitUsageDetails[0]

	assert.True(t, detail.Skipped, "PER_TRANSACTION limit should also be skipped when outside time window")
	assert.Equal(t, "outside_time_window", detail.SkipReason)
	assert.False(t, detail.Exceeded, "Skipped limit should not be exceeded")
}

// TestCheckLimits_TimeWindow_TableDriven provides comprehensive table-driven tests
// for various time window scenarios.
// Seeds: 9090-9099
func TestCheckLimits_TimeWindow_TableDriven(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(9090)

	tests := []struct {
		name             string
		serverTime       time.Time
		windowStart      string // "HH:MM" or empty for no window
		windowEnd        string
		expectSkipped    bool
		expectSkipReason string
	}{
		// Overnight window (20:00 to 06:00)
		{
			name:             "overnight window - 21:30 is inside",
			serverTime:       time.Date(2024, 1, 15, 21, 30, 0, 0, time.UTC),
			windowStart:      "20:00",
			windowEnd:        "06:00",
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "overnight window - 14:00 is outside",
			serverTime:       time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC),
			windowStart:      "20:00",
			windowEnd:        "06:00",
			expectSkipped:    true,
			expectSkipReason: "outside_time_window",
		},
		{
			name:             "overnight window - 03:00 is inside (early morning)",
			serverTime:       time.Date(2024, 1, 15, 3, 0, 0, 0, time.UTC),
			windowStart:      "20:00",
			windowEnd:        "06:00",
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "overnight window - 06:00 is outside (end exclusive)",
			serverTime:       time.Date(2024, 1, 15, 6, 0, 0, 0, time.UTC),
			windowStart:      "20:00",
			windowEnd:        "06:00",
			expectSkipped:    true,
			expectSkipReason: "outside_time_window",
		},
		{
			name:             "overnight window - 20:00 is inside (start inclusive)",
			serverTime:       time.Date(2024, 1, 15, 20, 0, 0, 0, time.UTC),
			windowStart:      "20:00",
			windowEnd:        "06:00",
			expectSkipped:    false,
			expectSkipReason: "",
		},
		// Business hours (09:00 to 17:00)
		{
			name:             "business hours - 09:00 is inside (start inclusive)",
			serverTime:       time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
			windowStart:      "09:00",
			windowEnd:        "17:00",
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "business hours - 17:00 is outside (end exclusive)",
			serverTime:       time.Date(2024, 1, 15, 17, 0, 0, 0, time.UTC),
			windowStart:      "09:00",
			windowEnd:        "17:00",
			expectSkipped:    true,
			expectSkipReason: "outside_time_window",
		},
		{
			name:             "business hours - 12:00 is inside",
			serverTime:       time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			windowStart:      "09:00",
			windowEnd:        "17:00",
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "business hours - 08:59 is outside (before start)",
			serverTime:       time.Date(2024, 1, 15, 8, 59, 0, 0, time.UTC),
			windowStart:      "09:00",
			windowEnd:        "17:00",
			expectSkipped:    true,
			expectSkipReason: "outside_time_window",
		},
		// No time window
		{
			name:             "no time window - midnight is evaluated",
			serverTime:       time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			windowStart:      "",
			windowEnd:        "",
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "no time window - noon is evaluated",
			serverTime:       time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			windowStart:      "",
			windowEnd:        "",
			expectSkipped:    false,
			expectSkipReason: "",
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limitID := testutil.MustDeterministicUUID(9091 + int64(i))

			mockClock := testutil.NewMockClock(tc.serverTime)
			periodKeyDaily := tc.serverTime.Format("2006-01-02")

			ctrl := gomock.NewController(t)

			mockLimitRepo := NewMockLimitRepository(ctrl)
			mockUsageRepo := NewMockUsageCounterRepository(ctrl)
			mockDB := dbmocks.NewMockDB(ctrl)

			status := model.LimitStatusActive
			currency := "USD"
			scopeKey := "acct:" + accountID.String()

			// Build limit with optional time window
			limit := model.Limit{
				ID:        limitID,
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			}

			if tc.windowStart != "" && tc.windowEnd != "" {
				start := testhelper.MustNewTimeOfDay(tc.windowStart)
				end := testhelper.MustNewTimeOfDay(tc.windowEnd)
				limit.ActiveTimeStart = &start
				limit.ActiveTimeEnd = &end
			}

			mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
				Status:   &status,
				Currency: &currency,
				Limit:    constant.MaxPaginationLimit,
				Cursor:   "",
			}).Return(&model.ListLimitsResult{
				Limits:  []model.Limit{limit},
				HasMore: false,
			}, nil)

			// Only expect counter call if NOT skipped
			if !tc.expectSkipped {
				mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
					gomock.Any(),
					mockDB,
					limitID,
					scopeKey,
					periodKeyDaily,
					decimal.RequireFromString("100"),
					decimal.RequireFromString("1000"),
					gomock.Any(),
				).Return(decimal.RequireFromString("100"), nil)
			}

			ctx := setupTest(t)

			checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
			require.NoError(t, err)

			// Use a different TransactionTimestamp than serverTime to prove
			// that skip decisions are based on the server clock, not the client timestamp.
			input := &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: tc.serverTime.Add(-2 * time.Hour),
			}

			output, err := checker.CheckLimits(ctx, mockDB, input)

			require.NoError(t, err)
			require.NotNil(t, output)
			assert.True(t, output.Allowed)
			require.Len(t, output.LimitUsageDetails, 1)

			detail := output.LimitUsageDetails[0]
			assert.Equal(t, tc.expectSkipped, detail.Skipped, "Skipped mismatch for %s", tc.name)
			assert.Equal(t, tc.expectSkipReason, detail.SkipReason, "SkipReason mismatch for %s", tc.name)
		})
	}
}

// =============================================================================
// Custom Period Skip Logic Tests
// Tests verify that CUSTOM limits with customStartDate/customEndDate
// skip evaluation when transactions fall outside the custom period.
// Seed range: 10000-10099
// =============================================================================

// TestCheckLimits_CustomPeriod_OutsideCustomPeriod_Skipped verifies that a CUSTOM limit
// is skipped (not evaluated, no counter call) when transaction is outside the custom period.
// Seeds: 10000-10009
func TestCheckLimits_CustomPeriod_OutsideCustomPeriod_Skipped(t *testing.T) {
	t.Parallel()

	// Seeds 10000-10009
	limitID := testutil.MustDeterministicUUID(10000)
	accountID := testutil.MustDeterministicUUID(10001)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: Mar 09 2025 10:00 UTC (outside custom period)
	serverTime := time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: No UpsertAndIncrementAtomic call should be made
	// because the transaction is outside the custom period.
	// gomock will fail if UpsertAndIncrementAtomic is called.

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "Should be allowed (skipped limits don't block)")
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.True(t, detail.Skipped, "Limit should be skipped when outside custom period")
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
	assert.False(t, detail.Exceeded, "Skipped limit should not be marked as exceeded")
	assert.True(t, decimal.Zero.Equal(detail.CurrentUsage), "Skipped limit should have zero current usage")
}

// TestCheckLimits_CustomPeriod_InsideCustomPeriod_Evaluated verifies that a CUSTOM limit
// is evaluated normally (counter call made) when transaction is inside the custom period.
// Seeds: 10010-10019
func TestCheckLimits_CustomPeriod_InsideCustomPeriod_Evaluated(t *testing.T) {
	t.Parallel()

	// Seeds 10010-10019
	limitID := testutil.MustDeterministicUUID(10010)
	accountID := testutil.MustDeterministicUUID(10011)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: Nov 27 2025 12:00 UTC (inside custom period)
	serverTime := time.Date(2025, 11, 27, 12, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()
	periodKey := "custom" // CUSTOM limits use "custom" as period key

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: UpsertAndIncrementAtomic MUST be called because
	// the transaction is inside the custom period.
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKey,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.False(t, detail.Skipped, "Limit should NOT be skipped when inside custom period")
	assert.Empty(t, detail.SkipReason)
	assert.False(t, detail.Exceeded)
	assert.True(t, decimal.RequireFromString("100").Equal(detail.CurrentUsage))
}

// TestCheckLimits_CustomPeriod_Boundary_Start_Inclusive verifies that a transaction
// exactly at customStartDate is evaluated (start boundary is inclusive).
// Seeds: 10020-10029
func TestCheckLimits_CustomPeriod_Boundary_Start_Inclusive(t *testing.T) {
	t.Parallel()

	// Seeds 10020-10029
	limitID := testutil.MustDeterministicUUID(10020)
	accountID := testutil.MustDeterministicUUID(10021)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: EXACTLY at customStartDate
	serverTime := customStartDate
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()
	periodKey := "custom"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: UpsertAndIncrementAtomic MUST be called because
	// start boundary is INCLUSIVE.
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKey,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.False(t, detail.Skipped, "Limit should NOT be skipped at start boundary (inclusive)")
	assert.Empty(t, detail.SkipReason)
}

// TestCheckLimits_CustomPeriod_Boundary_End_Exclusive verifies that a transaction
// exactly at customEndDate is skipped (end boundary is exclusive).
// Seeds: 10030-10039
func TestCheckLimits_CustomPeriod_Boundary_End_Exclusive(t *testing.T) {
	t.Parallel()

	// Seeds 10030-10039
	limitID := testutil.MustDeterministicUUID(10030)
	accountID := testutil.MustDeterministicUUID(10031)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: EXACTLY at customEndDate
	serverTime := customEndDate
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: No UpsertAndIncrementAtomic call should be made
	// because end boundary is EXCLUSIVE.
	// gomock will fail if UpsertAndIncrementAtomic is called.

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed, "Should be allowed (skipped limits don't block)")
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.True(t, detail.Skipped, "Limit should be skipped at end boundary (exclusive)")
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
}

// TestCheckLimits_CustomPeriod_BeforePeriod_Skipped verifies that a transaction
// before the custom period start is skipped.
// Seeds: 10040-10049
func TestCheckLimits_CustomPeriod_BeforePeriod_Skipped(t *testing.T) {
	t.Parallel()

	// Seeds 10040-10049
	limitID := testutil.MustDeterministicUUID(10040)
	accountID := testutil.MustDeterministicUUID(10041)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: Nov 26 2025 10:00 UTC (before custom period)
	serverTime := time.Date(2025, 11, 26, 10, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// No UpsertAndIncrementAtomic call expected

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.True(t, detail.Skipped)
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
}

// TestCheckLimits_CustomPeriod_AfterPeriod_Skipped verifies that a transaction
// after the custom period end is skipped.
// Seeds: 10050-10059
func TestCheckLimits_CustomPeriod_AfterPeriod_Skipped(t *testing.T) {
	t.Parallel()

	// Seeds 10050-10059
	limitID := testutil.MustDeterministicUUID(10050)
	accountID := testutil.MustDeterministicUUID(10051)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: Nov 29 2025 10:00 UTC (after custom period)
	serverTime := time.Date(2025, 11, 29, 10, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// No UpsertAndIncrementAtomic call expected

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.True(t, detail.Skipped)
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
}

// TestCheckLimits_CustomPeriod_TwoTransactionsSamePeriod_CounterAccumulates verifies
// that two transactions within the same custom period accumulate under a single counter.
// Seeds: 10060-10069
func TestCheckLimits_CustomPeriod_TwoTransactionsSamePeriod_CounterAccumulates(t *testing.T) {
	t.Parallel()

	// Setup tracing ONCE at the beginning to avoid deadlock
	ctx := setupTest(t)

	// Seeds 10060-10069
	limitID := testutil.MustDeterministicUUID(10060)
	accountID := testutil.MustDeterministicUUID(10061)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// First transaction: Nov 27 2025 12:00 UTC
	serverTime1 := time.Date(2025, 11, 27, 12, 0, 0, 0, time.UTC)

	// Second transaction: Nov 28 2025 10:00 UTC (different day, same custom period)
	serverTime2 := time.Date(2025, 11, 28, 10, 0, 0, 0, time.UTC)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()
	periodKey := "custom" // Both transactions use same "custom" period key

	// Use single controller and mocks for all calls
	ctrl := gomock.NewController(t)
	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	// Setup expectations for BOTH CheckLimits calls

	// First transaction List call
	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil).Times(2) // Called for both transactions

	// First transaction: 0 + 100 = 100
	firstCall := mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKey,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	// KEY ASSERTION: Second transaction uses SAME periodKey "custom"
	// and accumulates: 100 + 200 = 300
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID,
		scopeKey,
		periodKey, // Same "custom" period key for both days
		decimal.RequireFromString("200"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("300"), nil).After(firstCall) // 100 (existing) + 200 = 300

	// First transaction with clock at serverTime1
	mockClock1 := testutil.NewMockClock(serverTime1)
	checker1, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock1)
	require.NoError(t, err)

	input1 := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime1,
	}

	output1, err := checker1.CheckLimits(ctx, mockDB, input1)
	require.NoError(t, err)
	require.NotNil(t, output1)
	assert.True(t, output1.Allowed)
	require.Len(t, output1.LimitUsageDetails, 1)
	assert.True(t, decimal.RequireFromString("100").Equal(output1.LimitUsageDetails[0].CurrentUsage))

	// Second transaction with clock at serverTime2
	mockClock2 := testutil.NewMockClock(serverTime2)
	checker2, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock2)
	require.NoError(t, err)

	input2 := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("200"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime2,
	}

	output2, err := checker2.CheckLimits(ctx, mockDB, input2)
	require.NoError(t, err)
	require.NotNil(t, output2)
	assert.True(t, output2.Allowed)
	require.Len(t, output2.LimitUsageDetails, 1)
	// Current usage should be 300 (accumulated from both transactions)
	assert.True(t, decimal.RequireFromString("300").Equal(output2.LimitUsageDetails[0].CurrentUsage),
		"Expected 300 (accumulated), got %s", output2.LimitUsageDetails[0].CurrentUsage.String())
}

// TestCheckLimits_CustomPeriod_MixedLimits_SomeSkipped verifies that when multiple limits
// are present (some CUSTOM outside period, some DAILY), only the CUSTOM outside period is skipped.
// Seeds: 10070-10079
func TestCheckLimits_CustomPeriod_MixedLimits_SomeSkipped(t *testing.T) {
	t.Parallel()

	// Seeds 10070-10079
	customLimitID := testutil.MustDeterministicUUID(10070)
	dailyLimitID := testutil.MustDeterministicUUID(10071)
	accountID := testutil.MustDeterministicUUID(10072)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: Mar 09 2025 10:00 UTC (outside custom period)
	serverTime := time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"
	scopeKey := "acct:" + accountID.String()
	dailyPeriodKey := "2025-03-09" // DAILY uses date format

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              customLimitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
			{
				ID:        dailyLimitID,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("500"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	// KEY ASSERTION: Only DAILY limit should call UpsertAndIncrementAtomic
	// CUSTOM limit should be skipped (no counter call)
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		dailyLimitID, // Only DAILY limit is processed
		scopeKey,
		dailyPeriodKey,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("500"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 2, "Should have details for both limits")

	// Find the CUSTOM limit detail and verify it's skipped
	var customDetail, dailyDetail *model.LimitUsageDetail
	for i := range output.LimitUsageDetails {
		if output.LimitUsageDetails[i].LimitID == customLimitID {
			customDetail = &output.LimitUsageDetails[i]
		}
		if output.LimitUsageDetails[i].LimitID == dailyLimitID {
			dailyDetail = &output.LimitUsageDetails[i]
		}
	}

	require.NotNil(t, customDetail, "Should have CUSTOM limit detail")
	assert.True(t, customDetail.Skipped, "CUSTOM limit should be skipped")
	assert.Equal(t, "outside_custom_period", customDetail.SkipReason)

	require.NotNil(t, dailyDetail, "Should have DAILY limit detail")
	assert.False(t, dailyDetail.Skipped, "DAILY limit should NOT be skipped")
	assert.Empty(t, dailyDetail.SkipReason)
}

// TestCheckLimits_CustomPeriod_SkippedLimit_NoExceededFlag verifies that a skipped CUSTOM limit
// does not set exceeded=true even if the amount would have exceeded the limit.
// Seeds: 10080-10089
func TestCheckLimits_CustomPeriod_SkippedLimit_NoExceededFlag(t *testing.T) {
	t.Parallel()

	// Seeds 10080-10089
	limitID := testutil.MustDeterministicUUID(10080)
	accountID := testutil.MustDeterministicUUID(10081)

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	// Server clock: Mar 09 2025 10:00 UTC (outside custom period)
	serverTime := time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC)
	mockClock := testutil.NewMockClock(serverTime)

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:              limitID,
				Name:            "Custom Period Limit",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("100"), // MaxAmount is 100
				Currency:        "USD",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStartDate,
				CustomEndDate:   &customEndDate,
			},
		},
		HasMore: false,
	}, nil)

	// No UpsertAndIncrementAtomic call expected (skipped)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("500"), // Amount 500 > MaxAmount 100
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: serverTime,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	// KEY ASSERTION: Transaction should be ALLOWED because limit is skipped
	// (not because it wouldn't exceed the limit)
	assert.True(t, output.Allowed, "Transaction should be allowed when limit is skipped")
	assert.Empty(t, output.ExceededLimitIDs, "No limits should be marked as exceeded")
	require.Len(t, output.LimitUsageDetails, 1)

	detail := output.LimitUsageDetails[0]
	assert.True(t, detail.Skipped, "Limit should be skipped")
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
	assert.False(t, detail.Exceeded, "Skipped limit should NOT be marked as exceeded")
}

// TestCheckLimits_CustomPeriod_TableDriven tests comprehensive scenarios for custom period evaluation.
// Seeds: 10090-10099
func TestCheckLimits_CustomPeriod_TableDriven(t *testing.T) {
	t.Parallel()

	// Custom period: Nov 27 2025 08:00 UTC to Nov 28 2025 22:00 UTC
	customStartDate := time.Date(2025, 11, 27, 8, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2025, 11, 28, 22, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		serverTime       time.Time
		expectSkipped    bool
		expectSkipReason string
	}{
		{
			name:             "inside period - middle of first day",
			serverTime:       time.Date(2025, 11, 27, 14, 0, 0, 0, time.UTC),
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "inside period - middle of second day",
			serverTime:       time.Date(2025, 11, 28, 10, 0, 0, 0, time.UTC),
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "at start boundary - inclusive",
			serverTime:       customStartDate,
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "one second after start",
			serverTime:       customStartDate.Add(1 * time.Second),
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "one second before end",
			serverTime:       customEndDate.Add(-1 * time.Second),
			expectSkipped:    false,
			expectSkipReason: "",
		},
		{
			name:             "at end boundary - exclusive",
			serverTime:       customEndDate,
			expectSkipped:    true,
			expectSkipReason: "outside_custom_period",
		},
		{
			name:             "one second after end",
			serverTime:       customEndDate.Add(1 * time.Second),
			expectSkipped:    true,
			expectSkipReason: "outside_custom_period",
		},
		{
			name:             "one second before start",
			serverTime:       customStartDate.Add(-1 * time.Second),
			expectSkipped:    true,
			expectSkipReason: "outside_custom_period",
		},
		{
			name:             "way before period - Jan 2025",
			serverTime:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
			expectSkipped:    true,
			expectSkipReason: "outside_custom_period",
		},
		{
			name:             "way after period - Dec 2025",
			serverTime:       time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC),
			expectSkipped:    true,
			expectSkipReason: "outside_custom_period",
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Note: Not using t.Parallel() here to avoid deadlock with SetupTestTracing mutex

			// Seeds 10090-10099 (base 10090 + i*2 for limitID, 10090 + i*2 + 1 for accountID)
			limitID := testutil.MustDeterministicUUID(10090 + int64(i*2))
			accountID := testutil.MustDeterministicUUID(10090 + int64(i*2) + 1)

			mockClock := testutil.NewMockClock(tc.serverTime)

			ctrl := gomock.NewController(t)

			mockLimitRepo := NewMockLimitRepository(ctrl)
			mockUsageRepo := NewMockUsageCounterRepository(ctrl)
			mockDB := dbmocks.NewMockDB(ctrl)

			status := model.LimitStatusActive
			currency := "USD"
			scopeKey := "acct:" + accountID.String()
			periodKey := "custom"

			mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
				Status:   &status,
				Currency: &currency,
				Limit:    constant.MaxPaginationLimit,
				Cursor:   "",
			}).Return(&model.ListLimitsResult{
				Limits: []model.Limit{
					{
						ID:              limitID,
						Name:            "Custom Period Limit",
						LimitType:       model.LimitTypeCustom,
						MaxAmount:       decimal.RequireFromString("1000"),
						Currency:        "USD",
						Scopes:          []model.Scope{{AccountID: &accountID}},
						Status:          model.LimitStatusActive,
						CustomStartDate: &customStartDate,
						CustomEndDate:   &customEndDate,
					},
				},
				HasMore: false,
			}, nil)

			// Only expect counter call if NOT skipped
			if !tc.expectSkipped {
				mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
					gomock.Any(),
					mockDB,
					limitID,
					scopeKey,
					periodKey,
					decimal.RequireFromString("100"),
					decimal.RequireFromString("1000"),
					gomock.Any(),
				).Return(decimal.RequireFromString("100"), nil)
			}

			ctx := setupTest(t)

			checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, mockClock)
			require.NoError(t, err)

			input := &model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				AccountID:            accountID,
				TransactionTimestamp: tc.serverTime,
			}

			output, err := checker.CheckLimits(ctx, mockDB, input)

			require.NoError(t, err)
			require.NotNil(t, output)
			assert.True(t, output.Allowed)
			require.Len(t, output.LimitUsageDetails, 1)

			detail := output.LimitUsageDetails[0]
			assert.Equal(t, tc.expectSkipped, detail.Skipped, "Skipped mismatch for %s", tc.name)
			assert.Equal(t, tc.expectSkipReason, detail.SkipReason, "SkipReason mismatch for %s", tc.name)
		})
	}
}

// =============================================================================
// EvaluatedAt Timestamp Tests
// Tests verify that CheckLimitsOutput includes evaluatedAt timestamp that:
// 1. Is present in the response as ISO 8601 UTC string
// 2. Same value is used for all limit evaluations in the request
// Seed range: 11000-11099
// =============================================================================

// TestCheckLimits_EvaluatedAt_Consistency verifies that the evaluatedAt timestamp
// returned by CheckLimits matches the server clock time and is consistent across
// all limit evaluations within a single request.
func TestCheckLimits_EvaluatedAt_Consistency(t *testing.T) {
	t.Parallel()

	// Test UUIDs from seed range 11000-11099
	limitID1 := testutil.MustDeterministicUUID(11010)
	limitID2 := testutil.MustDeterministicUUID(11011)
	accountID := testutil.MustDeterministicUUID(11012)

	// Fixed server time from mock clock
	expectedEvaluatedAt := testutil.DefaultTestTime

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	// Setup: Two applicable limits of different types
	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID1,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
			{
				ID:        limitID2,
				Name:      "Per Transaction Limit",
				LimitType: model.LimitTypePerTransaction,
				MaxAmount: decimal.RequireFromString("500"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	scopeKey := "acct:" + accountID.String()
	periodKeyDaily := expectedEvaluatedAt.Format("2006-01-02")

	// First limit (DAILY) gets incremented atomically
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(
		gomock.Any(),
		mockDB,
		limitID1,
		scopeKey,
		periodKeyDaily,
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		gomock.Any(),
	).Return(decimal.RequireFromString("100"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	// Verify no errors
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 2)

	// Verify evaluatedAt is present in response
	// Verify same value is used for all limit evaluations
	assert.Equal(t, expectedEvaluatedAt, output.EvaluatedAt,
		"EvaluatedAt should match the mock clock time")
}

// TestCheckLimits_NoActiveLimits_HasEvaluatedAt verifies that evaluatedAt is
// populated even when there are no active limits to check (edge case).
func TestCheckLimits_NoActiveLimits_HasEvaluatedAt(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(11020)
	expectedEvaluatedAt := testutil.DefaultTestTime

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	// No active limits
	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
		Cursor:   "",
	}).Return(&model.ListLimitsResult{
		Limits:  []model.Limit{},
		HasMore: false,
	}, nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	assert.Empty(t, output.LimitUsageDetails)

	// Verify evaluatedAt is present even with no limits
	assert.Equal(t, expectedEvaluatedAt, output.EvaluatedAt,
		"EvaluatedAt should be set even when no active limits are found")
}

// =============================================================================
// Expired Usage Counters Are Automatically Cleaned Up
// Seed range: 12000-12099
// =============================================================================

// TestCalculateCounterExpiresAt tests the calculateCounterExpiresAt function.
// Acceptance Criteria:
// 1. DAILY counter: expiresAt = resetAt + 90 days
// 2. WEEKLY counter: expiresAt = resetAt + 90 days
// 3. MONTHLY counter: expiresAt = resetAt + 90 days
// 4. CUSTOM counter: expiresAt = customEndDate + 90 days
// 5. PER_TRANSACTION: no counter, nil expiresAt
// 6. NULL expiresAt: never deleted
func TestCalculateCounterExpiresAt(t *testing.T) {
	t.Parallel()

	// Deterministic test data using seed range 12000-12099
	resetAt := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	customEndDate := time.Date(2026, 11, 28, 22, 0, 0, 0, time.UTC)

	// Helper to create pointer to time
	ptr := func(t time.Time) *time.Time { return &t }

	tests := []struct {
		name          string
		limitType     model.LimitType
		resetAt       *time.Time
		customEndDate *time.Time
		expected      *time.Time
	}{
		{
			name:      "DAILY returns resetAt + 90 days",
			limitType: model.LimitTypeDaily,
			resetAt:   ptr(resetAt),
			expected:  ptr(resetAt.AddDate(0, 0, 90)), // June 9, 2026
		},
		{
			name:      "WEEKLY returns resetAt + 90 days",
			limitType: model.LimitTypeWeekly,
			resetAt:   ptr(resetAt),
			expected:  ptr(resetAt.AddDate(0, 0, 90)),
		},
		{
			name:      "MONTHLY returns resetAt + 90 days",
			limitType: model.LimitTypeMonthly,
			resetAt:   ptr(resetAt),
			expected:  ptr(resetAt.AddDate(0, 0, 90)),
		},
		{
			name:          "CUSTOM returns customEndDate + 90 days",
			limitType:     model.LimitTypeCustom,
			customEndDate: ptr(customEndDate),
			expected:      ptr(customEndDate.AddDate(0, 0, 90)), // February 26, 2027
		},
		{
			name:      "PER_TRANSACTION returns nil (no counter)",
			limitType: model.LimitTypePerTransaction,
			resetAt:   nil,
			expected:  nil,
		},
		{
			name:      "DAILY with nil resetAt returns nil (safety)",
			limitType: model.LimitTypeDaily,
			resetAt:   nil,
			expected:  nil, // Safety: don't crash on missing resetAt
		},
		{
			name:          "CUSTOM with nil customEndDate returns nil (safety)",
			limitType:     model.LimitTypeCustom,
			customEndDate: nil,
			expected:      nil, // Safety: don't crash on missing customEndDate
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := calculateCounterExpiresAt(tc.limitType, tc.resetAt, tc.customEndDate)

			if tc.expected == nil {
				assert.Nil(t, result, "expected nil expiresAt")
			} else {
				require.NotNil(t, result, "expected non-nil expiresAt")
				assert.True(t, tc.expected.Equal(*result),
					"expected %v, got %v", tc.expected, result)
			}
		})
	}
}

// TestCalculateCounterExpiresAt_RetentionDays validates that the retention
// period constant is used correctly (90 days).
func TestCalculateCounterExpiresAt_RetentionDays(t *testing.T) {
	t.Parallel()

	// Seed range: 12010
	resetAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ptr := func(t time.Time) *time.Time { return &t }

	// Call the function - should add exactly 90 days (not 3 months)
	result := calculateCounterExpiresAt(model.LimitTypeDaily, ptr(resetAt), nil)

	require.NotNil(t, result)
	expected := resetAt.AddDate(0, 0, 90) // April 1, 2026
	assert.Equal(t, expected, *result)
}

// =============================================================================
// CheckLimits Tests (Transactional Repository Methods)
// =============================================================================

// TestLimitCheckerService_CheckLimits_UsesProvidedDB verifies that
// CheckLimits propagates the db parameter to repository WithTx methods.
func TestLimitCheckerService_CheckLimits_UsesProvidedDB(t *testing.T) {
	limitID1 := testutil.MustDeterministicUUID(15201)
	accountID := testutil.MustDeterministicUUID(15200)

	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID1,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	scopeKey := "acct:" + accountID.String()
	mockUsageRepo.EXPECT().UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("1000"), gomock.Any()).
		Return(decimal.RequireFromString("50"), nil)

	ctx := setupTest(t)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("50"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	output, err := checker.CheckLimits(ctx, mockDB, input)

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
}

// TestLimitCheckerService_CheckLimits_PropagatesDB verifies that CheckLimits
// actually passes the db parameter to UpsertAndIncrementAtomic on the repository.
// CheckLimits calls UpsertAndIncrementAtomic (not UpsertAndIncrementAtomic),
// passing the provided db (transaction) through to the repository method.
// This enables atomic counter increments within the same transaction as validation persistence.
func TestLimitCheckerService_CheckLimits_PropagatesDB(t *testing.T) {
	// Use t.Cleanup to manage test resources properly
	ctx := context.Background()

	// Test UUIDs - seed range: 15400-15500
	limitID1 := testutil.MustDeterministicUUID(15401)
	accountID := testutil.MustDeterministicUUID(15400)
	timestamp := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	periodKeyDaily := serverPeriodKeyDaily

	ctrl := gomock.NewController(t)

	mockLimitRepo := NewMockLimitRepository(ctrl)
	mockUsageRepo := NewMockUsageCounterRepository(ctrl)
	mockDB := dbmocks.NewMockDB(ctrl)

	// Setup limit repository to return a DAILY limit
	status := model.LimitStatusActive
	currency := "USD"

	mockLimitRepo.EXPECT().List(gomock.Any(), &model.ListLimitsFilter{
		Status:   &status,
		Currency: &currency,
		Limit:    constant.MaxPaginationLimit,
	}).Return(&model.ListLimitsResult{
		Limits: []model.Limit{
			{
				ID:        limitID1,
				Name:      "Daily Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		HasMore: false,
	}, nil)

	scopeKey := "acct:" + accountID.String()

	var dbWasUsed bool

	mockUsageRepo.EXPECT().
		UpsertAndIncrementAtomic(gomock.Any(), mockDB, limitID1, scopeKey, periodKeyDaily, decimal.RequireFromString("50"), decimal.RequireFromString("1000"), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ pgdb.DB, _ uuid.UUID, _ string, _ string, _ decimal.Decimal, _ decimal.Decimal, _ *time.Time) (decimal.Decimal, error) {
			dbWasUsed = true
			return decimal.RequireFromString("50"), nil
		}).
		Times(1)

	checker, err := NewLimitChecker(mockLimitRepo, mockUsageRepo, testutil.NewDefaultMockClock())
	require.NoError(t, err)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("50"),
		Currency:             "USD",
		AccountID:            accountID,
		TransactionTimestamp: timestamp,
	}

	// Act: Call CheckLimits with the mock transaction
	output, err := checker.CheckLimits(ctx, mockDB, input)

	// Assert: Method runs successfully
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.True(t, output.Allowed)
	require.Len(t, output.LimitUsageDetails, 1)
	assert.Equal(t, limitID1, output.LimitUsageDetails[0].LimitID)

	assert.True(t, dbWasUsed, "CheckLimits should propagate db parameter to UpsertAndIncrementAtomic")
}
