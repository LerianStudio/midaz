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

// TestCRMTenantMiddlewareWiring pins the two CRM tenant middlewares to the stores their routes
// reach.
//
// The CRM middleware serves the holder CRUD, the instrument reads and updates, encryption and
// audit. Those read only the CRM Mongo (generic key), and the middleware resolves every
// registered manager eagerly, so it must carry nothing else: an onboarding store here would make
// every CRM route fail for a tenant whose onboarding provisioning is absent or down.
//
// Two CRM routes also read ledger stores in-process through the ledger account reader, and each
// gets a middleware carrying exactly the ledger stores it reads, beside the CRM Mongo:
//
//   - the CRM ledger-reads middleware serves instrument create, which verifies its
//     ledgerId/accountId references (ledger and account rows in onboarding PG, account metadata
//     in onboarding MB);
//   - the CRM holder-delete middleware serves holder delete, which counts the holder's accounts
//     (onboarding PG only).
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
	require.NotNil(t, mtSetup.crmLedgerReadsTenantMiddleware,
		"CRM ledger-reads tenant middleware must be built in multi-tenant mode")
	require.NotNil(t, mtSetup.crmHolderDeleteTenantMiddleware,
		"CRM holder-delete tenant middleware must be built in multi-tenant mode")

	crm := reflect.ValueOf(mtSetup.crmTenantMiddleware).Elem()

	assert.Empty(t, mapKeys(t, crm, "pgModules"),
		"CRM middleware must carry no PostgreSQL manager: its routes never read one, and eager resolution "+
			"would make them depend on onboarding provisioning")
	assert.Empty(t, mapKeys(t, crm, "mongoModules"),
		"CRM middleware must carry no module-keyed Mongo manager: its routes read only the CRM Mongo")
	assertGenericCRMMongo(t, crm, "CRM middleware")

	ledgerReads := reflect.ValueOf(mtSetup.crmLedgerReadsTenantMiddleware).Elem()

	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mapKeys(t, ledgerReads, "pgModules"),
		"CRM ledger-reads middleware must carry exactly the module-keyed onboarding PostgreSQL manager: the "+
			"instrument reference check and the holder-delete account count read onboarding PG with requireTenant set")
	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mapKeys(t, ledgerReads, "mongoModules"),
		"CRM ledger-reads middleware must carry exactly the module-keyed onboarding Mongo manager: the instrument "+
			"account reference check reads account metadata on the module key and would otherwise hit the CRM store")
	assertGenericCRMMongo(t, ledgerReads, "CRM ledger-reads middleware")

	holderDelete := reflect.ValueOf(mtSetup.crmHolderDeleteTenantMiddleware).Elem()

	assert.ElementsMatch(t, []string{constant.ModuleOnboarding}, mapKeys(t, holderDelete, "pgModules"),
		"CRM holder-delete middleware must carry exactly the module-keyed onboarding PostgreSQL manager: the "+
			"owned-account guard counts the holder's accounts there with requireTenant set")
	assert.Empty(t, mapKeys(t, holderDelete, "mongoModules"),
		"CRM holder-delete middleware must carry no module-keyed Mongo manager: holder delete reads no onboarding "+
			"Mongo, and eager resolution would make it depend on that provisioning")
	assertGenericCRMMongo(t, holderDelete, "CRM holder-delete middleware")

	// Single-tenant: buildUnifiedRouteSetup short-circuits to a zero-value setup before it builds
	// any tenant middleware, so both seams are nil (parallel to the nil route options).
	stSetup, err := buildUnifiedRouteSetup(&Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err, "single-tenant setup must not error")
	require.NotNil(t, stSetup, "single-tenant setup is a zero value, not nil")
	assert.Nil(t, stSetup.crmTenantMiddleware, "single-tenant CRM tenant middleware must be nil")
	assert.Nil(t, stSetup.crmLedgerReadsTenantMiddleware, "single-tenant CRM ledger-reads tenant middleware must be nil")
	assert.Nil(t, stSetup.crmHolderDeleteTenantMiddleware, "single-tenant CRM holder-delete tenant middleware must be nil")
}

// assertGenericCRMMongo asserts the middleware keeps the CRM Mongo registered with a no-module
// WithMB, which lib-commons stores in the single-manager mongo field (not mongoModules). The CRM
// holder/instrument repos read tmcore.GetMBContext(ctx) on the generic key.
func assertGenericCRMMongo(t *testing.T, mw reflect.Value, name string) {
	t.Helper()

	genericMB := mw.FieldByName("mongo")
	if !genericMB.IsValid() {
		t.Fatalf("lib-commons TenantMiddleware renamed field %q; update this test", "mongo")
	}

	assert.Falsef(t, genericMB.IsNil(), "%s must keep the generic (no-module) CRM Mongo manager: "+
		"the holder and instrument repos use tmcore.GetMBContext(ctx) on the generic key", name)
}
