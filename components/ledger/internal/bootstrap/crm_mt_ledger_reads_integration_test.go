//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegration_CRMMultiTenantLedgerReads drives the CRM routes whose use cases
// read LEDGER stores in-process — instrument create (ledger/account reference
// check) and holder delete (owned-account guard) — through the REAL
// buildUnifiedRouteSetup and its two CRM route options, against two tenants whose
// onboarding PostgreSQL, onboarding Mongo and CRM Mongo are distinct databases.
//
// Those two routes must resolve the tenant's onboarding PostgreSQL and onboarding
// Mongo on their module keys alongside the CRM Mongo on the generic key. Without
// the onboarding stores every one of these requests fails with a 500 "tenant
// postgres connection missing from context" instead of reaching the domain rule
// it exercises. Every other CRM route reads only the CRM Mongo, so a third tenant
// provisioned with the crm module alone must still create and read holders.
func TestIntegration_CRMMultiTenantLedgerReads(t *testing.T) {
	// The lib-commons clients reject plaintext URIs unless ALLOW_INSECURE_TLS=true;
	// the testcontainers speak plaintext.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	logger := &libLog.GoLogger{}

	mongoContainer := mongotestutil.SetupReusableContainer(t)

	tenantA := seedCompositionTenant(t, mongoContainer, "tenanta")
	tenantB := seedCompositionTenant(t, mongoContainer, "tenantb")

	tenantCRMOnly := seedCompositionTenant(t, mongoContainer, "tenantcrmonly")
	tenantCRMOnly.crmOnly = true

	tenants := map[string]*compositionTenant{
		tenantA.tenantID:       tenantA,
		tenantB.tenantID:       tenantB,
		tenantCRMOnly.tenantID: tenantCRMOnly,
	}

	tmServer := newFakeTenantManagerCompositionStores(t, mongoContainer, tenants)
	defer tmServer.Close()

	tenantClient, err := tmclient.NewClient(tmServer.URL, logger,
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tenantClient.Close() })

	onboardingPGManager := tmpostgres.NewManager(tenantClient, constant.ModuleOnboarding,
		tmpostgres.WithModule(constant.ModuleOnboarding), tmpostgres.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingPGManager.Close(context.Background()) })

	onboardingMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleOnboarding,
		tmmongo.WithModule(constant.ModuleOnboarding), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingMongoManager.Close(context.Background()) })

	crmMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleCRM,
		tmmongo.WithModule(constant.ModuleCRM), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = crmMongoManager.Close(context.Background()) })

	// The REAL composition root: the CRM route options under test are the ones
	// production mounts, so the test fails if buildUnifiedRouteSetup stops binding
	// any of the three stores the CRM routes reach.
	setup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		onboardingPGManager, &tmpostgres.Manager{},
		onboardingMongoManager, &tmmongo.Manager{}, crmMongoManager, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, setup.crmRouteOptions, "CRM route options must be built in multi-tenant mode")
	require.NotNil(t, setup.crmLedgerReadsRouteOptions, "CRM ledger-reads route options must be built in multi-tenant mode")
	require.NotNil(t, setup.crmHolderDeleteRouteOptions, "CRM holder-delete route options must be built in multi-tenant mode")

	// MT-style repositories throughout: nil static connections with requireTenant
	// set, so every store must come from the request context the middleware fills.
	ledgerReads := &query.UseCase{
		LedgerRepo:             ledger.NewLedgerPostgreSQLRepository(nil, true),
		AccountRepo:            account.NewAccountPostgreSQLRepository(nil, true),
		OnboardingMetadataRepo: onbMongo.NewMetadataMongoDBRepository(nil),
	}

	fieldEncryptor := cipherFieldEncryptor(t, testutils.SetupCrypto(t))
	holderRepo, err := holder.NewMongoDBRepository(nil, fieldEncryptor)
	require.NoError(t, err)
	instrumentRepo, err := instrument.NewMongoDBRepository(nil, fieldEncryptor)
	require.NoError(t, err)

	// Idempotency is nil, which the CRM use case treats as disabled.
	crmUC := &crmservices.UseCase{
		HolderRepo:     holderRepo,
		InstrumentRepo: instrumentRepo,
		LedgerAccounts: ledgerAccountReaderAdapter{query: ledgerReads},
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(ctx fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	app.Use(http.WithRecover(http.WithRecoverLogger(logger)))
	t.Cleanup(func() { _ = app.Shutdown() })

	// Auth disabled: Authorize is a pass-through, so the post-auth chain the CRM
	// options carry is what the request actually runs.
	mountCRMHuma(app, middleware.NewAuthClient("", false, nil),
		&httpin.HolderHandler{Service: crmUC}, &httpin.InstrumentHandler{Service: crmUC},
		nil, nil, nil, setup.crmRouteOptions, setup.crmLedgerReadsRouteOptions, setup.crmHolderDeleteRouteOptions)

	t.Run("holder_create_and_read_do_not_depend_on_onboarding_provisioning", func(t *testing.T) {
		orgID := tenantCRMOnly.orgID.String()

		holderID := createHolderHTTP(t, app, tenantCRMOnly.tenantID, orgID, "CRM Only Holder", "33333333333")

		assert.Equal(t, fiber.StatusOK, getHolderStatusHTTP(t, app, tenantCRMOnly.tenantID, orgID, holderID),
			"a tenant provisioned with the crm module alone must read its holder: the holder routes resolve only the CRM Mongo")
	})

	t.Run("instrument_create_with_valid_references_is_201_in_the_tenants_own_crm_mongo", func(t *testing.T) {
		holderID := createHolderHTTP(t, app, tenantA.tenantID, tenantA.orgID.String(), "Tenant A Instrument Holder", "11111111111")
		accountID := seedHolderOwnedAccount(t, tenantA, holderID)

		body, status := postInstrumentHTTP(t, app, tenantA, holderID, tenantA.ledgerID.String(), accountID)
		require.Equalf(t, stdhttp.StatusCreated, status,
			"valid ledger/account references must resolve through the tenant's onboarding stores; body: %s", body)

		var created mmodel.Instrument
		require.NoError(t, json.Unmarshal([]byte(body), &created), "body: %s", body)
		require.NotNil(t, created.AccountID)
		assert.Equal(t, accountID, *created.AccountID)

		collection := strings.ToLower("aliases_" + tenantA.orgID.String())

		inA, err := tenantA.crmMongo.Collection(collection).
			CountDocuments(context.Background(), bson.M{"account_id": accountID})
		require.NoError(t, err)
		assert.Equal(t, int64(1), inA, "the instrument must land in tenant A's own CRM Mongo")

		inB, err := tenantB.crmMongo.Collection(collection).
			CountDocuments(context.Background(), bson.M{"account_id": accountID})
		require.NoError(t, err)
		assert.Zero(t, inB, "tenant B's CRM Mongo must not carry tenant A's instrument")

		inOnboarding, err := tenantA.onboardingMongo.Collection(collection).
			CountDocuments(context.Background(), bson.M{"account_id": accountID})
		require.NoError(t, err)
		assert.Zero(t, inOnboarding,
			"the instrument reads the generic Mongo key, which must still resolve to the CRM store, "+
				"not to the module-keyed onboarding Mongo bound on the same middleware")
	})

	// KNOWN DEFECT — the reference check answers a missing ledger/account with the
	// query layer's 404 instead of the 422 referential error (0488 ledger, 0489
	// account). The ledger and account repositories report absence as the generic
	// entity-not-found business error (0007), while ledgerAccountReaderAdapter only
	// maps the query-specific codes (0037 ledger, 0052 account) to "absent", so the
	// 0007 propagates unchanged. The three subtests below assert that CURRENT
	// behavior on purpose: what they prove is that the lookup reached the tenant's
	// own onboarding PostgreSQL (a business not-found, never the 500 of a missing
	// tenant connection). Fixing the adapter flips them to 422 with the referential
	// codes.
	t.Run("instrument_create_with_an_unknown_ledger_is_a_business_not_found_not_500", func(t *testing.T) {
		holderID := createHolderHTTP(t, app, tenantA.tenantID, tenantA.orgID.String(), "Tenant A Unknown Ledger Holder", "33333333333")

		body, status := postInstrumentHTTP(t, app, tenantA, holderID, uuid.NewString(), uuid.NewString())
		assertProblem(t, body, status, stdhttp.StatusNotFound, constant.ErrEntityNotFound, constant.EntityLedger)
	})

	t.Run("instrument_create_with_another_tenants_ledger_is_not_found_in_this_tenant", func(t *testing.T) {
		// Tenant B addresses tenant A's organization, ledger and account ids, so the
		// organization filter cannot hide the rows: only the store the middleware
		// resolved decides the outcome. Tenant A's onboarding PostgreSQL holds them
		// (a misrouted lookup would answer 201); tenant B's does not.
		ownerA := createHolderHTTP(t, app, tenantA.tenantID, tenantA.orgID.String(), "Tenant A Ledger Owner", "44444444444")
		accountA := seedHolderOwnedAccount(t, tenantA, ownerA)

		tenantBOnOrgA := *tenantB
		tenantBOnOrgA.orgID = tenantA.orgID

		holderB := createHolderHTTP(t, app, tenantB.tenantID, tenantA.orgID.String(), "Tenant B Holder", "55555555555")

		body, status := postInstrumentHTTP(t, app, &tenantBOnOrgA, holderB, tenantA.ledgerID.String(), accountA)
		assertProblem(t, body, status, stdhttp.StatusNotFound, constant.ErrEntityNotFound, constant.EntityLedger)

		collection := strings.ToLower("aliases_" + tenantA.orgID.String())

		inB, err := tenantB.crmMongo.Collection(collection).CountDocuments(context.Background(), bson.M{"account_id": accountA})
		require.NoError(t, err)
		assert.Zero(t, inB, "a rejected reference must persist no instrument")

		inA, err := tenantA.crmMongo.Collection(collection).CountDocuments(context.Background(), bson.M{"account_id": accountA})
		require.NoError(t, err)
		assert.Zero(t, inA, "tenant B's request must write nothing into tenant A's CRM Mongo")
	})

	t.Run("instrument_create_with_an_unknown_account_is_a_business_not_found_not_500", func(t *testing.T) {
		holderID := createHolderHTTP(t, app, tenantA.tenantID, tenantA.orgID.String(), "Tenant A Unknown Account Holder", "66666666666")

		body, status := postInstrumentHTTP(t, app, tenantA, holderID, tenantA.ledgerID.String(), uuid.NewString())
		assertProblem(t, body, status, stdhttp.StatusNotFound, constant.ErrEntityNotFound, constant.EntityAccount)
	})

	t.Run("holder_delete_with_an_owned_account_is_the_ownership_guard_not_500", func(t *testing.T) {
		holderID := createHolderHTTP(t, app, tenantA.tenantID, tenantA.orgID.String(), "Tenant A Account Owner", "77777777777")
		seedHolderOwnedAccount(t, tenantA, holderID)

		body, status := deleteHolderHTTP(t, app, tenantA, holderID)
		assertProblem(t, body, status, stdhttp.StatusUnprocessableEntity, constant.ErrHolderHasAccounts, constant.EntityHolder)

		assert.Equal(t, stdhttp.StatusOK, getHolderStatusHTTP(t, app, tenantA.tenantID, tenantA.orgID.String(), holderID),
			"a guarded delete must leave the holder in place")
	})

	t.Run("holder_delete_without_accounts_is_204", func(t *testing.T) {
		// Tenant B deletes a holder under tenant A's organization id, and tenant A
		// owns an account under that same organization and holder id. The
		// organization filter matches in both stores, so only the resolved store
		// decides: tenant B's onboarding PostgreSQL has no such account (204), while
		// a misrouted count would read tenant A's and trip the ownership guard (422).
		tenantBOnOrgA := *tenantB
		tenantBOnOrgA.orgID = tenantA.orgID

		holderID := createHolderHTTP(t, app, tenantB.tenantID, tenantA.orgID.String(), "Tenant B Accountless Holder", "88888888888")
		seedHolderOwnedAccount(t, tenantA, holderID)

		body, status := deleteHolderHTTP(t, app, &tenantBOnOrgA, holderID)
		require.Equalf(t, stdhttp.StatusNoContent, status, "body: %s", body)

		assert.Equal(t, stdhttp.StatusNotFound, getHolderStatusHTTP(t, app, tenantB.tenantID, tenantA.orgID.String(), holderID),
			"the deleted holder must no longer be readable")
	})

	t.Run("holder_delete_depends_on_onboarding_provisioning", func(t *testing.T) {
		// Positive control for the crm-only tenant: its holder routes work, but the
		// owned-account guard needs the onboarding PostgreSQL the tenant-manager
		// does not provision for it, so tenant resolution rejects the delete.
		holderID := createHolderHTTP(t, app, tenantCRMOnly.tenantID, tenantCRMOnly.orgID.String(), "CRM Only Deleted Holder", "99999999999")

		body, status := deleteHolderHTTP(t, app, tenantCRMOnly, holderID)
		assert.Equalf(t, stdhttp.StatusServiceUnavailable, status,
			"a tenant without onboarding provisioning must fail tenant resolution on holder delete; body: %s", body)

		assert.Equal(t, fiber.StatusOK, getHolderStatusHTTP(t, app, tenantCRMOnly.tenantID, tenantCRMOnly.orgID.String(), holderID),
			"the rejected delete must leave the holder in place")
	})
}

// seedHolderOwnedAccount inserts an ACTIVE account owned by holderID into the
// tenant's onboarding PostgreSQL, under the tenant's seeded organization and
// ledger, and returns its id.
func seedHolderOwnedAccount(t *testing.T, tn *compositionTenant, holderID string) string {
	t.Helper()

	holderUUID, err := uuid.Parse(holderID)
	require.NoError(t, err)

	params := postgrestestutil.DefaultAccountParams()
	params.Alias = tn.alias("crm")
	params.Name = "crm reference account"
	params.HolderID = &holderUUID

	return postgrestestutil.CreateTestAccountWithParams(t, tn.onboardingPG.DB, tn.orgID, tn.ledgerID, params).String()
}

// postInstrumentHTTP issues the holder-scoped instrument create for a tenant,
// authenticated by the JWT tenantId claim the trusted-auth assertion reads.
func postInstrumentHTTP(t *testing.T, app *fiber.App, tn *compositionTenant, holderID, ledgerID, accountID string) (string, int) {
	t.Helper()

	payload, err := json.Marshal(mmodel.CreateInstrumentInput{
		LedgerID:  ledgerID,
		AccountID: accountID,
		BankingDetails: &mmodel.BankingDetails{
			Branch:  testutils.Ptr("0001"),
			Account: testutils.Ptr(fmt.Sprintf("%s-%s", tn.tenantID, uuid.NewString()[:8])),
		},
	})
	require.NoError(t, err)

	url := fmt.Sprintf("/v2/organizations/%s/holders/%s/instruments", tn.orgID, holderID)

	req := httptest.NewRequest(stdhttp.MethodPost, url, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+tn.authToken)

	return doCRMRequest(t, app, req)
}

func deleteHolderHTTP(t *testing.T, app *fiber.App, tn *compositionTenant, holderID string) (string, int) {
	t.Helper()

	req := httptest.NewRequest(stdhttp.MethodDelete,
		fmt.Sprintf("/v2/organizations/%s/holders/%s", tn.orgID, holderID), nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+tn.authToken)

	return doCRMRequest(t, app, req)
}

func doCRMRequest(t *testing.T, app *fiber.App, req *stdhttp.Request) (string, int) {
	t.Helper()

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 60 * time.Second})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body), resp.StatusCode
}

// assertProblem requires the response to be the given status carrying the given
// business code and entity type. The status is checked first so a 500 reports
// its body.
func assertProblem(t *testing.T, body string, status, wantStatus int, wantCode error, wantEntity string) {
	t.Helper()

	require.Equalf(t, wantStatus, status, "body: %s", body)

	var problem map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &problem), "body: %s", body)
	assert.Equal(t, wantCode.Error(), problem["code"], "body: %s", body)
	assert.Equal(t, wantEntity, problem["entityType"], "body: %s", body)
}
