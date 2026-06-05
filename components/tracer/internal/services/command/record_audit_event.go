// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"net"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// systemActorID is the fallback actor identifier used for events that
// originate outside an authenticated HTTP request (background workers,
// startup probes, internal callers). Requests that flow through an
// authenticated handler always carry a Principal in their context
// (stamped by the auth middleware) and never trip this fallback.
const systemActorID = "svc_tracer"

// RecordAuditEventCommand handles recording audit events.
type RecordAuditEventCommand struct {
	repo AuditEventRepository
}

// NewRecordAuditEventCommand creates a new RecordAuditEventCommand.
func NewRecordAuditEventCommand(repo AuditEventRepository) *RecordAuditEventCommand {
	return &RecordAuditEventCommand{repo: repo}
}

// systemActorName returns the human-friendly subsystem label for a system
// actor based on the resource type being audited. Preserves the granularity
// of the pre-Principal-aware fallback so audit reports can still distinguish
// "Validation Engine" from "Rule Manager" from "Limit Manager" when events
// originate outside an authenticated request.
func systemActorName(resourceType model.ResourceType) string {
	switch resourceType {
	case model.ResourceTypeTransaction:
		return "Tracer Validation Engine"
	case model.ResourceTypeRule:
		return "Tracer Rule Manager"
	case model.ResourceTypeLimit:
		return "Tracer Limit Manager"
	case model.ResourceTypeReservation:
		return "Tracer Reservation Manager"
	default:
		return "Tracer"
	}
}

// resolveActor builds the audit Actor from the request context.
//
// Order of resolution:
//
//  1. If a contextutil.Principal is present with a non-empty ID and a valid
//     ActorType, the Actor is built directly from it (user / api_key
//     attribution).
//  2. Otherwise, the Actor falls back to a system actor whose Name reflects
//     the resource being audited (preserves pre-Principal granularity).
//
// IP is always taken from ctx via ClientIPMiddleware and normalized — an
// unrecognized Principal.Type drops the principal entirely (returns system)
// to avoid misattribution: coercing only the type while keeping the
// principal's ID/Name would silently record, for example, type=system with
// id=<user-sub>.
func resolveActor(ctx context.Context, resourceType model.ResourceType) model.Actor {
	clientIP := normalizeIP(contextutil.GetClientIP(ctx))

	if p, ok := contextutil.GetPrincipal(ctx); ok && p.ID != "" {
		actorType := model.ActorType(p.Type)
		if actorType.IsValid() {
			return model.Actor{
				ActorType: actorType,
				ID:        p.ID,
				Name:      p.Name,
				IPAddress: clientIP,
			}
		}
	}

	return model.Actor{
		ActorType: model.ActorTypeSystem,
		ID:        systemActorID,
		Name:      systemActorName(resourceType),
		IPAddress: clientIP,
	}
}

// RecordValidationEvent records an audit event for a transaction validation.
// NOTE: evalResult is passed separately from responseContext to avoid embedding redundancy.
// The decision is extracted from evalResult and stored in AuditEvent.Result field.
//
// Actor identity (Principal) and client IP are resolved from ctx — see
// resolveActor for the contract.
func (c *RecordAuditEventCommand) RecordValidationEvent(
	ctx context.Context,
	validationID uuid.UUID,
	request map[string]any,
	evalResult model.EvaluationResult,
	responseContext model.ValidationResponseContext,
) error {
	result := model.DecisionToAuditResult(evalResult.Decision)

	event, err := model.NewAuditEvent(
		model.AuditEventTransactionValidated,
		model.AuditActionValidate,
		result,
		validationID.String(),
		model.ResourceTypeTransaction,
		resolveActor(ctx, model.ResourceTypeTransaction),
	)
	if err != nil {
		return fmt.Errorf("failed to create audit event: %w", err)
	}

	event.WithValidationContext(request, evalResult, responseContext)

	return c.repo.Insert(ctx, event)
}

// RecordValidationEventWithTx records an audit event for a transaction validation using the provided database connection.
// The db parameter accepts either a regular DB connection or a transaction (*sql.Tx via TxAdapter).
// Atomicity with other database changes is only guaranteed when a transaction handle is passed;
// a plain DB connection will execute the insert independently.
//
// Actor identity (Principal) and client IP are resolved from ctx — see
// resolveActor for the contract.
func (c *RecordAuditEventCommand) RecordValidationEventWithTx(
	ctx context.Context,
	db pgdb.DB,
	validationID uuid.UUID,
	request map[string]any,
	evalResult model.EvaluationResult,
	responseContext model.ValidationResponseContext,
) error {
	result := model.DecisionToAuditResult(evalResult.Decision)

	event, err := model.NewAuditEvent(
		model.AuditEventTransactionValidated,
		model.AuditActionValidate,
		result,
		validationID.String(),
		model.ResourceTypeTransaction,
		resolveActor(ctx, model.ResourceTypeTransaction),
	)
	if err != nil {
		return fmt.Errorf("failed to create audit event: %w", err)
	}

	event.WithValidationContext(request, evalResult, responseContext)

	return c.repo.InsertWithTx(ctx, db, event)
}

