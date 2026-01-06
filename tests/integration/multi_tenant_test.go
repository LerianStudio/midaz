package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Multi-Tenant Integration Tests
//
// These tests verify the multi-tenant functionality of the Midaz platform.
// They cover:
// - Backward compatibility with single-tenant mode (MULTI_TENANT_ENABLED=false)
// - Tenant isolation when multi-tenant mode is enabled
// - Error handling for missing tenant context
//
// Environment Variables:
// - MULTI_TENANT_ENABLED: Set to "true" to enable multi-tenant tests
// - POOL_MANAGER_URL: URL of the pool manager service (required when multi-tenant is enabled)
// - ONBOARDING_URL: URL of the onboarding service
// - TRANSACTION_URL: URL of the transaction service
//
// To run multi-tenant tests:
//   MULTI_TENANT_ENABLED=true POOL_MANAGER_URL=http://pool-manager:8080 go test ./tests/integration/... -v -run "MultiTenant"

// TestMultiTenant_BackwardCompatibility verifies that single-tenant mode works
// when MULTI_TENANT_ENABLED=false (default behavior).
// This test ensures existing functionality is not broken by multi-tenant additions.
func TestMultiTenant_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Skip if multi-tenant is explicitly enabled - this test is for single-tenant mode
	if h.IsMultiTenantEnabled() {
		t.Skip("Skipping backward compatibility test - multi-tenant mode is enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Test 1: Create organization without tenant context (should work in single-tenant mode)
	orgName := fmt.Sprintf("BackwardCompat Org %s", h.RandString(5))
	orgPayload := h.OrgPayload(orgName, h.RandString(12))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, orgPayload)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if code != 201 {
		t.Fatalf("expected 201 for organization creation in single-tenant mode, got %d: %s", code, string(body))
	}

	var org struct {
		ID        string `json:"id"`
		LegalName string `json:"legalName"`
	}
	if err := json.Unmarshal(body, &org); err != nil || org.ID == "" {
		t.Fatalf("failed to parse organization response: %v body=%s", err, string(body))
	}

	t.Logf("Created organization in single-tenant mode: id=%s name=%s", org.ID, org.LegalName)

	// Test 2: List organizations (should return all organizations in single-tenant mode)
	code, body, err = onboard.Request(ctx, "GET", "/v1/organizations", headers, nil)
	if err != nil {
		t.Fatalf("list organizations request failed: %v", err)
	}
	if code != 200 {
		t.Fatalf("expected 200 for organization list in single-tenant mode, got %d: %s", code, string(body))
	}

	var orgList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &orgList); err != nil {
		t.Fatalf("failed to parse organization list: %v", err)
	}

	// Verify our created org appears in the list
	found := false
	for _, item := range orgList.Items {
		if item.ID == org.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created organization %s not found in list", org.ID)
	}

	t.Logf("Backward compatibility test passed: %d organizations found", len(orgList.Items))
}

