// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"net"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
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
