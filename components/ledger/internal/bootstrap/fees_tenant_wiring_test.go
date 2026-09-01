// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"reflect"
	"testing"

	tmmongo "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestFeesTenantMiddlewareWiring pins the fee-route tenant middleware to the managers the
// fee/billing paths actually reach in multi-tenant mode. The fee create/update/estimate and
// billing paths resolve the onboarding account repo (onboarding PG), the transaction repo
// (transaction PG, billing volume branch) and the onboarding metadata repo (onboarding MB);
// the fee pack/billing_package repos read the generic Mongo key. A middleware missing any of
// those injections returns the 500 "tenant postgres connection missing from context" this test
// exists to prevent.
//
// The TenantMiddleware fields are unexported in lib-commons v6, so the module maps are read via
// reflect. A module-keyed WithPG/WithMB lands in pgModules/mongoModules; a no-module WithMB
// (the generic fees MB) lands in the single-manager mongo field, not the map. Both are asserted.
func TestFeesTenantMiddlewareWiring(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()

	// Multi-tenant: non-nil managers make buildUnifiedRouteSetup build the fee middleware. The
	// managers are zero-value structs because the wiring only registers them into the middleware;
	// no DB connection is opened here (pure, no-I/O unit test).
	mtSetup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		&tmpostgres.Manager{}, &tmpostgres.Manager{},
		&tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err, "multi-tenant setup must not error")
	require.NotNil(t, mtSetup, "multi-tenant setup must be non-nil")
	require.NotNil(t, mtSetup.feesTenantMiddleware, "fees tenant middleware must be built in multi-tenant mode")

	mw := reflect.ValueOf(mtSetup.feesTenantMiddleware).Elem()

	pgKeys := mapKeys(t, mw, "pgModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding, constant.ModuleTransaction}, pgKeys,
		"fees middleware must carry module-keyed onboarding+transaction PostgreSQL managers: fee paths reach the "+
			"onboarding account repo and the transaction volume repo, both requiring the tenant PG in context")

	mbKeys := mapKeys(t, mw, "mongoModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mbKeys,
		"fees middleware must carry exactly the module-keyed onboarding Mongo manager: account metadata enrichment "+
			"reads the onboarding metadata repo on the module key, and no transaction Mongo module is injected")

	// The generic fees MB is registered with a no-module WithMB, which lib-commons stores in the
	// single-manager mongo field (not mongoModules). The fee pack/billing_package repos read the
	// generic key, so this manager must remain present.
	genericMB := mw.FieldByName("mongo")
	if !genericMB.IsValid() {
		t.Fatalf("lib-commons TenantMiddleware renamed field %q; update this test", "mongo")
	}
	assert.False(t, genericMB.IsNil(), "fees middleware must keep the generic (no-module) fees Mongo manager: fee "+
		"pack/billing_package repos read tmcore.GetMBContext(ctx) on the generic key")

	// Single-tenant: buildUnifiedRouteSetup short-circuits to a zero-value setup before it builds
	// any tenant middleware, so the seam is nil (parallel to the nil route options).
	stSetup, err := buildUnifiedRouteSetup(&Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err, "single-tenant setup must not error")
	require.NotNil(t, stSetup, "single-tenant setup is a zero value, not nil")
	assert.Nil(t, stSetup.feesTenantMiddleware, "single-tenant fees tenant middleware must be nil")
}

// mapKeys reads the string keys of an unexported map[string]* field on the reflected middleware.
// The fields are unexported in lib-commons v6; reflect can enumerate a map's keys and read string
// values without the unexported-access restriction that blocks Interface(). A renamed field yields
// an invalid Value, which fails loud so a lib-commons bump does not silently pass this test.
func mapKeys(t *testing.T, v reflect.Value, field string) []string {
	t.Helper()

	f := v.FieldByName(field)
	if !f.IsValid() {
		t.Fatalf("lib-commons TenantMiddleware renamed field %q; update this test", field)
	}

	keys := make([]string, 0, f.Len())
	for _, k := range f.MapKeys() {
		keys = append(keys, k.String())
	}

	return keys
}
