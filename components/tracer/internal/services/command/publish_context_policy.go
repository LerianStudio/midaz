// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ContextPolicyCompiler checks a complete policy using the same typed CEL
// environment and resource bounds as evaluation. No database locks are held.
type ContextPolicyCompiler interface {
	Compile(context.Context, model.ContextPolicy) (*query.CompiledContextPolicy, error)
}

// PublishContextPolicyCommand persists a checked revision and its hash-chained
// audit record atomically. It does not activate a binding or publish a cache
// entry. The transport must authorize policy administration and attach the
// verified principal and tenant before calling Execute. Principal presence
// alone is not an authorization check.
type PublishContextPolicyCommand struct {
	repo     ContextPolicyPublisher
	audit    AuditEventRepository
	tx       pgdb.TxBeginner
	compiler ContextPolicyCompiler
	clock    clock.Clock
}

// NewPublishContextPolicyCommand requires every dependency, including audit.
func NewPublishContextPolicyCommand(repo ContextPolicyPublisher, audit AuditEventRepository, tx pgdb.TxBeginner, compiler ContextPolicyCompiler, clk clock.Clock) (*PublishContextPolicyCommand, error) {
	if repo == nil || audit == nil || tx == nil || compiler == nil || clk == nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &PublishContextPolicyCommand{repo: repo, audit: audit, tx: tx, compiler: compiler, clock: clk}, nil
}

// Execute validates before opening a transaction. A duplicate revision is a
// conflict, not a successful replay, and never creates another audit event.
// Commit errors remain errors: this command never retries an unknown outcome.
func (c *PublishContextPolicyCommand) Execute(ctx context.Context, policy model.ContextPolicy) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.publish_context_policy")
	defer span.End()
	defer func() { recordPolicyPublicationError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	actor, err := policyPublicationActor(ctx)
	if err != nil {
		return err
	}

	policy.Rules = slices.Clone(policy.Rules)

	compiled, err := c.compiler.Compile(ctx, policy)
	if err != nil {
		return err
	}

	if compiled == nil {
		return constant.ErrExpressionProgram
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	event, err := model.NewAuditEvent(model.AuditEventPolicyPublished, model.AuditActionCreate,
		model.AuditResultSuccess, policy.ID.String(), model.ResourceTypePolicy, actor)
	if err != nil {
		return err
	}

	event.CreatedAt = c.clock.Now().UTC()
	// Keep the audit snapshot independent of the repository's copy.
	snapshot := policy
	snapshot.Rules = slices.Clone(policy.Rules)
	event.WithContext(map[string]any{"policy": snapshot})

	if err := executeWithTx(ctx, c.tx, func(tx pgdb.Tx) error {
		if err := c.repo.PublishWithTx(ctx, tx, policy, actor.ID, event.CreatedAt); err != nil {
			return err
		}

		if err := c.audit.InsertWithTx(ctx, tx, event); err != nil {
			return fmt.Errorf("audit policy publication: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	logger.With(libLog.Int("rules.count", len(policy.Rules))).Log(ctx, libLog.LevelDebug, "Context policy revision published")

	return nil
}

func policyPublicationActor(ctx context.Context) (model.Actor, error) {
	principal, ok := contextutil.GetPrincipal(ctx)
	if !ok || strings.TrimSpace(principal.ID) == "" {
		return model.Actor{}, constant.ErrAuditEventActorIDRequired
	}

	if !model.ActorType(principal.Type).IsValid() {
		return model.Actor{}, constant.ErrAuditEventActorTypeInvalid
	}

	actor := resolveActor(ctx, model.ResourceTypePolicy)
	actor.ID = strings.TrimSpace(actor.ID)

	return actor, nil
}

func recordPolicyPublicationError(span trace.Span, err error) {
	if err == nil {
		return
	}

	classified := err
	if errors.Is(err, constant.ErrInvalidRequestBody) {
		classified = pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Err: err}
	}

	for cause := classified; cause != nil; cause = errors.Unwrap(cause) {
		if pkg.IsBusinessError(pkg.ValidateBusinessError(cause, constant.EntityRule)) {
			libOtel.HandleSpanBusinessErrorEvent(span, "policy publication rejected", err)
			return
		}
	}

	libOtel.HandleSpanError(span, "policy publication failed", err)
}
