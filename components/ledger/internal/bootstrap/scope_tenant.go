// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/bxcodec/dbresolver/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// errScopeTenantMissing refuses a multi-tenant lookup for a credential that names
// no tenant: there is no database to confine it to, and no default to fall back on.
var errScopeTenantMissing = errors.New("scope resolution in multi-tenant mode needs the tenant of a validated credential")

// scopeTenant attaches to a resolver's context the databases the lookups read.
type scopeTenant interface {
	attach(ctx context.Context) (context.Context, error)
}

// tenantPGSource is the PostgreSQL database of one module for a tenant.
type tenantPGSource interface {
	tenantDB(ctx context.Context, tenantID string) (dbresolver.DB, error)
}

// tenantMongoSource is the MongoDB database of one module for a tenant.
type tenantMongoSource interface {
	tenantDatabase(ctx context.Context, tenantID string) (*mongo.Database, error)
}

// multiTenantScope takes the tenant from the credential lib-auth validated before
// any resolver runs, and attaches that tenant's databases the way the tenant
// middleware attaches them to a request, under the same module keys.
type multiTenantScope struct {
	pg     map[string]tenantPGSource
	mongo  map[string]tenantMongoSource
	cache  *tenantcache.TenantCache
	loader *tenantcache.TenantLoader
}

func (s *multiTenantScope) attach(ctx context.Context) (context.Context, error) {
	principal, ok := middleware.PrincipalFromContext(ctx)
	if !ok || principal.TenantID == "" {
		return ctx, errScopeTenantMissing
	}

	tenantID, err := tmcore.CanonicalTenantID(principal.TenantID)
	if err != nil {
		return ctx, fmt.Errorf("scope resolution tenant: %w", err)
	}

	if s.cache != nil && s.loader != nil {
		if _, cached := s.cache.Get(tenantID); !cached {
			if _, err := s.loader.LoadTenant(ctx, tenantID); err != nil {
				return ctx, fmt.Errorf("scope resolution tenant %s: %w", tenantID, err)
			}
		}
	}

	ctx = tmcore.ContextWithTenantID(ctx, tenantID)

	for module, source := range s.pg {
		db, err := source.tenantDB(ctx, tenantID)
		if err != nil {
			return ctx, fmt.Errorf("scope resolution %s database: %w", module, err)
		}

		ctx = tmcore.ContextWithPG(ctx, db, module)
	}

	for module, source := range s.mongo {
		db, err := source.tenantDatabase(ctx, tenantID)
		if err != nil {
			return ctx, fmt.Errorf("scope resolution %s documents: %w", module, err)
		}

		ctx = tmcore.ContextWithMB(ctx, db, module)
	}

	return ctx, nil
}

// pgManagerSource reads a tenant's database from the tenant-manager.
type pgManagerSource struct{ manager *tmpostgres.Manager }

func (s pgManagerSource) tenantDB(ctx context.Context, tenantID string) (dbresolver.DB, error) {
	conn, err := s.manager.GetConnection(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return conn.GetDB()
}

// mongoManagerSource reads a tenant's document database from the tenant-manager.
type mongoManagerSource struct{ manager *tmmongo.Manager }

func (s mongoManagerSource) tenantDatabase(ctx context.Context, tenantID string) (*mongo.Database, error) {
	return s.manager.GetDatabaseForTenant(ctx, tenantID)
}

// newScopeTenant is the tenant attachment of the scope resolvers: nil in
// single-tenant mode, and in multi-tenant mode the onboarding and transaction
// databases the lookups read, plus the transaction documents the pending
// transaction lookup may fall back on.
func newScopeTenant(cfg *Config, onboardingPG, transactionPG *tmpostgres.Manager, transactionMongo *tmmongo.Manager,
	cache *tenantcache.TenantCache, loader *tenantcache.TenantLoader,
) scopeTenant {
	if !cfg.MultiTenantEnabled {
		return nil
	}

	scope := &multiTenantScope{
		pg:     map[string]tenantPGSource{},
		mongo:  map[string]tenantMongoSource{},
		cache:  cache,
		loader: loader,
	}

	if onboardingPG != nil {
		scope.pg[constant.ModuleOnboarding] = pgManagerSource{manager: onboardingPG}
	}

	if transactionPG != nil {
		scope.pg[constant.ModuleTransaction] = pgManagerSource{manager: transactionPG}
	}

	if transactionMongo != nil {
		scope.mongo[constant.ModuleTransaction] = mongoManagerSource{manager: transactionMongo}
	}

	return scope
}
