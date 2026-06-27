// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package backfill holds standalone, idempotent maintenance runners that
// reconcile cross-store state without participating in the request path.
package backfill

import (
	"context"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// orgPageSize is the page size used to walk the organization table. The walk is
// not latency-sensitive (a maintenance pass), so a modest page keeps memory flat
// without thrashing the database.
const orgPageSize = 100

// selfHolderType is the holder person-type used for the deterministic self-holder,
// matching the eager org-create provisioning path.
const selfHolderType = "LEGAL_PERSON"

// OrgLister enumerates organizations for a single resolved tenant context.
//
// It is satisfied by the onboarding organization repository: ownership is
// org-global, so the runner provisions one self-holder per organization. The
// list is tenant-scoped through the context the runner injects before calling.
type OrgLister interface {
	// FindAll returns a page of organizations for the tenant resolved from ctx.
	FindAll(ctx context.Context, filter libHTTP.QueryHeader) ([]*mmodel.Organization, error)
}

// HolderBackfiller provisions deterministic self-holders for existing
// organizations and materialises account.holder_id for non-external accounts.
//
// The two stores are touched in a mandatory order: the Mongo self-holder is
// written first, then the PG account.holder_id is materialised. This ordering
// guarantees holder_id never points at a non-existent holder. The runner is
// idempotent end to end: a duplicate self-holder ID is treated as success, and
// the PG materialisation only touches rows still NULL, so re-runs are no-ops.
type HolderBackfiller struct {
	orgs        OrgLister
	provisioner command.HolderProvisioner
}

// NewHolderBackfiller wires the runner from the dependencies it needs:
//   - orgs lists organizations per resolved tenant context;
//   - provisioner writes the deterministic self-holder (Mongo) idempotently.
//
// The PG account.holder_id materialisation resolves its connection from the
// tenant context the caller injects, mirroring the repositories.
func NewHolderBackfiller(orgs OrgLister, provisioner command.HolderProvisioner) *HolderBackfiller {
	return &HolderBackfiller{
		orgs:        orgs,
		provisioner: provisioner,
	}
}

// Result reports what a tenant backfill pass observed. Counts are diagnostic; the
// gate that proves idempotency is run-to-run equality of OrgsProcessed,
// HoldersProvisioned, and AccountsMaterialised.
type Result struct {
	OrgsProcessed        int
	HoldersProvisioned   int
	AccountsMaterialised int64
}

// RunTenant backfills a single tenant. The caller MUST inject the tenant's PG and
// Mongo connections into ctx before calling (the runner resolves the PG handle the
// same way repositories do, and the provisioner resolves Mongo from ctx). For
// single-tenant deployments the ambient connections satisfy this with no injection.
//
// Ordering is mandatory and per organization: provision the Mongo self-holder
// FIRST, then materialise PG account.holder_id. A self-holder provisioning failure
// for one organization aborts the pass — the PG materialisation for that org is
// NOT attempted, so holder_id never points at a missing holder.
func (b *HolderBackfiller) RunTenant(ctx context.Context) (Result, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "backfill.run_tenant_self_holders")
	defer span.End()

	var result Result

	page := 1

	for {
		if err := ctx.Err(); err != nil {
			libOpentelemetry.HandleSpanError(span, "Context cancelled during backfill", err)

			return result, err
		}

		orgs, err := b.orgs.FindAll(ctx, listAllOrgsFilter(page))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to list organizations", err)
			logger.Log(ctx, libLog.LevelError, "Backfill failed to list organizations", libLog.Err(err))

			return result, err
		}

		if len(orgs) == 0 {
			break
		}

		for _, org := range orgs {
			provisioned, materialised, err := b.backfillOrg(ctx, span, logger, org)
			if err != nil {
				return result, err
			}

			result.OrgsProcessed++
			if provisioned {
				result.HoldersProvisioned++
			}

			result.AccountsMaterialised += materialised
		}

		if len(orgs) < orgPageSize {
			break
		}

		page++
	}

	span.SetAttributes(
		attribute.Int("app.backfill.orgs_processed", result.OrgsProcessed),
		attribute.Int("app.backfill.holders_provisioned", result.HoldersProvisioned),
		attribute.Int64("app.backfill.accounts_materialised", result.AccountsMaterialised),
	)

	return result, nil
}

