# E2E Test Coverage Improvement Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add comprehensive integration tests for 16+ untested API endpoints including Asset Rates, Operation Routes, Transaction Routes, Operations Management, and Balance Mutations.

**Architecture:** Go integration tests following existing patterns in `tests/integration/`. Tests use the helpers package for HTTP client setup, authentication, and common payload construction. Each test creates isolated test data (org, ledger, accounts) to avoid interference.

**Tech Stack:** Go 1.21+, testify (assertions), shopspring/decimal (for balance checks), Midaz API (onboarding + transaction services)

**Global Prerequisites:**
- Environment: macOS/Linux with Go 1.21+
- Tools: Go toolchain, Docker (for local stack)
- Services Running: Midaz stack via `make up` or `docker compose up`
- Access: Local development (no auth required) or `TEST_AUTH_*` environment variables if using authenticated mode

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version                          # Expected: go version go1.21+ darwin/arm64
docker ps | grep -E "midaz|postgres|redis"  # Expected: containers running
curl -s http://localhost:3000/health | jq   # Expected: {"status":"ok"} or similar
curl -s http://localhost:3001/health | jq   # Expected: {"status":"ok"} or similar
```

## Historical Precedent

**Query:** "e2e tests integration testing coverage"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Overview

The plan is organized into 6 feature areas with a total of 24 tasks:

| Area | Priority | Tasks | Endpoints Covered |
|------|----------|-------|-------------------|
| 1. Asset Rates | High | 1.1-1.4 | PUT, GET by external_id, GET by asset_code |
| 2. Operation Routes | High | 2.1-2.4 | POST, GET, GET by ID, PATCH, DELETE |
| 3. Transaction Routes | High | 3.1-3.4 | POST, GET, GET by ID, PATCH, DELETE |
| 4. Operations Management | Medium | 4.1-4.3 | GET by account, GET single, PATCH |
| 5. Balance Mutations | Medium | 5.1-5.4 | PATCH, DELETE, POST additional, GET external |
| 6. Account Types | Lower | 6.1-6.2 | POST, PATCH, DELETE |

---

## 1. Asset Rates Tests (High Priority)

Asset rates enable multi-currency conversion. These endpoints are critical for financial applications dealing with multiple currencies.

### Task 1.1: Create Asset Rates Test File Structure

**File:** Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_rates_test.go`
**Estimated time:** 3 minutes
**Agent:** qa-analyst

**Description:** Create the test file with package declaration, imports, and helper constants. This establishes the foundation for all asset rate tests.

**Code:**
```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// assetRateResponse represents the API response for an asset rate
type assetRateResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	ExternalID     string         `json:"externalId"`
	From           string         `json:"from"`
	To             string         `json:"to"`
	Rate           float64        `json:"rate"`
	Scale          *float64       `json:"scale"`
	Source         *string        `json:"source"`
	TTL            int            `json:"ttl"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Metadata       map[string]any `json:"metadata"`
}

