// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ReserveCompletionConfig holds recovery storage bounds, never current rule or
// setting limits. They must cover every outstanding decision and its capacity.
type ReserveCompletionConfig struct {
	SingleTenant    bool
	MaxRules        int
	MaxReservations int
}

// ReserveCompletionReservation records the exact movement in the immutable audit
// event, not telemetry. Coordinates come from the stored reservation, never a
// re-evaluation of today's limits, account facts or policy.
type ReserveCompletionReservation struct {
	ID        uuid.UUID               `json:"reservationId"`
	LimitID   uuid.UUID               `json:"limitId"`
	ScopeKey  string                  `json:"scopeKey"`
	PeriodKey string                  `json:"periodKey"`
	Amount    decimal.Decimal         `json:"amount"`
	Before    model.ReservationStatus `json:"before"`
	After     model.ReservationStatus `json:"after"`
}

type ReserveCompletionAuditContext struct {
	IntegrationID string                         `json:"integrationId"`
	TransactionID uuid.UUID                      `json:"transactionId"`
	EvaluationID  *uuid.UUID                     `json:"evaluationId,omitempty"`
	Status        model.ReserveOperationStatus   `json:"status"`
	CompletedAt   time.Time                      `json:"completedAt"`
	Reservations  []ReserveCompletionReservation `json:"reservations"`
}

// CompleteReserveOperationCommand atomically records a known producer outcome,
// settles its existing capacity and appends one mandatory hash-chained audit
// event, including when completion precedes a decision. No current settings,
// policy, request fingerprint or TTL can erase that obligation or infer outcome.
// Transports must supply verified integration and resolved tenant context.
type CompleteReserveOperationCommand struct {
	operations ReserveOperationCompleter
	decisions  ReserveOperationDecisionReader
	capacity   DecisionCapacitySettler
	audit      AuditEventRepository
	tx         pgdb.TxBeginner
	clock      clock.Clock
	config     ReserveCompletionConfig
}

func NewCompleteReserveOperationCommand(operations ReserveOperationCompleter, decisions ReserveOperationDecisionReader, capacity DecisionCapacitySettler, audit AuditEventRepository, tx pgdb.TxBeginner, clk clock.Clock, config ReserveCompletionConfig) (*CompleteReserveOperationCommand, error) {
	if operations == nil || decisions == nil || capacity == nil || audit == nil || tx == nil || clk == nil {
		return nil, pgdb.ErrNilConnection
	}

	if config.MaxRules <= 0 || config.MaxReservations <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	return &CompleteReserveOperationCommand{operations: operations, decisions: decisions, capacity: capacity, audit: audit, tx: tx, clock: clk, config: config}, nil
}

// Execute accepts no integration or evaluation ID from the caller. Repeats of a
// committed outcome return its original timestamp without another audit event.
// A commit failure returns no successful result and is never automatically retried.
func (c *CompleteReserveOperationCommand) Execute(ctx context.Context, transactionID uuid.UUID, status model.ReserveOperationStatus) (*model.ReserveOperationState, error) {
	return c.execute(ctx, transactionID, status, nil)
}

// ExecuteReport adds transport confirmation of the contract and actual movement
// count. On replay it reads the immutable decision in the same transaction but
// never repeats settlement or audit. No decision means no evaluation ID.
func (c *CompleteReserveOperationCommand) ExecuteReport(ctx context.Context, transactionID uuid.UUID, status model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error) {
	report := &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: string(status)}
	if _, err := c.execute(ctx, transactionID, status, report); err != nil {
		return nil, err
	}

	return report, nil
}

func (c *CompleteReserveOperationCommand) execute(ctx context.Context, transactionID uuid.UUID, status model.ReserveOperationStatus, report *tracercontract.TransactionCompletionResult) (_ *model.ReserveOperationState, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.complete_reserve_operation")
	defer span.End()
	defer func() { recordReserveCompletionError(span, retErr) }()

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	if !c.config.SingleTenant {
		if tmcore.GetTenantIDContext(ctx) == "" {
			return nil, constant.ErrReservationTenantRequired
		}

		if tmcore.GetPGContext(ctx) == nil {
			return nil, pgdb.ErrNoTenantInContext
		}
	}

	key := model.ReserveOperationIdentity{IntegrationID: identity.ID, TransactionID: transactionID}
	if err := key.Validate(); err != nil {
		return nil, err
	}

	if status != model.OperationConfirmed && status != model.OperationReleased {
		return nil, constant.ErrInvalidRequestBody
	}

	at := c.clock.Now().UTC()

	var result *model.ReserveOperationState

	if err := executeWithTx(ctx, c.tx, func(tx pgdb.Tx) error {
		state, changed, err := c.operations.CompleteWithTx(ctx, tx, key, status, at)
		if err != nil {
			return err
		}

		if state == nil || state.Status != status || state.Validate() != nil {
			return constant.ErrInternalServer
		}
		// Detach the committed response from the repository's returned pointer.
		completedAt := *state.CompletedAt

		result = &model.ReserveOperationState{Status: state.Status, CompletedAt: &completedAt}
		if err := c.completeReport(ctx, tx, key, result, changed, report); err != nil {
			return err
		}

		return ctx.Err()
	}); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reserve operation completion committed")

	return result, nil
}

