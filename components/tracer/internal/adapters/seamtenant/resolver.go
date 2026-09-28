// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package seamtenant binds the per-tenant PostgreSQL pool for the reservation
// seam from the tenant id a platform producer requests, rather than from a JWT
// claim.
//
// The tenant id arrives as X-Tenant-Id (HTTP) or x-tenant-id (gRPC metadata)
// and is not trusted on its own. AuthorizeTenant runs after the transport has
// authenticated the producer (an M2M access token on HTTP, a mapped client
// certificate on gRPC): it has producerauth.TenantAuthorizer confirm that the
// tenant is active for that producer's service before any pool is resolved.
// It is wired only onto the reservation routes and RPCs. User-facing tracer
// routes keep their JWT-claim tenant path.
package seamtenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// HeaderName is the canonical requested-tenant header name. The REST
// adapter reads it as an HTTP header; the gRPC adapter reads its lower-cased
// form from incoming metadata (gRPC normalizes metadata keys to lower case).
// It matches the ledger client's TenantHeader so the wire key cannot drift.
const HeaderName = "X-Tenant-Id"

// MetadataKey is the gRPC metadata key for the requested tenant id — the
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

// Resolver resolves the per-tenant PostgreSQL pool for an authorized tenant
// id; AuthorizeTenant binds it into the request context. In single-tenant mode
// (mtEnabled=false, or a nil resolution function) it is inactive: the tenant
// key is ignored and the context is left unchanged.
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

// resolvePool validates the authorized tenant id and resolves its pool
// through the lib-commons tenant manager. An empty or invalid tenant id is
// ErrReservationTenantRequired and never reaches the pool. A pool the
// tenant-manager denies (tenant not found, or its tracer association suspended
// or purged) wraps ErrInsufficientPrivileges; any other failure (tenant-manager
// or database unreachable, open circuit breaker) wraps
// ErrTenantServiceUnavailable. Callers must check Active first.
func (r *Resolver) resolvePool(ctx context.Context, tenantID string) (dbresolver.DB, error) {
	if tenantID == "" || !tmcore.IsValidTenantID(tenantID) {
		return nil, constant.ErrReservationTenantRequired
	}

	db, err := r.pool(ctx, tenantID)
	if err != nil {
		return nil, classifyPoolError(err)
	}

	return db, nil
}

// bindTenant returns ctx carrying the tenant id and its pool, where
// repositories read them through tmcore.GetTenantIDContext and
// tmcore.GetPGContext, exactly as on the JWT path.
func bindTenant(ctx context.Context, tenantID string, db dbresolver.DB) context.Context {
	ctx = tmcore.ContextWithTenantID(ctx, tenantID)

	return tmcore.ContextWithPG(ctx, db)
}

func classifyPoolError(err error) error {
	if errors.Is(err, tmcore.ErrTenantNotFound) || errors.Is(err, tmcore.ErrTenantServiceAccessDenied) ||
		tmcore.IsTenantSuspendedError(err) {
		return fmt.Errorf("%w: %w", constant.ErrInsufficientPrivileges, err)
	}

	return fmt.Errorf("%w: %w", constant.ErrTenantServiceUnavailable, err)
}
