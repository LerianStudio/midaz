// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// ContextLimitDefinitionPolicy reuses admission invariants at administration.
// A nil policy is the legacy profile; bootstrap installs one for shared Reserve.
type ContextLimitDefinitionPolicy struct {
	bounds        tracercontract.Limits
	maxScopes     int
	maxScopeBytes int
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

func (p *ContextLimitDefinitionPolicy) validate(ctx context.Context, limit *model.Limit) error {
	if p == nil {
		return nil
	}

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
	if p == nil {
		return nil
	}

	if err := p.validate(ctx, limit); err != nil {
		return err
	}

	if err := utils.ValidateAssetCode(limit.Asset); err != nil {
		return constant.ErrContextLimitsUnavailable
	}

	return nil
}