func (c *CompleteReserveOperationCommand) completeReport(ctx context.Context, tx pgdb.Tx, key model.ReserveOperationIdentity, state *model.ReserveOperationState, changed bool, report *tracercontract.TransactionCompletionResult) error {
	if changed {
		if err := c.settleAndAudit(ctx, tx, key, state, report); err != nil {
			return err
		}
	} else if report != nil {
		decision, err := c.decisions.GetByOperationWithTx(ctx, tx, key)
		if err != nil {
			return err
		}

		if err := c.reportDecision(key, decision, report); err != nil {
			return err
		}
	}

	if report != nil {
		return report.Validate()
	}

	return nil
}

func (c *CompleteReserveOperationCommand) settleAndAudit(ctx context.Context, tx pgdb.Tx, key model.ReserveOperationIdentity, state *model.ReserveOperationState, report *tracercontract.TransactionCompletionResult) error {
	decision, err := c.decisions.GetByOperationWithTx(ctx, tx, key)
	if err != nil {
		return err
	}

	moved := make([]*model.Reservation, 0)
	status := model.StatusConfirmed
	eventType, action := model.AuditEventOperationConfirmed, model.AuditActionConfirm

	if state.Status == model.OperationReleased {
		status = model.StatusReleased
		eventType, action = model.AuditEventOperationReleased, model.AuditActionRelease
	}

	if decision != nil {
		if decision.Key.Identity() != key || decision.Validate(c.config.MaxRules, c.config.MaxReservations) != nil {
			return constant.ErrInternalServer
		}

		moved, err = c.capacity.SettleDecisionWithTx(ctx, tx, decision.Result.EvaluationID, status)
		if err != nil {
			return err
		}

		if err := validateCompletionCapacity(key, decision, moved); err != nil {
			return err
		}
	}

	details := make([]ReserveCompletionReservation, 0, len(moved))
	for _, res := range moved {
		details = append(details, ReserveCompletionReservation{ID: res.ID, LimitID: res.LimitID, ScopeKey: res.ScopeKey, PeriodKey: res.PeriodKey, Amount: res.Amount, Before: res.Status, After: status})
	}

	event, err := model.NewAuditEvent(eventType, action, model.AuditResultSuccess, key.TransactionID.String(), model.ResourceTypeReserveOperation,
		model.Actor{ActorType: model.ActorTypeSystem, ID: key.IntegrationID})
	if err != nil {
		return err
	}

	if report != nil {
		if err := c.reportDecision(key, decision, report); err != nil {
			return err
		}

		report.Flipped = len(moved)
	}
	event.CreatedAt = *state.CompletedAt
	event.WithContext(map[string]any{
		"integrationId": key.IntegrationID, "transactionId": key.TransactionID,
		"status": state.Status, "completedAt": *state.CompletedAt, "reservations": details,
	})

	if decision != nil {
		event.Context["evaluationId"] = decision.Result.EvaluationID
	}

	if err := c.audit.InsertWithTx(ctx, tx, event); err != nil {
		return fmt.Errorf("audit reserve operation completion: %w", err)
	}

	return nil
}

func validateCompletionCapacity(key model.ReserveOperationIdentity, decision *model.ReserveDecision, moved []*model.Reservation) error {
	if len(moved) != len(decision.Result.ReservationIDs) {
		return constant.ErrInternalServer
	}

	expected := make(map[uuid.UUID]struct{}, len(moved))
	for _, id := range decision.Result.ReservationIDs {
		expected[id] = struct{}{}
	}

	for _, res := range moved {
		if res == nil || res.TransactionID != key.TransactionID || res.Status != model.StatusReserved || !res.Amount.IsPositive() || res.Validate() != nil {
			return constant.ErrInternalServer
		}

		if _, ok := expected[res.ID]; !ok {
			return constant.ErrInternalServer
		}

		delete(expected, res.ID)
	}

	return nil
}

func recordReserveCompletionError(span trace.Span, err error) {
	if err == nil {
		return
	}

	for _, business := range []error{constant.ErrInvalidRequestBody, constant.ErrInsufficientPrivileges, constant.ErrReservationTenantRequired, constant.ErrReserveOperationConflict, constant.ErrReservationNotFound} {
		if errors.Is(err, business) {
			libOtel.HandleSpanBusinessErrorEvent(span, "reserve completion rejected", err)
			return
		}
	}

	libOtel.HandleSpanError(span, "reserve completion failed", err)
}

func (c *CompleteReserveOperationCommand) reportDecision(key model.ReserveOperationIdentity, decision *model.ReserveDecision, report *tracercontract.TransactionCompletionResult) error {
	if decision == nil {
		return nil
	}

	if decision.Key.Identity() != key || decision.Validate(c.config.MaxRules, c.config.MaxReservations) != nil {
		return constant.ErrInternalServer
	}

	id := decision.Result.EvaluationID
	report.EvaluationID = &id

	return nil
}
