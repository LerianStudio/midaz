// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=list_rules.go -destination=list_rules_repository_mock.go -package=query

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ListRulesRepository defines the interface for listing rules.
type ListRulesRepository interface {
	List(ctx context.Context, filter *model.ListRulesFilter) (*model.ListRulesResult, error)
}

// ListRulesQuery handles listing rules with pagination and filtering.
type ListRulesQuery struct {
	repo ListRulesRepository
}

// NewListRulesQuery creates a new ListRulesQuery instance.
func NewListRulesQuery(repo ListRulesRepository) *ListRulesQuery {
	return &ListRulesQuery{
		repo: repo,
	}
}

// Execute lists rules with cursor-based pagination and filtering.
func (q *ListRulesQuery) Execute(ctx context.Context, filter *model.ListRulesFilter) (*model.ListRulesResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Create a copy to avoid mutating caller's filter; use empty filter if nil
	var normalizedFilter model.ListRulesFilter
	if filter != nil {
		normalizedFilter = *filter
	}

	// Apply defaults
	if normalizedFilter.Limit == 0 {
		normalizedFilter.Limit = constant.DefaultPaginationLimit
	}

	// SortBy uses snake_case; repository maps to database column names
	if normalizedFilter.SortBy == "" {
		normalizedFilter.SortBy = "created_at"
	}

	if normalizedFilter.SortOrder == "" {
		normalizedFilter.SortOrder = "DESC"
	} else {
		normalizedFilter.SortOrder = strings.ToUpper(normalizedFilter.SortOrder)
	}

	logger.With(
		libLog.String("operation", "service.rule.list"),
		libLog.Int("list.limit", normalizedFilter.Limit),
		libLog.String("list.cursor", normalizedFilter.Cursor),
		libLog.String("list.sort_by", normalizedFilter.SortBy),
		libLog.String("list.sort_order", normalizedFilter.SortOrder),
	).Log(ctx, libLog.LevelInfo, "Listing rules")

	result, err := q.repo.List(ctx, &normalizedFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list rules", err)
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("app.response.rules_count", len(result.Rules)),
		attribute.Bool("app.response.has_more", result.HasMore),
	)

	logger.With(
		libLog.String("operation", "service.rule.list"),
		libLog.Int("list.count", len(result.Rules)),
		libLog.Bool("list.has_more", result.HasMore),
	).Log(ctx, libLog.LevelInfo, "Rules listed")

	return result, nil
}
