// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// VerifyAuditEventQuery handles hash chain verification.
type VerifyAuditEventQuery struct {
	repo AuditEventRepository
}

// NewVerifyAuditEventQuery creates a new VerifyAuditEventQuery.
// Returns error if repo is nil.
func NewVerifyAuditEventQuery(repo AuditEventRepository) (*VerifyAuditEventQuery, error) {
	if repo == nil {
		return nil, errors.New("audit event repository cannot be nil")
	}

	return &VerifyAuditEventQuery{repo: repo}, nil
}

// Execute verifies the hash chain integrity up to the given event.
// Returns constant.ErrAuditEventNotFound if eventID is nil/zero.
func (q *VerifyAuditEventQuery) Execute(ctx context.Context, eventID uuid.UUID) (*model.HashChainVerificationResult, error) {
	if eventID == uuid.Nil {
		return nil, constant.ErrAuditEventNotFound
	}

	return q.repo.VerifyHashChain(ctx, eventID)
}
