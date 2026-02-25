//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	nethttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/LerianStudio/midaz/v3/tests/utils/stubs"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRand is a deterministic random source for reproducible test runs.
// Using a fixed seed so failed tests can be debugged with the same sequences.
var testRand = rand.New(rand.NewSource(42))

// assetTestInfra holds all test infrastructure components for asset integration tests.
type assetTestInfra struct {
	pgContainer    *postgrestestutil.ContainerResult
	mongoContainer *mongotestutil.ContainerResult
	pgConn         *libPostgres.PostgresConnection
	app            *fiber.App
	orgHandler     *OrganizationHandler
	ledgerHandler  *LedgerHandler
	assetHandler   *AssetHandler
	accountHandler *AccountHandler
}

// setupAssetTestInfra initializes all containers and creates the handlers for asset tests.
func setupAssetTestInfra(t *testing.T) *assetTestInfra {
	t.Helper()

	infra := &assetTestInfra{}

	// Start containers in parallel (they don't depend on each other)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		infra.pgContainer = postgrestestutil.SetupContainer(t)
	}()

	go func() {
		defer wg.Done()
		infra.mongoContainer = mongotestutil.SetupContainer(t)
	}()

	wg.Wait()

	// Create PostgreSQL connection following lib-commons pattern
	logger := libZap.InitializeLogger()
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "onboarding")
	connStr := postgrestestutil.BuildConnectionString(infra.pgContainer.Host, infra.pgContainer.Port, infra.pgContainer.Config)

	infra.pgConn = &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           infra.pgContainer.Config.DBName,
		ReplicaDBName:           infra.pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Create MongoDB connection
	mongoConn := mongotestutil.CreateConnection(t, infra.mongoContainer.URI, "test_db")

	// Create repositories
	orgRepo := organization.NewOrganizationPostgreSQLRepository(infra.pgConn)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(infra.pgConn)
	assetRepo := asset.NewAssetPostgreSQLRepository(infra.pgConn)
	accountRepo := account.NewAccountPostgreSQLRepository(infra.pgConn)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(infra.pgConn)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(infra.pgConn)
	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongoConn)

	// Create use cases
	commandUC := &command.UseCase{
		OrganizationRepo: orgRepo,
		LedgerRepo:       ledgerRepo,
		AssetRepo:        assetRepo,
		AccountRepo:      accountRepo,
		PortfolioRepo:    portfolioRepo,
		SegmentRepo:      segmentRepo,
		MetadataRepo:     metadataRepo,
		BalancePort:      &stubs.BalancePortStub{},
	}
	queryUC := &query.UseCase{
		OrganizationRepo: orgRepo,
		LedgerRepo:       ledgerRepo,
		AssetRepo:        assetRepo,
		AccountRepo:      accountRepo,
		PortfolioRepo:    portfolioRepo,
		SegmentRepo:      segmentRepo,
		MetadataRepo:     metadataRepo,
	}

	// Create handlers
	infra.orgHandler = &OrganizationHandler{
		Command: commandUC,
		Query:   queryUC,
	}
	infra.ledgerHandler = &LedgerHandler{
		Command: commandUC,
		Query:   queryUC,
	}
	infra.assetHandler = &AssetHandler{
		Command: commandUC,
		Query:   queryUC,
	}
	infra.accountHandler = &AccountHandler{
		Command: commandUC,
		Query:   queryUC,
	}

	// Setup Fiber app with routes
	infra.app = fiber.New()
	infra.setupRoutes()

	// Register cleanup handlers.
	// NOTE on resource lifecycle:
	// - infra.pgContainer: Cleaned up by postgrestestutil.SetupContainer via t.Cleanup
	//   (closes DB connection and terminates container)
	// - infra.mongoContainer: Cleaned up by mongotestutil.SetupContainer via t.Cleanup
	//   (disconnects client and terminates container)
	// - infra.pgConn: Wrapper struct with connection strings; actual DB connections are
	//   managed lazily by lib-commons and cleaned when pgContainer terminates
	// - mongoConn: Wrapper struct; underlying client cleaned by mongoContainer cleanup
	// - infra.app: Fiber app must be explicitly shut down to release resources
	t.Cleanup(func() {
		if err := infra.app.Shutdown(); err != nil {
			t.Logf("failed to shutdown Fiber app: %v", err)
		}
	})

	return infra
}

