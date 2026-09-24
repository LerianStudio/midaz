// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/reservationlock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ReserveAdmissionConfig explicitly bounds facts, stored results and time.
// ReservationLifetime only populates the historical column: decision-owned
// reservations are never expired by TTL. Known completion must settle them.
type ReserveAdmissionConfig struct {
	Plan                query.ContextReservationConfig
	MaxRules            int
	SingleTenant        bool
	MaxTimestampAge     time.Duration
	ClockSkewTolerance  time.Duration
	ReservationLifetime time.Duration
}

type ReserveAdmissionDependencies struct {
	Decisions    ReserveAdmissionDecisions
	Operations   ReserveAdmissionOperations
	Capacity     ReserveAdmissionCapacity
	Limits       ReserveAdmissionLimits
	Policies     ReserveAdmissionPolicies
	Evaluator    ReserveAdmissionEvaluator
	Audit        AuditEventRepository
	Transactions pgdb.TxBeginner
}

// ReserveAdmissionCommand composes rules, capacity, immutable decision and audit.
// It never commits accounting, retries unknown outcomes or consumes usage directly.
type ReserveAdmissionCommand struct {
	deps    ReserveAdmissionDependencies
	clock   clock.Clock
	config  ReserveAdmissionConfig
	planner *query.ContextReservationResolver
}

func NewReserveAdmissionCommand(deps ReserveAdmissionDependencies, clk clock.Clock, config ReserveAdmissionConfig) (*ReserveAdmissionCommand, error) {
	if deps.Decisions == nil || deps.Operations == nil || deps.Capacity == nil || deps.Limits == nil || deps.Policies == nil || deps.Evaluator == nil || deps.Audit == nil || deps.Transactions == nil || clk == nil {
		return nil, pgdb.ErrNilConnection
	}

	if config.MaxRules <= 0 || config.MaxTimestampAge <= 0 || config.ClockSkewTolerance < 0 || config.ReservationLifetime <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	planner, err := query.NewContextReservationResolver(clk, config.Plan)
	if err != nil {
		return nil, err
	}

	return &ReserveAdmissionCommand{deps: deps, clock: clk, config: config, planner: planner}, nil
}

