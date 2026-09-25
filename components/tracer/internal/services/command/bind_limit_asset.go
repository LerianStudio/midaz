// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// LimitAssetBindingConfig bounds the full account fact set and limit scopes.
// SingleTenant must be explicit; multi-tenant calls need a resolved tenant pool.
type LimitAssetBindingConfig struct {
	Facts        tracercontract.Limits
	MaxScopes    int
	SingleTenant bool
}

// BindLimitAssetCommand associates producer-attested official facts, never a
// human-entered guess from a currency code. The transport must verify producer
// identity and administrative authorization for this limit; a user principal
// alone cannot attest assets, and a service certificate alone cannot administer.
// Association and mandatory audit commit together without changing capacity.
type BindLimitAssetCommand struct {
	repo   LimitAssetRepository
	audit  AuditEventRepository
	tx     pgdb.TxBeginner
	clock  clock.Clock
	config LimitAssetBindingConfig
}

func NewBindLimitAssetCommand(repo LimitAssetRepository, audit AuditEventRepository, tx pgdb.TxBeginner, clk clock.Clock, config LimitAssetBindingConfig) (*BindLimitAssetCommand, error) {
	if repo == nil || audit == nil || tx == nil || clk == nil {
		return nil, pgdb.ErrNilConnection
	}

	if config.MaxScopes <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := config.Facts.Validate(); err != nil {
		return nil, err
	}

	return &BindLimitAssetCommand{repo: repo, audit: audit, tx: tx, clock: clk, config: config}, nil
}

// Execute accepts facts only from the verified producer's namespace. It locks
// the current limit, checks exact account coverage and one asset, then binds and
// audits. It does not query the producer's database or prove record existence:
// that responsibility stays with the authenticated source, as for Reserve facts.
// Duplicate binding conflicts; uncertain commit errors are not retried.
func (c *BindLimitAssetCommand) Execute(ctx context.Context, limitID uuid.UUID, input []tracercontract.AccountAsset) (_ *tracercontract.AssetRef, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.bind_limit_asset")
	defer span.End()
	defer func() { recordLimitAssetBindingError(span, retErr) }()

	identity, actor, err := c.bindingIdentities(ctx)
	if err != nil {
		return nil, err
	}

	if limitID == uuid.Nil {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := tracercontract.ValidateAccountAssets(ctx, input, identity.AssetNamespace, c.config.Facts); err != nil {
		return nil, err
	}

	facts := slices.Clone(input)
	slices.SortFunc(facts, func(a, b tracercontract.AccountAsset) int { return bytes.Compare(a.AccountID[:], b.AccountID[:]) })

	at := c.clock.Now().UTC()
	if at.IsZero() || at.Year() < 1 || at.Year() > 9999 {
		return nil, constant.ErrInternalServer
	}

	var result tracercontract.AssetRef

	if err := executeWithTx(ctx, c.tx, func(tx pgdb.Tx) error {
		limit, err := c.repo.GetForAssetBindingWithTx(ctx, tx, limitID)
		if err != nil {
			return err
		}

		if limit == nil {
			return constant.ErrLimitNotFound
		}

		if limit.Definition.ID != limitID {
			return constant.ErrInternalServer
		}

		if limit.Asset != (tracercontract.AssetRef{}) {
			return constant.ErrLimitAssetReferenceConflict
		}

		asset, err := c.matchAccountAssets(ctx, *limit, facts, identity.AssetNamespace)
		if err != nil {
			return err
		}

		event, err := model.NewAuditEvent(model.AuditEventLimitUpdated, model.AuditActionUpdate, model.AuditResultSuccess, limitID.String(), model.ResourceTypeLimit, actor)
		if err != nil {
			return err
		}

		event.CreatedAt = at
		// Preserve the existing LIMIT_UPDATED CRUD snapshot and add the reference
		// transition. Association does not activate or rewrite the stored limit.
		before, after := LimitToMap(&limit.Definition), LimitToMap(&limit.Definition)
		before["assetRef"], after["assetRef"] = nil, asset
		event.WithContext(map[string]any{
			"operation": "asset_reference_binding", "integrationId": identity.ID, "accountAssets": slices.Clone(facts),
			"before": before, "after": after,
		})

		if err := c.repo.BindAssetWithTx(ctx, tx, limitID, asset); err != nil {
			return err
		}

		if err := c.audit.InsertWithTx(ctx, tx, event); err != nil {
			return fmt.Errorf("audit limit asset association: %w", err)
		}

		result = asset

		return ctx.Err()
	}); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).With(libLog.Int("accounts.count", len(facts))).Log(ctx, libLog.LevelDebug, "Limit asset associated with audit")

	return &result, nil
}

