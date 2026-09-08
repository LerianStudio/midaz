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

// TestCompositionTenantMiddlewareWiring pins the composition tenant middleware to every store the
// holder-account POST reaches: the account row and the ledger/alias reads (onboarding PG), the
// default balance CreateAccount always writes (transaction PG), the account metadata write
// (onboarding MB, module-keyed) and the instrument write plus holder-existence read (CRM MB,
// generic key).
//
// Two failure modes are pinned here, both observed in multi-tenant staging:
//
//   - a missing transaction PG makes the balance repo fail requireTenant with "tenant postgres
//     connection missing from context", so the whole POST 500s;
//   - a missing module-keyed onboarding Mongo makes the metadata repo fall back to the generic key
//     and write the account metadata into the CRM store — silent, not an error.
//
// The TenantMiddleware fields are unexported in lib-commons v6, so the module maps are read via
// reflect (see mapKeys in fees_tenant_wiring_test.go).
func TestCompositionTenantMiddlewareWiring(t *testing.T) {
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
	require.NotNil(t, mtSetup.compositionTenantMiddleware,
		"composition tenant middleware must be built in multi-tenant mode")

	mw := reflect.ValueOf(mtSetup.compositionTenantMiddleware).Elem()

	pgKeys := mapKeys(t, mw, "pgModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding, constant.ModuleTransaction}, pgKeys,
		"composition middleware must carry module-keyed onboarding+transaction PostgreSQL managers: the account "+
			"write resolves the onboarding module key and the default balance the transaction module key, the "+
			"latter with requireTenant so its absence is a hard 500")

	mbKeys := mapKeys(t, mw, "mongoModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mbKeys,
		"composition middleware must carry the module-keyed onboarding Mongo manager: the account metadata write "+
			"resolves the module key first and would otherwise fall back to the generic key and land in the CRM store")

	// The CRM Mongo is registered with a no-module WithMB, which lib-commons stores in the
	// single-manager mongo field (not mongoModules). The CRM instrument/holder repos read the
	// generic key, so this manager must remain present.
	genericMB := mw.FieldByName("mongo")
	if !genericMB.IsValid() {
		t.Fatalf("lib-commons TenantMiddleware renamed field %q; update this test", "mongo")
	}

	assert.False(t, genericMB.IsNil(), "composition middleware must keep the generic (no-module) CRM Mongo manager: "+
		"the instrument repo and the holder-existence read use tmcore.GetMBContext(ctx) on the generic key")

	// Single-tenant: buildUnifiedRouteSetup short-circuits to a zero-value setup before it builds
	// any tenant middleware, so the seam is nil (parallel to the nil route options).
	stSetup, err := buildUnifiedRouteSetup(&Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err, "single-tenant setup must not error")
	require.NotNil(t, stSetup, "single-tenant setup is a zero value, not nil")
	assert.Nil(t, stSetup.compositionTenantMiddleware, "single-tenant composition tenant middleware must be nil")
}
