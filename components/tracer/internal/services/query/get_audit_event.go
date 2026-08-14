// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// GetAuditEventQuery handles retrieving a single audit event.
type GetAuditEventQuery struct {
	repo AuditEventRepository
}

// NewGetAuditEventQuery creates a new GetAuditEventQuery.
func NewGetAuditEventQuery(repo AuditEventRepository) (*GetAuditEventQuery, error) {
	if repo == nil {
		return nil, errors.New("audit event repository cannot be nil")
	}

	return &GetAuditEventQuery{repo: repo}, nil
}

// Execute retrieves an audit event by its event ID.
func (q *GetAuditEventQuery) Execute(ctx context.Context, eventID uuid.UUID) (*model.AuditEvent, error) {
	return q.repo.GetByID(ctx, eventID)
}
