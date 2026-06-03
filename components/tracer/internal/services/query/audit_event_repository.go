// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=audit_event_repository.go -destination=audit_event_repository_mock.go -package=query

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// AuditEventRepository defines the read interface for audit event queries.
type AuditEventRepository interface {
	GetByID(ctx context.Context, eventID uuid.UUID) (*model.AuditEvent, error)
	List(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error)
	VerifyHashChain(ctx context.Context, eventID uuid.UUID) (*model.HashChainVerificationResult, error)
}
