//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	crmholder "github.com/LerianStudio/midaz/v4/components/crm/adapters/mongodb/holder"
	crminstrument "github.com/LerianStudio/midaz/v4/components/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/crm/services"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	nethttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/LerianStudio/midaz/v4/tests/utils/stubs"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compositionTestInfra holds the real cross-store infrastructure the composition
// route binds: an onboarding-PG account use case and a CRM-Mongo holder/instrument
// use case, composed behind the real composition Service and route registrar. It is
// the integration analogue of setupAssetTestInfra; the CRM repos use a static
// single-tenant Mongo connection (the multi-tenant key-isolation proof is F4-T14).
type compositionTestInfra struct {
	pgConn      *libPostgres.Client
	mongoConn   *libMongo.Client
	app         *fiber.App
	commandUC   *command.UseCase
	crmUC       *crmservices.UseCase
	compService *composition.Service
	// instrumentCreator is the InstrumentCreator the mounted Service uses. By
	// default it is the real CRM use case; the partial-failure test swaps it for
	// an erroring stub WITHOUT touching the account leg, so the account still
	// commits to real PG.
	instrumentCreator composition.InstrumentCreator
	// accountRepo lets tests assert PG persistence directly (holder_id, survival).
	accountRepo account.Repository
	// instrumentRepo lets tests assert Mongo persistence directly (count, linkage).
	instrumentRepo crminstrument.Repository
}

// erroringInstrumentCreator satisfies composition.InstrumentCreator and always
// fails AFTER the (real) account commit, with a typed business error so the
// partial-failure reason code is the stable business code, never raw text.
type erroringInstrumentCreator struct {
	err error
}

func (e erroringInstrumentCreator) CreateInstrument(_ context.Context, _ string, _ uuid.UUID, _ *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	return nil, e.err
}

// setupCompositionTestInfra spins up PG + Mongo containers, builds the real
// account and CRM use cases over them, composes them behind the real composition
// registrar, and mounts the route on a bare Fiber app with auth disabled (the
// Authorize chain becomes a pass-through, single-tenant routeOptions=nil). The
// instrumentCreator is the real CRM use case unless overridden.
func setupCompositionTestInfra(t *testing.T, instrumentCreator composition.InstrumentCreator) *compositionTestInfra {
	t.Helper()

	infra := &compositionTestInfra{}

	var (
		pgContainer    *postgrestestutil.ContainerResult
		mongoContainer *mongotestutil.ContainerResult
		wg             sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		pgContainer = postgrestestutil.SetupContainer(t)
	}()

	go func() {
		defer wg.Done()
		mongoContainer = mongotestutil.SetupContainer(t)
	}()

	wg.Wait()

	// Onboarding-PG connection + repos for the account leg.
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "onboarding")
	connStr := postgrestestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	infra.pgConn = postgrestestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)

	infra.mongoConn = mongotestutil.CreateConnection(t, mongoContainer.URI, "composition_test_db")

	orgRepo := organization.NewOrganizationPostgreSQLRepository(infra.pgConn)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(infra.pgConn)
	assetRepo := asset.NewAssetPostgreSQLRepository(infra.pgConn)
	infra.accountRepo = account.NewAccountPostgreSQLRepository(infra.pgConn)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(infra.pgConn)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(infra.pgConn)
	metadataRepo := mongodb.NewMetadataMongoDBRepository(infra.mongoConn)

	infra.commandUC = &command.UseCase{
		OrganizationRepo:       orgRepo,
		LedgerRepo:             ledgerRepo,
		AssetRepo:              assetRepo,
		AccountRepo:            infra.accountRepo,
		PortfolioRepo:          portfolioRepo,
		SegmentRepo:            segmentRepo,
		OnboardingMetadataRepo: metadataRepo,
		BalanceRepo:            stubs.NewBalanceRepoStub(),
	}

	// CRM-Mongo holder/instrument repos with a static single-tenant connection.
	cipher := testutils.SetupCrypto(t)

	holderRepo, err := crmholder.NewMongoDBRepository(infra.mongoConn, cipher)
	require.NoError(t, err)

	infra.instrumentRepo, err = crminstrument.NewMongoDBRepository(infra.mongoConn, cipher)
	require.NoError(t, err)

	infra.crmUC = &crmservices.UseCase{HolderRepo: holderRepo, InstrumentRepo: infra.instrumentRepo}

	infra.instrumentCreator = instrumentCreator
	if infra.instrumentCreator == nil {
		infra.instrumentCreator = infra.crmUC
	}

	infra.compService = composition.NewService(infra.commandUC, infra.instrumentCreator)

	// Mount the REAL composition registrar (Gate 1 + Gate 7): auth disabled makes
	// Authorize a pass-through, single-tenant routeOptions=nil. The account/CRM
	// org+ledger context arrives via the static connections, so no tenant
	// middleware is needed for the single-tenant integration proof.
	auth := middleware.NewAuthClient("", false, nil)
	compositionHandler := &CompositionHandler{Service: infra.compService}

	// Org/ledger setup for the account leg are created directly through the use
	// cases on a side app (the onboarding routes are not part of the composition
	// surface). The composition route itself is mounted on infra.app.
	infra.app = fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterCompositionRoutesToApp(infra.app, auth, compositionHandler, nil)

	t.Cleanup(func() {
		if err := infra.app.Shutdown(); err != nil {
			t.Logf("failed to shutdown Fiber app: %v", err)
		}
	})

	return infra
}