// backfillOrg provisions one organization's self-holder (Mongo first), then
// materialises its non-external accounts' holder_id (PG). A provisioning failure
// aborts the whole pass: a partial pass that materialised holder_id pointing at a
// holder that failed to provision would violate the no-orphan invariant.
func (b *HolderBackfiller) backfillOrg(ctx context.Context, span trace.Span, logger libLog.Logger, org *mmodel.Organization) (bool, int64, error) {
	organizationID, err := uuid.Parse(org.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse organization ID", err)
		logger.Log(ctx, libLog.LevelError, "Backfill failed to parse organization ID", libLog.Err(err))

		return false, 0, err
	}

	selfHolderID := command.DeriveSelfHolderID(organizationID)

	holderType := selfHolderType
	input := &mmodel.CreateHolderInput{
		Type:     &holderType,
		Name:     org.LegalName,
		Document: org.LegalDocument,
	}

	// Mongo FIRST: a duplicate deterministic _id is idempotent success.
	if _, err := b.provisioner.CreateHolderWithID(ctx, org.ID, selfHolderID, input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to provision self-holder during backfill", err)
		logger.Log(ctx, libLog.LevelError, "Backfill failed to provision self-holder", libLog.Err(err))

		return false, 0, err
	}

	// PG SECOND: materialise holder_id only on rows still NULL and not @external.
	materialised, err := b.materialiseAccounts(ctx, organizationID, selfHolderID)
	if err != nil {
		return false, 0, err
	}

	return true, materialised, nil
}

// materialiseAccounts sets account.holder_id to the org's deterministic
// self-holder for every non-external account in the organization whose holder_id
// is still NULL. It is idempotent: the WHERE clause skips rows already set, so a
// second pass affects zero rows. External accounts (D-3 exempt) are left NULL.
func (b *HolderBackfiller) materialiseAccounts(ctx context.Context, organizationID, selfHolderID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "backfill.materialise_account_holder_id")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.organization_id", organizationID.String()))

	db, err := resolveOnboardingDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Backfill failed to get database connection", libLog.Err(err))

		return 0, err
	}

	query, args, err := buildMaterialiseQuery(organizationID, selfHolderID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Backfill failed to build update query", libLog.Err(err))

		return 0, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Backfill account holder_id materialisation", libLog.String("query", query))

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, "Backfill failed to materialise account holder_id", libLog.Err(err))

		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read rows affected", err)
		logger.Log(ctx, libLog.LevelError, "Backfill failed to read rows affected", libLog.Err(err))

		return 0, err
	}

	span.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	return rowsAffected, nil
}

// buildMaterialiseQuery assembles the idempotent, @external-exempt
// account.holder_id UPDATE for one organization. It is a pure builder (no I/O) so
// the WHERE invariants — only-NULL, non-deleted, non-external rows — can be locked
// by a unit test without a database.
func buildMaterialiseQuery(organizationID, selfHolderID uuid.UUID) (string, []any, error) {
	return squirrel.Update("account").
		Set("holder_id", selfHolderID).
		Where(squirrel.Eq{"organization_id": organizationID, "holder_id": nil}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("lower(type) <> ?", constant.ExternalAccountType)).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
}

// resolveOnboardingDB resolves the onboarding PostgreSQL connection from the
// tenant context, mirroring the repositories: the tenant middleware injects a
// module-scoped connection in multi-tenant mode; the generic context entry is the
// single-tenant fallback.
func resolveOnboardingDB(ctx context.Context) (dbresolver.DB, error) {
	if db := tmcore.GetPGContext(ctx, constant.ModuleOnboarding); db != nil {
		return db, nil
	}

	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	return nil, fmt.Errorf("onboarding postgres connection missing from context")
}

// listAllOrgsFilter builds a QueryHeader that returns every organization on the
// requested page. The date bounds are widened explicitly because FindAll filters
// on created_at: the zero EndDate would otherwise exclude every row.
func listAllOrgsFilter(page int) libHTTP.QueryHeader {
	return libHTTP.QueryHeader{
		Limit:     orgPageSize,
		Page:      page,
		SortOrder: "asc",
		StartDate: time.Unix(0, 0).UTC(),
		EndDate:   time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
	}
}
