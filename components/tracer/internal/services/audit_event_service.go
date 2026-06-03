// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"tracer/internal/services/query"
	"tracer/pkg/model"
)

// AuditEventService is a facade for audit event queries.
// Used by HTTP handler for GET endpoints only.
// Note: Write operations use AuditWriter interface directly in services.
type AuditEventService struct {
	getQuery    *query.GetAuditEventQuery
	listQuery   *query.ListAuditEventsQuery
	verifyQuery *query.VerifyAuditEventQuery
}

// NewAuditEventService creates a new AuditEventService.
// Returns an error if any of the required query dependencies are nil.
func NewAuditEventService(
	getQuery *query.GetAuditEventQuery,
	listQuery *query.ListAuditEventsQuery,
	verifyQuery *query.VerifyAuditEventQuery,
) (*AuditEventService, error) {
	if getQuery == nil {
		return nil, fmt.Errorf("getQuery cannot be nil")
	}

	if listQuery == nil {
		return nil, fmt.Errorf("listQuery cannot be nil")
	}

	if verifyQuery == nil {
		return nil, fmt.Errorf("verifyQuery cannot be nil")
	}

	return &AuditEventService{
		getQuery:    getQuery,
		listQuery:   listQuery,
		verifyQuery: verifyQuery,
	}, nil
}

// GetByID retrieves an audit event by its event ID.
func (s *AuditEventService) GetAuditEvent(ctx context.Context, eventID uuid.UUID) (*model.AuditEvent, error) {
	return s.getQuery.Execute(ctx, eventID)
}

// List retrieves audit events with filters.
func (s *AuditEventService) ListAuditEvents(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
	return s.listQuery.Execute(ctx, filters)
}

// ListValidations retrieves only validation events (for GET /v1/validations endpoint).
// Maintains consistency with POST /v1/validations by filtering event_type = TRANSACTION_VALIDATED.
func (s *AuditEventService) ListValidations(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
	return s.listQuery.ExecuteValidationsOnly(ctx, filters)
}

// VerifyHashChain verifies the integrity of the hash chain up to the given event.
func (s *AuditEventService) VerifyHashChain(ctx context.Context, eventID uuid.UUID) (*model.HashChainVerificationResult, error) {
	return s.verifyQuery.Execute(ctx, eventID)
}
