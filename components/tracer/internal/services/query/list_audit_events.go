// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ListAuditEventsQuery handles listing audit events with filters.
type ListAuditEventsQuery struct {
	repo AuditEventRepository
}

// NewListAuditEventsQuery creates a new ListAuditEventsQuery.
func NewListAuditEventsQuery(repo AuditEventRepository) (*ListAuditEventsQuery, error) {
	if repo == nil {
		return nil, errors.New("audit event repository cannot be nil")
	}

	return &ListAuditEventsQuery{repo: repo}, nil
}

// Execute retrieves audit events matching the provided filters.
func (q *ListAuditEventsQuery) Execute(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
	if filters == nil {
		filters = &model.AuditEventFilters{}
	}

	filters.SetDefaults()

	return q.repo.List(ctx, filters)
}

// ExecuteValidationsOnly retrieves only validation events (for GET /v1/validations endpoint).
// Filters by event_type = 'TRANSACTION_VALIDATED' to maintain consistency with POST /v1/validations.
func (q *ListAuditEventsQuery) ExecuteValidationsOnly(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
	if filters == nil {
		filters = &model.AuditEventFilters{}
	}

	validationType := model.AuditEventTransactionValidated
	filters.EventType = &validationType
	filters.SetDefaults()

	return q.repo.List(ctx, filters)
}
