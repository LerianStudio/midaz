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

// TestCRMTenantMiddlewareWiring pins the CRM tenant middleware to every store the CRM routes
// reach. Besides the CRM Mongo (generic key) the holder and instrument repos read, two CRM
// paths read ledger stores in-process through the ledger account reader:
//
//   - instrument create verifies its ledgerId/accountId references (ledger and account rows in
//     onboarding PG, account metadata in onboarding MB);
//   - holder delete counts the holder's accounts (onboarding PG).
//
// Without the module-keyed onboarding PG those reads fail requireTenant with "tenant postgres
// connection missing from context" and the request is a 500. Without the module-keyed
// onboarding Mongo the account metadata read falls back to the generic key and queries the CRM
// store instead.
//
// The TenantMiddleware fields are unexported in lib-commons v7, so the module maps are read via
// reflect (see mapKeys in fees_tenant_wiring_test.go).
func TestCRMTenantMiddlewareWiring(t *testing.T) {
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
	require.NotNil(t, mtSetup.crmTenantMiddleware, "CRM tenant middleware must be built in multi-tenant mode")

	mw := reflect.ValueOf(mtSetup.crmTenantMiddleware).Elem()

	pgKeys := mapKeys(t, mw, "pgModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, pgKeys,
		"CRM middleware must carry exactly the module-keyed onboarding PostgreSQL manager: the instrument "+
			"reference check and the holder-delete account count read onboarding PG with requireTenant set")

	mbKeys := mapKeys(t, mw, "mongoModules")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mbKeys,
		"CRM middleware must carry exactly the module-keyed onboarding Mongo manager: the instrument account "+
			"reference check reads account metadata on the module key and would otherwise hit the CRM store")

	// The CRM Mongo is registered with a no-module WithMB, which lib-commons stores in the
	// single-manager mongo field (not mongoModules). The CRM holder/instrument repos read the
	// generic key, so this manager must remain present.
	genericMB := mw.FieldByName("mongo")
	if !genericMB.IsValid() {
		t.Fatalf("lib-commons TenantMiddleware renamed field %q; update this test", "mongo")
	}

	assert.False(t, genericMB.IsNil(), "CRM middleware must keep the generic (no-module) CRM Mongo manager: "+
		"the holder and instrument repos use tmcore.GetMBContext(ctx) on the generic key")

	// Single-tenant: buildUnifiedRouteSetup short-circuits to a zero-value setup before it builds
	// any tenant middleware, so the seam is nil (parallel to the nil route options).
	stSetup, err := buildUnifiedRouteSetup(&Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err, "single-tenant setup must not error")
	require.NotNil(t, stSetup, "single-tenant setup is a zero value, not nil")
	assert.Nil(t, stSetup.crmTenantMiddleware, "single-tenant CRM tenant middleware must be nil")
}
