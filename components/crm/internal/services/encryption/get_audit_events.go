// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// AuditQueryService reads protection audit events for an organization.
//
// It is the read-side counterpart of AuditWriter: a thin, read-only
// orchestration over the audit repository. The repository owns paging clamping,
// sort defaulting, filtering, and opaque cursor handling; this service passes
// the query straight through and returns the repository output unchanged so the
// HTTP handler can map any error (including libHTTP.ErrInvalidCursor) to the
// appropriate status code.
type AuditQueryService interface {
	// GetAuditEvents returns the audit events for organizationID matching query,
	// together with the opaque next/prev cursor pagination produced by the
	// repository. The query is forwarded verbatim — limit clamping and sort
	// defaulting are the repository's responsibility. Any repository error is
	// returned unchanged for the caller to translate to an HTTP status.
	GetAuditEvents(ctx context.Context, organizationID string, query audit.AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error)
}

// auditQueryService is the repository-backed AuditQueryService implementation.
type auditQueryService struct {
	repo audit.Repository
}

// NewAuditQueryService returns an AuditQueryService backed by the given audit
// repository. The repository is supplied by the composition root and is always
// a real, non-nil repository, so there is no nil guard.
func NewAuditQueryService(repo audit.Repository) AuditQueryService {
	return &auditQueryService{repo: repo}
}

func (s *auditQueryService) GetAuditEvents(ctx context.Context, organizationID string, query audit.AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.protection.get_audit_events")
	defer span.End()

	// Safe attributes only: org id and paging shape, plus which filters are set —
	// never the filter VALUES (action/actor/outcome), which could carry context
	// not appropriate for telemetry.
	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID),
		attribute.Int("app.request.limit", query.Limit),
		attribute.String("app.request.sort_order", query.SortOrder),
		attribute.Bool("app.request.filter_action", query.Action != ""),
		attribute.Bool("app.request.filter_actor", query.Actor != ""),
		attribute.Bool("app.request.filter_outcome", query.Outcome != ""),
	)

	events, pagination, err := s.repo.FindByOrganization(ctx, organizationID, query)
	if err != nil {
		// Return the error unchanged: the handler maps repository errors
		// (including libHTTP.ErrInvalidCursor) to the correct HTTP status.
		libOpenTelemetry.HandleSpanError(span, "Failed to query audit events", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to query audit events", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	return events, pagination, nil
}