// seedOrgLedgerAsset creates an organization, ledger, and USD asset through the
// real command use case (the prerequisites the account leg validates). It returns
// org and ledger IDs.
func (infra *compositionTestInfra) seedOrgLedgerAsset(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()

	ctx := context.Background()

	org, err := infra.commandUC.CreateOrganization(ctx, &mmodel.CreateOrganizationInput{
		LegalName:     "Composition Test Org",
		LegalDocument: "12345678901234",
		Address: mmodel.Address{
			Line1:   "123 Test Street",
			ZipCode: "10001",
			City:    "New York",
			State:   "NY",
			Country: "US",
		},
	})
	require.NoError(t, err, "seed organization must succeed")

	orgID, err := uuid.Parse(org.ID)
	require.NoError(t, err)

	led, err := infra.commandUC.CreateLedger(ctx, orgID, &mmodel.CreateLedgerInput{Name: "Composition Test Ledger"})
	require.NoError(t, err, "seed ledger must succeed")

	ledgerID, err := uuid.Parse(led.ID)
	require.NoError(t, err)

	_, err = infra.commandUC.CreateAsset(ctx, orgID, ledgerID, &mmodel.CreateAssetInput{
		Name: "US Dollar",
		Code: "USD",
		Type: "currency",
	}, "")
	require.NoError(t, err, "seed USD asset must succeed")

	return orgID, ledgerID
}

// seedHolder creates a real CRM holder so the instrument leg's holder gate passes.
func (infra *compositionTestInfra) seedHolder(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()

	natural := "NATURAL_PERSON"
	holder, err := infra.crmUC.CreateHolder(context.Background(), orgID.String(), &mmodel.CreateHolderInput{
		Type:     &natural,
		Name:     "Composition Holder",
		Document: "11122233344",
	})
	require.NoError(t, err, "seed holder must succeed")
	require.NotNil(t, holder.ID)

	return *holder.ID
}

