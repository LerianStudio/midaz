// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"math"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// BindContextPolicyCommand activates an immutable revision on one exact binding.
// The administration transport must authorize this integration/context and set
// the verified principal and tenant. Transaction requests never call this port.
// Binding writes and audit share a transaction; compilation precedes row locks.
type BindContextPolicyCommand struct {
	repo     ContextPolicyBindingRepository
	audit    AuditEventRepository
	tx       pgdb.TxBeginner
	compiler ContextPolicyCompiler
	clock    clock.Clock
}

func NewBindContextPolicyCommand(repo ContextPolicyBindingRepository, audit AuditEventRepository, tx pgdb.TxBeginner, compiler ContextPolicyCompiler, clk clock.Clock) (*BindContextPolicyCommand, error) {
	if repo == nil || audit == nil || tx == nil || compiler == nil || clk == nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &BindContextPolicyCommand{repo: repo, audit: audit, tx: tx, compiler: compiler, clock: clk}, nil
}

// Execute creates only when expected is nil; replacement requires the current
// binding version. Even an A-to-B-to-A change advances the version. Commit
// uncertainty is returned without retry or a purported successful result.
func (c *BindContextPolicyCommand) Execute(ctx context.Context, key model.PolicyBindingKey, target model.PolicyRevision, expected *int64) (_ *model.PolicyBindingState, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.bind_context_policy")
	defer span.End()
	defer func() { recordPolicyAdministrationError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	actor, err := policyAdministrationActor(ctx)
	if err != nil {
		return nil, err
	}

	if err := key.Validate(); err != nil {
		return nil, err
	}

	if target.ID == uuid.Nil || target.Revision <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	after := model.PolicyBindingState{Policy: target, Version: 1}

	if expected != nil {
		version := *expected
		if version <= 0 || version == math.MaxInt64 {
			return nil, constant.ErrInvalidRequestBody
		}

		expected = &version
		after.Version = version + 1
	}

	if err := c.checkPublishedRevision(ctx, target); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	event, err := model.NewAuditEvent(model.AuditEventPolicyBound, model.AuditActionActivate,
		model.AuditResultSuccess, target.ID.String(), model.ResourceTypePolicy, actor)
	if err != nil {
		return nil, err
	}

	event.CreatedAt = c.clock.Now().UTC()
	event.WithContext(map[string]any{"binding": key, "after": after})

	if err := executeWithTx(ctx, c.tx, func(tx pgdb.Tx) error {
		before, err := c.repo.GetBindingWithTx(ctx, tx, key)
		if err != nil {
			return err
		}

		if !bindingVersionMatches(before, expected) {
			return constant.ErrContextPolicyConflict
		}

		if before != nil {
			event.Context["before"] = *before
		}

		if err := c.repo.BindWithTx(ctx, tx, key, target, expected, actor.ID, event.CreatedAt); err != nil {
			return err
		}

		if err := c.audit.InsertWithTx(ctx, tx, event); err != nil {
			return fmt.Errorf("audit policy binding: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Context policy binding activated")

	return &after, nil
}

// Recheck a published snapshot against the current evaluator before acquiring
// binding locks. Immutable storage prevents a change between this check and CAS.
func (c *BindContextPolicyCommand) checkPublishedRevision(ctx context.Context, target model.PolicyRevision) error {
	policy, err := c.repo.GetRevision(ctx, target)
	if err != nil {
		return err
	}

	if policy == nil || policy.ID != target.ID || policy.Revision != target.Revision {
		return constant.ErrContextPolicyUnavailable
	}

	compiled, err := c.compiler.Compile(ctx, *policy)
	if err != nil {
		return err
	}

	if compiled == nil {
		return constant.ErrExpressionProgram
	}

	return nil
}

func bindingVersionMatches(before *model.PolicyBindingState, expected *int64) bool {
	if before == nil {
		return expected == nil
	}

	return expected != nil && before.Version == *expected
}