// RecordRuleEventWithTx records an audit event for a rule operation using the
// provided database connection.
// The db parameter accepts either a regular DB connection or a transaction
// (*sql.Tx via pgdb.Tx adapter). Atomicity with other database changes is only
// guaranteed when a transaction handle is passed; a plain DB connection will
// execute the insert independently.
//
// Actor identity (Principal) and client IP are resolved from ctx — see
// resolveActor for the contract.
func (c *RecordAuditEventCommand) RecordRuleEventWithTx(
	ctx context.Context,
	db pgdb.DB,
	eventType model.AuditEventType,
	action model.AuditAction,
	ruleID uuid.UUID,
	before map[string]any,
	after map[string]any,
	reason string,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "service.RecordAuditEventCommand.RecordRuleEventWithTx")
	defer span.End()

	event, err := c.buildRuleEvent(ctx, eventType, action, ruleID, before, after, reason)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build rule audit event", err)
		return fmt.Errorf("record rule audit event with tx: %w", err)
	}

	if err := c.repo.InsertWithTx(ctx, db, event); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert rule audit event", err)
		return fmt.Errorf("record rule audit event with tx: %w", err)
	}

	return nil
}

// RecordLimitEventWithTx records an audit event for a limit operation using
// the provided database connection.
// The db parameter accepts either a regular DB connection or a transaction
// (*sql.Tx via pgdb.Tx adapter). Atomicity with other database changes is only
// guaranteed when a transaction handle is passed; a plain DB connection will
// execute the insert independently.
//
// Actor identity (Principal) and client IP are resolved from ctx — see
// resolveActor for the contract.
func (c *RecordAuditEventCommand) RecordLimitEventWithTx(
	ctx context.Context,
	db pgdb.DB,
	eventType model.AuditEventType,
	action model.AuditAction,
	limitID uuid.UUID,
	before map[string]any,
	after map[string]any,
	reason string,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "service.RecordAuditEventCommand.RecordLimitEventWithTx")
	defer span.End()

	event, err := c.buildLimitEvent(ctx, eventType, action, limitID, before, after, reason)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build limit audit event", err)
		return fmt.Errorf("record limit audit event with tx: %w", err)
	}

	if err := c.repo.InsertWithTx(ctx, db, event); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert limit audit event", err)
		return fmt.Errorf("record limit audit event with tx: %w", err)
	}

	return nil
}

// ReservationAuditContext is the forensic payload recorded for a single
// reservation transition. It carries the resolved limit coordinates the
// reservation already holds (R38) so the audit row is self-describing without a
// limit re-query. Amount is the smallest currency unit (cents).
type ReservationAuditContext struct {
	TransactionID uuid.UUID
	LimitID       uuid.UUID
	ScopeKey      string
	PeriodKey     string
	Amount        int64
	Status        string
}

// RecordReservationEventWithTx records an audit event for a reserve / confirm /
// release transition using the provided database connection. The db parameter
// accepts a transaction (*sql.Tx via the pgdb.Tx adapter), so the audit row commits
// in the SAME tx as the counter move and the reservation-row flip — mirroring
// RecordRuleEventWithTx / RecordLimitEventWithTx.
//
// SKIPPED is NOT recorded here: it is a ledger fail-open decision with no counter
// move, so it flows through the non-tx RecordReservationEvent surface.
//
// Actor identity (Principal) and client IP are resolved from ctx — see resolveActor.
func (c *RecordAuditEventCommand) RecordReservationEventWithTx(
	ctx context.Context,
	db pgdb.DB,
	eventType model.AuditEventType,
	action model.AuditAction,
	reservationID uuid.UUID,
	auditCtx ReservationAuditContext,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "service.RecordAuditEventCommand.RecordReservationEventWithTx")
	defer span.End()

	event, err := c.buildReservationEvent(ctx, eventType, action, reservationID, auditCtx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build reservation audit event", err)
		return fmt.Errorf("record reservation audit event with tx: %w", err)
	}

	if err := c.repo.InsertWithTx(ctx, db, event); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert reservation audit event", err)
		return fmt.Errorf("record reservation audit event with tx: %w", err)
	}

	return nil
}

// RecordReservationEvent records a reservation audit event OUTSIDE any
// transaction. This is the SKIPPED surface: the ledger failed open (tracer
// unreachable) so no counter move happened and there is no tx to join. The event
// is inserted independently.
//
// Actor identity (Principal) and client IP are resolved from ctx — see resolveActor.
func (c *RecordAuditEventCommand) RecordReservationEvent(
	ctx context.Context,
	eventType model.AuditEventType,
	action model.AuditAction,
	reservationID uuid.UUID,
	auditCtx ReservationAuditContext,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "service.RecordAuditEventCommand.RecordReservationEvent")
	defer span.End()

	event, err := c.buildReservationEvent(ctx, eventType, action, reservationID, auditCtx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build reservation audit event", err)
		return fmt.Errorf("record reservation audit event: %w", err)
	}

	if err := c.repo.Insert(ctx, event); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert reservation audit event", err)
		return fmt.Errorf("record reservation audit event: %w", err)
	}

	return nil
}

