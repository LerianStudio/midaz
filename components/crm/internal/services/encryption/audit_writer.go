// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libRuntime "github.com/LerianStudio/lib-commons/v5/commons/runtime"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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

// auditWriter is the repository-backed AuditWriter implementation.
type auditWriter struct {
	repo   audit.Repository
	logger libLog.Logger
}

// NewAuditWriter returns an AuditWriter backed by the given repository.
//
// The writer is constructed only in envelope mode, alongside the keyset and
// registry repositories, so repo is always a real (non-nil) repository. There
// is no no-op fallback: a caller that has no repository has no AuditWriter.
func NewAuditWriter(repo audit.Repository, logger libLog.Logger) AuditWriter {
	return &auditWriter{repo: repo, logger: logger}
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
