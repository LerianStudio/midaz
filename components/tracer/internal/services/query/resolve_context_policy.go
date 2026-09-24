// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"slices"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

//go:generate mockgen -source=resolve_context_policy.go -destination=mocks/active_context_policy_repository_mock.go -package=mocks

// ActiveContextPolicyRepository reads exactly one complete binding in the
// already resolved tenant database. Missing bindings must not use a fallback.
type ActiveContextPolicyRepository interface {
	GetActive(context.Context, model.PolicyBindingKey) (*model.BoundContextPolicy, error)
}

// ResolvedContextPolicy couples the immutable revision/binding version to the
// authenticated producer and asset namespace used for context validation.
type ResolvedContextPolicy struct {
	Binding  model.PolicyBindingKey
	Identity contextutil.IntegrationIdentity
	Policy   model.BoundContextPolicy
}

// ResolveContextPolicyQuery selects configuration, not a transaction decision.
// Transport authentication and tenant/pool resolution must run before it.
type ResolveContextPolicyQuery struct {
	repository ActiveContextPolicyRepository
	maxRules   int
}

func NewResolveContextPolicyQuery(repository ActiveContextPolicyRepository, maxRules int) (*ResolveContextPolicyQuery, error) {
	if repository == nil || maxRules <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &ResolveContextPolicyQuery{repository: repository, maxRules: maxRules}, nil
}

// Execute accepts only an opaque context derived by the trusted producer. The
// payload cannot select a policy, an integration, or an asset namespace. Tenant
// context is preserved unchanged; policy identity alone is not a tenant key.
func (q *ResolveContextPolicyQuery) Execute(ctx context.Context, contextID string) (_ *ResolvedContextPolicy, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.resolve_context_policy")
	defer span.End()
	defer func() { recordContextPolicyError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	key := model.PolicyBindingKey{IntegrationID: identity.ID, ContextID: contextID}
	if err := key.Validate(); err != nil {
		return nil, err
	}

	policy, err := q.repository.GetActive(ctx, key)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if policy == nil || policy.BindingVersion <= 0 || policy.Validate(q.maxRules) != nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	snapshot := *policy
	snapshot.Rules = slices.Clone(policy.Rules)
	logger.With(libLog.Int("rules.count", len(snapshot.Rules))).Log(ctx, libLog.LevelDebug, "Context policy resolved")

	return &ResolvedContextPolicy{Binding: key, Identity: identity, Policy: snapshot}, nil
}