// postComposition issues the composition request and returns status + raw body.
func (infra *compositionTestInfra) postComposition(t *testing.T, orgID, ledgerID, holderID uuid.UUID, body map[string]any) (int, []byte) {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(fiber.MethodPost, "/v1/holders/"+holderID.String()+"/accounts", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-Id", orgID.String())
	req.Header.Set("X-Ledger-Id", ledgerID.String())
	req.Header.Set(fiber.HeaderAuthorization, "Bearer test-token")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, respBody
}

// TestIntegration_CompositionHappyPath proves the composition composes both
// primitives end-to-end (Gate 3): a 201 with a composite body, the PG account
// persisted with holder_id == the path holder, and the Mongo instrument persisted
// and linked to the new account ID + ledger ID.
func TestIntegration_CompositionHappyPath(t *testing.T) {
	infra := setupCompositionTestInfra(t, nil)

	orgID, ledgerID := infra.seedOrgLedgerAsset(t)
	holderID := infra.seedHolder(t, orgID)

	status, body := infra.postComposition(t, orgID, ledgerID, holderID, map[string]any{
		"name":      "Corporate Checking",
		"assetCode": "USD",
		"type":      "deposit",
		"bankingDetails": map[string]any{
			"branch":  "0001",
			"account": "123450",
		},
	})

	require.Equal(t, fiber.StatusCreated, status, "happy path must be 201, got body: %s", string(body))

	var resp mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Account, "account must be present")
	require.NotNil(t, resp.Instrument, "instrument must be present on the happy path")
	assert.Nil(t, resp.InstrumentError, "no failure block on the happy path")

	// PG leg: the account is persisted with holder_id == the path holder.
	require.NotNil(t, resp.Account.HolderID, "account holder_id must be set")
	assert.Equal(t, holderID.String(), *resp.Account.HolderID, "account holder_id must be the path holder")

	accountID, err := uuid.Parse(resp.Account.ID)
	require.NoError(t, err)

	persisted, err := infra.accountRepo.Find(context.Background(), orgID, ledgerID, nil, accountID)
	require.NoError(t, err, "account must be queryable in PG")
	require.NotNil(t, persisted.HolderID)
	assert.Equal(t, holderID.String(), *persisted.HolderID, "persisted PG account holder_id must be the path holder")

	// Mongo leg: exactly one instrument for the holder, linked to the new account
	// and the path ledger.
	count, err := infra.instrumentRepo.Count(context.Background(), orgID.String(), holderID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "exactly one instrument must be written")

	instruments, err := infra.instrumentRepo.FindAll(context.Background(), orgID.String(), holderID, nethttp.QueryHeader{Limit: 100, Page: 1}, false)
	require.NoError(t, err)
	require.Len(t, instruments, 1)
	require.NotNil(t, instruments[0].AccountID)
	assert.Equal(t, resp.Account.ID, *instruments[0].AccountID, "instrument must link the new account ID")
	require.NotNil(t, instruments[0].LedgerID)
	assert.Equal(t, ledgerID.String(), *instruments[0].LedgerID, "instrument must link the path ledger ID")
	require.NotNil(t, instruments[0].HolderID)
	assert.Equal(t, holderID, *instruments[0].HolderID, "instrument must link the path holder ID")
}

// TestIntegration_CompositionAccountOnlyNoInstrument proves the D-8 explicit-only
// gate (Gate 2): a request with no instrument fields creates the account, returns
// 201 with a null instrument, and writes ZERO instrument documents to Mongo.
func TestIntegration_CompositionAccountOnlyNoInstrument(t *testing.T) {
	infra := setupCompositionTestInfra(t, nil)

	orgID, ledgerID := infra.seedOrgLedgerAsset(t)
	holderID := infra.seedHolder(t, orgID)

	status, body := infra.postComposition(t, orgID, ledgerID, holderID, map[string]any{
		"name":      "Account Only",
		"assetCode": "USD",
		"type":      "deposit",
	})

	require.Equal(t, fiber.StatusCreated, status, "account-only must be 201, got body: %s", string(body))

	var resp mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Account)
	assert.Nil(t, resp.Instrument, "instrument must be null on the account-only path")
	assert.Nil(t, resp.InstrumentError, "no failure block on the account-only path")

	// The account is real.
	require.NotNil(t, resp.Account.HolderID)
	assert.Equal(t, holderID.String(), *resp.Account.HolderID)

	// D-8 proof: ZERO instrument documents written for this holder.
	count, err := infra.instrumentRepo.Count(context.Background(), orgID.String(), holderID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "account-only path must write NO instrument documents (D-8)")
}

