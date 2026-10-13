// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

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

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// The service clock sits one month after the stored reset instant, so a
// response that echoes the stored value instead of recomputing it is visible.
var (
	limitServiceNow       = time.Date(2026, 10, 2, 0, 30, 0, 0, time.UTC)
	staleStoredResetAt    = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	nextMidnightAfterNow  = time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	nextNineAMAfterNow    = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	limitServiceCreatedAt = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
)

// limitServiceAuditStub accepts the limit audit event written inside the
// create transaction; any other audit call panics through the nil embed.
type limitServiceAuditStub struct {
	command.AuditWriter
}

func (limitServiceAuditStub) RecordLimitEventWithTx(
	context.Context, pgdb.DB, model.AuditEventType, model.AuditAction, uuid.UUID, map[string]any, map[string]any, string,
) error {
	return nil
}

type limitServiceFixture struct {
	service     *LimitService
	commandRepo *command.MockLimitRepository
	queryRepo   *query.MockLimitRepository
	usageRepo   *query.MockUsageCounterRepository
	txBeginner  *pgdbMocks.MockTxBeginner
	tx          *pgdbMocks.MockTx
}

// newLimitServiceFixture wires the real commands and queries over mocked
// repositories. commandNow drives the commands; serviceNow drives the service.
func newLimitServiceFixture(t *testing.T, commandNow, serviceNow time.Time) limitServiceFixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	commandRepo := command.NewMockLimitRepository(ctrl)
	queryRepo := query.NewMockLimitRepository(ctrl)
	usageRepo := query.NewMockUsageCounterRepository(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	tx := pgdbMocks.NewMockTx(ctrl)
	commandClock := clock.NewFixedClock(commandNow)
	audit := limitServiceAuditStub{}

	createCmd, err := command.NewCreateLimitCommand(commandRepo, commandClock, audit, txBeginner)
	require.NoError(t, err)

	updateCmd, err := command.NewUpdateLimitCommand(commandRepo, commandClock, audit, txBeginner)
	require.NoError(t, err)

	activateCmd, err := command.NewActivateLimitCommand(commandRepo, commandClock, audit, txBeginner)
	require.NoError(t, err)

	deactivateCmd, err := command.NewDeactivateLimitCommand(commandRepo, commandClock, audit, txBeginner)
	require.NoError(t, err)

	draftCmd, err := command.NewDraftLimitCommand(commandRepo, commandClock, audit, txBeginner)
	require.NoError(t, err)

	deleteCmd, err := command.NewDeleteLimitCommand(commandRepo, commandClock, audit, txBeginner)
	require.NoError(t, err)

	listQuery, err := query.NewListLimitsQuery(queryRepo)
	require.NoError(t, err)

	service, err := NewLimitService(
		createCmd, updateCmd, activateCmd, deactivateCmd, draftCmd, deleteCmd,
		query.NewGetLimitQuery(queryRepo), listQuery, usageRepo, clock.NewFixedClock(serviceNow),
	)
	require.NoError(t, err)

	return limitServiceFixture{
		service:     service,
		commandRepo: commandRepo,
		queryRepo:   queryRepo,
		usageRepo:   usageRepo,
		txBeginner:  txBeginner,
		tx:          tx,
	}
}

// storedLimit is a limit as the repository returns it: reset_at frozen at an
// instant that has already passed.
func storedLimit(t *testing.T, seed int64, limitType model.LimitType, status model.LimitStatus, resetTime string) *model.Limit {
	t.Helper()

	resetAt := staleStoredResetAt

	limit := &model.Limit{
		ID:        testutil.MustDeterministicUUID(seed),
		Name:      "Stored Limit",
		LimitType: limitType,
		MaxAmount: decimal.RequireFromString("1000"),
		Asset:     "BRL",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(100 + seed))}},
		Status:    status,
		ResetAt:   &resetAt,
		CreatedAt: limitServiceCreatedAt,
		UpdatedAt: limitServiceCreatedAt,
	}

	if resetTime != "" {
		tod, err := model.NewTimeOfDay(resetTime)
		require.NoError(t, err)

		limit.ResetTime = &tod
	}

	return limit
}

func requireResetAt(t *testing.T, want time.Time, got *time.Time) {
	t.Helper()

	require.NotNil(t, got, "resetAt must be set")
	assert.True(t, want.Equal(*got), "resetAt: want %s, got %s", want, *got)
}

func TestNewLimitService_NilClock(t *testing.T) {
	service, err := NewLimitService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	require.ErrorIs(t, err, ErrNilLimitServiceClock)
	assert.Nil(t, service)
}