// TestMultiTenant_TenantIsolation_Organizations verifies tenant data isolation.
// When multi-tenant mode is enabled:
// - Organizations created by Tenant A should only be visible to Tenant A
// - Tenant B should not be able to see or access Tenant A's organizations
func TestMultiTenant_TenantIsolation_Organizations(t *testing.T) {
	t.Parallel()

	// Skip if multi-tenant is NOT enabled
	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping tenant isolation test - multi-tenant mode is not enabled. Set MULTI_TENANT_ENABLED=true to run this test.")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Define two distinct tenants
	tenantA := "tenant-a-" + h.RandString(6)
	tenantB := "tenant-b-" + h.RandString(6)

	// Create headers for each tenant
	headersTenantA := h.TenantAuthHeaders(h.RandHex(8), tenantA)
	headersTenantB := h.TenantAuthHeaders(h.RandHex(8), tenantB)

	// Step 1: Create organization with Tenant A
	orgNameA := fmt.Sprintf("Tenant A Org %s", h.RandString(5))
	orgPayloadA := h.OrgPayload(orgNameA, h.RandString(12))

	codeA, bodyA, errA := onboard.Request(ctx, "POST", "/v1/organizations", headersTenantA, orgPayloadA)
	if errA != nil {
		t.Fatalf("Tenant A create organization request failed: %v", errA)
	}
	if codeA != 201 {
		t.Fatalf("expected 201 for Tenant A organization creation, got %d: %s", codeA, string(bodyA))
	}

	var orgA struct {
		ID        string `json:"id"`
		LegalName string `json:"legalName"`
	}
	if err := json.Unmarshal(bodyA, &orgA); err != nil || orgA.ID == "" {
		t.Fatalf("failed to parse Tenant A organization: %v body=%s", err, string(bodyA))
	}
	t.Logf("Tenant A created organization: id=%s name=%s", orgA.ID, orgA.LegalName)

	// Step 2: Create organization with Tenant B
	orgNameB := fmt.Sprintf("Tenant B Org %s", h.RandString(5))
	orgPayloadB := h.OrgPayload(orgNameB, h.RandString(12))

	codeB, bodyB, errB := onboard.Request(ctx, "POST", "/v1/organizations", headersTenantB, orgPayloadB)
	if errB != nil {
		t.Fatalf("Tenant B create organization request failed: %v", errB)
	}
	if codeB != 201 {
		t.Fatalf("expected 201 for Tenant B organization creation, got %d: %s", codeB, string(bodyB))
	}

	var orgB struct {
		ID        string `json:"id"`
		LegalName string `json:"legalName"`
	}
	if err := json.Unmarshal(bodyB, &orgB); err != nil || orgB.ID == "" {
		t.Fatalf("failed to parse Tenant B organization: %v body=%s", err, string(bodyB))
	}
	t.Logf("Tenant B created organization: id=%s name=%s", orgB.ID, orgB.LegalName)

	// Step 3: List organizations with Tenant A - should only see Tenant A's org
	code, body, err := onboard.Request(ctx, "GET", "/v1/organizations", headersTenantA, nil)
	if err != nil {
		t.Fatalf("Tenant A list organizations failed: %v", err)
	}
	if code != 200 {
		t.Fatalf("expected 200 for Tenant A list, got %d: %s", code, string(body))
	}

	var listA struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &listA); err != nil {
		t.Fatalf("failed to parse Tenant A list: %v", err)
	}

	// Verify Tenant A sees only their org
	foundA := false
	foundBInA := false
	for _, item := range listA.Items {
		if item.ID == orgA.ID {
			foundA = true
		}
		if item.ID == orgB.ID {
			foundBInA = true
		}
	}
	if !foundA {
		t.Errorf("Tenant A's organization %s not found in Tenant A's list", orgA.ID)
	}
	if foundBInA {
		t.Errorf("ISOLATION VIOLATION: Tenant B's organization %s found in Tenant A's list", orgB.ID)
	}

	// Step 4: List organizations with Tenant B - should only see Tenant B's org
	code, body, err = onboard.Request(ctx, "GET", "/v1/organizations", headersTenantB, nil)
	if err != nil {
		t.Fatalf("Tenant B list organizations failed: %v", err)
	}
	if code != 200 {
		t.Fatalf("expected 200 for Tenant B list, got %d: %s", code, string(body))
	}

	var listB struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &listB); err != nil {
		t.Fatalf("failed to parse Tenant B list: %v", err)
	}

	foundB := false
	foundAInB := false
	for _, item := range listB.Items {
		if item.ID == orgB.ID {
			foundB = true
		}
		if item.ID == orgA.ID {
			foundAInB = true
		}
	}
	if !foundB {
		t.Errorf("Tenant B's organization %s not found in Tenant B's list", orgB.ID)
	}
	if foundAInB {
		t.Errorf("ISOLATION VIOLATION: Tenant A's organization %s found in Tenant B's list", orgA.ID)
	}

	t.Log("Tenant isolation test passed for organizations")
}