// Execute authenticates and checks replay before temporal/policy validation.
// New operations serialize before the second replay check; commit failure
// returns no result and no automatic retry. Transport wiring is deliberately
// separate so both REST and gRPC use this same admission transaction.
func (c *ReserveAdmissionCommand) Execute(ctx context.Context, r tracercontract.ReserveRequest) (_ *tracercontract.ReserveResult, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reserve_admission")
	defer span.End()
	defer func() { recordReserveAdmissionError(span, retErr) }()

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	tenant := tmcore.GetTenantIDContext(ctx)
	if !c.config.SingleTenant {
		if tenant == "" {
			return nil, constant.ErrReservationTenantRequired
		}

		if tmcore.GetPGContext(ctx) == nil {
			return nil, pgdb.ErrNoTenantInContext
		}
	}

	hash, err := r.Fingerprint(ctx, tracercontract.ReserveScope{TenantID: tenant, IntegrationID: identity.ID, AssetNamespace: identity.AssetNamespace, SingleTenant: c.config.SingleTenant}, c.config.Plan.Facts)
	if err != nil {
		return nil, err
	}

	key := model.ReserveOperationKey{IntegrationID: identity.ID, TransactionID: r.TransactionID, RequestID: r.RequestID}

	stored, err := c.deps.Decisions.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if stored != nil {
		return c.replay(r, key, hash, stored)
	}

	var result *tracercontract.ReserveResult

	err = executeWithTx(ctx, c.deps.Transactions, func(tx pgdb.Tx) error {
		state, err := c.deps.Operations.LockWithTx(ctx, tx, key.Identity())
		if err != nil {
			return err
		}

		lockedDecision, err := c.deps.Decisions.GetWithTx(ctx, tx, key)
		if err != nil {
			return err
		}

		if lockedDecision != nil {
			result, err = c.replay(r, key, hash, lockedDecision)
			return err
		}

		if state == nil || state.Validate() != nil {
			return constant.ErrInternalServer
		}

		if state.Status != model.OperationOpen {
			return constant.ErrReserveOperationConflict
		}

		decision, err := c.admit(ctx, tx, r, key, hash, identity.AssetNamespace)
		if err != nil {
			return err
		}

		result = &decision.Result

		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reserve decision committed")

	return result, nil
}

func (c *ReserveAdmissionCommand) replay(r tracercontract.ReserveRequest, key model.ReserveOperationKey, hash [sha256.Size]byte, d *model.ReserveDecision) (*tracercontract.ReserveResult, error) {
	if d.Validate(c.config.MaxRules, c.config.Plan.MaxReservations) != nil {
		return nil, constant.ErrInternalServer
	}

	if d.Key != key || d.ContextID != r.ContextID || d.ValidationMode != r.ValidationMode || d.Fingerprint != hash {
		return nil, constant.ErrReserveDecisionConflict
	}

	snapshot := d.Clone()

	return &snapshot.Result, nil
}

func (c *ReserveAdmissionCommand) admit(ctx context.Context, tx pgdb.Tx, r tracercontract.ReserveRequest, key model.ReserveOperationKey, hash [sha256.Size]byte, namespace string) (*model.ReserveDecision, error) {
	now := c.clock.Now().UTC()
	if now.IsZero() {
		return nil, constant.ErrInternalServer
	}

	if r.TransactionTimestamp.After(now.Add(c.config.ClockSkewTolerance)) {
		return nil, constant.ErrValidationTimestampFuture
	}

	if !r.TransactionTimestamp.After(now.Add(-c.config.MaxTimestampAge)) {
		return nil, constant.ErrValidationTimestampPast
	}

	d := &model.ReserveDecision{
		Key: key, ContextID: r.ContextID, Fingerprint: hash, ValidationMode: r.ValidationMode, CreatedAt: now,
		Result: tracercontract.ReserveResult{
			ContractRevision: r.ContractRevision, TransactionID: r.TransactionID, EvaluationID: uuid.New(),
			Decision:       tracercontract.DecisionAllow,
			Controls:       tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated},
			ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{},
		},
	}
	if r.ValidationMode == tracercontract.ValidationRulesAndLimits {
		if err := c.rules(ctx, tx, r, namespace, d); err != nil {
			return nil, err
		}
	}

	if d.Result.Decision == tracercontract.DecisionDeny {
		d.Result.Controls.Limits = tracercontract.LimitsSkippedRuleDeny
	} else if err := c.reserve(ctx, tx, r, namespace, d); err != nil {
		return nil, err
	}

	slices.Sort(d.Result.Reasons)

	if err := d.Validate(c.config.MaxRules, c.config.Plan.MaxReservations); err != nil {
		return nil, err
	}

	if err := c.deps.Decisions.CreateWithTx(ctx, tx, *d); err != nil {
		return nil, err
	}

	if err := c.audit(ctx, tx, d); err != nil {
		return nil, err
	}

	return d, nil
}

func (c *ReserveAdmissionCommand) rules(ctx context.Context, tx pgdb.Tx, r tracercontract.ReserveRequest, namespace string, d *model.ReserveDecision) error {
	prepared, err := c.deps.Policies.ExecuteWithTx(ctx, tx, r.ContextID)
	if err != nil {
		return err
	}

	if prepared == nil || prepared.Resolved == nil || prepared.Program == nil {
		return constant.ErrContextPolicyUnavailable
	}

	resolved := prepared.Resolved
	if resolved.Binding != (model.PolicyBindingKey{IntegrationID: d.Key.IntegrationID, ContextID: r.ContextID}) || resolved.Identity.AssetNamespace != namespace || resolved.Identity.ID != d.Key.IntegrationID {
		return constant.ErrContextPolicyUnavailable
	}

	result, err := c.deps.Evaluator.Execute(ctx, prepared.Program, r.Context, namespace)
	if err != nil {
		return err
	}

	if result == nil || result.PolicyID != resolved.Policy.ID || result.PolicyRevision != resolved.Policy.Revision {
		return constant.ErrContextPolicyUnavailable
	}

	d.Policy = &model.ReserveDecisionPolicy{ID: result.PolicyID, Revision: result.PolicyRevision, BindingVersion: resolved.Policy.BindingVersion, DefaultUsed: len(result.MatchedRules) == 0, EvaluatedRules: result.EvaluatedRules, MatchedRules: result.MatchedRules}
	d.Result.Decision = tracercontract.Decision(result.Decision)
	d.Result.Controls.Rules = tracercontract.RulesEvaluated
	reason := tracercontract.ReasonRuleAllow

	switch d.Result.Decision {
	case tracercontract.DecisionDeny:
		reason = tracercontract.ReasonRuleDeny
	case tracercontract.DecisionReview:
		reason = tracercontract.ReasonRuleReview
	case tracercontract.DecisionAllow:
	default:
		return constant.ErrContextPolicyUnavailable
	}

	if d.Policy.DefaultUsed {
		switch d.Result.Decision {
		case tracercontract.DecisionDeny:
			reason = tracercontract.ReasonPolicyDefaultDeny
		case tracercontract.DecisionAllow:
			reason = tracercontract.ReasonPolicyDefaultAllow
		default:
			return constant.ErrContextPolicyUnavailable
		}
	}

	d.Result.Reasons = append(d.Result.Reasons, reason)

	return nil
}

func (c *ReserveAdmissionCommand) reserve(ctx context.Context, tx pgdb.Tx, r tracercontract.ReserveRequest, namespace string, d *model.ReserveDecision) error {
	debits, err := model.AccountDebits(ctx, r.Context, namespace, c.config.Plan.Facts)
	if err != nil {
		return err
	}

	ids := make([]uuid.UUID, len(debits))
	for i, debit := range debits {
		ids[i] = debit.AccountID
	}

	for _, key := range reservationlock.AccountKeys(ids) {
		if err := c.deps.Capacity.AcquireReserveScopeLock(ctx, tx, key); err != nil {
			return err
		}
	}

	limits, err := c.deps.Limits.ListCandidatesWithTx(ctx, tx, namespace, ids)
	if err != nil {
		return err
	}

	plan, err := c.planner.Execute(ctx, r.Context, namespace, limits)
	if err != nil {
		return err
	}

	denied := plan.Denied
	if !denied {
		denied, err = c.capacity(ctx, tx, plan, d)
		if err != nil {
			return err
		}
	}

	if denied {
		d.Result.Decision = tracercontract.DecisionDeny
		d.Result.Reasons = append(d.Result.Reasons, tracercontract.ReasonLimitExceeded)
	} else {
		d.Result.Reasons = append(d.Result.Reasons, tracercontract.ReasonLimitsSatisfied)
	}

	return nil
}

func (c *ReserveAdmissionCommand) capacity(ctx context.Context, tx pgdb.Tx, plan *query.ContextReservationPlan, d *model.ReserveDecision) (bool, error) {
	if len(plan.Reservations) == 0 {
		return false, nil
	}

	if _, err := tx.ExecContext(ctx, "SAVEPOINT reserve_capacity"); err != nil {
		return false, fmt.Errorf("begin capacity savepoint: %w", err)
	}

	denied := false

	for _, spec := range plan.Reservations {
		res, err := model.NewReservation(spec.LimitID, d.Key.TransactionID, spec.ScopeKey, spec.PeriodKey, spec.Amount, d.CreatedAt.Add(c.config.ReservationLifetime), d.CreatedAt)
		if err != nil {
			return false, err
		}

		err = c.deps.Capacity.ReserveForDecisionWithTx(ctx, tx, d.Result.EvaluationID, res, spec.MaxAmount, spec.CounterExpiresAt)
		if errors.Is(err, constant.ErrUsageCounterExceedsLimit) {
			denied = true
			break
		}

		if err != nil {
			return false, err
		}

		d.Result.ReservationIDs = append(d.Result.ReservationIDs, res.ID)
	}

	if denied || d.Result.Decision == tracercontract.DecisionReview {
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT reserve_capacity"); err != nil {
			return false, fmt.Errorf("rollback provisional capacity: %w", err)
		}

		d.Result.ReservationIDs = []uuid.UUID{}
	}

	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT reserve_capacity"); err != nil {
		return false, fmt.Errorf("release capacity savepoint: %w", err)
	}

	return denied, nil
}

