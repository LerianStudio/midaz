// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libLog "github.com/LerianStudio/lib-observability/log"
	libRuntime "github.com/LerianStudio/lib-observability/runtime"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// AuditWriter emits protection audit events on a best-effort basis.
//
// Audit is non-blocking by design: neither method returns an error. A failed
// write is recorded as a warning and never propagated to, nor allowed to block,
// the caller. The caller's primary operation must succeed or fail on its own
// merits, independent of whether the audit trail was persisted.
type AuditWriter interface {
	// Emit writes the event synchronously, swallowing any repository error as a
	// warning. A nil event is ignored.
	Emit(ctx context.Context, event *mmodel.ProtectionAuditEvent)
	// EmitAsync writes the event on a detached, panic-safe goroutine so the audit
	// survives parent-request cancellation. It returns immediately.
	EmitAsync(ctx context.Context, event *mmodel.ProtectionAuditEvent)
}

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

// auditWriter is the repository-backed AuditWriter implementation.
type auditWriter struct {
	repo   audit.Repository
	logger libLog.Logger
}

// auditQueryService is the repository-backed AuditQueryService implementation.
type auditQueryService struct {
	repo audit.Repository
}

// NewAuditWriter returns an AuditWriter backed by the given repository.
//
// The writer is constructed only in envelope mode, alongside the keyset and
// registry repositories, so repo is always a real (non-nil) repository. There
// is no no-op fallback: a caller that has no repository has no AuditWriter.
func NewAuditWriter(repo audit.Repository, logger libLog.Logger) AuditWriter {
	return &auditWriter{repo: repo, logger: logger}
}

// NewAuditQueryService returns an AuditQueryService backed by the given audit
// repository. The repository is supplied by the composition root and is always
// a real, non-nil repository, so there is no nil guard.
func NewAuditQueryService(repo audit.Repository) AuditQueryService {
	return &auditQueryService{repo: repo}
}

// Emit writes the event synchronously. The repository error is intentionally
// swallowed: it is logged as a warning and attached to the span, but never
// returned or propagated, because audit is best-effort and must not block the
// caller. A nil event is rejected with a warning before any repository call.
func (w *auditWriter) Emit(ctx context.Context, event *mmodel.ProtectionAuditEvent) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // only the tracer is needed; the logger is injected and metrics/tracking-id are unused here

	ctx, span := tracer.Start(ctx, "service.audit.emit")
	defer span.End()

	if event == nil {
		w.logger.Log(ctx, libLog.LevelWarn, "audit emit skipped: nil event")

		return
	}

	if err := w.repo.Create(ctx, event); err != nil {
		// Best-effort contract: warn + span, never propagate.
		w.logger.Log(ctx, libLog.LevelWarn, "audit emit failed", safeAuditLogFields(event)...)
		libOpenTelemetry.HandleSpanError(span, "failed to write audit event", err)

		return
	}

	w.logger.Log(ctx, libLog.LevelDebug, "audit event emitted", safeAuditLogFields(event)...)
}

// EmitAsync writes the event on a detached, panic-safe goroutine.
//
// The goroutine uses context.WithoutCancel(ctx) so the write survives
// cancellation of the parent request: an audit record describing a completed
// action must still be persisted even if the originating HTTP request has
// already returned and its context been cancelled. This detachment is safe
// precisely because audit is best-effort — a lost write degrades observability
// but never corrupts business state. The goroutine is launched through
// lib-commons runtime.SafeGoWithContextAndComponent with the KeepRunning
// policy, which recovers any panic (e.g. from a misbehaving repository), logs
// it with a stack trace, and lets the process continue — preserving the
// best-effort, never-crash contract.
func (w *auditWriter) EmitAsync(ctx context.Context, event *mmodel.ProtectionAuditEvent) {
	detached := context.WithoutCancel(ctx)

	libRuntime.SafeGoWithContextAndComponent(detached, w.logger, "crm", "audit.emit_async",
		libRuntime.KeepRunning, func(c context.Context) {
			w.Emit(c, event)
		})
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

// safeAuditLogFields adapts the event's redacted field set to logger fields,
// guarding a nil event. It never includes sensitive data: it relies on
// ProtectionAuditEvent.SafeLogFields, which excludes free-text and PII.
func safeAuditLogFields(event *mmodel.ProtectionAuditEvent) []libLog.Field {
	if event == nil {
		return nil
	}

	safe := event.SafeLogFields()
	fields := make([]libLog.Field, 0, len(safe))

	for key, value := range safe {
		fields = append(fields, libLog.Any(key, value))
	}

	return fields
}