// TestIntegration_CompositionPartialFailureNoCompensation proves the
// non-compensating partial-failure contract (Gate 4 / R23): when the instrument
// write fails AFTER the account commits, the account survives in PG and is
// queryable, NO compensating delete fires, the response is a 201 carrying the
// typed instrumentError block, and the standalone instrument create succeeds on
// retry against the surviving account.
//
// The failure is injected through the InstrumentCreator port (an erroring stub),
// not by corrupting Mongo, so the account commit path is real and untouched.
func TestIntegration_CompositionPartialFailureNoCompensation(t *testing.T) {
	wantErr := pkg.ValidateBusinessError(constant.ErrInstrumentNotFound, constant.EntityInstrument)
	infra := setupCompositionTestInfra(t, erroringInstrumentCreator{err: wantErr})

	orgID, ledgerID := infra.seedOrgLedgerAsset(t)
	holderID := infra.seedHolder(t, orgID)

	status, body := infra.postComposition(t, orgID, ledgerID, holderID, map[string]any{
		"name":      "Partial Failure Account",
		"assetCode": "USD",
		"type":      "deposit",
		"bankingDetails": map[string]any{
			"branch": "0001",
		},
	})

	// (c) HTTP 201 with the typed failure block: the request does NOT fail.
	require.Equal(t, fiber.StatusCreated, status, "partial failure must still be 201, got body: %s", string(body))

	var resp mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Account, "account must be present despite instrument failure")
	assert.Nil(t, resp.Instrument, "instrument must be null on failure")
	require.NotNil(t, resp.InstrumentError, "typed failure block must be surfaced")
	assert.Equal(t, "FAILED", resp.InstrumentError.Status)
	assert.Equal(t, constant.ErrInstrumentNotFound.Error(), resp.InstrumentError.Reason, "reason is the stable business code, not raw error text")

	accountID, err := uuid.Parse(resp.Account.ID)
	require.NoError(t, err)

	// (a)+(b) The account row SURVIVES in PG and is queryable: no compensating
	// delete fired. Deleting a balance-bearing ledger account is unacceptable.
	persisted, err := infra.accountRepo.Find(context.Background(), orgID, ledgerID, nil, accountID)
	require.NoError(t, err, "account must SURVIVE and be queryable in PG (no compensating delete)")
	require.NotNil(t, persisted.HolderID)
	assert.Equal(t, holderID.String(), *persisted.HolderID)

	// No instrument was written either (the write failed).
	count, err := infra.instrumentRepo.Count(context.Background(), orgID.String(), holderID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "no instrument should exist after the failed write")

	// (d) Retry succeeds against the SURVIVING account: the standalone instrument
	// create (the real CRM use case) links to the persisted account ID + ledger.
	retryInstrument, err := infra.crmUC.CreateInstrument(context.Background(), orgID.String(), holderID, &mmodel.CreateInstrumentInput{
		LedgerID:       ledgerID.String(),
		AccountID:      resp.Account.ID,
		BankingDetails: &mmodel.BankingDetails{Branch: testutils.Ptr("0001")},
	})
	require.NoError(t, err, "standalone instrument create must succeed on retry against the surviving account")
	require.NotNil(t, retryInstrument)
	require.NotNil(t, retryInstrument.AccountID)
	assert.Equal(t, resp.Account.ID, *retryInstrument.AccountID, "retried instrument must link the surviving account")

	countAfter, err := infra.instrumentRepo.Count(context.Background(), orgID.String(), holderID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countAfter, "exactly one instrument exists after a successful retry")
}