// assetRatesListResponse represents paginated asset rates response
type assetRatesListResponse struct {
	Items []assetRateResponse `json:"items"`
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/integration/...
```
Expected output: No errors (silent success)

**Failure Recovery:**
1. **Syntax error:** Check import paths match go.mod module name
2. **Module not found:** Run `go mod tidy` in project root
3. **Can't recover:** Document error and consult existing test files

---

### Task 1.2: Implement Asset Rate Create/Update Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_rates_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add test for PUT /v1/organizations/{org}/ledgers/{ledger}/asset-rates endpoint. Tests both creation of new asset rates and updating existing ones via the same PUT endpoint.

**Code:** (append to file)
```go

// TestIntegration_AssetRates_CreateAndUpdate tests PUT endpoint for asset rates
// which creates a new rate or updates existing one based on from/to pair.
func TestIntegration_AssetRates_CreateAndUpdate(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization
	orgName := fmt.Sprintf("AssetRate Org %s", h.RandString(5))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil {
		t.Fatalf("parse org: %v", err)
	}

	// Setup: Create ledger
	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("Ledger %s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil {
		t.Fatalf("parse ledger: %v", err)
	}

	// Setup: Create USD and BRL assets
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	brlPayload := map[string]any{"name": "Brazilian Real", "type": "currency", "code": "BRL"}
	assetsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", assetsPath, headers, brlPayload)
	if err != nil || (code != 201 && code != 409) {
		t.Fatalf("create BRL asset: code=%d err=%v body=%s", code, err, string(body))
	}

	// Test 1: Create new asset rate (USD to BRL)
	assetRatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", org.ID, ledger.ID)
	source := "Central Bank"
	createPayload := map[string]any{
		"from":   "USD",
		"to":     "BRL",
		"rate":   550,    // 5.50 BRL per USD
		"scale":  2,
		"source": source,
		"ttl":    3600,
		"metadata": map[string]any{
			"provider": "test",
		},
	}

	code, body, err = trans.Request(ctx, "PUT", assetRatePath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create asset rate: code=%d err=%v body=%s", code, err, string(body))
	}

	var created assetRateResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse created asset rate: %v body=%s", err, string(body))
	}

	// Verify response fields
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.From != "USD" {
		t.Errorf("expected From=USD, got %s", created.From)
	}
	if created.To != "BRL" {
		t.Errorf("expected To=BRL, got %s", created.To)
	}
	if created.Rate != 550 {
		t.Errorf("expected Rate=550, got %f", created.Rate)
	}

	// Test 2: Update existing rate (same from/to, different rate)
	updatePayload := map[string]any{
		"from":  "USD",
		"to":    "BRL",
		"rate":  560, // Updated rate
		"scale": 2,
	}

	code, body, err = trans.Request(ctx, "PUT", assetRatePath, headers, updatePayload)
	if err != nil || code != 201 {
		t.Fatalf("update asset rate: code=%d err=%v body=%s", code, err, string(body))
	}

	var updated assetRateResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("parse updated asset rate: %v body=%s", err, string(body))
	}

	if updated.Rate != 560 {
		t.Errorf("expected updated Rate=560, got %f", updated.Rate)
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_AssetRates_CreateAndUpdate ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_AssetRates_CreateAndUpdate`

**Failure Recovery:**
1. **Connection refused:** Verify services running with `docker ps`
2. **401 Unauthorized:** Check `TEST_AUTH_*` env vars or disable auth mode
3. **400 Bad Request:** Verify assets exist before creating rate
4. **Can't recover:** Document error with full request/response

---

### Task 1.3: Implement Asset Rate Get by External ID Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_rates_test.go`
**Estimated time:** 4 minutes
**Agent:** qa-analyst

**Description:** Add test for GET /v1/organizations/{org}/ledgers/{ledger}/asset-rates/{external_id} endpoint. Tests retrieval of asset rate by external identifier.

**Code:** (append to file)
```go

// TestIntegration_AssetRates_GetByExternalID tests retrieval of asset rate by external ID
func TestIntegration_AssetRates_GetByExternalID(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger, assets
	orgName := fmt.Sprintf("ExtID Org %s", h.RandString(5))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	eurPayload := map[string]any{"name": "Euro", "type": "currency", "code": "EUR"}
	assetsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", org.ID, ledger.ID)
	code, _, _ = onboard.Request(ctx, "POST", assetsPath, headers, eurPayload)
	if code != 201 && code != 409 {
		t.Fatalf("create EUR: code=%d", code)
	}

	// Create asset rate with specific external ID
	externalID := fmt.Sprintf("ext-%s", h.RandHex(16))
	assetRatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", org.ID, ledger.ID)
	createPayload := map[string]any{
		"from":       "USD",
		"to":         "EUR",
		"rate":       92,
		"scale":      2,
		"externalId": externalID,
	}

	code, body, err = trans.Request(ctx, "PUT", assetRatePath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create asset rate: code=%d err=%v body=%s", code, err, string(body))
	}

	var created assetRateResponse
	_ = json.Unmarshal(body, &created)

	// Test: GET by external ID
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/%s", org.ID, ledger.ID, created.ExternalID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get by external ID: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetched assetRateResponse
	if err := json.Unmarshal(body, &fetched); err != nil {
		t.Fatalf("parse fetched: %v body=%s", err, string(body))
	}

	if fetched.ExternalID != created.ExternalID {
		t.Errorf("external ID mismatch: want=%s got=%s", created.ExternalID, fetched.ExternalID)
	}
	if fetched.From != "USD" || fetched.To != "EUR" {
		t.Errorf("from/to mismatch: got from=%s to=%s", fetched.From, fetched.To)
	}

	// Test: GET non-existent external ID returns 404
	badPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/%s", org.ID, ledger.ID, "00000000-0000-0000-0000-000000000000")
	code, body, err = trans.Request(ctx, "GET", badPath, headers, nil)
	if err != nil || code != 404 {
		t.Fatalf("expected 404 for non-existent external ID, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_AssetRates_GetByExternalID ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_AssetRates_GetByExternalID`

**Failure Recovery:**
1. **404 on GET:** Verify asset rate was created successfully
2. **UUID parse error:** Ensure external ID is valid UUID format
3. **Can't recover:** Check if external ID field is being persisted correctly

---

### Task 1.4: Implement Asset Rate Get by Asset Code Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_rates_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add test for GET /v1/organizations/{org}/ledgers/{ledger}/asset-rates/from/{asset_code} endpoint. Tests retrieval of all asset rates from a specific source currency with pagination.

**Code:** (append to file)
```go

// TestIntegration_AssetRates_GetByAssetCode tests listing rates by source asset code
func TestIntegration_AssetRates_GetByAssetCode(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger
	orgName := fmt.Sprintf("AssetCode Org %s", h.RandString(5))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	// Create multiple assets
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	assetsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", org.ID, ledger.ID)
	currencies := []map[string]any{
		{"name": "Euro", "type": "currency", "code": "EUR"},
		{"name": "British Pound", "type": "currency", "code": "GBP"},
		{"name": "Japanese Yen", "type": "currency", "code": "JPY"},
	}
	for _, cur := range currencies {
		code, _, _ = onboard.Request(ctx, "POST", assetsPath, headers, cur)
		if code != 201 && code != 409 {
			t.Fatalf("create %s: code=%d", cur["code"], code)
		}
	}

	// Create multiple asset rates from USD
	assetRatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", org.ID, ledger.ID)
	rates := []map[string]any{
		{"from": "USD", "to": "EUR", "rate": 92, "scale": 2},
		{"from": "USD", "to": "GBP", "rate": 79, "scale": 2},
		{"from": "USD", "to": "JPY", "rate": 15000, "scale": 2},
	}

	for _, rate := range rates {
		code, body, err = trans.Request(ctx, "PUT", assetRatePath, headers, rate)
		if err != nil || code != 201 {
			t.Fatalf("create rate %s->%s: code=%d err=%v body=%s", rate["from"], rate["to"], code, err, string(body))
		}
	}

	// Test: GET all rates from USD
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/from/USD", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get by asset code: code=%d err=%v body=%s", code, err, string(body))
	}

	var list assetRatesListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("parse list: %v body=%s", err, string(body))
	}

	if len(list.Items) < 3 {
		t.Errorf("expected at least 3 rates from USD, got %d", len(list.Items))
	}

	// Verify all items have From=USD
	for _, item := range list.Items {
		if item.From != "USD" {
			t.Errorf("expected all items From=USD, got %s", item.From)
		}
	}

	// Test: GET with limit parameter
	limitPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/from/USD?limit=2", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "GET", limitPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get with limit: code=%d err=%v body=%s", code, err, string(body))
	}

	var limitedList assetRatesListResponse
	if err := json.Unmarshal(body, &limitedList); err != nil {
		t.Fatalf("parse limited list: %v", err)
	}

	if len(limitedList.Items) > 2 {
		t.Errorf("expected max 2 items with limit=2, got %d", len(limitedList.Items))
	}

	// Test: GET rates from non-existent asset code returns 200 with empty items
	emptyPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/from/XXX", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "GET", emptyPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get non-existent asset: code=%d err=%v body=%s", code, err, string(body))
	}

	var emptyList assetRatesListResponse
	_ = json.Unmarshal(body, &emptyList)
	if len(emptyList.Items) != 0 {
		t.Errorf("expected empty list for non-existent asset, got %d items", len(emptyList.Items))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_AssetRates_GetByAssetCode ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_AssetRates_GetByAssetCode`

**Failure Recovery:**
1. **Less than expected rates:** Check if all PUT requests succeeded
2. **Pagination not working:** Verify API supports limit parameter
3. **Can't recover:** Document and check API implementation

---

### Task 1.5: Code Review Checkpoint - Asset Rates

**Estimated time:** 3 minutes
**Agent:** code-reviewer

**Description:** Run code review on the asset rates test file before proceeding.

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location
- Format: `TODO(review): [Issue description] (reported by [reviewer] on [date], severity: Low)`

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_AssetRates ./tests/integration/... -count=1 -timeout 120s
```
Expected output: All asset rates tests pass

---

## 2. Operation Routes Tests (High Priority)

Operation routes define rules for how operations are matched to accounts. Full CRUD testing is essential.

### Task 2.1: Create Operation Routes Test File

**File:** Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operation_routes_test.go`
**Estimated time:** 3 minutes
**Agent:** qa-analyst

**Description:** Create test file with response types for operation routes.

**Code:**
```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// operationRouteResponse represents the API response for an operation route
type operationRouteResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Code           string         `json:"code"`
	OperationType  string         `json:"operationType"`
	Account        *accountRule   `json:"account"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// accountRule represents account selection rules
type accountRule struct {
	RuleType string `json:"ruleType"`
	ValidIf  any    `json:"validIf"`
}

// operationRoutesListResponse represents paginated operation routes
type operationRoutesListResponse struct {
	Items []operationRouteResponse `json:"items"`
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/integration/...
```
Expected output: No errors

**Failure Recovery:**
1. **Type conflicts:** Ensure no duplicate type definitions across test files
2. **Import errors:** Check import paths match project structure

---

### Task 2.2: Implement Operation Routes CRUD Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operation_routes_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add comprehensive CRUD test for operation routes covering create, read, update, delete, and list operations.

**Code:** (append to file)
```go

// TestIntegration_OperationRoutes_CRUD tests full lifecycle of operation routes
func TestIntegration_OperationRoutes_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger
	orgName := fmt.Sprintf("OpRoute Org %s", h.RandString(5))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	routesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", org.ID, ledger.ID)

	// CREATE: Post new operation route
	createPayload := map[string]any{
		"title":         "Cashin Source",
		"description":   "Source route for cash-in operations",
		"code":          fmt.Sprintf("SRC-%s", h.RandString(4)),
		"operationType": "source",
		"account": map[string]any{
			"ruleType": "alias",
			"validIf":  "@cash_account",
		},
		"metadata": map[string]any{
			"priority": "high",
		},
	}

	code, body, err = trans.Request(ctx, "POST", routesPath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create operation route: code=%d err=%v body=%s", code, err, string(body))
	}

	var created operationRouteResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse created: %v body=%s", err, string(body))
	}

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Title != "Cashin Source" {
		t.Errorf("expected Title='Cashin Source', got %s", created.Title)
	}
	if created.OperationType != "source" {
		t.Errorf("expected OperationType='source', got %s", created.OperationType)
	}

	// READ: Get by ID
	getPath := fmt.Sprintf("%s/%s", routesPath, created.ID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get operation route: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetched operationRouteResponse
	if err := json.Unmarshal(body, &fetched); err != nil {
		t.Fatalf("parse fetched: %v body=%s", err, string(body))
	}

	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: want=%s got=%s", created.ID, fetched.ID)
	}

	// UPDATE: Patch operation route
	updatePayload := map[string]any{
		"title":       "Updated Cashin Source",
		"description": "Updated description for source route",
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update operation route: code=%d err=%v body=%s", code, err, string(body))
	}

	var updated operationRouteResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("parse updated: %v body=%s", err, string(body))
	}

	if updated.Title != "Updated Cashin Source" {
		t.Errorf("expected updated Title, got %s", updated.Title)
	}

	// LIST: Get all operation routes
	code, body, err = trans.Request(ctx, "GET", routesPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list operation routes: code=%d err=%v body=%s", code, err, string(body))
	}

	var list operationRoutesListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("parse list: %v body=%s", err, string(body))
	}

	if len(list.Items) < 1 {
		t.Error("expected at least 1 operation route in list")
	}

	// DELETE: Remove operation route
	code, body, err = trans.Request(ctx, "DELETE", getPath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete operation route: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion: GET should return 404
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 404 {
		t.Fatalf("expected 404 after delete, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_OperationRoutes_CRUD ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_OperationRoutes_CRUD`

**Failure Recovery:**
1. **403 Forbidden:** Check if routing authorization differs from midaz auth
2. **400 on create:** Verify required fields (title, operationType)
3. **Delete returns non-204:** Some APIs return 200 with empty body - adjust assertion

---

### Task 2.3: Implement Operation Routes Validation Tests

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operation_routes_test.go`
**Estimated time:** 4 minutes
**Agent:** qa-analyst

**Description:** Add negative test cases for operation routes validation.

**Code:** (append to file)
```go

// TestIntegration_OperationRoutes_Validation tests input validation for operation routes
func TestIntegration_OperationRoutes_Validation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Val Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	routesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", org.ID, ledger.ID)

	// Test 1: Missing required title field
	code, body, err = trans.Request(ctx, "POST", routesPath, headers, map[string]any{
		"operationType": "source",
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for missing title, got code=%d body=%s", code, string(body))
	}

	// Test 2: Missing required operationType field
	code, body, err = trans.Request(ctx, "POST", routesPath, headers, map[string]any{
		"title": "Test Route",
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for missing operationType, got code=%d body=%s", code, string(body))
	}

	// Test 3: Invalid operationType value
	code, body, err = trans.Request(ctx, "POST", routesPath, headers, map[string]any{
		"title":         "Test Route",
		"operationType": "invalid_type",
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for invalid operationType, got code=%d body=%s", code, string(body))
	}

	// Test 4: Title exceeds max length (50 chars)
	longTitle := h.RandString(60)
	code, body, err = trans.Request(ctx, "POST", routesPath, headers, map[string]any{
		"title":         longTitle,
		"operationType": "source",
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for title too long, got code=%d body=%s", code, string(body))
	}

	// Test 5: GET non-existent route returns 404
	badPath := fmt.Sprintf("%s/%s", routesPath, "00000000-0000-0000-0000-000000000000")
	code, body, err = trans.Request(ctx, "GET", badPath, headers, nil)
	if err != nil || code != 404 {
		t.Errorf("expected 404 for non-existent route, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_OperationRoutes_Validation ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_OperationRoutes_Validation`

**Failure Recovery:**
1. **Different error codes:** API may return 422 instead of 400 - adjust expectations
2. **Length validation not enforced:** Check model validation tags
3. **Can't recover:** Document actual validation behavior

---

### Task 2.4: Code Review Checkpoint - Operation Routes

**Estimated time:** 3 minutes
**Agent:** code-reviewer

**Description:** Run code review on operation routes tests.

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Focus on: operation_routes_test.go

2. **Handle findings by severity**

3. **Proceed only when zero Critical/High/Medium issues remain**

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_OperationRoutes ./tests/integration/... -count=1 -timeout 120s
```
Expected output: All operation routes tests pass

---

## 3. Transaction Routes Tests (High Priority)

Transaction routes group operation routes for complex transaction flows.

### Task 3.1: Create Transaction Routes Test File

**File:** Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/transaction_routes_test.go`
**Estimated time:** 3 minutes
**Agent:** qa-analyst

**Description:** Create test file with response types for transaction routes.

**Code:**
```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// transactionRouteResponse represents the API response for a transaction route
type transactionRouteResponse struct {
	ID              string                   `json:"id"`
	OrganizationID  string                   `json:"organizationId"`
	LedgerID        string                   `json:"ledgerId"`
	Title           string                   `json:"title"`
	Description     string                   `json:"description"`
	OperationRoutes []operationRouteResponse `json:"operationRoutes"`
	Metadata        map[string]any           `json:"metadata"`
	CreatedAt       time.Time                `json:"createdAt"`
	UpdatedAt       time.Time                `json:"updatedAt"`
}

// transactionRoutesListResponse represents paginated transaction routes
type transactionRoutesListResponse struct {
	Items []transactionRouteResponse `json:"items"`
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/integration/...
```
Expected output: No errors

**Failure Recovery:**
1. **Type conflicts:** Ensure operationRouteResponse is defined once
2. **Import errors:** Verify all imports are correct

---

### Task 3.2: Implement Transaction Routes CRUD Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/transaction_routes_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add comprehensive CRUD test for transaction routes. Must first create operation routes to reference.

**Code:** (append to file)
```go

// TestIntegration_TransactionRoutes_CRUD tests full lifecycle of transaction routes
func TestIntegration_TransactionRoutes_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger
	orgName := fmt.Sprintf("TxRoute Org %s", h.RandString(5))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	opRoutesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", org.ID, ledger.ID)
	txRoutesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", org.ID, ledger.ID)

	// Create two operation routes to reference
	sourceRoute := map[string]any{
		"title":         "Source Route",
		"operationType": "source",
	}
	code, body, err = trans.Request(ctx, "POST", opRoutesPath, headers, sourceRoute)
	if err != nil || code != 201 {
		t.Fatalf("create source op route: code=%d err=%v body=%s", code, err, string(body))
	}
	var srcRoute struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &srcRoute)

	destRoute := map[string]any{
		"title":         "Destination Route",
		"operationType": "destination",
	}
	code, body, err = trans.Request(ctx, "POST", opRoutesPath, headers, destRoute)
	if err != nil || code != 201 {
		t.Fatalf("create dest op route: code=%d err=%v body=%s", code, err, string(body))
	}
	var dstRoute struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &dstRoute)

	// CREATE: Post new transaction route
	createPayload := map[string]any{
		"title":           "Payment Flow",
		"description":     "Standard payment transaction flow",
		"operationRoutes": []string{srcRoute.ID, dstRoute.ID},
		"metadata": map[string]any{
			"category": "payment",
		},
	}

	code, body, err = trans.Request(ctx, "POST", txRoutesPath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create transaction route: code=%d err=%v body=%s", code, err, string(body))
	}

	var created transactionRouteResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse created: %v body=%s", err, string(body))
	}

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Title != "Payment Flow" {
		t.Errorf("expected Title='Payment Flow', got %s", created.Title)
	}
	if len(created.OperationRoutes) != 2 {
		t.Errorf("expected 2 operation routes, got %d", len(created.OperationRoutes))
	}

	// READ: Get by ID
	getPath := fmt.Sprintf("%s/%s", txRoutesPath, created.ID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get transaction route: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetched transactionRouteResponse
	if err := json.Unmarshal(body, &fetched); err != nil {
		t.Fatalf("parse fetched: %v body=%s", err, string(body))
	}

	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: want=%s got=%s", created.ID, fetched.ID)
	}

	// UPDATE: Patch transaction route
	updatePayload := map[string]any{
		"title":       "Updated Payment Flow",
		"description": "Updated flow description",
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update transaction route: code=%d err=%v body=%s", code, err, string(body))
	}

	var updated transactionRouteResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("parse updated: %v body=%s", err, string(body))
	}

	if updated.Title != "Updated Payment Flow" {
		t.Errorf("expected updated Title, got %s", updated.Title)
	}

	// LIST: Get all transaction routes
	code, body, err = trans.Request(ctx, "GET", txRoutesPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list transaction routes: code=%d err=%v body=%s", code, err, string(body))
	}

	var list transactionRoutesListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("parse list: %v body=%s", err, string(body))
	}

	if len(list.Items) < 1 {
		t.Error("expected at least 1 transaction route in list")
	}

	// DELETE: Remove transaction route
	code, body, err = trans.Request(ctx, "DELETE", getPath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete transaction route: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 404 {
		t.Fatalf("expected 404 after delete, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_TransactionRoutes_CRUD ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_TransactionRoutes_CRUD`

**Failure Recovery:**
1. **Operation routes not found:** Ensure operation routes created before transaction route
2. **Validation errors:** Check if operationRoutes requires UUIDs as strings
3. **Delete returns 200:** Adjust assertion if API doesn't return 204

---

### Task 3.3: Implement Transaction Routes Validation Tests

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/transaction_routes_test.go`
**Estimated time:** 4 minutes
**Agent:** qa-analyst

**Description:** Add negative test cases for transaction routes validation.

**Code:** (append to file)
```go

// TestIntegration_TransactionRoutes_Validation tests input validation
func TestIntegration_TransactionRoutes_Validation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("TxVal Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	txRoutesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", org.ID, ledger.ID)

	// Test 1: Missing required title field
	code, body, err = trans.Request(ctx, "POST", txRoutesPath, headers, map[string]any{
		"operationRoutes": []string{},
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for missing title, got code=%d body=%s", code, string(body))
	}

	// Test 2: Missing required operationRoutes field
	code, body, err = trans.Request(ctx, "POST", txRoutesPath, headers, map[string]any{
		"title": "Test Route",
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for missing operationRoutes, got code=%d body=%s", code, string(body))
	}

	// Test 3: Invalid UUID in operationRoutes
	code, body, err = trans.Request(ctx, "POST", txRoutesPath, headers, map[string]any{
		"title":           "Test Route",
		"operationRoutes": []string{"not-a-uuid"},
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for invalid UUID, got code=%d body=%s", code, string(body))
	}

	// Test 4: Non-existent operation route UUID
	code, body, err = trans.Request(ctx, "POST", txRoutesPath, headers, map[string]any{
		"title":           "Test Route",
		"operationRoutes": []string{"00000000-0000-0000-0000-000000000000"},
	})
	// May return 400 or 404 depending on implementation
	if err != nil || (code != 400 && code != 404) {
		t.Errorf("expected 400 or 404 for non-existent operation route, got code=%d body=%s", code, string(body))
	}

	// Test 5: GET non-existent transaction route returns 404
	badPath := fmt.Sprintf("%s/%s", txRoutesPath, "00000000-0000-0000-0000-000000000000")
	code, body, err = trans.Request(ctx, "GET", badPath, headers, nil)
	if err != nil || code != 404 {
		t.Errorf("expected 404 for non-existent route, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_TransactionRoutes_Validation ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_TransactionRoutes_Validation`

**Failure Recovery:**
1. **Empty operationRoutes accepted:** May be valid - check business rules
2. **Different error responses:** Document actual API behavior

---

### Task 3.4: Code Review Checkpoint - Transaction Routes

**Estimated time:** 3 minutes
**Agent:** code-reviewer

**Description:** Run code review on transaction routes tests.

1. **Dispatch all 3 reviewers in parallel**
2. **Handle findings by severity**
3. **Proceed only when zero Critical/High/Medium issues remain**

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_TransactionRoutes ./tests/integration/... -count=1 -timeout 120s
```
Expected output: All transaction routes tests pass

---

## 4. Operations Management Tests (Medium Priority)

Operations are the individual movements within transactions. These tests cover viewing and updating operation metadata.

### Task 4.1: Create Operations Management Test File

**File:** Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operations_management_test.go`
**Estimated time:** 3 minutes
**Agent:** qa-analyst

**Description:** Create test file with response types for operations.

**Code:**
```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// operationResponse represents the API response for an operation
type operationResponse struct {
	ID              string          `json:"id"`
	TransactionID   string          `json:"transactionId"`
	Description     string          `json:"description"`
	Type            string          `json:"type"`
	AssetCode       string          `json:"assetCode"`
	AccountID       string          `json:"accountId"`
	AccountAlias    string          `json:"accountAlias"`
	BalanceID       string          `json:"balanceId"`
	OrganizationID  string          `json:"organizationId"`
	LedgerID        string          `json:"ledgerId"`
	Amount          operationAmount `json:"amount"`
	Balance         operationBal    `json:"balance"`
	BalanceAfter    operationBal    `json:"balanceAfter"`
	Metadata        map[string]any  `json:"metadata"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// operationAmount represents amount in operation
type operationAmount struct {
	Value *decimal.Decimal `json:"value"`
}

// operationBal represents balance snapshot in operation
type operationBal struct {
	Available *decimal.Decimal `json:"available"`
	OnHold    *decimal.Decimal `json:"onHold"`
	Version   *int64           `json:"version"`
}

// operationsListResponse represents paginated operations
type operationsListResponse struct {
	Items []operationResponse `json:"items"`
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/integration/...
```
Expected output: No errors

**Failure Recovery:**
1. **Decimal import issues:** Ensure shopspring/decimal is in go.mod
2. **Type conflicts:** Check for duplicate type definitions

---

### Task 4.2: Implement Operations Get by Account Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operations_management_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add test for GET /v1/organizations/{org}/ledgers/{ledger}/accounts/{account}/operations endpoint.

**Code:** (append to file)
```go

// TestIntegration_Operations_GetByAccount tests listing operations for an account
func TestIntegration_Operations_GetByAccount(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger, asset, account
	orgName := fmt.Sprintf("Ops Org %s", h.RandString(5))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	alias := fmt.Sprintf("ops-acct-%s", h.RandString(5))
	accPayload := map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias}
	accountsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", accountsPath, headers, accPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &acct)

	// Create transaction to generate operations
	inflowPayload := map[string]any{
		"code":        fmt.Sprintf("TX-%s", h.RandString(6)),
		"description": "test inflow",
		"send": map[string]any{
			"asset": "USD",
			"value": "100.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "100.00"},
					"description":  "credit test",
				}},
			},
		},
	}

	inflowPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "POST", inflowPath, headers, inflowPayload)
	if err != nil || code != 201 {
		t.Fatalf("create inflow: code=%d err=%v body=%s", code, err, string(body))
	}

	// Wait briefly for async processing
	time.Sleep(500 * time.Millisecond)

	// GET operations by account
	opsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations", org.ID, ledger.ID, acct.ID)
	code, body, err = trans.Request(ctx, "GET", opsPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get operations: code=%d err=%v body=%s", code, err, string(body))
	}

	var ops operationsListResponse
	if err := json.Unmarshal(body, &ops); err != nil {
		t.Fatalf("parse operations: %v body=%s", err, string(body))
	}

	if len(ops.Items) < 1 {
		t.Error("expected at least 1 operation for account")
	}

	// Verify operation belongs to correct account
	for _, op := range ops.Items {
		if op.AccountID != acct.ID {
			t.Errorf("operation accountID mismatch: want=%s got=%s", acct.ID, op.AccountID)
		}
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Operations_GetByAccount ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_Operations_GetByAccount`

**Failure Recovery:**
1. **Empty operations list:** Increase wait time for async processing
2. **Transaction failed:** Check inflow payload format
3. **Can't recover:** Check if operations are being created

---

### Task 4.3: Implement Operation Get Single and Update Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operations_management_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add tests for GET single operation and PATCH operation metadata.

**Code:** (append to file)
```go

// TestIntegration_Operations_GetSingleAndUpdate tests GET and PATCH for single operation
func TestIntegration_Operations_GetSingleAndUpdate(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger, asset, account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("OpsSingle Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	alias := fmt.Sprintf("single-ops-%s", h.RandString(5))
	accountsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", accountsPath, headers, map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &acct)

	// Create transaction
	inflowPayload := map[string]any{
		"code":        fmt.Sprintf("TX-%s", h.RandString(6)),
		"description": "single op test",
		"send": map[string]any{
			"asset": "USD",
			"value": "50.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "50.00"},
				}},
			},
		},
	}

	inflowPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "POST", inflowPath, headers, inflowPayload)
	if err != nil || code != 201 {
		t.Fatalf("create inflow: code=%d err=%v body=%s", code, err, string(body))
	}
	var txn struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &txn)

	time.Sleep(500 * time.Millisecond)

	// Get operations for account to find operation ID
	opsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations", org.ID, ledger.ID, acct.ID)
	code, body, err = trans.Request(ctx, "GET", opsPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list operations: code=%d err=%v body=%s", code, err, string(body))
	}

	var ops operationsListResponse
	_ = json.Unmarshal(body, &ops)
	if len(ops.Items) == 0 {
		t.Fatal("no operations found")
	}

	opID := ops.Items[0].ID
	txID := ops.Items[0].TransactionID

	// GET single operation
	singleOpPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations/%s", org.ID, ledger.ID, acct.ID, opID)
	code, body, err = trans.Request(ctx, "GET", singleOpPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get single operation: code=%d err=%v body=%s", code, err, string(body))
	}

	var singleOp operationResponse
	if err := json.Unmarshal(body, &singleOp); err != nil {
		t.Fatalf("parse single op: %v body=%s", err, string(body))
	}

	if singleOp.ID != opID {
		t.Errorf("operation ID mismatch: want=%s got=%s", opID, singleOp.ID)
	}

	// PATCH operation metadata
	updatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/operations/%s", org.ID, ledger.ID, txID, opID)
	updatePayload := map[string]any{
		"description": "Updated description",
		"metadata": map[string]any{
			"reviewed":   true,
			"reviewedBy": "test-user",
		},
	}

	code, body, err = trans.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update operation: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedOp operationResponse
	if err := json.Unmarshal(body, &updatedOp); err != nil {
		t.Fatalf("parse updated op: %v body=%s", err, string(body))
	}

	if updatedOp.Description != "Updated description" {
		t.Errorf("expected updated description, got %s", updatedOp.Description)
	}

	// Verify metadata was updated
	if updatedOp.Metadata == nil {
		t.Error("expected metadata to be set")
	} else if updatedOp.Metadata["reviewed"] != true {
		t.Error("expected metadata.reviewed=true")
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Operations_GetSingleAndUpdate ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_Operations_GetSingleAndUpdate`

**Failure Recovery:**
1. **PATCH path incorrect:** Verify operation update uses transaction path not account path
2. **Metadata not persisting:** Check if metadata field is properly handled
3. **Can't recover:** Document actual PATCH endpoint behavior

---

### Task 4.4: Code Review Checkpoint - Operations Management

**Estimated time:** 3 minutes
**Agent:** code-reviewer

**Description:** Run code review on operations management tests.

1. **Dispatch all 3 reviewers in parallel**
2. **Handle findings by severity**
3. **Proceed only when zero Critical/High/Medium issues remain**

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Operations ./tests/integration/... -count=1 -timeout 120s
```
Expected output: All operations tests pass

---

## 5. Balance Mutations Tests (Medium Priority)

Balance mutation endpoints allow direct manipulation of balance states.

### Task 5.1: Create Balance Mutations Test File

**File:** Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/balance_mutations_test.go`
**Estimated time:** 3 minutes
**Agent:** qa-analyst

**Description:** Create test file for balance mutation tests.

**Code:**
```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// balanceResponse represents the API response for a balance
type balanceResponse struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	LedgerID       string          `json:"ledgerId"`
	AccountID      string          `json:"accountId"`
	Alias          string          `json:"alias"`
	Key            string          `json:"key"`
	AssetCode      string          `json:"assetCode"`
	Available      decimal.Decimal `json:"available"`
	OnHold         decimal.Decimal `json:"onHold"`
	Version        int64           `json:"version"`
	AccountType    string          `json:"accountType"`
	AllowSending   bool            `json:"allowSending"`
	AllowReceiving bool            `json:"allowReceiving"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	Metadata       map[string]any  `json:"metadata"`
}

// balancesListResponse represents paginated balances
type balancesListResponse struct {
	Items []balanceResponse `json:"items"`
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./tests/integration/...
```
Expected output: No errors

**Failure Recovery:**
1. **Decimal import issues:** Verify shopspring/decimal in go.mod
2. **Type conflicts:** Rename if conflicts with other test files

---

### Task 5.2: Implement Balance Update Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/balance_mutations_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add test for PATCH /v1/organizations/{org}/ledgers/{ledger}/balances/{balance_id} endpoint.

**Code:** (append to file)
```go

// TestIntegration_Balance_Update tests PATCH balance permissions
func TestIntegration_Balance_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger, asset, account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("BalUpd Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	alias := fmt.Sprintf("bal-upd-%s", h.RandString(5))
	accountsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", accountsPath, headers, map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &acct)

	// Get balances for account to find balance ID
	balancesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", org.ID, ledger.ID, acct.ID)
	code, body, err = trans.Request(ctx, "GET", balancesPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list balances: code=%d err=%v body=%s", code, err, string(body))
	}

	var balances balancesListResponse
	_ = json.Unmarshal(body, &balances)
	if len(balances.Items) == 0 {
		t.Fatal("no balances found")
	}

	balanceID := balances.Items[0].ID
	originalAllowSending := balances.Items[0].AllowSending
	originalAllowReceiving := balances.Items[0].AllowReceiving

	// PATCH: Update balance permissions
	updatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", org.ID, ledger.ID, balanceID)
	updatePayload := map[string]any{
		"allowSending":   !originalAllowSending,
		"allowReceiving": !originalAllowReceiving,
	}

	code, body, err = trans.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update balance: code=%d err=%v body=%s", code, err, string(body))
	}

	var updated balanceResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("parse updated: %v body=%s", err, string(body))
	}

	if updated.AllowSending == originalAllowSending {
		t.Error("expected allowSending to be toggled")
	}
	if updated.AllowReceiving == originalAllowReceiving {
		t.Error("expected allowReceiving to be toggled")
	}

	// Verify: GET should reflect update
	code, body, err = trans.Request(ctx, "GET", updatePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated balance: code=%d err=%v body=%s", code, err, string(body))
	}

	var verified balanceResponse
	_ = json.Unmarshal(body, &verified)
	if verified.AllowSending != updated.AllowSending {
		t.Error("update not persisted for allowSending")
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Balance_Update ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_Balance_Update`

**Failure Recovery:**
1. **No balances found:** Account may not auto-create default balance - check implementation
2. **PATCH fails:** Verify update payload matches UpdateBalance struct
3. **Can't recover:** Check if balance permissions are mutable

---

### Task 5.3: Implement Additional Balance Creation Test

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/balance_mutations_test.go`
**Estimated time:** 4 minutes
**Agent:** qa-analyst

**Description:** Add test for POST /v1/organizations/{org}/ledgers/{ledger}/accounts/{account}/balances endpoint.

**Code:** (append to file)
```go

// TestIntegration_Balance_CreateAdditional tests POST additional balance
func TestIntegration_Balance_CreateAdditional(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger, asset, account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("AddBal Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	alias := fmt.Sprintf("addbal-%s", h.RandString(5))
	accountsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", accountsPath, headers, map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &acct)

	// POST: Create additional balance with custom key
	createPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", org.ID, ledger.ID, acct.ID)
	balanceKey := fmt.Sprintf("escrow-%s", h.RandString(4))
	createPayload := map[string]any{
		"key":            balanceKey,
		"allowSending":   true,
		"allowReceiving": true,
	}

	code, body, err = trans.Request(ctx, "POST", createPath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create additional balance: code=%d err=%v body=%s", code, err, string(body))
	}

	var created balanceResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse created: %v body=%s", err, string(body))
	}

	if created.Key != balanceKey {
		t.Errorf("expected Key=%s, got %s", balanceKey, created.Key)
	}
	if !created.AllowSending {
		t.Error("expected allowSending=true")
	}
	if !created.AllowReceiving {
		t.Error("expected allowReceiving=true")
	}

	// Verify: List balances should now have 2 (default + additional)
	code, body, err = trans.Request(ctx, "GET", createPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list balances: code=%d err=%v body=%s", code, err, string(body))
	}

	var balances balancesListResponse
	_ = json.Unmarshal(body, &balances)
	if len(balances.Items) < 2 {
		t.Errorf("expected at least 2 balances, got %d", len(balances.Items))
	}

	// Test: Duplicate key should fail
	code, body, err = trans.Request(ctx, "POST", createPath, headers, createPayload)
	if err != nil || code != 409 {
		t.Errorf("expected 409 for duplicate key, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Balance_CreateAdditional ./tests/integration/... -count=1 -timeout 60s
```
Expected output: `--- PASS: TestIntegration_Balance_CreateAdditional`

**Failure Recovery:**
1. **No default balance:** Account might need transaction to create balance
2. **409 not returned for duplicate:** May return 400 - adjust assertion
3. **Can't recover:** Document actual additional balance behavior

---

### Task 5.4: Implement Balance Delete and External Code Tests

**File:** Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/balance_mutations_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Add tests for DELETE balance and GET by external code.

**Code:** (append to file)
```go

// TestIntegration_Balance_Delete tests DELETE balance endpoint
func TestIntegration_Balance_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("DelBal Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	alias := fmt.Sprintf("delbal-%s", h.RandString(5))
	accountsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", accountsPath, headers, map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &acct)

	// Create additional balance to delete
	balancesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", org.ID, ledger.ID, acct.ID)
	balanceKey := fmt.Sprintf("todelete-%s", h.RandString(4))
	code, body, err = trans.Request(ctx, "POST", balancesPath, headers, map[string]any{"key": balanceKey})
	if err != nil || code != 201 {
		t.Fatalf("create balance: code=%d err=%v body=%s", code, err, string(body))
	}
	var created balanceResponse
	_ = json.Unmarshal(body, &created)

	// DELETE balance
	deletePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", org.ID, ledger.ID, created.ID)
	code, body, err = trans.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete balance: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion
	code, body, err = trans.Request(ctx, "GET", deletePath, headers, nil)
	if err != nil || code != 404 {
		t.Fatalf("expected 404 after delete, got code=%d body=%s", code, string(body))
	}
}

// TestIntegration_Balance_GetByExternalCode tests GET balances by external code
func TestIntegration_Balance_GetByExternalCode(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("ExtCode Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD: %v", err)
	}

	// Create account with external code
	externalCode := fmt.Sprintf("EXT-%s", h.RandString(6))
	alias := fmt.Sprintf("extcode-%s", h.RandString(5))
	accountsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	accPayload := map[string]any{
		"name":         alias,
		"assetCode":    "USD",
		"type":         "deposit",
		"alias":        alias,
		"externalCode": externalCode,
	}
	code, body, err = onboard.Request(ctx, "POST", accountsPath, headers, accPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}

	// GET balances by external code
	extPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/external/%s/balances", org.ID, ledger.ID, externalCode)
	code, body, err = trans.Request(ctx, "GET", extPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get by external code: code=%d err=%v body=%s", code, err, string(body))
	}

	var balances balancesListResponse
	if err := json.Unmarshal(body, &balances); err != nil {
		t.Fatalf("parse balances: %v body=%s", err, string(body))
	}

	// Should have default balance
	if len(balances.Items) < 1 {
		t.Error("expected at least 1 balance for external code lookup")
	}

	// Test: Non-existent external code returns 200 with empty items
	badPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/external/%s/balances", org.ID, ledger.ID, "NONEXISTENT-CODE")
	code, body, err = trans.Request(ctx, "GET", badPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("expected 200 for non-existent code, got code=%d body=%s", code, string(body))
	}

	var emptyList balancesListResponse
	_ = json.Unmarshal(body, &emptyList)
	if len(emptyList.Items) != 0 {
		t.Errorf("expected empty list for non-existent code, got %d items", len(emptyList.Items))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Balance ./tests/integration/... -count=1 -timeout 120s
```
Expected output: All balance tests pass

**Failure Recovery:**
1. **Delete returns 200:** Adjust assertion if API doesn't return 204
2. **External code not supported:** Check if account model has externalCode field
3. **Can't recover:** Document actual balance mutation behavior

---

### Task 5.5: Code Review Checkpoint - Balance Mutations

**Estimated time:** 3 minutes
**Agent:** code-reviewer

**Description:** Run code review on balance mutation tests.

1. **Dispatch all 3 reviewers in parallel**
2. **Handle findings by severity**
3. **Proceed only when zero Critical/High/Medium issues remain**

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_Balance ./tests/integration/... -count=1 -timeout 180s
```
Expected output: All balance tests pass

---

## 6. Account Types Tests (Lower Priority)

Account types define behavior categories for accounts.

### Task 6.1: Create Account Types Test File

**File:** Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/account_types_test.go`
**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Create comprehensive test for account types CRUD operations.

**Code:**
```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// accountTypeResponse represents the API response for an account type
type accountTypeResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// accountTypesListResponse represents paginated account types
type accountTypesListResponse struct {
	Items []accountTypeResponse `json:"items"`
}

// TestIntegration_AccountTypes_CRUD tests full lifecycle of account types
func TestIntegration_AccountTypes_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: org, ledger
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("AccType Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	typesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", org.ID, ledger.ID)

	// CREATE: Post new account type
	typeName := fmt.Sprintf("savings-%s", h.RandString(4))
	createPayload := map[string]any{
		"name":        typeName,
		"description": "Savings account type for long-term deposits",
		"metadata": map[string]any{
			"interestRate": "3.5%",
		},
	}

	code, body, err = onboard.Request(ctx, "POST", typesPath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account type: code=%d err=%v body=%s", code, err, string(body))
	}

	var created accountTypeResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse created: %v body=%s", err, string(body))
	}

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Name != typeName {
		t.Errorf("expected Name=%s, got %s", typeName, created.Name)
	}

	// READ: List account types
	code, body, err = onboard.Request(ctx, "GET", typesPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list account types: code=%d err=%v body=%s", code, err, string(body))
	}

	var list accountTypesListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("parse list: %v body=%s", err, string(body))
	}

	found := false
	for _, item := range list.Items {
		if item.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created account type not found in list")
	}

	// UPDATE: Patch account type
	updatePath := fmt.Sprintf("%s/%s", typesPath, created.ID)
	updatePayload := map[string]any{
		"description": "Updated savings description",
	}

	code, body, err = onboard.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update account type: code=%d err=%v body=%s", code, err, string(body))
	}

	var updated accountTypeResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("parse updated: %v body=%s", err, string(body))
	}

	if updated.Description != "Updated savings description" {
		t.Errorf("expected updated description, got %s", updated.Description)
	}

	// DELETE: Remove account type
	code, body, err = onboard.Request(ctx, "DELETE", updatePath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete account type: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion: should return 404
	code, body, err = onboard.Request(ctx, "GET", updatePath, headers, nil)
	if err != nil || code != 404 {
		t.Fatalf("expected 404 after delete, got code=%d body=%s", code, string(body))
	}
}

// TestIntegration_AccountTypes_Validation tests input validation
func TestIntegration_AccountTypes_Validation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("TypeVal Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	ledgerPath := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", ledgerPath, headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	typesPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", org.ID, ledger.ID)

	// Test 1: Missing required name
	code, body, err = onboard.Request(ctx, "POST", typesPath, headers, map[string]any{
		"description": "No name provided",
	})
	if err != nil || code != 400 {
		t.Errorf("expected 400 for missing name, got code=%d body=%s", code, string(body))
	}

	// Test 2: Duplicate name should fail
	typeName := fmt.Sprintf("unique-%s", h.RandString(4))
	code, body, err = onboard.Request(ctx, "POST", typesPath, headers, map[string]any{"name": typeName})
	if err != nil || code != 201 {
		t.Fatalf("first create: code=%d err=%v body=%s", code, err, string(body))
	}

	code, body, err = onboard.Request(ctx, "POST", typesPath, headers, map[string]any{"name": typeName})
	if err != nil || code != 409 {
		t.Errorf("expected 409 for duplicate name, got code=%d body=%s", code, string(body))
	}

	// Test 3: GET non-existent type returns 404
	badPath := fmt.Sprintf("%s/%s", typesPath, "00000000-0000-0000-0000-000000000000")
	code, body, err = onboard.Request(ctx, "GET", badPath, headers, nil)
	if err != nil || code != 404 {
		t.Errorf("expected 404 for non-existent type, got code=%d body=%s", code, string(body))
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_AccountTypes ./tests/integration/... -count=1 -timeout 60s
```
Expected output: All account types tests pass

**Failure Recovery:**
1. **404 on endpoints:** Account types may be in onboarding service - verify routes
2. **Duplicate name accepted:** May not enforce uniqueness - adjust test
3. **Can't recover:** Document actual account types API behavior

---

### Task 6.2: Code Review Checkpoint - Account Types

**Estimated time:** 3 minutes
**Agent:** code-reviewer

**Description:** Final code review for account types tests.

1. **Dispatch all 3 reviewers in parallel**
2. **Handle findings by severity**
3. **Proceed only when zero Critical/High/Medium issues remain**

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run TestIntegration_AccountTypes ./tests/integration/... -count=1 -timeout 60s
```
Expected output: All account types tests pass

---

## Final Verification

### Task Final: Run Complete Integration Test Suite

**Estimated time:** 5 minutes
**Agent:** qa-analyst

**Description:** Run all new integration tests to verify complete coverage.

**Verification:**
```bash
# Run all new tests
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./tests/integration/... -count=1 -timeout 600s 2>&1 | tee /tmp/integration-test-results.txt

# Check for failures
grep -E "(PASS|FAIL)" /tmp/integration-test-results.txt | tail -20
```

Expected output: All tests pass with no failures

**Failure Recovery:**
1. **Timeout:** Increase timeout or run tests in smaller batches
2. **Flaky tests:** Add retry logic or increase wait times for async operations
3. **Service unavailable:** Restart Midaz stack with `make restart` or `docker compose restart`

---

## Plan Summary

| Task | File | Description | Priority |
|------|------|-------------|----------|
| 1.1-1.4 | asset_rates_test.go | Asset rates CRUD + validation | High |
| 2.1-2.3 | operation_routes_test.go | Operation routes CRUD + validation | High |
| 3.1-3.3 | transaction_routes_test.go | Transaction routes CRUD + validation | High |
| 4.1-4.3 | operations_management_test.go | Operations get/update | Medium |
| 5.1-5.4 | balance_mutations_test.go | Balance CRUD + external code | Medium |
| 6.1-6.2 | account_types_test.go | Account types CRUD + validation | Lower |

**Total new test files:** 6
**Total test functions:** 14
**Estimated total time:** 60-90 minutes

---

## Checklist

- [ ] Historical precedent queried (none available - new project)
- [x] Header with goal, architecture, tech stack, prerequisites
- [x] Verification commands with expected output
- [x] Tasks broken into bite-sized steps (2-5 min each)
- [x] Exact file paths for all files
- [x] Complete code (no placeholders)
- [x] Exact commands with expected output
- [x] Failure recovery steps for each task
- [x] Code review checkpoints after batches
- [x] Severity-based issue handling documented
- [x] Passes Zero-Context Test
