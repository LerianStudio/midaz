// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	"tracer/internal/services/command"
	"tracer/internal/services/query"
	"tracer/pkg/logging"
	"tracer/pkg/model"
)

// LimitService is a facade that combines limit commands and queries.
// It implements the LimitService interface expected by the HTTP handler.
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
}

// NewLimitService creates a new limit service facade.
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
) *LimitService {
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
	}
}

// CreateLimit creates a new limit.
func (s *LimitService) CreateLimit(ctx context.Context, input *command.CreateLimitInput) (*model.Limit, error) {
	return s.createCmd.Execute(ctx, input)
}

// UpdateLimit updates an existing limit.
func (s *LimitService) UpdateLimit(ctx context.Context, id uuid.UUID, input *command.UpdateLimitInput) (*model.Limit, error) {
	return s.updateCmd.Execute(ctx, id, input)
}

// ActivateLimit activates an inactive limit.
func (s *LimitService) ActivateLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.activateCmd.Execute(ctx, id)
}

// DeactivateLimit deactivates an active limit.
func (s *LimitService) DeactivateLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.deactivateCmd.Execute(ctx, id)
}

// DraftLimit transitions a limit to draft (INACTIVE -> DRAFT).
func (s *LimitService) DraftLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.draftCmd.Execute(ctx, id)
}

// DeleteLimit soft-deletes a limit.
func (s *LimitService) DeleteLimit(ctx context.Context, id uuid.UUID) error {
	return s.deleteCmd.Execute(ctx, id)
}

// GetLimit retrieves a limit by ID.
func (s *LimitService) GetLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	return s.getQuery.Execute(ctx, id)
}

// ListLimits retrieves limits with filters.
func (s *LimitService) ListLimits(ctx context.Context, filter *model.ListLimitsFilter) (*model.ListLimitsResult, error) {
	return s.listQuery.Execute(ctx, filter)
}

// GetLimitUsage retrieves a usage snapshot for a limit.
// Returns aggregated usage information including currentUsage (sum of all counters),
// utilizationPercent, nearLimit flag (>80%), and resetAt time.
// For PER_TRANSACTION limits, currentUsage is always 0 and resetAt is nil.
func (s *LimitService) GetLimitUsage(ctx context.Context, limitID uuid.UUID) (*model.UsageSnapshot, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.get_usage")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Get the limit to access MaxAmount and ResetAt
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

	counters, err := s.usageCounterRepo.GetByLimitID(ctx, limitID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get usage counters", err)

		logger.With(
			libLog.String("operation", "service.limit.get_usage"),
			libLog.String("limit_id", limitID.String()),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to retrieve usage counters")

		return nil, err
	}

	// Create the usage snapshot
	snapshot := model.NewUsageSnapshot(limit, counters)

	logger.With(
		libLog.String("operation", "service.limit.get_usage"),
		libLog.String("limit_id", limitID.String()),
		libLog.Any("current_usage", snapshot.CurrentUsage),
		libLog.Any("limit_amount", snapshot.LimitAmount),
		libLog.Any("utilization_percent", snapshot.UtilizationPercent),
		libLog.Bool("near_limit", snapshot.NearLimit),
	).Log(ctx, libLog.LevelInfo, "Retrieved usage snapshot")

	return snapshot, nil
}