func (c *ReserveAdmissionCommand) audit(ctx context.Context, tx pgdb.Tx, d *model.ReserveDecision) error {
	event, err := model.NewAuditEvent(model.AuditEventTransactionValidated, model.AuditActionValidate, model.AuditResult(d.Result.Decision), d.Key.TransactionID.String(), model.ResourceTypeReserveOperation, model.Actor{ActorType: model.ActorTypeSystem, ID: d.Key.IntegrationID})
	if err != nil {
		return err
	}

	event.CreatedAt = d.CreatedAt
	event.WithContext(map[string]any{
		"integrationId":    d.Key.IntegrationID,
		"transactionId":    d.Key.TransactionID,
		"requestId":        d.Key.RequestID,
		"contextId":        d.ContextID,
		"evaluationId":     d.Result.EvaluationID,
		"decision":         d.Result.Decision,
		"controls":         d.Result.Controls,
		"reasons":          d.Result.Reasons,
		"reservationIds":   d.Result.ReservationIDs,
		"policy":           d.Policy,
		"fingerprint":      hex.EncodeToString(d.Fingerprint[:]),
		"validationMode":   d.ValidationMode,
		"contractRevision": d.Result.ContractRevision,
	})

	if err := c.deps.Audit.InsertWithTx(ctx, tx, event); err != nil {
		return fmt.Errorf("audit reserve admission: %w", err)
	}

	return nil
}

func recordReserveAdmissionError(span trace.Span, err error) {
	for _, business := range []error{constant.ErrReserveDecisionConflict, constant.ErrValidationTimestampFuture, constant.ErrValidationTimestampPast} {
		if errors.Is(err, business) {
			libOtel.HandleSpanBusinessErrorEvent(span, "reserve admission rejected", err)
			return
		}
	}

	recordReserveCompletionError(span, err)
}
