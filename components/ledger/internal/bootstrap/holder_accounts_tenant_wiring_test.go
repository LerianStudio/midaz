// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"reflect"
	"testing"

	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestHolderAccountsTenantMiddlewareWiring pins the holder-accounts tenant middleware to the
// two stores the org-wide holder account listing reads: the onboarding account repo (onboarding
// PG) and the onboarding metadata repo (onboarding MB). Both must be MODULE-keyed — the metadata
// repo looks the module key up first, so binding the onboarding Mongo on the generic key sends
// the metadata read to whichever store owns that key.
//
// This route previously ran on the CRM options, whose middleware carries no onboarding PG at all;
// the account read then failed with "tenant postgres connection missing from context". The
// assertions below are what keeps the route off that instance.
//
// The TenantMiddleware fields are unexported in lib-commons v6, so the module maps are read via
// reflect (see mapKeys in fees_tenant_wiring_test.go).
func TestHolderAccountsTenantMiddlewareWiring(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()

	// Multi-tenant: non-nil managers make buildUnifiedRouteSetup build the middleware. The
	// managers are zero-value structs because the wiring only registers them; no DB connection
	// is opened here (pure, no-I/O unit test).
	mtSetup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		&tmpostgres.Manager{}, &tmpostgres.Manager{},
		&tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err, "multi-tenant setup must not error")
	require.NotNil(t, mtSetup, "multi-tenant setup must be non-nil")
	require.NotNil(t, mtSetup.holderAccountsTenantMiddleware,
		"holder-accounts tenant middleware must be built in multi-tenant mode")

	mw := reflect.ValueOf(mtSetup.holderAccountsTenantMiddleware).Elem()

	pgKeys := mapKeys(t, mw, "pgModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, pgKeys,
		"holder-accounts middleware must carry exactly the module-keyed onboarding PostgreSQL manager: the listing "+
			"reads the onboarding account repo, which requires the tenant PG in context on the module key")

	mbKeys := mapKeys(t, mw, "mongoModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mbKeys,
		"holder-accounts middleware must carry exactly the module-keyed onboarding Mongo manager: account metadata "+
			"enrichment reads the onboarding metadata repo on the module key")

	// No generic (no-module) Mongo manager: the listing never reads a CRM collection, and this
	// middleware resolves every registered manager eagerly, so a generic CRM Mongo would make the
	// route depend on provisioning it does not use.
	genericMB := mw.FieldByName("mongo")
	if !genericMB.IsValid() {
		t.Fatalf("lib-commons TenantMiddleware renamed field %q; update this test", "mongo")
	}
	assert.True(t, genericMB.IsNil(), "holder-accounts middleware must register no generic (no-module) Mongo manager: "+
		"the listing reads no CRM collection, so binding the CRM Mongo would add an unused eager resolution")

	// Single-tenant: buildUnifiedRouteSetup short-circuits to a zero-value setup before it builds
	// any tenant middleware, so the seam is nil (parallel to the nil route options).
	stSetup, err := buildUnifiedRouteSetup(&Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err, "single-tenant setup must not error")
	require.NotNil(t, stSetup, "single-tenant setup is a zero value, not nil")
	assert.Nil(t, stSetup.holderAccountsTenantMiddleware, "single-tenant holder-accounts tenant middleware must be nil")
}
