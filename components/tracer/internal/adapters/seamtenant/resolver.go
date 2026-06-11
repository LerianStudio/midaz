// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package seamtenant resolves the per-tenant PostgreSQL pool for the
// service-to-service reservation seam from a TRUSTED tenant id, rather than
// from a JWT claim.
//
// The reservation surface is reachable only over the mTLS/mesh-protected
// transport (gRPC or REST behind the verified peer). On that connection the
// ledger is a verified service, so the `x-tenant-id` it forwards is trusted as
// the tenant key — the verified peer IS the identity. User-facing tracer routes
// keep their JWT-claim tenant path; this resolver is wired ONLY onto the
// reservation routes/RPCs, never onto a header-trust path reachable without the
// verified peer.
package seamtenant

import (
	"context"
	"strings"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// HeaderName is the canonical trusted-tenant header/metadata name. The REST
// adapter reads it as an HTTP header; the gRPC adapter reads its lower-cased
// form from incoming metadata (gRPC normalizes metadata keys to lower case).
// It matches the ledger client's TenantHeader so the wire key cannot drift.
const HeaderName = "X-Tenant-Id"

// MetadataKey is the gRPC metadata key for the trusted tenant id — the
// lower-cased HeaderName, since gRPC normalizes metadata keys to lower case.
// Derived from HeaderName so the two cannot drift, mirroring how the ledger
// client derives its gRPC key from TenantHeader.
var MetadataKey = strings.ToLower(HeaderName)

// PoolFunc resolves the tenant-scoped PostgreSQL pool for tenantID. It is
// satisfied in production by a thin wrapper over the lib-commons
// *tmpostgres.Manager (see NewResolver), and can be supplied directly via
// NewResolverWithPool so the resolution branches are exercisable without a live
// database.
type PoolFunc func(ctx context.Context, tenantID string) (dbresolver.DB, error)

// Resolver binds the per-tenant PostgreSQL pool into a context from the trusted
// tenant id. In single-tenant mode (mtEnabled=false, or a nil resolution
// function) it is a no-op: the tenant key is ignored and ctx is returned
// unchanged.
//
// The hard invariant: under multi-tenant mode a missing/empty tenant key is a
// clean failure (ErrReservationTenantRequired) and NEVER falls back to a
// default/wrong pool — that would break cross-tenant isolation.
type Resolver struct {
	pool      PoolFunc
	mtEnabled bool
}

// NewResolver builds a Resolver backed by the lib-commons tenant manager. When
// mtEnabled is false or pgManager is nil the resolver runs in no-op
// (single-tenant) mode.
func NewResolver(pgManager *tmpostgres.Manager, mtEnabled bool) *Resolver {
	var pool PoolFunc
	if pgManager != nil {
		pool = func(ctx context.Context, tenantID string) (dbresolver.DB, error) {
			conn, err := pgManager.GetConnection(ctx, tenantID)
			if err != nil {
				return nil, err
			}

			return conn.GetDB()
		}
	}

	return NewResolverWithPool(pool, mtEnabled)
}

// NewResolverWithPool builds a Resolver from a pool resolution function. It is
// the DI seam NewResolver delegates to, and lets callers (including tests) wire
// the per-tenant pool source directly. A nil pool yields no-op (single-tenant)
// mode regardless of mtEnabled.
func NewResolverWithPool(pool PoolFunc, mtEnabled bool) *Resolver {
	return &Resolver{
		pool:      pool,
		mtEnabled: mtEnabled,
	}
}

// Active reports whether the resolver enforces tenant resolution. False means
// single-tenant / no-op mode (the tenant key, present or absent, is ignored).
func (r *Resolver) Active() bool {
	return r != nil && r.mtEnabled && r.pool != nil
}

// Resolve validates the trusted tenant id, resolves the per-tenant pool through
// the lib-commons tenant manager, and returns a context carrying both the tenant
// id and the resolved PG connection (via tmcore.ContextWith*). Repositories pick
// them up through tmcore.GetPGContext / tmcore.GetTenantIDContext, exactly as on
// the JWT path.
//
// In no-op mode it returns ctx unchanged with a nil error, regardless of whether
// a tenant key was supplied. Under MT an empty/invalid tenant key yields
// ErrReservationTenantRequired; a pool-resolution failure is returned so the
// caller can classify it as technical.
func (r *Resolver) Resolve(ctx context.Context, tenantID string) (context.Context, error) {
	if !r.Active() {
		return ctx, nil
	}

	if tenantID == "" || !tmcore.IsValidTenantID(tenantID) {
		return ctx, constant.ErrReservationTenantRequired
	}

	db, err := r.pool(ctx, tenantID)
	if err != nil {
		return ctx, err
	}

	ctx = tmcore.ContextWithTenantID(ctx, tenantID)
	ctx = tmcore.ContextWithPG(ctx, db)

	return ctx, nil
}