// TestMultiTenant_CrossTenantAccessDenied verifies that one tenant cannot access
// another tenant's resources directly by ID.
func TestMultiTenant_CrossTenantAccessDenied(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping cross-tenant access test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Define two distinct tenants
	tenantA := "tenant-xaccess-a-" + h.RandString(6)
	tenantB := "tenant-xaccess-b-" + h.RandString(6)

	headersTenantA := h.TenantAuthHeaders(h.RandHex(8), tenantA)
	headersTenantB := h.TenantAuthHeaders(h.RandHex(8), tenantB)

	// Create organization with Tenant A
	orgPayload := h.OrgPayload(fmt.Sprintf("XAccess Org %s", h.RandString(5)), h.RandString(12))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headersTenantA, orgPayload)
	if err != nil || code != 201 {
		t.Fatalf("Tenant A create organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var orgA struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &orgA); err != nil {
		t.Fatalf("failed to parse org: %v", err)
	}
	t.Logf("Tenant A created organization: %s", orgA.ID)

	// Try to access Tenant A's organization with Tenant B's credentials
	code, body, err = onboard.Request(ctx, "GET", "/v1/organizations/"+orgA.ID, headersTenantB, nil)
	if err != nil {
		t.Fatalf("cross-tenant access request failed: %v", err)
	}

	// Expected: 404 (not found) or 403 (forbidden)
	// The organization should not be accessible to Tenant B
	if code == 200 {
		t.Errorf("ISOLATION VIOLATION: Tenant B was able to access Tenant A's organization %s", orgA.ID)
	} else if code != 404 && code != 403 {
		t.Logf("Cross-tenant access returned code %d (expected 404 or 403): %s", code, string(body))
	} else {
		t.Logf("Cross-tenant access correctly denied with status %d", code)
	}
}

// TestMultiTenant_TenantIsolation_Ledgers verifies tenant isolation for ledgers.
func TestMultiTenant_TenantIsolation_Ledgers(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping ledger isolation test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	tenantA := "tenant-ledger-a-" + h.RandString(6)
	tenantB := "tenant-ledger-b-" + h.RandString(6)

	headersTenantA := h.TenantAuthHeaders(h.RandHex(8), tenantA)
	headersTenantB := h.TenantAuthHeaders(h.RandHex(8), tenantB)

	// Create organization and ledger for Tenant A
	orgPayloadA := h.OrgPayload(fmt.Sprintf("Ledger Test Org A %s", h.RandString(5)), h.RandString(12))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headersTenantA, orgPayloadA)
	if err != nil || code != 201 {
		t.Fatalf("Tenant A create org failed: %d %v %s", code, err, string(body))
	}
	var orgA struct{ ID string }
	json.Unmarshal(body, &orgA)

	ledgerPayloadA := map[string]any{"name": fmt.Sprintf("Ledger A %s", h.RandString(5))}
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgA.ID), headersTenantA, ledgerPayloadA)
	if err != nil || code != 201 {
		t.Fatalf("Tenant A create ledger failed: %d %v %s", code, err, string(body))
	}
	var ledgerA struct{ ID string }
	json.Unmarshal(body, &ledgerA)
	t.Logf("Tenant A created ledger: %s", ledgerA.ID)

	// Create organization and ledger for Tenant B
	orgPayloadB := h.OrgPayload(fmt.Sprintf("Ledger Test Org B %s", h.RandString(5)), h.RandString(12))
	code, body, err = onboard.Request(ctx, "POST", "/v1/organizations", headersTenantB, orgPayloadB)
	if err != nil || code != 201 {
		t.Fatalf("Tenant B create org failed: %d %v %s", code, err, string(body))
	}
	var orgB struct{ ID string }
	json.Unmarshal(body, &orgB)

	ledgerPayloadB := map[string]any{"name": fmt.Sprintf("Ledger B %s", h.RandString(5))}
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgB.ID), headersTenantB, ledgerPayloadB)
	if err != nil || code != 201 {
		t.Fatalf("Tenant B create ledger failed: %d %v %s", code, err, string(body))
	}
	var ledgerB struct{ ID string }
	json.Unmarshal(body, &ledgerB)
	t.Logf("Tenant B created ledger: %s", ledgerB.ID)

	// Tenant B tries to access Tenant A's ledger
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgA.ID, ledgerA.ID), headersTenantB, nil)
	if err != nil {
		t.Fatalf("cross-tenant ledger access request failed: %v", err)
	}
	if code == 200 {
		t.Errorf("ISOLATION VIOLATION: Tenant B accessed Tenant A's ledger")
	} else {
		t.Logf("Cross-tenant ledger access correctly denied with status %d", code)
	}
}

