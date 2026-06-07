// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=get_rule.go -destination=repository_mock.go -package=query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// RuleRepository defines the interface for rule persistence (read operations).
// Interface defined in the package that USES it (per PROJECT_RULES.md).
type RuleRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Rule, error)
}

// GetRuleQuery handles retrieving a rule by ID.
type GetRuleQuery struct {
	repo RuleRepository
}

// NewGetRuleQuery creates a new GetRuleQuery instance.
func NewGetRuleQuery(repo RuleRepository) *GetRuleQuery {
	return &GetRuleQuery{
		repo: repo,
	}
}

// Execute retrieves a rule by ID.
func (q *GetRuleQuery) Execute(ctx context.Context, id uuid.UUID) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	logger.With(
		libLog.String("operation", "service.rule.get"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting rule")

	rule, err := q.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, constant.ErrRuleNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get rule", err)

		return nil, err
	}

	logger.With(
		libLog.String("operation", "service.rule.get"),
		libLog.String("rule.id", rule.ID.String()),
		libLog.String("rule.name", rule.Name),
	).Log(ctx, libLog.LevelInfo, "Rule retrieved")

	return rule, nil
}