// TestIntegration_CompositionRouteMounted proves Gate 1: the unified app mounts
// POST /v1/holders/:id/accounts through the real registrar, and a request returns
// the composite body.
func TestIntegration_CompositionRouteMounted(t *testing.T) {
	infra := setupCompositionTestInfra(t, nil)

	// Route-table assertion: the composition route is registered on the app.
	found := false
	for _, route := range infra.app.GetRoutes() {
		if route.Method == fiber.MethodPost && route.Path == "/v1/holders/:id/accounts" {
			found = true
			break
		}
	}
	assert.True(t, found, "POST /v1/holders/:id/accounts must be registered on the app")

	// And it actually serves a composite body.
	orgID, ledgerID := infra.seedOrgLedgerAsset(t)
	holderID := infra.seedHolder(t, orgID)

	status, body := infra.postComposition(t, orgID, ledgerID, holderID, map[string]any{
		"name":      "Mounted Route Account",
		"assetCode": "USD",
		"type":      "deposit",
	})
	require.Equal(t, fiber.StatusCreated, status, "mounted route must serve 201, got body: %s", string(body))

	var resp mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.NotNil(t, resp.Account, "mounted route must return the composite account body")
}

// errorCode decodes the canonical midaz error envelope and returns its "code"
// field, the stable token the API surfaces.
func errorCode(t *testing.T, body []byte) string {
	t.Helper()

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(body, &envelope), "error body must be JSON: %s", string(body))

	code, _ := envelope["code"].(string)

	return code
}

// TestIntegration_CompositionRejectsInvalidAssetCode proves Gate 6 for the
// account leg: the composition layer does NOT bypass the account-side asset-code
// validation. An invalid asset code via composition is rejected with the SAME
// business error the standalone CreateAccount use case returns (ErrAssetCodeNotFound
// / HTTP 404), and no instrument is attempted.
func TestIntegration_CompositionRejectsInvalidAssetCode(t *testing.T) {
	infra := setupCompositionTestInfra(t, nil)

	orgID, ledgerID := infra.seedOrgLedgerAsset(t)
	holderID := infra.seedHolder(t, orgID)

	// Baseline: the standalone account-create use case rejects an unknown asset.
	// ValidateBusinessError maps the sentinel onto a typed error whose Code is the
	// sentinel string (it does not wrap the sentinel), so the canonical comparison
	// is against the typed error's Code, not errors.Is.
	_, standaloneErr := infra.commandUC.CreateAccount(context.Background(), orgID, ledgerID, &mmodel.CreateAccountInput{
		Name:      "Standalone Invalid Asset",
		AssetCode: "FAKE",
		Type:      "deposit",
	}, "")
	require.Error(t, standaloneErr, "standalone CreateAccount must reject an unknown asset code")

	var standaloneNotFound pkg.EntityNotFoundError
	require.ErrorAs(t, standaloneErr, &standaloneNotFound, "standalone error must be a typed not-found error")
	require.Equal(t, constant.ErrAssetCodeNotFound.Error(), standaloneNotFound.Code, "standalone error must carry the asset-code-not-found code")

	// Composition must reject identically: same business code, NOT bypassed.
	status, body := infra.postComposition(t, orgID, ledgerID, holderID, map[string]any{
		"name":      "Composition Invalid Asset",
		"assetCode": "FAKE",
		"type":      "deposit",
		"bankingDetails": map[string]any{
			"branch": "0001",
		},
	})

	assert.Equal(t, fiber.StatusNotFound, status, "invalid asset code must map to 404 via composition, got body: %s", string(body))
	assert.Equal(t, constant.ErrAssetCodeNotFound.Error(), errorCode(t, body),
		"composition must surface the SAME business code standalone CreateAccount returns")

	// The account never committed, so no instrument can exist for the holder.
	count, err := infra.instrumentRepo.Count(context.Background(), orgID.String(), holderID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "account-create rejection must NOT attempt the instrument")
}