// setupRoutes registers handler routes on the Fiber app.
func (infra *assetTestInfra) setupRoutes() {
	// Middleware to inject path params as locals
	paramMiddleware := func(c *fiber.Ctx) error {
		orgIDStr := c.Params("organization_id")
		ledgerIDStr := c.Params("ledger_id")
		assetIDStr := c.Params("id")

		if orgIDStr != "" {
			if orgID, err := uuid.Parse(orgIDStr); err == nil {
				c.Locals("organization_id", orgID)
			}
		}

		if ledgerIDStr != "" {
			if ledgerID, err := uuid.Parse(ledgerIDStr); err == nil {
				c.Locals("ledger_id", ledgerID)
			}
		}

		if assetIDStr != "" {
			if assetID, err := uuid.Parse(assetIDStr); err == nil {
				c.Locals("id", assetID)
			}
		}

		return c.Next()
	}

	// Middleware to inject organization ID for organization routes
	orgParamMiddleware := func(c *fiber.Ctx) error {
		idStr := c.Params("id")
		if idStr != "" {
			if id, err := uuid.Parse(idStr); err == nil {
				c.Locals("id", id)
			}
		}
		return c.Next()
	}

	// Organization routes
	infra.app.Post("/v1/organizations",
		nethttp.WithBody(new(mmodel.CreateOrganizationInput), infra.orgHandler.CreateOrganization))

	// Ledger routes
	infra.app.Post("/v1/organizations/:organization_id/ledgers",
		paramMiddleware, nethttp.WithBody(new(mmodel.CreateLedgerInput), infra.ledgerHandler.CreateLedger))

	// Asset routes
	infra.app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets",
		paramMiddleware, nethttp.WithBody(new(mmodel.CreateAssetInput), infra.assetHandler.CreateAsset))
	infra.app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets",
		paramMiddleware, infra.assetHandler.GetAllAssets)
	infra.app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id",
		paramMiddleware, infra.assetHandler.GetAssetByID)
	infra.app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id",
		paramMiddleware, infra.assetHandler.DeleteAssetByID)

	// Account routes
	infra.app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts",
		paramMiddleware, nethttp.WithBody(new(mmodel.CreateAccountInput), infra.accountHandler.CreateAccount))

	// Organization GET (for ID-based lookup)
	infra.app.Get("/v1/organizations/:id",
		orgParamMiddleware, infra.orgHandler.GetOrganizationByID)
}

// createOrganization creates an organization via HTTP and returns its ID.
func (infra *assetTestInfra) createOrganization(t *testing.T, name string) uuid.UUID {
	t.Helper()

	requestBody := map[string]any{
		"legalName":     name,
		"legalDocument": "12345678901234",
		"address": map[string]any{
			"line1":   "123 Test Street",
			"zipCode": "10001",
			"city":    "New York",
			"state":   "NY",
			"country": "US",
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err, "failed to marshal organization request body")

	req := httptest.NewRequest("POST", "/v1/organizations", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "failed to create organization")

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read organization response body")
	require.Equal(t, 201, resp.StatusCode, "expected 201, got %d: %s", resp.StatusCode, string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result), "failed to parse response")

	idValue, ok := result["id"].(string)
	require.True(t, ok, "response 'id' field is missing or not a string: %v", result)

	orgID, err := uuid.Parse(idValue)
	require.NoError(t, err, "failed to parse organization ID")

	return orgID
}

// createLedger creates a ledger via HTTP and returns its ID.
func (infra *assetTestInfra) createLedger(t *testing.T, orgID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	requestBody := map[string]any{
		"name": name,
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err, "failed to marshal ledger request body")

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "failed to create ledger")

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read ledger response body")
	require.Equal(t, 201, resp.StatusCode, "expected 201, got %d: %s", resp.StatusCode, string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result), "failed to parse response")

	idValue, ok := result["id"].(string)
	require.True(t, ok, "response 'id' field is missing or not a string: %v", result)

	ledgerID, err := uuid.Parse(idValue)
	require.NoError(t, err, "failed to parse ledger ID")

	return ledgerID
}

