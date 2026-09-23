// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TenantMongoResolver resolves a tenant's Mongo database for one module. The
// signature mirrors tmmongo.Manager.GetDatabaseForTenant, so the concrete manager
// is injected at bootstrap and faked in tests.
type TenantMongoResolver interface {
	GetDatabaseForTenant(ctx context.Context, tenantID string) (*mongo.Database, error)
}

// ResolveTenantMongoContext returns a context derived from ctx whose generic Mongo
// key carries the caller's tenant database from resolver. It serves seams whose
// repositories read the generic key on a route whose tenant middleware binds a
// different store there, or none at all. The database is always resolved, never
// reused from ctx, because a generic database already on ctx belongs to that
// route's own store.
//
// The derived context must stay scoped to the seam's own reads: returning it to
// the request would put this store under the generic key for every later read.
// A missing tenant ID fails with an error wrapping tmcore.ErrTenantNotFound rather
// than falling through to a static single-tenant connection; seam names the caller
// in that error. Resolution failures are mapped with MapTenantError.
func ResolveTenantMongoContext(ctx context.Context, resolver TenantMongoResolver, seam string) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return nil, fmt.Errorf("%s: %w", seam, tmcore.ErrTenantNotFound)
	}

	db, err := resolver.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return nil, MapTenantError(ctx, err, tenantID)
	}

	return tmcore.ContextWithMB(ctx, db), nil
}