// ReservationExpiryBatchSummary describes one reaper sweep: how many reservations
// expired and the time window they covered. A SINGLE audit row per sweep caps
// hash-chain advisory-lock contention on the high-volume / low-forensic-value
// expiry path (Q11).
type ReservationExpiryBatchSummary struct {
	ExpiredCount int
	SweptAt      time.Time
	OldestExpiry *time.Time
}

// RecordReservationExpiryBatch writes ONE audit row summarizing a reaper sweep of
// expired reservations, rather than one row per expired reservation. It goes
// through the same hash-chain advisory-lock path as every other audit insert
// (unchanged lock semantics) but amortizes the lock over the whole batch.
//
// The summary's ResourceID is the sweep timestamp (there is no single reservation
// id for a batch). Recorded outside a tx — the per-row EXPIRED counter moves the
// reaper performed already committed individually via ReleaseWithTx.
func (c *RecordAuditEventCommand) RecordReservationExpiryBatch(
	ctx context.Context,
	summary ReservationExpiryBatchSummary,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "service.RecordAuditEventCommand.RecordReservationExpiryBatch")
	defer span.End()

	event, err := model.NewAuditEvent(
		model.AuditEventReservationExpired,
		model.AuditActionExpire,
		model.AuditResultSuccess,
		summary.SweptAt.UTC().Format(time.RFC3339Nano),
		model.ResourceTypeReservation,
		resolveActor(ctx, model.ResourceTypeReservation),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build reservation expiry batch event", err)
		return fmt.Errorf("record reservation expiry batch: %w", err)
	}

	batchContext := map[string]any{
		"expiredCount": summary.ExpiredCount,
		"sweptAt":      summary.SweptAt.UTC().Format(time.RFC3339Nano),
	}

	if summary.OldestExpiry != nil {
		batchContext["oldestExpiry"] = summary.OldestExpiry.UTC().Format(time.RFC3339Nano)
	}

	event.WithContext(batchContext)

	if err := c.repo.Insert(ctx, event); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert reservation expiry batch event", err)
		return fmt.Errorf("record reservation expiry batch: %w", err)
	}

	return nil
}

// buildReservationEvent constructs a reservation audit event with the resolved
// actor and the transition's forensic context. Used by the WithTx, non-tx, and
// SKIPPED reservation recorders.
func (c *RecordAuditEventCommand) buildReservationEvent(
	ctx context.Context,
	eventType model.AuditEventType,
	action model.AuditAction,
	reservationID uuid.UUID,
	auditCtx ReservationAuditContext,
) (*model.AuditEvent, error) {
	event, err := model.NewAuditEvent(
		eventType,
		action,
		model.AuditResultSuccess,
		reservationID.String(),
		model.ResourceTypeReservation,
		resolveActor(ctx, model.ResourceTypeReservation),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit event: %w", err)
	}

	event.WithContext(map[string]any{
		"reservationId": reservationID.String(),
		"transactionId": auditCtx.TransactionID.String(),
		"limitId":       auditCtx.LimitID.String(),
		"scopeKey":      auditCtx.ScopeKey,
		"periodKey":     auditCtx.PeriodKey,
		"amount":        auditCtx.Amount,
		"status":        auditCtx.Status,
	})

	return event, nil
}

// buildRuleEvent constructs a rule audit event with the resolved actor and
// CRUD context. Used by RecordRuleEventWithTx.
func (c *RecordAuditEventCommand) buildRuleEvent(
	ctx context.Context,
	eventType model.AuditEventType,
	action model.AuditAction,
	ruleID uuid.UUID,
	before map[string]any,
	after map[string]any,
	reason string,
) (*model.AuditEvent, error) {
	event, err := model.NewAuditEvent(
		eventType,
		action,
		model.AuditResultSuccess,
		ruleID.String(),
		model.ResourceTypeRule,
		resolveActor(ctx, model.ResourceTypeRule),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit event: %w", err)
	}

	event.WithCRUDContext(before, after, reason)

	return event, nil
}

// buildLimitEvent constructs a limit audit event with the resolved actor and
// CRUD context. Used by RecordLimitEventWithTx.
func (c *RecordAuditEventCommand) buildLimitEvent(
	ctx context.Context,
	eventType model.AuditEventType,
	action model.AuditAction,
	limitID uuid.UUID,
	before map[string]any,
	after map[string]any,
	reason string,
) (*model.AuditEvent, error) {
	event, err := model.NewAuditEvent(
		eventType,
		action,
		model.AuditResultSuccess,
		limitID.String(),
		model.ResourceTypeLimit,
		resolveActor(ctx, model.ResourceTypeLimit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit event: %w", err)
	}

	event.WithCRUDContext(before, after, reason)

	return event, nil
}

// normalizeIP normalizes IP address for storage.
func normalizeIP(ip string) string {
	if ip == "" {
		return "0.0.0.0"
	}

	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		ip = host
	}

	if net.ParseIP(ip) == nil {
		return "0.0.0.0"
	}

	return ip
}