// TestMultiTenant_TenantIsolation_Accounts verifies tenant isolation for accounts.
func TestMultiTenant_TenantIsolation_Accounts(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping account isolation test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	tenantA := "tenant-account-a-" + h.RandString(6)
	tenantB := "tenant-account-b-" + h.RandString(6)

	headersTenantA := h.TenantAuthHeaders(h.RandHex(8), tenantA)
	headersTenantB := h.TenantAuthHeaders(h.RandHex(8), tenantB)

	// Setup Tenant A: org → ledger → asset → account
	orgPayloadA := h.OrgPayload(fmt.Sprintf("Account Test Org A %s", h.RandString(5)), h.RandString(12))
	code, body, _ := onboard.Request(ctx, "POST", "/v1/organizations", headersTenantA, orgPayloadA)
	if code != 201 {
		t.Fatalf("Tenant A create org failed: %d %s", code, string(body))
	}
	var orgA struct{ ID string }
	json.Unmarshal(body, &orgA)

	ledgerPayload := map[string]any{"name": fmt.Sprintf("Ledger %s", h.RandString(5))}
	code, body, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgA.ID), headersTenantA, ledgerPayload)
	if code != 201 {
		t.Fatalf("Tenant A create ledger failed: %d %s", code, string(body))
	}
	var ledgerA struct{ ID string }
	json.Unmarshal(body, &ledgerA)

	// Create USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgA.ID, ledgerA.ID, headersTenantA); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	// Create account
	alias := fmt.Sprintf("account-%s", h.RandString(6))
	accountPayload := map[string]any{
		"name":      "Test Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
	}
	code, body, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgA.ID, ledgerA.ID), headersTenantA, accountPayload)
	if code != 201 {
		t.Fatalf("Tenant A create account failed: %d %s", code, string(body))
	}
	var accountA struct{ ID string }
	json.Unmarshal(body, &accountA)
	t.Logf("Tenant A created account: %s", accountA.ID)

	// Tenant B tries to access Tenant A's account
	code, body, err := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgA.ID, ledgerA.ID, accountA.ID), headersTenantB, nil)
	if err != nil {
		t.Fatalf("cross-tenant account access request failed: %v", err)
	}
	if code == 200 {
		t.Errorf("ISOLATION VIOLATION: Tenant B accessed Tenant A's account")
	} else {
		t.Logf("Cross-tenant account access correctly denied with status %d", code)
	}
}

// TestMultiTenant_TenantIsolation_Transactions verifies tenant isolation for transactions.
func TestMultiTenant_TenantIsolation_Transactions(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping transaction isolation test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	tenantA := "tenant-tx-a-" + h.RandString(6)
	tenantB := "tenant-tx-b-" + h.RandString(6)

	headersTenantA := h.TenantAuthHeaders(h.RandHex(8), tenantA)
	headersTenantB := h.TenantAuthHeaders(h.RandHex(8), tenantB)

	// Setup Tenant A: org → ledger → asset → account → transaction
	orgPayload := h.OrgPayload(fmt.Sprintf("TX Test Org A %s", h.RandString(5)), h.RandString(12))
	code, body, _ := onboard.Request(ctx, "POST", "/v1/organizations", headersTenantA, orgPayload)
	if code != 201 {
		t.Fatalf("Tenant A create org failed: %d %s", code, string(body))
	}
	var orgA struct{ ID string }
	json.Unmarshal(body, &orgA)

	ledgerPayload := map[string]any{"name": fmt.Sprintf("Ledger %s", h.RandString(5))}
	code, body, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgA.ID), headersTenantA, ledgerPayload)
	if code != 201 {
		t.Fatalf("Tenant A create ledger failed: %d %s", code, string(body))
	}
	var ledgerA struct{ ID string }
	json.Unmarshal(body, &ledgerA)

	if err := h.CreateUSDAsset(ctx, onboard, orgA.ID, ledgerA.ID, headersTenantA); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	alias := fmt.Sprintf("tx-account-%s", h.RandString(6))
	accountPayload := map[string]any{
		"name":      "TX Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
	}
	code, body, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgA.ID, ledgerA.ID), headersTenantA, accountPayload)
	if code != 201 {
		t.Fatalf("Tenant A create account failed: %d %s", code, string(body))
	}

	// Create transaction
	inflow := map[string]any{
		"code":        fmt.Sprintf("TR-INF-%s", h.RandString(5)),
		"description": "test inflow",
		"send": map[string]any{
			"asset": "USD",
			"value": "100.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "100.00"},
				}},
			},
		},
	}
	pathInflow := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgA.ID, ledgerA.ID)
	code, body, err := trans.Request(ctx, "POST", pathInflow, headersTenantA, inflow)
	if err != nil || code != 201 {
		t.Fatalf("Tenant A create transaction failed: %d %v %s", code, err, string(body))
	}
	var txA struct{ ID string }
	json.Unmarshal(body, &txA)
	t.Logf("Tenant A created transaction: %s", txA.ID)

	// Wait a moment for transaction to be processed
	time.Sleep(500 * time.Millisecond)

	// Tenant B tries to access Tenant A's transaction
	code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgA.ID, ledgerA.ID, txA.ID), headersTenantB, nil)
	if err != nil {
		t.Fatalf("cross-tenant transaction access request failed: %v", err)
	}
	if code == 200 {
		t.Errorf("ISOLATION VIOLATION: Tenant B accessed Tenant A's transaction")
	} else {
		t.Logf("Cross-tenant transaction access correctly denied with status %d", code)
	}
}

