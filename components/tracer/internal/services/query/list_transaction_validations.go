// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// formatTimeOrNotSet formats a time value for logging, returning "not set" for zero time.
func formatTimeOrNotSet(t time.Time) string {
	if t.IsZero() {
		return "not set"
	}

	return t.Format(time.RFC3339)
}

// ListTransactionValidationsQuery handles listing transaction validation records with cursor-based pagination and filtering.
type ListTransactionValidationsQuery struct {
	repo TransactionValidationRepository
}

// NewListTransactionValidationsQuery creates a new ListTransactionValidationsQuery instance.
func NewListTransactionValidationsQuery(repo TransactionValidationRepository) *ListTransactionValidationsQuery {
	return &ListTransactionValidationsQuery{
		repo: repo,
	}
}

// ListTransactionValidationsResult is an alias for model.ListTransactionValidationsResult.
// Cursor-based pagination returns: TransactionValidations, NextCursor, HasMore.
type ListTransactionValidationsResult = model.ListTransactionValidationsResult

// Execute lists transaction validation records with cursor-based pagination and filtering.
func (q *ListTransactionValidationsQuery) Execute(ctx context.Context, filters *model.TransactionValidationFilters) (*ListTransactionValidationsResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.transaction-validation.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Check for context cancellation at the very start, before any filter initialization or mutation
	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled before repository call", err)
		return nil, fmt.Errorf("list transaction validations: %w", err)
	}

	// Initialize filters if nil
	if filters == nil {
		filters = &model.TransactionValidationFilters{}
	}

	// Apply defaults BEFORE validation to ensure we validate the final state.
	// This ensures default values (e.g., Limit=100, date ranges) are present during validation.
	filters.SetDefaults()

	// Validate filters after defaults are applied
	if err := filters.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction validation filters", err)
		return nil, fmt.Errorf("%w: %w", constant.ErrInvalidTransactionValidationFilters, err)
	}

	// Log the filters AFTER SetDefaults() so we log the actual values being used
	logger.With(
		libLog.String("operation", "service.transaction-validation.list"),
		libLog.Int("filters.limit", filters.Limit),
		libLog.String("filters.cursor", filters.Cursor),
		libLog.String("filters.sort_by", filters.SortBy),
		libLog.String("filters.sort_order", filters.SortOrder),
		libLog.String("filters.start_date", formatTimeOrNotSet(filters.StartDate)),
		libLog.String("filters.end_date", formatTimeOrNotSet(filters.EndDate)),
	).Log(ctx, libLog.LevelInfo, "Listing transaction validation records")

	// Get transaction validation records with cursor-based pagination
	result, err := q.repo.List(ctx, filters)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list transaction validations", err)
		return nil, fmt.Errorf("repository list failed: %w", err)
	}

	span.SetAttributes(
		attribute.Int("app.response.validations_count", len(result.TransactionValidations)),
		attribute.Bool("app.response.has_more", result.HasMore),
	)

	logger.With(
		libLog.String("operation", "service.transaction-validation.list"),
		libLog.Int("list.count", len(result.TransactionValidations)),
		libLog.Bool("list.has_more", result.HasMore),
	).Log(ctx, libLog.LevelInfo, "Transaction validation records listed")

	return result, nil
}
