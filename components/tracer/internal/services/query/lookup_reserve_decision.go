// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

//go:generate mockgen -source=lookup_reserve_decision.go -destination=mocks/reserve_decision_reader_mock.go -package=mocks

// ReserveDecisionReader returns a complete immutable record from the primary
// tenant database. Nil means absent; identity reuse must return a conflict.
type ReserveDecisionReader interface {
	Get(context.Context, model.ReserveOperationKey) (*model.ReserveDecision, error)
}

// ReserveReplayConfig defines hard parsing/storage bounds, not the current CEL
// policy's limits. Do not reduce them below recoverable frozen envelopes.
type ReserveReplayConfig struct {
	Limits          tracercontract.Limits
	MaxRules        int
	MaxReservations int
	SingleTenant    bool
}

// LookupReserveDecisionQuery performs replay before policy resolution or
// evaluation. It owns no evaluator, capacity writer or audit writer. A miss
// still requires an operation lock and a second lookup in the write transaction.
type LookupReserveDecisionQuery struct {
	repository ReserveDecisionReader
	config     ReserveReplayConfig
}

func NewLookupReserveDecisionQuery(repository ReserveDecisionReader, config ReserveReplayConfig) (*LookupReserveDecisionQuery, error) {
	if repository == nil || config.MaxRules <= 0 || config.MaxReservations <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := config.Limits.Validate(); err != nil {
		return nil, err
	}

	return &LookupReserveDecisionQuery{repository: repository, config: config}, nil
}

// Execute requires verified producer identity and prior tenant/pool resolution.
// Timestamps are structurally validated without applying a new-operation age
// window. Rebinding a policy cannot replace the original response or rule refs.
func (q *LookupReserveDecisionQuery) Execute(ctx context.Context, request tracercontract.ReserveRequest) (_ *model.ReserveDecision, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.lookup_reserve_decision")
	defer span.End()
	defer func() { recordReserveReplayError(span, retErr) }()

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	tenant := tmcore.GetTenantIDContext(ctx)
	if tenant == "" && !q.config.SingleTenant {
		return nil, constant.ErrReservationTenantRequired
	}

	scope := tracercontract.ReserveScope{TenantID: tenant, IntegrationID: identity.ID, AssetNamespace: identity.AssetNamespace, SingleTenant: q.config.SingleTenant}

	fingerprint, err := request.Fingerprint(ctx, scope, q.config.Limits)
	if err != nil {
		return nil, err
	}

	key := model.ReserveOperationKey{IntegrationID: identity.ID, TransactionID: request.TransactionID, RequestID: request.RequestID}

	stored, err := q.repository.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if stored == nil {
		return nil, nil
	}

	if err := stored.Validate(q.config.MaxRules, q.config.MaxReservations); err != nil {
		return nil, constant.ErrInternalServer
	}

	if stored.Key != key || stored.ContextID != request.ContextID || stored.ValidationMode != request.ValidationMode || stored.Fingerprint != fingerprint {
		return nil, constant.ErrReserveDecisionConflict
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Replaying stored reserve decision")

	snapshot := stored.Clone()

	return &snapshot, nil
}

func recordReserveReplayError(span trace.Span, err error) {
	if err == nil {
		return
	}

	for _, business := range []error{constant.ErrInvalidRequestBody, constant.ErrInsufficientPrivileges, constant.ErrReservationTenantRequired, constant.ErrReserveDecisionConflict} {
		if errors.Is(err, business) {
			libOtel.HandleSpanBusinessErrorEvent(span, "reserve replay rejected", err)
			return
		}
	}

	libOtel.HandleSpanError(span, "reserve replay lookup failed", err)
}
