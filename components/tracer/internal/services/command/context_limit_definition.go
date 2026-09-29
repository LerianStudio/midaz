// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// ReservationTenancy reports whether the tenant of a request takes part in
// the reservation seam, so its limits must satisfy the account-scoped
// definition policy. An error means the answer is unknown, and the write
// that asked must not proceed.
type ReservationTenancy interface {
	Applies(ctx context.Context) (bool, error)
}

// ContextLimitDefinitionPolicy reuses admission invariants at administration.
// A nil policy is the legacy profile; bootstrap installs one for shared Reserve.
// A policy scoped to a ReservationTenancy applies only to the tenants that
// tenancy reports.
type ContextLimitDefinitionPolicy struct {
	bounds        tracercontract.Limits
	maxScopes     int
	maxScopeBytes int
	tenancy       ReservationTenancy
}

func NewContextLimitDefinitionPolicy(bounds tracercontract.Limits, maxScopes, maxScopeBytes int) (*ContextLimitDefinitionPolicy, error) {
	if maxScopes <= 0 || maxScopeBytes <= 0 {
		return nil, constant.ErrContextLimitsUnavailable
	}

	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	return &ContextLimitDefinitionPolicy{bounds: bounds, maxScopes: maxScopes, maxScopeBytes: maxScopeBytes}, nil
}

// ScopedTo returns a copy of the policy that applies only to the tenants
// tenancy reports as taking part in the reservation seam.
func (p *ContextLimitDefinitionPolicy) ScopedTo(tenancy ReservationTenancy) *ContextLimitDefinitionPolicy {
	if p == nil {
		return nil
	}

	scoped := *p
	scoped.tenancy = tenancy

	return &scoped
}

func (p *ContextLimitDefinitionPolicy) validate(ctx context.Context, limit *model.Limit) error {
	applies, err := p.applies(ctx)
	if err != nil || !applies {
		return err
	}

	return p.check(ctx, limit)
}

// applies reports whether the policy governs the tenant of ctx: never for a
// nil policy, always for an unscoped one.
func (p *ContextLimitDefinitionPolicy) applies(ctx context.Context) (bool, error) {
	if p == nil {
		return false, nil
	}

	if p.tenancy == nil {
		return true, nil
	}

	return p.tenancy.Applies(ctx)
}

func (p *ContextLimitDefinitionPolicy) check(ctx context.Context, limit *model.Limit) error {
	if limit == nil {
		return constant.ErrContextLimitsUnavailable
	}

	if err := model.ValidateContextLimitDefinition(ctx, *limit, p.bounds, p.maxScopes); err != nil {
		return err
	}

	scopes, err := json.Marshal(limit.Scopes)
	if err != nil || len(scopes) > p.maxScopeBytes {
		return constant.ErrContextLimitsUnavailable
	}

	return nil
}

// validateActivation admits a limit to the shared profile only when its
// definition is account-scoped within bounds and its asset is a valid code;
// limits match reservations by exact code equality.
func (p *ContextLimitDefinitionPolicy) validateActivation(ctx context.Context, limit *model.Limit) error {
	applies, err := p.applies(ctx)
	if err != nil || !applies {
		return err
	}

	if err := p.check(ctx, limit); err != nil {
		return err
	}

	if err := utils.ValidateAssetCode(limit.Asset); err != nil {
		return constant.ErrContextLimitsUnavailable
	}

	return nil
}

// recordDefinitionPolicyError records a policy refusal on span: a tenancy
// that could not answer is a technical failure, any other refusal a business
// one.
func recordDefinitionPolicyError(span trace.Span, message string, err error) {
	if errors.Is(err, constant.ErrTenantServiceUnavailable) {
		libOpentelemetry.HandleSpanError(span, message, err)

		return
	}

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)
}