func TestLimitService_GetLimit_ResetAtFromServiceClock(t *testing.T) {
	customStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	customEnd := time.Date(2026, 12, 31, 15, 0, 0, 0, time.UTC)
	customReset := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		limit func(t *testing.T) *model.Limit
		want  *time.Time
	}{
		{
			name: "daily without reset time resets at the next midnight UTC",
			limit: func(t *testing.T) *model.Limit {
				return storedLimit(t, 1, model.LimitTypeDaily, model.LimitStatusActive, "")
			},
			want: &nextMidnightAfterNow,
		},
		{
			name: "daily with reset time resets at its next occurrence",
			limit: func(t *testing.T) *model.Limit {
				return storedLimit(t, 2, model.LimitTypeDaily, model.LimitStatusActive, "09:00")
			},
			want: &nextNineAMAfterNow,
		},
		{
			name: "custom resets the day after its end date",
			limit: func(t *testing.T) *model.Limit {
				limit := storedLimit(t, 3, model.LimitTypeCustom, model.LimitStatusActive, "")
				limit.CustomStartDate = &customStart
				limit.CustomEndDate = &customEnd

				return limit
			},
			want: &customReset,
		},
		{
			name: "per transaction has no reset",
			limit: func(t *testing.T) *model.Limit {
				limit := storedLimit(t, 4, model.LimitTypePerTransaction, model.LimitStatusActive, "")
				limit.ResetAt = nil

				return limit
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
			stored := tt.limit(t)

			f.queryRepo.EXPECT().GetByID(gomock.Any(), stored.ID).Return(stored, nil)

			got, err := f.service.GetLimit(context.Background(), stored.ID)
			require.NoError(t, err)

			if tt.want == nil {
				assert.Nil(t, got.ResetAt)
				return
			}

			requireResetAt(t, *tt.want, got.ResetAt)
		})
	}
}

func TestLimitService_GetLimit_ErrorPassesThrough(t *testing.T) {
	f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
	id := testutil.MustDeterministicUUID(5)
	repoErr := errors.New("connection refused")

	f.queryRepo.EXPECT().GetByID(gomock.Any(), id).Return(nil, repoErr)

	got, err := f.service.GetLimit(context.Background(), id)

	require.ErrorIs(t, err, repoErr)
	assert.Nil(t, got)
}

func TestLimitService_ListLimits_ResetAtFromServiceClock(t *testing.T) {
	f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
	plain := storedLimit(t, 6, model.LimitTypeDaily, model.LimitStatusActive, "")
	withResetTime := storedLimit(t, 7, model.LimitTypeDaily, model.LimitStatusActive, "09:00")

	f.queryRepo.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListLimitsResult{
		Limits: []model.Limit{*plain, *withResetTime},
	}, nil)

	got, err := f.service.ListLimits(context.Background(), &model.ListLimitsFilter{})
	require.NoError(t, err)
	require.Len(t, got.Limits, 2)

	requireResetAt(t, nextMidnightAfterNow, got.Limits[0].ResetAt)
	requireResetAt(t, nextNineAMAfterNow, got.Limits[1].ResetAt)
}

func TestLimitService_GetLimitUsage_ResetAtFromServiceClock(t *testing.T) {
	tests := []struct {
		name      string
		resetTime string
		want      time.Time
	}{
		{name: "without reset time", resetTime: "", want: nextMidnightAfterNow},
		{name: "with reset time", resetTime: "09:00", want: nextNineAMAfterNow},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
			stored := storedLimit(t, int64(10+i), model.LimitTypeDaily, model.LimitStatusActive, tt.resetTime)

			f.queryRepo.EXPECT().GetByID(gomock.Any(), stored.ID).Return(stored, nil)
			f.usageRepo.EXPECT().GetByLimitIDAndPeriod(gomock.Any(), stored.ID, gomock.Any()).Return([]model.UsageCounter{
				{LimitID: stored.ID, ScopeKey: "account", PeriodKey: "2026-10-01", CurrentUsage: decimal.RequireFromString("250")},
			}, nil)

			got, err := f.service.GetLimitUsage(context.Background(), stored.ID)
			require.NoError(t, err)

			requireResetAt(t, tt.want, got.ResetAt)
			assert.True(t, decimal.RequireFromString("250").Equal(got.CurrentUsage), "usage is unchanged")
		})
	}
}

// TestLimitService_GetLimitUsage_ReadsCurrentPeriodOnly proves the snapshot
// reads the counters of the period that contains the service clock's now, so
// consumption from earlier periods never surfaces as current usage. The
// period follows the limit's reset time: at 00:30 a 09:00 reset is still in
// the previous day's period.
func TestLimitService_GetLimitUsage_ReadsCurrentPeriodOnly(t *testing.T) {
	tests := []struct {
		name       string
		limitType  model.LimitType
		resetTime  string
		wantPeriod string
	}{
		{name: "daily at midnight", limitType: model.LimitTypeDaily, wantPeriod: "2026-10-02"},
		{name: "daily reset at 09:00", limitType: model.LimitTypeDaily, resetTime: "09:00", wantPeriod: "2026-10-01"},
		{name: "monthly", limitType: model.LimitTypeMonthly, wantPeriod: "2026-10"},
		{name: "weekly", limitType: model.LimitTypeWeekly, wantPeriod: "2026-W40"},
		{name: "custom", limitType: model.LimitTypeCustom, wantPeriod: "custom"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
			stored := storedLimit(t, int64(30+i), tt.limitType, model.LimitStatusActive, tt.resetTime)

			f.queryRepo.EXPECT().GetByID(gomock.Any(), stored.ID).Return(stored, nil)
			f.usageRepo.EXPECT().GetByLimitIDAndPeriod(gomock.Any(), stored.ID, tt.wantPeriod).Return([]model.UsageCounter{
				{LimitID: stored.ID, ScopeKey: "acct:a", PeriodKey: tt.wantPeriod, CurrentUsage: decimal.RequireFromString("900")},
			}, nil)

			got, err := f.service.GetLimitUsage(context.Background(), stored.ID)
			require.NoError(t, err)

			assert.True(t, decimal.RequireFromString("900").Equal(got.CurrentUsage), "want 900, got %s", got.CurrentUsage)
			assert.Equal(t, 90.0, got.UtilizationPercent)
			assert.True(t, got.NearLimit)
		})
	}
}

