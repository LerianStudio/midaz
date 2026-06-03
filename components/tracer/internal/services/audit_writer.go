// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

//go:generate mockgen -source=audit_writer.go -destination=mocks/audit_writer_mock.go -package=mocks

import (
	"context"

	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// AuditWriter defines the interface for recording audit events.
// Implemented by RecordAuditEventCommand.
// Used by ValidationService, RuleService, and LimitService.
//
// Actor identity (type, id, name) and client IP are resolved from the request
// context — never from explicit parameters. The auth middleware stamps a
// contextutil.Principal on every authenticated request and ClientIPMiddleware
// stamps the client IP; the implementation reads both and falls back to a
// system actor when no Principal is present (background workers, startup
// probes, internal callers).
type AuditWriter interface {
	RecordValidationEvent(
		ctx context.Context,
		validationID uuid.UUID,
		request map[string]any,
		evalResult model.EvaluationResult,
		responseContext model.ValidationResponseContext,
	) error

	// RecordValidationEventWithTx records a validation event using the provided database connection.
	// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
	// enabling atomic operations with other database changes.
	RecordValidationEventWithTx(
		ctx context.Context,
		db pgdb.DB,
		validationID uuid.UUID,
		request map[string]any,
		evalResult model.EvaluationResult,
		responseContext model.ValidationResponseContext,
	) error

	// RecordRuleEventWithTx records a rule audit event using the provided database
	// connection. The db parameter accepts either a regular DB connection or a
	// transaction (*sql.Tx via the pgdb.Tx adapter), enabling atomic persistence
	// of rule mutations together with their audit trail.
	RecordRuleEventWithTx(
		ctx context.Context,
		db pgdb.DB,
		eventType model.AuditEventType,
		action model.AuditAction,
		ruleID uuid.UUID,
		before map[string]any,
		after map[string]any,
		reason string,
	) error

	// RecordLimitEventWithTx records a limit audit event using the provided database
	// connection. The db parameter accepts either a regular DB connection or a
	// transaction (*sql.Tx via the pgdb.Tx adapter), enabling atomic persistence
	// of limit mutations together with their audit trail.
	RecordLimitEventWithTx(
		ctx context.Context,
		db pgdb.DB,
		eventType model.AuditEventType,
		action model.AuditAction,
		limitID uuid.UUID,
		before map[string]any,
		after map[string]any,
		reason string,
	) error
}