// Presence and source verification are checked here; resource-level RBAC is
// still the responsibility of the authenticated administrative transport.
func (c *BindLimitAssetCommand) bindingIdentities(ctx context.Context) (contextutil.IntegrationIdentity, model.Actor, error) {
	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return contextutil.IntegrationIdentity{}, model.Actor{}, constant.ErrInsufficientPrivileges
	}

	actor, err := limitAssetAdministrationActor(ctx)
	if err != nil {
		return contextutil.IntegrationIdentity{}, model.Actor{}, err
	}

	if !c.config.SingleTenant {
		if tmcore.GetTenantIDContext(ctx) == "" {
			return contextutil.IntegrationIdentity{}, model.Actor{}, constant.ErrReservationTenantRequired
		}

		if tmcore.GetPGContext(ctx) == nil {
			return contextutil.IntegrationIdentity{}, model.Actor{}, pgdb.ErrNoTenantInContext
		}
	}

	return identity, actor, nil
}

func (c *BindLimitAssetCommand) matchAccountAssets(ctx context.Context, limit model.ContextAccountLimit, facts []tracercontract.AccountAsset, namespace string) (tracercontract.AssetRef, error) {
	asset := facts[0].Asset // Execute validated a nonempty bounded fact set.

	if !limit.Definition.Status.IsValid() || limit.Definition.Status == model.LimitStatusDeleted {
		return tracercontract.AssetRef{}, constant.ErrContextLimitsUnavailable
	}
	// Prepare DRAFT/INACTIVE limits without activating them. Only the local copy
	// is validated against the active account-only profile; no state is written.
	limit.Definition.Status = model.LimitStatusActive

	limit.Asset = asset
	if err := limit.Validate(ctx, namespace, c.config.Facts, c.config.MaxScopes); err != nil {
		return tracercontract.AssetRef{}, err
	}

	if len(facts) != len(limit.Definition.Scopes) {
		return tracercontract.AssetRef{}, constant.ErrContextLimitsUnavailable
	}

	byAccount := make(map[uuid.UUID]tracercontract.AssetRef, len(facts))
	for _, fact := range facts {
		if err := ctx.Err(); err != nil {
			return tracercontract.AssetRef{}, err
		}

		if fact.Asset != asset {
			return tracercontract.AssetRef{}, constant.ErrContextLimitsUnavailable
		}

		byAccount[fact.AccountID] = fact.Asset
	}

	for _, scope := range limit.Definition.Scopes {
		if _, exists := byAccount[*scope.AccountID]; !exists {
			return tracercontract.AssetRef{}, constant.ErrContextLimitsUnavailable
		}
	}

	return asset, nil
}

func limitAssetAdministrationActor(ctx context.Context) (model.Actor, error) {
	principal, ok := contextutil.GetPrincipal(ctx)
	if !ok || strings.TrimSpace(principal.ID) == "" {
		return model.Actor{}, constant.ErrAuditEventActorIDRequired
	}

	kind := model.ActorType(principal.Type)
	if !kind.IsValid() {
		return model.Actor{}, constant.ErrAuditEventActorTypeInvalid
	}

	if kind == model.ActorTypeAPIKey {
		return model.Actor{}, constant.ErrInsufficientPrivileges
	}

	actor := resolveActor(ctx, model.ResourceTypeLimit)
	actor.ID = strings.TrimSpace(actor.ID)

	return actor, nil
}

func recordLimitAssetBindingError(span trace.Span, err error) {
	if err == nil {
		return
	}

	if errors.Is(err, constant.ErrInvalidRequestBody) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid limit asset facts", err)
		return
	}

	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if pkg.IsBusinessError(pkg.ValidateBusinessError(cause, constant.EntityLimit)) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Limit asset association rejected", err)
			return
		}
	}

	libOtel.HandleSpanError(span, "Limit asset association failed", err)
}