// createAsset creates an asset via HTTP and returns its ID.
func (infra *assetTestInfra) createAsset(t *testing.T, orgID, ledgerID uuid.UUID, name, code, assetType string) uuid.UUID {
	t.Helper()

	requestBody := map[string]any{
		"name": name,
		"code": code,
		"type": assetType,
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err, "failed to marshal asset request body")

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "failed to create asset")

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read asset response body")
	require.Equal(t, 201, resp.StatusCode, "expected 201, got %d: %s", resp.StatusCode, string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result), "failed to parse response")

	idValue, ok := result["id"].(string)
	require.True(t, ok, "response 'id' field is missing or not a string: %v", result)

	assetID, err := uuid.Parse(idValue)
	require.NoError(t, err, "failed to parse asset ID")

	return assetID
}

// deleteAsset deletes an asset via HTTP.
func (infra *assetTestInfra) deleteAsset(t *testing.T, orgID, ledgerID, assetID uuid.UUID) {
	t.Helper()

	req := httptest.NewRequest("DELETE",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(),
		nil)

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "failed to delete asset")

	require.Equal(t, 204, resp.StatusCode, "expected 204, got %d", resp.StatusCode)
}

// TestIntegration_AssetHandler_CreateAssetThenAccount validates the complete flow:
// 1. Create Organization -> 201
// 2. Create Ledger -> 201
// 3. Create USD Asset -> 201
// 4. GET Assets -> 200, verify asset exists
// 5. Create Account with assetCode: "USD" -> 201 (NOT 404)
func TestIntegration_AssetHandler_CreateAssetThenAccount(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	// Step 1: Create Organization
	orgID := infra.createOrganization(t, "Test Organization for Asset")
	t.Logf("Created organization: %s", orgID)

	// Step 2: Create Ledger
	ledgerID := infra.createLedger(t, orgID, "Test Ledger for Asset")
	t.Logf("Created ledger: %s", ledgerID)

	// Step 3: Create USD Asset
	assetID := infra.createAsset(t, orgID, ledgerID, "US Dollar", "USD", "currency")
	t.Logf("Created asset: %s", assetID)

	// Step 4: GET Asset by ID to verify it exists
	req := httptest.NewRequest("GET",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(),
		nil)

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "GET asset by ID request should not fail")

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read asset response body")
	assert.Equal(t, 200, resp.StatusCode, "expected 200, got %d: %s", resp.StatusCode, string(respBody))

	var assetResult map[string]any
	require.NoError(t, json.Unmarshal(respBody, &assetResult), "response should be valid JSON")

	// Verify USD asset fields
	assert.Equal(t, "USD", assetResult["code"], "asset code should be USD")
	assert.Equal(t, "US Dollar", assetResult["name"], "asset name should match")
	assert.Equal(t, "currency", assetResult["type"], "asset type should match")

	// Step 5: Create Account with assetCode: "USD"
	accountRequestBody := map[string]any{
		"name":      "Test Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     "@test-account",
	}
	accountBody, err := json.Marshal(accountRequestBody)
	require.NoError(t, err, "failed to marshal account request body")

	accountReq := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts",
		bytes.NewBuffer(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")

	accountResp, err := infra.app.Test(accountReq, -1)
	require.NoError(t, err, "create account request should not fail")

	accountRespBody, err := io.ReadAll(accountResp.Body)
	require.NoError(t, err, "failed to read account response body")
	t.Logf("Account response: %s", string(accountRespBody))

	// The key assertion: Account creation should succeed (201) because the asset exists
	assert.Equal(t, 201, accountResp.StatusCode,
		"account creation should succeed with existing asset, got %d: %s", accountResp.StatusCode, string(accountRespBody))

	// Parse and verify account response
	var accountResult map[string]any
	require.NoError(t, json.Unmarshal(accountRespBody, &accountResult), "response should be valid JSON")

	assert.Equal(t, "USD", accountResult["assetCode"], "account should have correct assetCode")
	assert.Equal(t, "Test Account", accountResult["name"], "account should have correct name")
	assert.Equal(t, "@test-account", accountResult["alias"], "account should have correct alias")
}