// TestMultiTenant_MissingTenantContext verifies proper error handling when
// multi-tenant mode is enabled but the request lacks tenant context.
func TestMultiTenant_MissingTenantContext(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping missing tenant context test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Use headers without tenant context (empty tenant ID)
	headersNoTenant := h.TenantAuthHeaders(h.RandHex(8), "")

	// Try to create organization without tenant context
	orgPayload := h.OrgPayload(fmt.Sprintf("No Tenant Org %s", h.RandString(5)), h.RandString(12))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headersNoTenant, orgPayload)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Expected: Should fail with 401 (unauthorized) or 400 (bad request)
	// because multi-tenant mode requires tenant context
	if code == 201 {
		t.Errorf("Request without tenant context succeeded when multi-tenant mode is enabled - expected failure")
	} else if code == 401 || code == 400 || code == 403 {
		t.Logf("Missing tenant context correctly rejected with status %d", code)
	} else {
		t.Logf("Missing tenant context returned status %d: %s", code, string(body))
	}
}

// TestMultiTenant_InvalidTenantToken verifies proper error handling for invalid JWT tokens.
func TestMultiTenant_InvalidTenantToken(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping invalid token test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Create headers with an invalid/malformed token
	headers := map[string]string{
		"Content-Type":  "application/json",
		"X-Request-Id":  h.RandHex(8),
		"Authorization": "Bearer invalid.token.here",
	}

	orgPayload := h.OrgPayload(fmt.Sprintf("Invalid Token Org %s", h.RandString(5)), h.RandString(12))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, orgPayload)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Expected: Should fail with 401 (unauthorized)
	if code == 201 {
		t.Errorf("Request with invalid token succeeded - expected authentication failure")
	} else if code == 401 {
		t.Logf("Invalid token correctly rejected with status 401")
	} else {
		t.Logf("Invalid token returned status %d: %s", code, string(body))
	}
}

// TestMultiTenant_ExpiredTenantToken verifies proper handling of expired JWT tokens.
func TestMultiTenant_ExpiredTenantToken(t *testing.T) {
	t.Parallel()

	if !h.IsMultiTenantEnabled() {
		t.Skip("Skipping expired token test - multi-tenant mode is not enabled")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Generate an expired token
	expiredToken, err := h.GenerateExpiredTestJWT("test-tenant", "test-user")
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"X-Request-Id":  h.RandHex(8),
		"Authorization": "Bearer " + expiredToken,
	}

	orgPayload := h.OrgPayload(fmt.Sprintf("Expired Token Org %s", h.RandString(5)), h.RandString(12))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, orgPayload)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Expected: Should fail with 401 (unauthorized) for expired token
	if code == 201 {
		t.Errorf("Request with expired token succeeded - expected authentication failure")
	} else if code == 401 {
		t.Logf("Expired token correctly rejected with status 401")
	} else {
		t.Logf("Expired token returned status %d: %s", code, string(body))
	}
}

// TestMultiTenant_HealthEndpointNoAuth verifies health endpoints work without authentication.
func TestMultiTenant_HealthEndpointNoAuth(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Health endpoint should work without any authentication, even in multi-tenant mode
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Test onboarding health
	code, _, err := onboard.Request(ctx, "GET", "/health", headers, nil)
	if err != nil {
		t.Fatalf("onboarding health check failed: %v", err)
	}
	if code != 200 {
		t.Errorf("onboarding health check expected 200, got %d", code)
	} else {
		t.Log("Onboarding health endpoint accessible without auth")
	}

	// Test transaction health
	code, _, err = trans.Request(ctx, "GET", "/health", headers, nil)
	if err != nil {
		t.Fatalf("transaction health check failed: %v", err)
	}
	if code != 200 {
		t.Errorf("transaction health check expected 200, got %d", code)
	} else {
		t.Log("Transaction health endpoint accessible without auth")
	}
}