// TestIntegration_CompositionRejectsNonexistentHolder proves Gate 6 for the
// instrument leg: the composition layer does NOT bypass the instrument-side
// holder-existence gate. With a non-existent holder, the account commits first
// (its own holder gate is permissive by default), then the instrument leg's
// GetHolderByID fires and fails with the SAME business error the standalone
// CreateInstrument use case returns (ErrHolderNotFound). Per the non-compensating
// contract this surfaces as a 201 carrying the typed instrumentError holding that
// exact code — the validation is PRESERVED, not weakened.
//
// Documented NON-assertion (R39): the instrument step does NOT validate account
// existence. Inside the composition flow this is benign because the account was
// just created in-call; this test deliberately does not claim an
// account-existence guarantee in the instrument step.
func TestIntegration_CompositionRejectsNonexistentHolder(t *testing.T) {
	infra := setupCompositionTestInfra(t, nil)

	orgID, ledgerID := infra.seedOrgLedgerAsset(t)

	// A holder ID that was never created in CRM Mongo.
	nonexistentHolder := uuid.New()

	// Baseline: the standalone instrument-create use case rejects an absent holder
	// (the holder gate fires) with ErrHolderNotFound. We need a real account ID to
	// pass, so create one first under the (permissive) account holder gate.
	account, err := infra.commandUC.CreateAccount(context.Background(), orgID, ledgerID, &mmodel.CreateAccountInput{
		Name:      "Baseline Account",
		AssetCode: "USD",
		Type:      "deposit",
		HolderID:  testutils.Ptr(nonexistentHolder.String()),
	}, "")
	require.NoError(t, err, "account create is permissive on the holder gate by default")

	_, standaloneErr := infra.crmUC.CreateInstrument(context.Background(), orgID.String(), nonexistentHolder, &mmodel.CreateInstrumentInput{
		LedgerID:       ledgerID.String(),
		AccountID:      account.ID,
		BankingDetails: &mmodel.BankingDetails{Branch: testutils.Ptr("0001")},
	})
	require.Error(t, standaloneErr, "standalone CreateInstrument must reject a non-existent holder")

	var standaloneHolderNotFound pkg.EntityNotFoundError
	require.ErrorAs(t, standaloneErr, &standaloneHolderNotFound, "standalone instrument error must be a typed not-found error")
	require.Equal(t, constant.ErrHolderNotFound.Error(), standaloneHolderNotFound.Code, "standalone instrument error must carry the holder-not-found code")

	// Composition with the same non-existent holder: the holder gate fires inside
	// the instrument leg, surfaced (non-compensating) as the typed failure block.
	status, body := infra.postComposition(t, orgID, ledgerID, nonexistentHolder, map[string]any{
		"name":      "Composition Nonexistent Holder",
		"assetCode": "USD",
		"type":      "deposit",
		"bankingDetails": map[string]any{
			"branch": "0001",
		},
	})

	require.Equal(t, fiber.StatusCreated, status, "non-compensating contract: account commits, 201 with failure block, got body: %s", string(body))

	var resp mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Account, "account commits before the holder gate fires")
	assert.Nil(t, resp.Instrument)
	require.NotNil(t, resp.InstrumentError, "the preserved holder gate must surface a typed failure")
	assert.Equal(t, constant.ErrHolderNotFound.Error(), resp.InstrumentError.Reason,
		"composition must surface the SAME holder-gate business code standalone CreateInstrument returns")
}

// TestIntegration_CompositionDependsOnUseCasesNotAdapters documents Gate 6 (c):
// the composition Service composes the two use-case surfaces through narrow ports
// and owns no adapters. The compile-time port satisfaction lives in
// services/composition/ports.go (var _ AccountCreator = (*command.UseCase)(nil);
// var _ InstrumentCreator = (*crmservices.UseCase)(nil)); here we assert the
// mounted Service holds exactly those use-case-typed ports, never a repository.
func TestIntegration_CompositionDependsOnUseCasesNotAdapters(t *testing.T) {
	infra := setupCompositionTestInfra(t, nil)

	// The Service's ports are satisfied by the use cases, not adapters: the
	// account port is the command use case and the instrument port is the CRM
	// use case. Concrete-type assertions prove the wiring composes use cases.
	_, accountIsUseCase := infra.compService.Accounts.(*command.UseCase)
	assert.True(t, accountIsUseCase, "AccountCreator must be the command use case, never an adapter")

	_, instrumentIsUseCase := infra.compService.Instruments.(*crmservices.UseCase)
	assert.True(t, instrumentIsUseCase, "InstrumentCreator must be the CRM use case, never an adapter")
}
