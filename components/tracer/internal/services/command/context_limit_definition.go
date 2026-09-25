// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextLimitDefinitionPolicy reuses admission invariants at administration.
// A nil policy is the legacy profile; bootstrap installs one for shared Reserve.
type ContextLimitDefinitionPolicy struct {
	repository    LimitAssetRepository
	bounds        tracercontract.Limits
	maxScopes     int
	maxScopeBytes int
}

func NewContextLimitDefinitionPolicy(repository LimitAssetRepository, bounds tracercontract.Limits, maxScopes, maxScopeBytes int) (*ContextLimitDefinitionPolicy, error) {
	if repository == nil || maxScopes <= 0 || maxScopeBytes <= 0 {
		return nil, constant.ErrContextLimitsUnavailable
	}

	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	return &ContextLimitDefinitionPolicy{repository: repository, bounds: bounds, maxScopes: maxScopes, maxScopeBytes: maxScopeBytes}, nil
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

func (p *ContextLimitDefinitionPolicy) validateActivation(ctx context.Context, tx pgdb.Tx, id uuid.UUID) error {
	if p == nil {
		return nil
	}

	current, err := p.repository.GetForAssetBindingWithTx(ctx, tx, id)
	if err != nil {
		return err
	}

	if current == nil {
		return constant.ErrLimitNotFound
	}

	if err := p.validate(ctx, &current.Definition); err != nil {
		return err
	}
	// This is the proposed status; the actual transition remains in the command.
	proposed := *current
	proposed.Definition.Status = model.LimitStatusActive

	return proposed.Validate(ctx, current.Asset.Namespace, p.bounds, p.maxScopes)
}