// TestIntegration_AssetHandler_AccountWithNonExistentAsset validates that:
// Creating an account with a non-existent asset code should fail with an appropriate error.
func TestIntegration_AssetHandler_AccountWithNonExistentAsset(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	// Create Organization and Ledger (but NO asset)
	orgID := infra.createOrganization(t, "Test Organization No Asset")
	ledgerID := infra.createLedger(t, orgID, "Test Ledger No Asset")

	// Act: Try to create Account with assetCode: "FAKE" (no asset created)
	accountRequestBody := map[string]any{
		"name":      "Test Account Fake Asset",
		"assetCode": "FAKE",
		"type":      "deposit",
		"alias":     "@fake-asset-account",
	}
	accountBody, err := json.Marshal(accountRequestBody)
	require.NoError(t, err, "failed to marshal account request body")

	accountReq := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts",
		bytes.NewBuffer(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")

	accountResp, err := infra.app.Test(accountReq, -1)
	require.NoError(t, err, "create account request should not fail")

	accountRespBody, err := io.ReadAll(accountResp.Body)
	require.NoError(t, err, "failed to read account response body")
	t.Logf("Account with non-existent asset response: status=%d, body=%s", accountResp.StatusCode, string(accountRespBody))

	// Assert: Should return 404 Not Found for non-existent asset
	assert.Equal(t, http.StatusNotFound, accountResp.StatusCode,
		"account creation with non-existent asset should return 404, got %d: %s",
		accountResp.StatusCode, string(accountRespBody))
}

// TestIntegration_AssetHandler_AccountWithDeletedAsset validates that:
// Creating an account with a deleted asset code should fail with an appropriate error.
func TestIntegration_AssetHandler_AccountWithDeletedAsset(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	// Create Organization, Ledger, and Asset
	orgID := infra.createOrganization(t, "Test Organization Deleted Asset")
	ledgerID := infra.createLedger(t, orgID, "Test Ledger Deleted Asset")
	assetID := infra.createAsset(t, orgID, ledgerID, "Euro", "EUR", "currency")

	t.Logf("Created asset EUR with ID: %s", assetID)

	// Verify asset exists before deletion
	getReq := httptest.NewRequest("GET",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(),
		nil)
	getResp, err := infra.app.Test(getReq, -1)
	require.NoError(t, err, "GET asset request should not fail")
	require.Equal(t, 200, getResp.StatusCode, "asset should exist before deletion")

	// Delete the Asset
	infra.deleteAsset(t, orgID, ledgerID, assetID)
	t.Logf("Deleted asset EUR")

	// Verify asset is deleted (should return 404)
	getReqAfterDelete := httptest.NewRequest("GET",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(),
		nil)
	getRespAfterDelete, err := infra.app.Test(getReqAfterDelete, -1)
	require.NoError(t, err, "GET asset after delete request should not fail")
	assert.Equal(t, 404, getRespAfterDelete.StatusCode, "deleted asset should return 404")

	// Act: Try to create Account with assetCode: "EUR" (deleted asset)
	accountRequestBody := map[string]any{
		"name":      "Test Account Deleted Asset",
		"assetCode": "EUR",
		"type":      "deposit",
		"alias":     "@deleted-asset-account",
	}
	accountBody, err := json.Marshal(accountRequestBody)
	require.NoError(t, err, "failed to marshal account request body")

	accountReq := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts",
		bytes.NewBuffer(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")

	accountResp, err := infra.app.Test(accountReq, -1)
	require.NoError(t, err, "create account request should not fail")

	accountRespBody, err := io.ReadAll(accountResp.Body)
	require.NoError(t, err, "failed to read account response body")
	t.Logf("Account with deleted asset response: status=%d, body=%s", accountResp.StatusCode, string(accountRespBody))

	// Assert: Should return 404 Not Found for deleted asset
	assert.Equal(t, http.StatusNotFound, accountResp.StatusCode,
		"account creation with deleted asset should return 404, got %d: %s",
		accountResp.StatusCode, string(accountRespBody))
}

// randString generates a random string of length n using common characters.
func randString(n int) string {
	if n <= 0 {
		return ""
	}
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-@:/")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[testRand.Intn(len(letters))]
	}
	return string(b)
}

// TestIntegration_Property_Account_AliasAndType tests that various account alias lengths
// and types don't cause 5xx errors. This is a property-based integration test that
// validates the system handles edge cases gracefully.
func TestIntegration_Property_Account_AliasAndType(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	// Create baseline org/ledger/asset
	orgID := infra.createOrganization(t, "Fuzz Account Test Org")
	ledgerID := infra.createLedger(t, orgID, "Fuzz Account Test Ledger")
	_ = infra.createAsset(t, orgID, ledgerID, "US Dollar", "USD", "currency")

	// Test cases with various alias lengths and types
	testCases := []struct {
		aliasLen int
		accType  string
	}{
		{0, "deposit"},
		{1, "deposit"},
		{10, "deposit"},
		{50, "external"},
		{100, "deposit"},
		{150, "external"},
	}

	for i, tc := range testCases {
		tc := tc // capture loop variable for subtest closure
		i := i
		t.Run(
			fmt.Sprintf("aliasLen=%d_type=%s", tc.aliasLen, tc.accType),
			func(t *testing.T) {
				alias := ""
				if tc.aliasLen > 0 {
					prefix := "@fuzz-"
					suffixLen := tc.aliasLen - len(prefix)
					if suffixLen > 0 {
						alias = prefix + randString(suffixLen)
					} else {
						// aliasLen is shorter than prefix, just use truncated prefix
						alias = prefix[:tc.aliasLen]
					}
				}

				accountRequestBody := map[string]any{
					"name":      fmt.Sprintf("Fuzz Account %d", i),
					"assetCode": "USD",
					"type":      tc.accType,
				}
				if alias != "" {
					accountRequestBody["alias"] = alias
				}

				accountBody, err := json.Marshal(accountRequestBody)
				require.NoError(t, err, "failed to marshal account request body")
				accountReq := httptest.NewRequest("POST",
					"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts",
					bytes.NewBuffer(accountBody))
				accountReq.Header.Set("Content-Type", "application/json")

				accountResp, err := infra.app.Test(accountReq, -1)
				require.NoError(t, err, "create account request should not fail")

				// Property: Should never return 5xx
				assert.Less(t, accountResp.StatusCode, 500,
					"server should not return 5xx for aliasLen=%d type=%s", tc.aliasLen, tc.accType)
			},
		)
	}
}

// TestIntegration_Property_Account_DuplicateAlias tests that duplicate alias submission
// is handled correctly (should return 409 Conflict, not 5xx).
func TestIntegration_Property_Account_DuplicateAlias(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	// Create baseline org/ledger/asset
	orgID := infra.createOrganization(t, "Fuzz Duplicate Alias Org")
	ledgerID := infra.createLedger(t, orgID, "Fuzz Duplicate Alias Ledger")
	_ = infra.createAsset(t, orgID, ledgerID, "US Dollar", "USD", "currency")

	// Create first account with a specific alias
	alias := "@fuzz-dup-test"
	accountRequestBody := map[string]any{
		"name":      "First Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
	}
	accountBody, err := json.Marshal(accountRequestBody)
	require.NoError(t, err, "failed to marshal account request body")

	accountReq := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts",
		bytes.NewBuffer(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(accountReq, -1)
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode, "first account creation should succeed")

	// Act: Try to create second account with same alias
	accountRequestBody["name"] = "Second Account"
	accountBody, err = json.Marshal(accountRequestBody)
	require.NoError(t, err, "failed to marshal account request body")

	duplicateReq := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts",
		bytes.NewBuffer(accountBody))
	duplicateReq.Header.Set("Content-Type", "application/json")

	duplicateResp, err := infra.app.Test(duplicateReq, -1)
	require.NoError(t, err)

	respBody, err := io.ReadAll(duplicateResp.Body)
	require.NoError(t, err, "failed to read duplicate response body")

	// Assert: Should return 409 Conflict for duplicate alias
	assert.Equal(t, http.StatusConflict, duplicateResp.StatusCode,
		"duplicate alias should return 409 Conflict, got %d: %s", duplicateResp.StatusCode, string(respBody))
}