// TestLimitService_GetLimitUsage_PerTransactionReadsNoCounters proves a
// PER_TRANSACTION limit, which keeps no counters, answers zero usage without a
// counter read.
func TestLimitService_GetLimitUsage_PerTransactionReadsNoCounters(t *testing.T) {
	f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
	stored := storedLimit(t, 40, model.LimitTypePerTransaction, model.LimitStatusActive, "")

	f.queryRepo.EXPECT().GetByID(gomock.Any(), stored.ID).Return(stored, nil)

	got, err := f.service.GetLimitUsage(context.Background(), stored.ID)
	require.NoError(t, err)

	assert.True(t, got.CurrentUsage.IsZero())
	assert.Equal(t, 0.0, got.UtilizationPercent)
	assert.False(t, got.NearLimit)
	assert.Nil(t, got.ResetAt)
}

func TestLimitService_CreateLimit_ResetAtFromServiceClock(t *testing.T) {
	// The command stamps resetAt from its own clock at creation; the response
	// carries the boundary after the service clock's now.
	f := newLimitServiceFixture(t, limitServiceCreatedAt, limitServiceNow)

	gomock.InOrder(
		f.txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(f.tx, nil),
		f.commandRepo.EXPECT().CreateWithTx(gomock.Any(), f.tx, gomock.Any()).Return(nil),
		f.tx.EXPECT().Commit().Return(nil),
	)

	got, err := f.service.CreateLimit(context.Background(), &command.CreateLimitInput{
		Name:      "Daily Pix Limit",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Asset:     "BRL",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(20))}},
	})
	require.NoError(t, err)

	requireResetAt(t, nextMidnightAfterNow, got.ResetAt)
}

func TestLimitService_WritePaths_ResetAtFromServiceClock(t *testing.T) {
	// Each case drives a write path that returns the stored limit as read, so
	// only the service can turn its stale resetAt into the current boundary.
	tests := []struct {
		name   string
		status model.LimitStatus
		call   func(s *LimitService, id uuid.UUID) (*model.Limit, error)
	}{
		{
			name:   "update",
			status: model.LimitStatusActive,
			call: func(s *LimitService, id uuid.UUID) (*model.Limit, error) {
				return s.UpdateLimit(context.Background(), id, &command.UpdateLimitInput{})
			},
		},
		{
			name:   "activate",
			status: model.LimitStatusActive,
			call: func(s *LimitService, id uuid.UUID) (*model.Limit, error) {
				return s.ActivateLimit(context.Background(), id)
			},
		},
		{
			name:   "deactivate",
			status: model.LimitStatusInactive,
			call: func(s *LimitService, id uuid.UUID) (*model.Limit, error) {
				return s.DeactivateLimit(context.Background(), id)
			},
		},
		{
			name:   "draft",
			status: model.LimitStatusDraft,
			call: func(s *LimitService, id uuid.UUID) (*model.Limit, error) {
				return s.DraftLimit(context.Background(), id)
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
			stored := storedLimit(t, int64(30+i), model.LimitTypeDaily, tt.status, "09:00")

			f.commandRepo.EXPECT().GetByID(gomock.Any(), stored.ID).Return(stored, nil)

			got, err := tt.call(f.service, stored.ID)
			require.NoError(t, err)

			requireResetAt(t, nextNineAMAfterNow, got.ResetAt)
		})
	}
}

func TestLimitService_UpdateLimit_PersistedChange_ResetAtFromServiceClock(t *testing.T) {
	f := newLimitServiceFixture(t, limitServiceNow, limitServiceNow)
	stored := storedLimit(t, 40, model.LimitTypeDaily, model.LimitStatusActive, "")
	newName := "Renamed Limit"

	f.commandRepo.EXPECT().GetByID(gomock.Any(), stored.ID).Return(stored, nil)
	gomock.InOrder(
		f.txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(f.tx, nil),
		f.commandRepo.EXPECT().UpdateWithTx(gomock.Any(), f.tx, gomock.Any()).Return(nil),
		f.tx.EXPECT().Commit().Return(nil),
	)

	got, err := f.service.UpdateLimit(context.Background(), stored.ID, &command.UpdateLimitInput{Name: &newName})
	require.NoError(t, err)

	assert.Equal(t, newName, got.Name)
	requireResetAt(t, nextMidnightAfterNow, got.ResetAt)
}
