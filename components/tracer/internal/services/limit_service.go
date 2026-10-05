// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ErrNilLimitServiceClock is returned when NewLimitService receives a nil clock.
var ErrNilLimitServiceClock = errors.New("limit service: clock cannot be nil")

// LimitService is a facade that combines limit commands and queries.
// It implements the LimitService interface expected by the HTTP handler.
//
// Every limit it returns carries the ResetAt of the period current at the
// service clock's now. The stored reset_at is fixed at creation, so echoing it
// would report a boundary that has already passed.
type LimitService struct {
	createCmd        *command.CreateLimitCommand
	updateCmd        *command.UpdateLimitCommand
	activateCmd      *command.ActivateLimitCommand
	deactivateCmd    *command.DeactivateLimitCommand
	draftCmd         *command.DraftLimitCommand
	deleteCmd        *command.DeleteLimitCommand
	getQuery         *query.GetLimitQuery
	listQuery        *query.ListLimitsQuery
	usageCounterRepo query.UsageCounterRepository
	clock            clock.Clock
}

// NewLimitService creates a new limit service facade.
// clk is the reference "now" for the resetAt of every returned limit; a nil
// clk is rejected with ErrNilLimitServiceClock.
func NewLimitService(
	createCmd *command.CreateLimitCommand,
	updateCmd *command.UpdateLimitCommand,
	activateCmd *command.ActivateLimitCommand,
	deactivateCmd *command.DeactivateLimitCommand,
	draftCmd *command.DraftLimitCommand,
	deleteCmd *command.DeleteLimitCommand,
	getQuery *query.GetLimitQuery,
	listQuery *query.ListLimitsQuery,
	usageCounterRepo query.UsageCounterRepository,
	clk clock.Clock,
) (*LimitService, error) {
	if clk == nil {
		return nil, ErrNilLimitServiceClock
	}

	return &LimitService{
		createCmd:        createCmd,
		updateCmd:        updateCmd,
		activateCmd:      activateCmd,
		deactivateCmd:    deactivateCmd,
		draftCmd:         draftCmd,
		deleteCmd:        deleteCmd,
		getQuery:         getQuery,
		listQuery:        listQuery,
		usageCounterRepo: usageCounterRepo,
		clock:            clk,
	}, nil
}

// CreateLimit creates a new limit.
func (s *LimitService) CreateLimit(ctx context.Context, input *command.CreateLimitInput) (*model.Limit, error) {
	return s.withCurrentResetAt(s.createCmd.Execute(ctx, input))
}

// UpdateLimit updates an existing limit.
func (s *LimitService) UpdateLimit(ctx context.Context, id uuid.UUID, input *command.UpdateLimitInput) (*model.Limit, error) {
	return s.withCurrentResetAt(s.updateCmd.Execute(ctx, id, input))
}

// ActivateLimit activates an inactive limit.
func (s *LimitService) ActivateLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.withCurrentResetAt(s.activateCmd.Execute(ctx, id))
}

// DeactivateLimit deactivates an active limit.
func (s *LimitService) DeactivateLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.withCurrentResetAt(s.deactivateCmd.Execute(ctx, id))
}

// DraftLimit transitions a limit to draft (INACTIVE -> DRAFT).
func (s *LimitService) DraftLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.withCurrentResetAt(s.draftCmd.Execute(ctx, id))
}

// DeleteLimit soft-deletes a limit.
func (s *LimitService) DeleteLimit(ctx context.Context, id uuid.UUID) error {
	return s.deleteCmd.Execute(ctx, id)
}

// GetLimit retrieves a limit by ID.
func (s *LimitService) GetLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.withCurrentResetAt(s.getQuery.Execute(ctx, id))
}

// ListLimits retrieves limits with filters.
func (s *LimitService) ListLimits(ctx context.Context, filter *model.ListLimitsFilter) (*model.ListLimitsResult, error) {
	result, err := s.listQuery.Execute(ctx, filter)
	if err != nil || result == nil {
		return result, err
	}

	now := s.clock.Now()

	for i := range result.Limits {
		setCurrentResetAt(&result.Limits[i], now)
	}

	return result, nil
}

// GetLimitUsage retrieves the usage snapshot of a limit in the period that
// contains the service clock's now: the usage of its most consumed scope,
// utilizationPercent, nearLimit flag (>80%), and the limit's next reset after
// now. For PER_TRANSACTION limits, currentUsage is always 0 and resetAt is nil.
func (s *LimitService) GetLimitUsage(ctx context.Context, limitID uuid.UUID) (*model.UsageSnapshot, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.get_usage")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	limit, err := s.getQuery.Execute(ctx, limitID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get limit", err)

		logger.With(
			libLog.String("operation", "service.limit.get_usage"),
			libLog.String("limit_id", limitID.String()),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to retrieve limit")

		return nil, err
	}

	now := s.clock.Now()

	counters, err := s.currentPeriodCounters(ctx, limit, now)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get usage counters", err)

		logger.With(
			libLog.String("operation", "service.limit.get_usage"),
			libLog.String("limit_id", limitID.String()),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to retrieve usage counters")

		return nil, err
	}

	setCurrentResetAt(limit, now)

	snapshot := model.NewUsageSnapshot(limit, counters)

	logger.With(
		libLog.String("operation", "service.limit.get_usage"),
		libLog.String("limit_id", limitID.String()),
		libLog.Any("current_usage", snapshot.CurrentUsage),
		libLog.Any("limit_amount", snapshot.LimitAmount),
		libLog.Any("utilization_percent", snapshot.UtilizationPercent),
		libLog.Bool("near_limit", snapshot.NearLimit),
	).Log(ctx, libLog.LevelDebug, "Retrieved usage snapshot")

	return snapshot, nil
}

// currentPeriodCounters reads the counters of the period containing now. A
// PER_TRANSACTION limit keeps no counters, so nothing is read for it.
func (s *LimitService) currentPeriodCounters(ctx context.Context, limit *model.Limit, now time.Time) ([]model.UsageCounter, error) {
	if limit.LimitType == model.LimitTypePerTransaction {
		return nil, nil
	}

	periodKey, err := limit.PeriodKey(now)
	if err != nil {
		return nil, err
	}

	return s.usageCounterRepo.GetByLimitIDAndPeriod(ctx, limit.ID, periodKey)
}

// withCurrentResetAt refreshes the ResetAt of a limit returned without error.
func (s *LimitService) withCurrentResetAt(limit *model.Limit, err error) (*model.Limit, error) {
	if err != nil {
		return limit, err
	}

	setCurrentResetAt(limit, s.clock.Now())

	return limit, nil
}

// setCurrentResetAt sets ResetAt to the limit's next reset after now: nil for
// PER_TRANSACTION, the day after the end date for CUSTOM.
func setCurrentResetAt(limit *model.Limit, now time.Time) {
	if limit == nil {
		return
	}

	limit.ResetAt = limit.NextResetAt(now)
}