// TestIntegration_Property_Structural_InvalidJSON tests that various malformed inputs
// are handled gracefully without causing 5xx errors.
func TestIntegration_Property_Structural_InvalidJSON(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	// Create baseline org for ledger tests
	orgID := infra.createOrganization(t, "Fuzz Structural Org")

	testCases := []struct {
		name        string
		contentType string
		body        []byte
		expect4xx   bool // false means server may accept it (lenient behavior)
	}{
		{"empty_body", "application/json", []byte{}, true},
		{"invalid_json", "application/json", []byte("{ invalid json }"), true},
		{"null_body", "application/json", []byte("null"), true},
		{"array_instead_of_object", "application/json", []byte("[]"), true},
		{"missing_required_fields", "application/json", []byte("{}"), true},
		// Server is lenient: accepts valid JSON regardless of Content-Type header
		{"wrong_content_type", "text/plain", []byte(`{"name": "Test"}`), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST",
				"/v1/organizations/"+orgID.String()+"/ledgers",
				bytes.NewBuffer(tc.body))
			req.Header.Set("Content-Type", tc.contentType)

			resp, err := infra.app.Test(req, -1)
			require.NoError(t, err)

			// Property: Should never return 5xx
			assert.Less(t, resp.StatusCode, 500,
				"server should not return 5xx for %s", tc.name)

			// Should return 4xx for structurally invalid inputs
			if tc.expect4xx {
				assert.GreaterOrEqual(t, resp.StatusCode, 400,
					"server should return 4xx for bad input %s, got %d", tc.name, resp.StatusCode)
			}
		})
	}
}

// TestIntegration_Property_Structural_LargeMetadata tests that large metadata payloads
// are handled gracefully without causing 5xx errors.
func TestIntegration_Property_Structural_LargeMetadata(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	orgID := infra.createOrganization(t, "Fuzz Large Metadata Org")

	// Create a large metadata value (~250KB)
	largeValue := make([]byte, 250*1024)
	for i := range largeValue {
		largeValue[i] = 'A'
	}

	ledgerRequestBody := map[string]any{
		"name": "Large Metadata Ledger",
		"metadata": map[string]any{
			"largeBlob": string(largeValue),
		},
	}
	body, err := json.Marshal(ledgerRequestBody)
	require.NoError(t, err, "failed to marshal ledger request body")

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err)

	// Property: Should never return 5xx (may return 413 or accept it)
	assert.Less(t, resp.StatusCode, 500,
		"server should not return 5xx for large metadata payload")
}

// TestIntegration_Property_Structural_UnknownFields tests that payloads with unknown fields
// are handled gracefully without causing server errors. The server may either reject unknown
// fields with 4xx (strict validation) or ignore them and return 2xx (lenient behavior).
// This test verifies the server never crashes (5xx) regardless of validation strategy.
func TestIntegration_Property_Structural_UnknownFields(t *testing.T) {
	// Arrange
	infra := setupAssetTestInfra(t)

	orgID := infra.createOrganization(t, "Fuzz Unknown Fields Org")

	// Payload with unknown field
	ledgerRequestBody := map[string]any{
		"name":         "Test Ledger",
		"unknownField": "should be rejected or ignored",
		"anotherFake":  123,
	}
	body, err := json.Marshal(ledgerRequestBody)
	require.NoError(t, err, "failed to marshal ledger request body")

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err)

	// Property: Should not return 5xx for unknown fields
	// Note: behavior may be 4xx (strict validation) or 2xx (lenient, ignores unknown)
	assert.Less(t, resp.StatusCode, 500,
		"server should not return 5xx for payload with unknown fields")
}
