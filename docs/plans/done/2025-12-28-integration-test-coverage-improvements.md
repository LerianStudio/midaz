# Integration Test Coverage Improvements Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Achieve comprehensive integration test coverage for critical untested API endpoints in Midaz, focusing on CRM, Routing, and PATCH/DELETE operations.

**Architecture:** Integration tests follow a consistent pattern: create isolated test data using helper functions, execute HTTP requests against running services, and verify responses. Tests run in parallel with unique identifiers to avoid conflicts. Each test creates its own organization/ledger to ensure isolation.

**Tech Stack:** Go 1.21+, standard testing package, existing test helpers (`tests/helpers/`), HTTP client with retry support

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Services: Midaz stack running locally (onboarding:3000, transaction:3001, CRM:3002)
- Tools: Docker Compose for local stack
- State: Clean working tree on current branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version              # Expected: go version go1.21+ or higher
curl -s http://localhost:3000/health | jq .status  # Expected: "ok"
curl -s http://localhost:3001/health | jq .status  # Expected: "ok"
curl -s http://localhost:3002/health | jq .status  # Expected: "ok"
git status              # Expected: clean working tree
```

## Historical Precedent

**Query:** "integration test coverage CRM routing"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Phase 1: Infrastructure Updates

### Task 1.1: Add CRM URL to Environment Configuration

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/helpers/env.go`

**Prerequisites:**
- Go 1.21+ installed
- Editor access to codebase

**Step 1: Add CRM URL field to Environment struct**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/tests/helpers/env.go` to add `CRMURL` field:

```go
// Environment holds base URLs for Midaz services and behavior flags.
type Environment struct {
	OnboardingURL  string
	TransactionURL string
	CRMURL         string
	ManageStack    bool // if true, tests may start/stop stack via Makefile
	HTTPTimeout    time.Duration
}

// LoadEnvironment loads environment configuration with sensible defaults
// matching the local docker-compose setup.
func LoadEnvironment() Environment {
	onboarding := getenv("ONBOARDING_URL", "http://localhost:3000")
	transaction := getenv("TRANSACTION_URL", "http://localhost:3001")
	crm := getenv("CRM_URL", "http://localhost:3002")
	manage := getenv("MIDAZ_TEST_MANAGE_STACK", "false") == "true"
	timeoutStr := getenv("MIDAZ_TEST_HTTP_TIMEOUT", "20s")

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = envDefaultHTTPTimeout
	}

	return Environment{
		OnboardingURL:  onboarding,
		TransactionURL: transaction,
		CRMURL:         crm,
		ManageStack:    manage,
		HTTPTimeout:    timeout,
	}
}
```

**Step 2: Verify compilation**

Run: `go build ./tests/helpers/...`

**Expected output:**
```
(no output - successful compilation)
```

**If Task Fails:**
1. **Syntax error:** Check for missing commas or field alignment
2. **Import error:** Ensure all imports are present
3. **Rollback:** `git checkout -- tests/helpers/env.go`

---

### Task 1.2: Create CRM Test Helper Functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/helpers/crm.go`

**Prerequisites:**
- Task 1.1 completed
- Go 1.21+ installed

**Step 1: Create CRM helper file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/helpers/crm.go`:

```go
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	holderTypeNaturalPerson = "NATURAL_PERSON"
	holderTypeLegalPerson   = "LEGAL_PERSON"
)

// HolderResponse represents a holder API response.
type HolderResponse struct {
	ID         string         `json:"id"`
	ExternalID *string        `json:"externalId,omitempty"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Document   string         `json:"document"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// AliasResponse represents an alias API response.
type AliasResponse struct {
	ID        string         `json:"id"`
	HolderID  string         `json:"holderId"`
	LedgerID  string         `json:"ledgerId"`
	AccountID string         `json:"accountId"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// CreateHolderPayload returns a valid holder creation payload.
func CreateHolderPayload(name, document string, holderType string) map[string]any {
	return map[string]any{
		"type":     holderType,
		"name":     name,
		"document": document,
	}
}

// CreateNaturalPersonPayload returns a natural person holder payload.
func CreateNaturalPersonPayload(name, cpf string) map[string]any {
	return CreateHolderPayload(name, cpf, holderTypeNaturalPerson)
}

// CreateLegalPersonPayload returns a legal person holder payload.
func CreateLegalPersonPayload(name, cnpj string) map[string]any {
	return CreateHolderPayload(name, cnpj, holderTypeLegalPerson)
}

// SetupHolder creates a holder and returns its ID.
func SetupHolder(ctx context.Context, crm *HTTPClient, headers map[string]string, name, document, holderType string) (string, error) {
	payload := CreateHolderPayload(name, document, holderType)

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create holder failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var holder HolderResponse
	if err := json.Unmarshal(body, &holder); err != nil || holder.ID == "" {
		return "", fmt.Errorf("parse holder: %w body=%s", err, string(body))
	}

	return holder.ID, nil
}

// CreateAliasPayload returns a valid alias creation payload.
func CreateAliasPayload(ledgerID, accountID string) map[string]any {
	return map[string]any{
		"ledgerId":  ledgerID,
		"accountId": accountID,
	}
}

// SetupAlias creates an alias for a holder and returns its ID.
func SetupAlias(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID, ledgerID, accountID string) (string, error) {
	payload := CreateAliasPayload(ledgerID, accountID)

	path := fmt.Sprintf("/v1/holders/%s/aliases", holderID)
	code, body, err := crm.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create alias failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var alias AliasResponse
	if err := json.Unmarshal(body, &alias); err != nil || alias.ID == "" {
		return "", fmt.Errorf("parse alias: %w body=%s", err, string(body))
	}

	return alias.ID, nil
}

// GenerateValidCPF returns a valid Brazilian CPF for testing.
// Uses a known valid CPF format (calculation-valid).
func GenerateValidCPF() string {
	// Base CPF digits (first 9 digits) with random variation
	base := fmt.Sprintf("%09d", randIntN(999999999))

	// Calculate first verification digit
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(base[i]-'0') * (10 - i)
	}
	d1 := 11 - (sum % 11)
	if d1 >= 10 {
		d1 = 0
	}

	// Calculate second verification digit
	sum = 0
	for i := 0; i < 9; i++ {
		sum += int(base[i]-'0') * (11 - i)
	}
	sum += d1 * 2
	d2 := 11 - (sum % 11)
	if d2 >= 10 {
		d2 = 0
	}

	return fmt.Sprintf("%s%d%d", base, d1, d2)
}

// GenerateValidCNPJ returns a valid Brazilian CNPJ for testing.
// Uses a known valid CNPJ format (calculation-valid).
func GenerateValidCNPJ() string {
	// Base CNPJ digits (first 12 digits) with random variation
	base := fmt.Sprintf("%08d0001", randIntN(99999999))

	// Weights for first digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(base[i]-'0') * weights1[i]
	}
	d1 := 11 - (sum % 11)
	if d1 >= 10 {
		d1 = 0
	}

	// Weights for second digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 12; i++ {
		sum += int(base[i]-'0') * weights2[i]
	}
	sum += d1 * weights2[12]
	d2 := 11 - (sum % 11)
	if d2 >= 10 {
		d2 = 0
	}

	return fmt.Sprintf("%s%d%d", base, d1, d2)
}

// randIntN generates a random int in [0, n) using crypto/rand.
func randIntN(n int) int {
	if n <= 0 {
		return 0
	}
	// Use RandString to get random bytes and convert to int
	hex := RandHex(4)
	var val int
	fmt.Sscanf(hex, "%x", &val)
	return val % n
}
```

**Step 2: Verify compilation**

Run: `go build ./tests/helpers/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run existing helper tests to ensure no regression**

Run: `go test ./tests/helpers/... -v -count=1`

**Expected output:**
```
=== RUN   TestRandString
--- PASS: TestRandString
...
PASS
```

**If Task Fails:**
1. **Import error:** Verify all imports are correct
2. **Compilation error:** Check struct field types match
3. **Rollback:** `rm tests/helpers/crm.go`

---

### Task 1.3: Create Routing Test Helper Functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/helpers/routing.go`

**Prerequisites:**
- Task 1.1 completed
- Go 1.21+ installed

**Step 1: Create routing helper file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/helpers/routing.go`:

```go
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
)

// OperationRouteResponse represents an operation route API response.
type OperationRouteResponse struct {
	ID            string         `json:"id"`
	OrganizationID string        `json:"organizationId"`
	LedgerID      string         `json:"ledgerId"`
	Title         string         `json:"title"`
	Description   string         `json:"description,omitempty"`
	Code          string         `json:"code,omitempty"`
	OperationType string         `json:"operationType"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// TransactionRouteResponse represents a transaction route API response.
type TransactionRouteResponse struct {
	ID             string                   `json:"id"`
	OrganizationID string                   `json:"organizationId"`
	LedgerID       string                   `json:"ledgerId"`
	Title          string                   `json:"title"`
	Description    string                   `json:"description,omitempty"`
	Metadata       map[string]any           `json:"metadata,omitempty"`
	OperationRoutes []OperationRouteResponse `json:"operationRoutes,omitempty"`
}

// CreateOperationRoutePayload returns a valid operation route creation payload.
func CreateOperationRoutePayload(title, operationType string) map[string]any {
	return map[string]any{
		"title":         title,
		"operationType": operationType,
	}
}

// CreateOperationRoutePayloadFull returns a complete operation route creation payload.
func CreateOperationRoutePayloadFull(title, description, code, operationType string, metadata map[string]any) map[string]any {
	payload := map[string]any{
		"title":         title,
		"operationType": operationType,
	}
	if description != "" {
		payload["description"] = description
	}
	if code != "" {
		payload["code"] = code
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}
	return payload
}

// SetupOperationRoute creates an operation route and returns its ID.
func SetupOperationRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, title, operationType string) (string, error) {
	payload := CreateOperationRoutePayload(title, operationType)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create operation route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route OperationRouteResponse
	if err := json.Unmarshal(body, &route); err != nil || route.ID == "" {
		return "", fmt.Errorf("parse operation route: %w body=%s", err, string(body))
	}

	return route.ID, nil
}

// CreateTransactionRoutePayload returns a valid transaction route creation payload.
func CreateTransactionRoutePayload(title string, operationRouteIDs []string) map[string]any {
	return map[string]any{
		"title":           title,
		"operationRoutes": operationRouteIDs,
	}
}

// SetupTransactionRoute creates a transaction route and returns its ID.
func SetupTransactionRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, title string, operationRouteIDs []string) (string, error) {
	payload := CreateTransactionRoutePayload(title, operationRouteIDs)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create transaction route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route TransactionRouteResponse
	if err := json.Unmarshal(body, &route); err != nil || route.ID == "" {
		return "", fmt.Errorf("parse transaction route: %w body=%s", err, string(body))
	}

	return route.ID, nil
}

// AccountTypeResponse represents an account type API response.
type AccountTypeResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	KeyValue       string         `json:"keyValue"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// CreateAccountTypePayload returns a valid account type creation payload.
func CreateAccountTypePayload(name, keyValue string) map[string]any {
	return map[string]any{
		"name":     name,
		"keyValue": keyValue,
	}
}

// SetupAccountType creates an account type and returns its ID.
func SetupAccountType(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, name, keyValue string) (string, error) {
	payload := CreateAccountTypePayload(name, keyValue)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create account type failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var accountType AccountTypeResponse
	if err := json.Unmarshal(body, &accountType); err != nil || accountType.ID == "" {
		return "", fmt.Errorf("parse account type: %w body=%s", err, string(body))
	}

	return accountType.ID, nil
}

// AssetRateResponse represents an asset rate API response.
type AssetRateResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	ExternalID     string         `json:"externalId"`
	From           string         `json:"from"`
	To             string         `json:"to"`
	Rate           float64        `json:"rate"`
	Scale          *float64       `json:"scale,omitempty"`
	Source         *string        `json:"source,omitempty"`
	TTL            int            `json:"ttl"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// CreateAssetRatePayload returns a valid asset rate creation payload.
func CreateAssetRatePayload(from, to string, rate int, externalID string) map[string]any {
	return map[string]any{
		"from":       from,
		"to":         to,
		"rate":       rate,
		"externalId": externalID,
	}
}

// SetupAssetRate creates or updates an asset rate and returns the response.
func SetupAssetRate(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, from, to string, rate int, externalID string) (*AssetRateResponse, error) {
	payload := CreateAssetRatePayload(from, to, rate, externalID)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "PUT", path, headers, payload)
	if err != nil || (code != 200 && code != 201) {
		return nil, fmt.Errorf("create asset rate failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var assetRate AssetRateResponse
	if err := json.Unmarshal(body, &assetRate); err != nil {
		return nil, fmt.Errorf("parse asset rate: %w body=%s", err, string(body))
	}

	return &assetRate, nil
}
```

**Step 2: Verify compilation**

Run: `go build ./tests/helpers/...`

**Expected output:**
```
(no output - successful compilation)
```

**If Task Fails:**
1. **Import error:** Verify all imports are correct
2. **Compilation error:** Check struct field types match
3. **Rollback:** `rm tests/helpers/routing.go`

---

## Phase 2: CRM Integration Tests

### Task 2.1: Create Holder CRUD Lifecycle Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/holder_crud_test.go`

**Prerequisites:**
- Tasks 1.1, 1.2 completed
- CRM service running on localhost:3002

**Step 1: Create holder CRUD test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/holder_crud_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Holder_CRUD tests the complete holder lifecycle:
// CREATE -> GET -> LIST -> UPDATE -> GET (verify update) -> DELETE -> GET (verify 404)
func TestIntegration_Holder_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Generate valid CPF for natural person
	cpf := h.GenerateValidCPF()
	holderName := fmt.Sprintf("Test Holder %s", h.RandString(6))

	// 1) CREATE holder
	createPayload := h.CreateNaturalPersonPayload(holderName, cpf)
	createPayload["metadata"] = map[string]any{"env": "integration-test"}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var holder h.HolderResponse
	if err := json.Unmarshal(body, &holder); err != nil || holder.ID == "" {
		t.Fatalf("parse holder response: %v body=%s", err, string(body))
	}

	t.Logf("Created holder: id=%s name=%s", holder.ID, holder.Name)

	// 2) GET holder by ID
	code, body, err = crm.Request(ctx, "GET", fmt.Sprintf("/v1/holders/%s", holder.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var holderGet h.HolderResponse
	if err := json.Unmarshal(body, &holderGet); err != nil {
		t.Fatalf("parse get holder response: %v body=%s", err, string(body))
	}
	if holderGet.ID != holder.ID {
		t.Fatalf("holder ID mismatch: want %s got %s", holder.ID, holderGet.ID)
	}
	if holderGet.Name != holderName {
		t.Fatalf("holder name mismatch: want %s got %s", holderName, holderGet.Name)
	}

	// 3) LIST all holders (verify created holder is in list)
	code, body, err = crm.Request(ctx, "GET", "/v1/holders", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list holders failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var holderList struct {
		Items []h.HolderResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &holderList); err != nil {
		t.Fatalf("parse holder list: %v body=%s", err, string(body))
	}

	found := false
	for _, h := range holderList.Items {
		if h.ID == holder.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created holder not found in list")
	}

	// 4) UPDATE holder
	updatedName := fmt.Sprintf("Updated Holder %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"env":     "integration-test",
			"updated": true,
		},
	}

	code, body, err = crm.Request(ctx, "PATCH", fmt.Sprintf("/v1/holders/%s", holder.ID), headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 5) GET holder to verify update
	code, body, err = crm.Request(ctx, "GET", fmt.Sprintf("/v1/holders/%s", holder.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var holderUpdated h.HolderResponse
	if err := json.Unmarshal(body, &holderUpdated); err != nil {
		t.Fatalf("parse updated holder: %v body=%s", err, string(body))
	}
	if holderUpdated.Name != updatedName {
		t.Fatalf("holder name not updated: want %s got %s", updatedName, holderUpdated.Name)
	}

	// 6) DELETE holder
	code, body, err = crm.Request(ctx, "DELETE", fmt.Sprintf("/v1/holders/%s", holder.ID), headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 7) GET holder (should return 404)
	code, body, err = crm.Request(ctx, "GET", fmt.Sprintf("/v1/holders/%s", holder.ID), headers, nil)
	if err != nil {
		t.Fatalf("get deleted holder request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted holder, got: code=%d body=%s", code, string(body))
	}

	t.Log("Holder CRUD lifecycle test completed successfully")
}

// TestIntegration_Holder_LegalPerson tests holder creation for a legal person (company).
func TestIntegration_Holder_LegalPerson(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Generate valid CNPJ for legal person
	cnpj := h.GenerateValidCNPJ()
	companyName := fmt.Sprintf("Test Company %s LTDA", h.RandString(6))

	// CREATE legal person holder
	createPayload := h.CreateLegalPersonPayload(companyName, cnpj)
	createPayload["legalPerson"] = map[string]any{
		"tradeName":    fmt.Sprintf("Trade %s", h.RandString(4)),
		"activity":     "Technology Services",
		"type":         "Limited Liability",
		"foundingDate": "2020-01-15",
		"size":         "Medium",
		"status":       "Active",
	}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create legal person holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var holder h.HolderResponse
	if err := json.Unmarshal(body, &holder); err != nil || holder.ID == "" {
		t.Fatalf("parse holder response: %v body=%s", err, string(body))
	}

	if holder.Type != "LEGAL_PERSON" {
		t.Fatalf("expected LEGAL_PERSON type, got: %s", holder.Type)
	}

	t.Logf("Created legal person holder: id=%s name=%s", holder.ID, holder.Name)

	// Cleanup
	code, _, err = crm.Request(ctx, "DELETE", fmt.Sprintf("/v1/holders/%s", holder.ID), headers, nil)
	if err != nil || code != 204 {
		t.Logf("Warning: cleanup delete failed: code=%d err=%v", code, err)
	}
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_Holder_CRUD ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_Holder_CRUD
=== PARALLEL TestIntegration_Holder_CRUD
    holder_crud_test.go:XX: Created holder: id=... name=Test Holder ...
    holder_crud_test.go:XX: Holder CRUD lifecycle test completed successfully
--- PASS: TestIntegration_Holder_CRUD (X.XXs)
PASS
```

**If Task Fails:**
1. **CRM service not running:** Start with `make up` or `docker-compose up`
2. **Connection refused:** Check CRM_URL environment variable
3. **Validation error on CPF:** Verify GenerateValidCPF produces valid documents
4. **Rollback:** `rm tests/integration/holder_crud_test.go`

---

### Task 2.2: Create Alias CRUD Lifecycle Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/alias_crud_test.go`

**Prerequisites:**
- Tasks 1.1, 1.2, 2.1 completed
- All services running (onboarding, transaction, CRM)

**Step 1: Create alias CRUD test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/alias_crud_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Alias_CRUD tests the complete alias lifecycle:
// Setup (org, ledger, account, holder) -> CREATE alias -> GET -> LIST -> UPDATE -> DELETE
func TestIntegration_Alias_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Alias Test Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}
	t.Logf("Created organization: %s", orgID)

	// Setup: Create ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Alias Test Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger: %v", err)
	}
	t.Logf("Created ledger: %s", ledgerID)

	// Setup: Create USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// Setup: Create account
	alias := fmt.Sprintf("alias-test-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("setup account: %v", err)
	}
	t.Logf("Created account: %s with alias: %s", accountID, alias)

	// Setup: Create holder
	cpf := h.GenerateValidCPF()
	holderID, err := h.SetupHolder(ctx, crm, headers, fmt.Sprintf("Alias Holder %s", h.RandString(6)), cpf, "NATURAL_PERSON")
	if err != nil {
		t.Fatalf("setup holder: %v", err)
	}
	t.Logf("Created holder: %s", holderID)

	// 1) CREATE alias
	createPayload := map[string]any{
		"ledgerId":  ledgerID,
		"accountId": accountID,
		"metadata":  map[string]any{"test": "alias-crud"},
	}

	path := fmt.Sprintf("/v1/holders/%s/aliases", holderID)
	code, body, err := crm.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdAlias h.AliasResponse
	if err := json.Unmarshal(body, &createdAlias); err != nil || createdAlias.ID == "" {
		t.Fatalf("parse alias response: %v body=%s", err, string(body))
	}
	t.Logf("Created alias: %s", createdAlias.ID)

	// 2) GET alias by ID
	getPath := fmt.Sprintf("/v1/holders/%s/aliases/%s", holderID, createdAlias.ID)
	code, body, err = crm.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var aliasGet h.AliasResponse
	if err := json.Unmarshal(body, &aliasGet); err != nil {
		t.Fatalf("parse get alias response: %v body=%s", err, string(body))
	}
	if aliasGet.ID != createdAlias.ID {
		t.Fatalf("alias ID mismatch: want %s got %s", createdAlias.ID, aliasGet.ID)
	}
	if aliasGet.AccountID != accountID {
		t.Fatalf("alias accountId mismatch: want %s got %s", accountID, aliasGet.AccountID)
	}

	// 3) LIST all aliases (global endpoint)
	code, body, err = crm.Request(ctx, "GET", "/v1/aliases", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list aliases failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var aliasList struct {
		Items []h.AliasResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &aliasList); err != nil {
		t.Fatalf("parse alias list: %v body=%s", err, string(body))
	}

	found := false
	for _, a := range aliasList.Items {
		if a.ID == createdAlias.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created alias not found in global list")
	}

	// 4) UPDATE alias
	updatePayload := map[string]any{
		"metadata": map[string]any{
			"test":    "alias-crud",
			"updated": true,
		},
		"bankingDetails": map[string]any{
			"branch":      "0001",
			"account":     "123456",
			"type":        "CACC",
			"countryCode": "US",
		},
	}

	code, body, err = crm.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 5) Verify update
	code, body, err = crm.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedAlias struct {
		ID             string `json:"id"`
		BankingDetails *struct {
			Branch string `json:"branch"`
		} `json:"bankingDetails"`
	}
	if err := json.Unmarshal(body, &updatedAlias); err != nil {
		t.Fatalf("parse updated alias: %v body=%s", err, string(body))
	}
	if updatedAlias.BankingDetails == nil || updatedAlias.BankingDetails.Branch != "0001" {
		t.Fatalf("alias banking details not updated correctly")
	}

	// 6) DELETE alias
	code, body, err = crm.Request(ctx, "DELETE", getPath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 7) Verify deletion (should return 404)
	code, body, err = crm.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted alias request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted alias, got: code=%d body=%s", code, string(body))
	}

	t.Log("Alias CRUD lifecycle test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_Alias_CRUD ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_Alias_CRUD
=== PARALLEL TestIntegration_Alias_CRUD
    alias_crud_test.go:XX: Created organization: ...
    alias_crud_test.go:XX: Created ledger: ...
    alias_crud_test.go:XX: Created account: ... with alias: ...
    alias_crud_test.go:XX: Created holder: ...
    alias_crud_test.go:XX: Created alias: ...
    alias_crud_test.go:XX: Alias CRUD lifecycle test completed successfully
--- PASS: TestIntegration_Alias_CRUD (X.XXs)
PASS
```

**If Task Fails:**
1. **Setup failure:** Check all services are running
2. **Alias creation fails:** Verify ledgerID and accountID are valid UUIDs
3. **Rollback:** `rm tests/integration/alias_crud_test.go`

---

## Phase 3: Routing Integration Tests

### Task 3.1: Create Operation Route CRUD Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operation_route_crud_test.go`

**Prerequisites:**
- Tasks 1.1, 1.3 completed
- Transaction service running on localhost:3001

**Step 1: Create operation route CRUD test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/operation_route_crud_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_OperationRoute_CRUD tests the complete operation route lifecycle:
// CREATE -> GET -> LIST -> UPDATE -> DELETE
func TestIntegration_OperationRoute_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization and ledger
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("OpRoute Test Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("OpRoute Test Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger: %v", err)
	}

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	// 1) CREATE operation route (source type)
	title := fmt.Sprintf("Source Route %s", h.RandString(6))
	createPayload := h.CreateOperationRoutePayloadFull(
		title,
		"Test source operation route",
		fmt.Sprintf("SRC-%s", h.RandString(4)),
		"source",
		map[string]any{"env": "test"},
	)

	code, body, err := trans.Request(ctx, "POST", basePath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var route h.OperationRouteResponse
	if err := json.Unmarshal(body, &route); err != nil || route.ID == "" {
		t.Fatalf("parse operation route: %v body=%s", err, string(body))
	}
	t.Logf("Created operation route: id=%s title=%s type=%s", route.ID, route.Title, route.OperationType)

	// 2) GET operation route by ID
	getPath := fmt.Sprintf("%s/%s", basePath, route.ID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var routeGet h.OperationRouteResponse
	if err := json.Unmarshal(body, &routeGet); err != nil {
		t.Fatalf("parse get operation route: %v body=%s", err, string(body))
	}
	if routeGet.ID != route.ID {
		t.Fatalf("route ID mismatch: want %s got %s", route.ID, routeGet.ID)
	}
	if routeGet.OperationType != "source" {
		t.Fatalf("operation type mismatch: want source got %s", routeGet.OperationType)
	}

	// 3) LIST operation routes
	code, body, err = trans.Request(ctx, "GET", basePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list operation routes failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var routeList struct {
		Items []h.OperationRouteResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &routeList); err != nil {
		t.Fatalf("parse route list: %v body=%s", err, string(body))
	}

	found := false
	for _, r := range routeList.Items {
		if r.ID == route.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created operation route not found in list")
	}

	// 4) UPDATE operation route
	updatedTitle := fmt.Sprintf("Updated Route %s", h.RandString(6))
	updatePayload := map[string]any{
		"title":       updatedTitle,
		"description": "Updated description",
		"metadata": map[string]any{
			"env":     "test",
			"updated": true,
		},
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRoute h.OperationRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated route: %v body=%s", err, string(body))
	}
	if updatedRoute.Title != updatedTitle {
		t.Fatalf("title not updated: want %s got %s", updatedTitle, updatedRoute.Title)
	}

	// 5) DELETE operation route
	code, body, err = trans.Request(ctx, "DELETE", getPath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 6) Verify deletion
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted route request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted route, got: code=%d body=%s", code, string(body))
	}

	t.Log("Operation Route CRUD lifecycle test completed successfully")
}

// TestIntegration_OperationRoute_DestinationType tests destination operation route creation.
func TestIntegration_OperationRoute_DestinationType(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, _ := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("DestRoute Org %s", h.RandString(6)))
	ledgerID, _ := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("DestRoute Ledger %s", h.RandString(6)))

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	// CREATE destination type operation route
	createPayload := map[string]any{
		"title":         fmt.Sprintf("Destination Route %s", h.RandString(6)),
		"operationType": "destination",
		"description":   "Test destination operation route",
	}

	code, body, err := trans.Request(ctx, "POST", basePath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create destination route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var route h.OperationRouteResponse
	if err := json.Unmarshal(body, &route); err != nil {
		t.Fatalf("parse route: %v body=%s", err, string(body))
	}

	if route.OperationType != "destination" {
		t.Fatalf("expected destination type, got: %s", route.OperationType)
	}

	t.Logf("Created destination operation route: id=%s", route.ID)

	// Cleanup
	trans.Request(ctx, "DELETE", fmt.Sprintf("%s/%s", basePath, route.ID), headers, nil)
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_OperationRoute ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_OperationRoute_CRUD
=== PARALLEL TestIntegration_OperationRoute_CRUD
    operation_route_crud_test.go:XX: Created operation route: id=... title=... type=source
    operation_route_crud_test.go:XX: Operation Route CRUD lifecycle test completed successfully
--- PASS: TestIntegration_OperationRoute_CRUD (X.XXs)
=== RUN   TestIntegration_OperationRoute_DestinationType
--- PASS: TestIntegration_OperationRoute_DestinationType (X.XXs)
PASS
```

**If Task Fails:**
1. **Authorization error:** Verify routing permissions in auth middleware
2. **Invalid operation type:** Use only "source" or "destination"
3. **Rollback:** `rm tests/integration/operation_route_crud_test.go`

---

### Task 3.2: Create Transaction Route CRUD Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/transaction_route_crud_test.go`

**Prerequisites:**
- Task 3.1 completed
- Transaction service running

**Step 1: Create transaction route CRUD test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/transaction_route_crud_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_TransactionRoute_CRUD tests the complete transaction route lifecycle.
func TestIntegration_TransactionRoute_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization and ledger
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("TxRoute Test Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("TxRoute Test Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger: %v", err)
	}

	// Setup: Create operation routes (need at least one for transaction route)
	sourceRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Source %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("setup source operation route: %v", err)
	}
	t.Logf("Created source operation route: %s", sourceRouteID)

	destRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Dest %s", h.RandString(4)), "destination")
	if err != nil {
		t.Fatalf("setup destination operation route: %v", err)
	}
	t.Logf("Created destination operation route: %s", destRouteID)

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	// 1) CREATE transaction route
	title := fmt.Sprintf("Transaction Route %s", h.RandString(6))
	createPayload := map[string]any{
		"title":           title,
		"description":     "Test transaction route linking source and destination",
		"operationRoutes": []string{sourceRouteID, destRouteID},
		"metadata":        map[string]any{"env": "test"},
	}

	code, body, err := trans.Request(ctx, "POST", basePath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var txRoute h.TransactionRouteResponse
	if err := json.Unmarshal(body, &txRoute); err != nil || txRoute.ID == "" {
		t.Fatalf("parse transaction route: %v body=%s", err, string(body))
	}
	t.Logf("Created transaction route: id=%s title=%s", txRoute.ID, txRoute.Title)

	// 2) GET transaction route by ID
	getPath := fmt.Sprintf("%s/%s", basePath, txRoute.ID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var txRouteGet h.TransactionRouteResponse
	if err := json.Unmarshal(body, &txRouteGet); err != nil {
		t.Fatalf("parse get transaction route: %v body=%s", err, string(body))
	}
	if txRouteGet.ID != txRoute.ID {
		t.Fatalf("route ID mismatch: want %s got %s", txRoute.ID, txRouteGet.ID)
	}
	if len(txRouteGet.OperationRoutes) != 2 {
		t.Fatalf("expected 2 operation routes, got %d", len(txRouteGet.OperationRoutes))
	}

	// 3) LIST transaction routes
	code, body, err = trans.Request(ctx, "GET", basePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list transaction routes failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var txRouteList struct {
		Items []h.TransactionRouteResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &txRouteList); err != nil {
		t.Fatalf("parse route list: %v body=%s", err, string(body))
	}

	found := false
	for _, r := range txRouteList.Items {
		if r.ID == txRoute.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created transaction route not found in list")
	}

	// 4) UPDATE transaction route
	updatedTitle := fmt.Sprintf("Updated TxRoute %s", h.RandString(6))
	updatePayload := map[string]any{
		"title":       updatedTitle,
		"description": "Updated transaction route description",
		"metadata": map[string]any{
			"env":     "test",
			"updated": true,
		},
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRoute h.TransactionRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated route: %v body=%s", err, string(body))
	}
	if updatedRoute.Title != updatedTitle {
		t.Fatalf("title not updated: want %s got %s", updatedTitle, updatedRoute.Title)
	}

	// 5) DELETE transaction route
	code, body, err = trans.Request(ctx, "DELETE", getPath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 6) Verify deletion
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted route request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted route, got: code=%d body=%s", code, string(body))
	}

	t.Log("Transaction Route CRUD lifecycle test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_TransactionRoute_CRUD ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_TransactionRoute_CRUD
=== PARALLEL TestIntegration_TransactionRoute_CRUD
    transaction_route_crud_test.go:XX: Created source operation route: ...
    transaction_route_crud_test.go:XX: Created destination operation route: ...
    transaction_route_crud_test.go:XX: Created transaction route: id=... title=...
    transaction_route_crud_test.go:XX: Transaction Route CRUD lifecycle test completed successfully
--- PASS: TestIntegration_TransactionRoute_CRUD (X.XXs)
PASS
```

**If Task Fails:**
1. **Operation routes required:** Transaction routes need at least one operation route ID
2. **Invalid UUID:** Ensure operation route IDs are valid UUIDs
3. **Rollback:** `rm tests/integration/transaction_route_crud_test.go`

---

## Phase 4: Asset Rate Integration Tests

### Task 4.1: Create Asset Rate CRUD Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_rate_crud_test.go`

**Prerequisites:**
- Tasks 1.1, 1.3 completed
- Transaction service running

**Step 1: Create asset rate CRUD test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_rate_crud_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_AssetRate_CRUD tests asset rate creation, retrieval, and update.
func TestIntegration_AssetRate_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization and ledger
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetRate Test Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetRate Test Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger: %v", err)
	}

	// Setup: Create required assets (USD and BRL)
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// Create BRL asset
	brlPayload := map[string]any{
		"name": "Brazilian Real",
		"type": "currency",
		"code": "BRL",
	}
	code, body, err := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID), headers, brlPayload)
	if err != nil || (code != 201 && code != 409) {
		t.Fatalf("create BRL asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)
	externalID := uuid.New().String()

	// 1) CREATE (PUT) asset rate - USD to BRL
	createPayload := map[string]any{
		"from":       "USD",
		"to":         "BRL",
		"rate":       550, // 5.50 with scale 2
		"scale":      2,
		"externalId": externalID,
		"source":     "Test Integration",
		"ttl":        3600,
		"metadata":   map[string]any{"env": "test"},
	}

	code, body, err = trans.Request(ctx, "PUT", basePath, headers, createPayload)
	if err != nil || (code != 200 && code != 201) {
		t.Fatalf("create asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var assetRate h.AssetRateResponse
	if err := json.Unmarshal(body, &assetRate); err != nil || assetRate.ID == "" {
		t.Fatalf("parse asset rate: %v body=%s", err, string(body))
	}
	t.Logf("Created asset rate: id=%s from=%s to=%s rate=%v", assetRate.ID, assetRate.From, assetRate.To, assetRate.Rate)

	// 2) GET asset rate by external ID
	getPath := fmt.Sprintf("%s/%s", basePath, externalID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get asset rate by external ID failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var assetRateGet h.AssetRateResponse
	if err := json.Unmarshal(body, &assetRateGet); err != nil {
		t.Fatalf("parse get asset rate: %v body=%s", err, string(body))
	}
	if assetRateGet.ExternalID != externalID {
		t.Fatalf("external ID mismatch: want %s got %s", externalID, assetRateGet.ExternalID)
	}
	if assetRateGet.From != "USD" || assetRateGet.To != "BRL" {
		t.Fatalf("asset codes mismatch: want USD->BRL got %s->%s", assetRateGet.From, assetRateGet.To)
	}

	// 3) GET all asset rates by asset code (from)
	listPath := fmt.Sprintf("%s/from/USD", basePath)
	code, body, err = trans.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get asset rates by asset code failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var rateList struct {
		Items []h.AssetRateResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &rateList); err != nil {
		t.Fatalf("parse rate list: %v body=%s", err, string(body))
	}

	found := false
	for _, r := range rateList.Items {
		if r.ExternalID == externalID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created asset rate not found in list")
	}

	// 4) UPDATE asset rate (PUT with same external ID)
	updatePayload := map[string]any{
		"from":       "USD",
		"to":         "BRL",
		"rate":       560, // Updated rate: 5.60
		"scale":      2,
		"externalId": externalID,
		"source":     "Test Integration Updated",
		"metadata": map[string]any{
			"env":     "test",
			"updated": true,
		},
	}

	code, body, err = trans.Request(ctx, "PUT", basePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRate h.AssetRateResponse
	if err := json.Unmarshal(body, &updatedRate); err != nil {
		t.Fatalf("parse updated rate: %v body=%s", err, string(body))
	}
	if updatedRate.Rate != 560 {
		t.Fatalf("rate not updated: want 560 got %v", updatedRate.Rate)
	}

	t.Log("Asset Rate CRUD test completed successfully")
}

// TestIntegration_AssetRate_MultipleRates tests creating multiple asset rates for the same source asset.
func TestIntegration_AssetRate_MultipleRates(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, _ := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("MultiRate Org %s", h.RandString(6)))
	ledgerID, _ := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("MultiRate Ledger %s", h.RandString(6)))

	// Create assets
	h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers)

	for _, code := range []string{"EUR", "GBP", "JPY"} {
		payload := map[string]any{"name": code + " Currency", "type": "currency", "code": code}
		onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID), headers, payload)
	}

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)

	// Create multiple rates from USD
	rates := []struct {
		to   string
		rate int
	}{
		{"EUR", 92},  // 0.92
		{"GBP", 79},  // 0.79
		{"JPY", 15000}, // 150.00
	}

	for _, r := range rates {
		payload := map[string]any{
			"from":       "USD",
			"to":         r.to,
			"rate":       r.rate,
			"scale":      2,
			"externalId": uuid.New().String(),
		}
		code, body, err := trans.Request(ctx, "PUT", basePath, headers, payload)
		if err != nil || (code != 200 && code != 201) {
			t.Fatalf("create rate USD->%s failed: code=%d err=%v body=%s", r.to, code, err, string(body))
		}
		t.Logf("Created rate: USD -> %s = %d", r.to, r.rate)
	}

	// List all rates from USD
	listPath := fmt.Sprintf("%s/from/USD", basePath)
	code, body, err := trans.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list rates failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var rateList struct {
		Items []h.AssetRateResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &rateList); err != nil {
		t.Fatalf("parse rate list: %v body=%s", err, string(body))
	}

	if len(rateList.Items) < 3 {
		t.Fatalf("expected at least 3 rates, got %d", len(rateList.Items))
	}

	t.Logf("Found %d asset rates from USD", len(rateList.Items))
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_AssetRate ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_AssetRate_CRUD
=== PARALLEL TestIntegration_AssetRate_CRUD
    asset_rate_crud_test.go:XX: Created asset rate: id=... from=USD to=BRL rate=550
    asset_rate_crud_test.go:XX: Asset Rate CRUD test completed successfully
--- PASS: TestIntegration_AssetRate_CRUD (X.XXs)
=== RUN   TestIntegration_AssetRate_MultipleRates
--- PASS: TestIntegration_AssetRate_MultipleRates (X.XXs)
PASS
```

**If Task Fails:**
1. **Asset not found:** Create source and target assets before creating rates
2. **Invalid rate:** Rate must be a positive integer
3. **Rollback:** `rm tests/integration/asset_rate_crud_test.go`

---

## Phase 5: Account Type Integration Tests

### Task 5.1: Create Account Type CRUD Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/account_type_crud_test.go`

**Prerequisites:**
- Task 1.3 completed
- Onboarding service running

**Step 1: Create account type CRUD test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/account_type_crud_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_AccountType_CRUD tests the complete account type lifecycle.
func TestIntegration_AccountType_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization and ledger
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AccType Test Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AccType Test Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger: %v", err)
	}

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)

	// 1) CREATE account type
	keyValue := fmt.Sprintf("test_type_%s", h.RandString(6))
	createPayload := map[string]any{
		"name":        fmt.Sprintf("Test Account Type %s", h.RandString(6)),
		"description": "A test account type for integration testing",
		"keyValue":    keyValue,
		"metadata":    map[string]any{"env": "test"},
	}

	code, body, err := onboard.Request(ctx, "POST", basePath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account type failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var accountType h.AccountTypeResponse
	if err := json.Unmarshal(body, &accountType); err != nil || accountType.ID == "" {
		t.Fatalf("parse account type: %v body=%s", err, string(body))
	}
	t.Logf("Created account type: id=%s name=%s keyValue=%s", accountType.ID, accountType.Name, accountType.KeyValue)

	// 2) GET account type by ID
	getPath := fmt.Sprintf("%s/%s", basePath, accountType.ID)
	code, body, err = onboard.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get account type failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var accountTypeGet h.AccountTypeResponse
	if err := json.Unmarshal(body, &accountTypeGet); err != nil {
		t.Fatalf("parse get account type: %v body=%s", err, string(body))
	}
	if accountTypeGet.ID != accountType.ID {
		t.Fatalf("account type ID mismatch: want %s got %s", accountType.ID, accountTypeGet.ID)
	}
	if accountTypeGet.KeyValue != keyValue {
		t.Fatalf("keyValue mismatch: want %s got %s", keyValue, accountTypeGet.KeyValue)
	}

	// 3) LIST account types
	code, body, err = onboard.Request(ctx, "GET", basePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list account types failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var typeList struct {
		Items []h.AccountTypeResponse `json:"items"`
	}
	if err := json.Unmarshal(body, &typeList); err != nil {
		t.Fatalf("parse type list: %v body=%s", err, string(body))
	}

	found := false
	for _, at := range typeList.Items {
		if at.ID == accountType.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created account type not found in list")
	}

	// 4) UPDATE account type
	updatedName := fmt.Sprintf("Updated Type %s", h.RandString(6))
	updatePayload := map[string]any{
		"name":        updatedName,
		"description": "Updated description",
		"metadata": map[string]any{
			"env":     "test",
			"updated": true,
		},
	}

	code, body, err = onboard.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update account type failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = onboard.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated type failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedType h.AccountTypeResponse
	if err := json.Unmarshal(body, &updatedType); err != nil {
		t.Fatalf("parse updated type: %v body=%s", err, string(body))
	}
	if updatedType.Name != updatedName {
		t.Fatalf("name not updated: want %s got %s", updatedName, updatedType.Name)
	}

	// 5) DELETE account type
	code, body, err = onboard.Request(ctx, "DELETE", getPath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete account type failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 6) Verify deletion
	code, body, err = onboard.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted type request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted type, got: code=%d body=%s", code, string(body))
	}

	t.Log("Account Type CRUD lifecycle test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_AccountType_CRUD ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_AccountType_CRUD
=== PARALLEL TestIntegration_AccountType_CRUD
    account_type_crud_test.go:XX: Created account type: id=... name=... keyValue=...
    account_type_crud_test.go:XX: Account Type CRUD lifecycle test completed successfully
--- PASS: TestIntegration_AccountType_CRUD (X.XXs)
PASS
```

**If Task Fails:**
1. **Invalid keyValue:** keyValue cannot be reserved types like "deposit", "savings"
2. **Authorization:** Verify routing permissions (account-types use routingName)
3. **Rollback:** `rm tests/integration/account_type_crud_test.go`

---

## Phase 6: PATCH/DELETE Operations Tests

### Task 6.1: Create Organization Update/Delete Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/organization_update_delete_test.go`

**Prerequisites:**
- Onboarding service running

**Step 1: Create organization update/delete test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/organization_update_delete_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Organization_Update tests organization PATCH operation.
func TestIntegration_Organization_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization
	orgName := fmt.Sprintf("Update Test Org %s", h.RandString(6))
	createPayload := h.OrgPayload(orgName, h.RandString(12))
	createPayload["metadata"] = map[string]any{"version": 1}

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var org struct {
		ID        string `json:"id"`
		LegalName string `json:"legalName"`
	}
	if err := json.Unmarshal(body, &org); err != nil || org.ID == "" {
		t.Fatalf("parse organization: %v body=%s", err, string(body))
	}
	t.Logf("Created organization: id=%s name=%s", org.ID, org.LegalName)

	// UPDATE organization
	updatedName := fmt.Sprintf("Updated Org %s", h.RandString(6))
	updatePayload := map[string]any{
		"legalName": updatedName,
		"metadata": map[string]any{
			"version": 2,
			"updated": true,
		},
	}

	code, body, err = onboard.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedOrg struct {
		LegalName string         `json:"legalName"`
		Metadata  map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(body, &updatedOrg); err != nil {
		t.Fatalf("parse updated organization: %v body=%s", err, string(body))
	}
	if updatedOrg.LegalName != updatedName {
		t.Fatalf("name not updated: want %s got %s", updatedName, updatedOrg.LegalName)
	}

	t.Log("Organization update test completed successfully")
}

// TestIntegration_Organization_Delete tests organization DELETE operation.
func TestIntegration_Organization_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization
	createPayload := h.OrgPayload(fmt.Sprintf("Delete Test Org %s", h.RandString(6)), h.RandString(12))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil || org.ID == "" {
		t.Fatalf("parse organization: %v body=%s", err, string(body))
	}
	t.Logf("Created organization for deletion: id=%s", org.ID)

	// DELETE organization
	code, body, err = onboard.Request(ctx, "DELETE", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion (should return 404)
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
	if err != nil {
		t.Fatalf("get deleted organization request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted organization, got: code=%d body=%s", code, string(body))
	}

	t.Log("Organization delete test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_Organization_ ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_Organization_Update
--- PASS: TestIntegration_Organization_Update (X.XXs)
=== RUN   TestIntegration_Organization_Delete
--- PASS: TestIntegration_Organization_Delete (X.XXs)
PASS
```

**If Task Fails:**
1. **Validation error:** Check required fields in update payload
2. **Cannot delete:** Organization may have dependent resources
3. **Rollback:** `rm tests/integration/organization_update_delete_test.go`

---

### Task 6.2: Create Ledger Update/Delete Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/ledger_update_delete_test.go`

**Prerequisites:**
- Onboarding service running

**Step 1: Create ledger update/delete test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/ledger_update_delete_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Ledger_Update tests ledger PATCH operation.
func TestIntegration_Ledger_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("LedgerUpdate Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}

	// Create ledger
	ledgerName := fmt.Sprintf("Update Test Ledger %s", h.RandString(6))
	createPayload := map[string]any{
		"name":     ledgerName,
		"metadata": map[string]any{"version": 1},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers", orgID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var ledger struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil || ledger.ID == "" {
		t.Fatalf("parse ledger: %v body=%s", err, string(body))
	}
	t.Logf("Created ledger: id=%s name=%s", ledger.ID, ledger.Name)

	// UPDATE ledger
	updatedName := fmt.Sprintf("Updated Ledger %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"version": 2,
			"updated": true,
		},
	}

	updatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, ledger.ID)
	code, body, err = onboard.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = onboard.Request(ctx, "GET", updatePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedLedger struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &updatedLedger); err != nil {
		t.Fatalf("parse updated ledger: %v body=%s", err, string(body))
	}
	if updatedLedger.Name != updatedName {
		t.Fatalf("name not updated: want %s got %s", updatedName, updatedLedger.Name)
	}

	t.Log("Ledger update test completed successfully")
}

// TestIntegration_Ledger_Delete tests ledger DELETE operation.
func TestIntegration_Ledger_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Create organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("LedgerDelete Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization: %v", err)
	}

	// Create ledger
	createPayload := map[string]any{
		"name": fmt.Sprintf("Delete Test Ledger %s", h.RandString(6)),
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers", orgID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil || ledger.ID == "" {
		t.Fatalf("parse ledger: %v body=%s", err, string(body))
	}
	t.Logf("Created ledger for deletion: id=%s", ledger.ID)

	// DELETE ledger
	deletePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, ledger.ID)
	code, body, err = onboard.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion
	code, body, err = onboard.Request(ctx, "GET", deletePath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted ledger request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted ledger, got: code=%d body=%s", code, string(body))
	}

	t.Log("Ledger delete test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_Ledger_ ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_Ledger_Update
--- PASS: TestIntegration_Ledger_Update (X.XXs)
=== RUN   TestIntegration_Ledger_Delete
--- PASS: TestIntegration_Ledger_Delete (X.XXs)
PASS
```

**If Task Fails:**
1. **Cannot delete:** Ledger may have dependent resources (accounts, assets)
2. **Rollback:** `rm tests/integration/ledger_update_delete_test.go`

---

### Task 6.3: Create Account Update/Delete Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/account_update_delete_test.go`

**Prerequisites:**
- Onboarding service running

**Step 1: Create account update/delete test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/account_update_delete_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Account_Update tests account PATCH operation.
func TestIntegration_Account_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, _ := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AccUpdate Org %s", h.RandString(6)))
	ledgerID, _ := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AccUpdate Ledger %s", h.RandString(6)))
	h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers)

	// Create account
	alias := fmt.Sprintf("update-test-%s", h.RandString(6))
	createPayload := map[string]any{
		"name":      "Update Test Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
		"metadata":  map[string]any{"version": 1},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var account struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &account); err != nil || account.ID == "" {
		t.Fatalf("parse account: %v body=%s", err, string(body))
	}
	t.Logf("Created account: id=%s name=%s", account.ID, account.Name)

	// UPDATE account
	updatedName := fmt.Sprintf("Updated Account %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"version": 2,
			"updated": true,
		},
	}

	updatePath := fmt.Sprintf("%s/%s", path, account.ID)
	code, body, err = onboard.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = onboard.Request(ctx, "GET", updatePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedAccount struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &updatedAccount); err != nil {
		t.Fatalf("parse updated account: %v body=%s", err, string(body))
	}
	if updatedAccount.Name != updatedName {
		t.Fatalf("name not updated: want %s got %s", updatedName, updatedAccount.Name)
	}

	t.Log("Account update test completed successfully")
}

// TestIntegration_Account_Delete tests account DELETE operation.
func TestIntegration_Account_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, _ := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AccDelete Org %s", h.RandString(6)))
	ledgerID, _ := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AccDelete Ledger %s", h.RandString(6)))
	h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers)

	// Create account (without transactions - can be deleted)
	alias := fmt.Sprintf("delete-test-%s", h.RandString(6))
	createPayload := map[string]any{
		"name":      "Delete Test Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var account struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &account); err != nil || account.ID == "" {
		t.Fatalf("parse account: %v body=%s", err, string(body))
	}
	t.Logf("Created account for deletion: id=%s", account.ID)

	// DELETE account
	deletePath := fmt.Sprintf("%s/%s", path, account.ID)
	code, body, err = onboard.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion
	code, body, err = onboard.Request(ctx, "GET", deletePath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted account request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted account, got: code=%d body=%s", code, string(body))
	}

	t.Log("Account delete test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_Account_Update -run TestIntegration_Account_Delete ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_Account_Update
--- PASS: TestIntegration_Account_Update (X.XXs)
=== RUN   TestIntegration_Account_Delete
--- PASS: TestIntegration_Account_Delete (X.XXs)
PASS
```

**If Task Fails:**
1. **Cannot delete:** Account may have balances or transactions
2. **Rollback:** `rm tests/integration/account_update_delete_test.go`

---

### Task 6.4: Create Asset Update/Delete Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_update_delete_test.go`

**Prerequisites:**
- Onboarding service running

**Step 1: Create asset update/delete test file**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/integration/asset_update_delete_test.go`:

```go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Asset_Update tests asset PATCH operation.
func TestIntegration_Asset_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, _ := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetUpdate Org %s", h.RandString(6)))
	ledgerID, _ := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetUpdate Ledger %s", h.RandString(6)))

	// Create custom asset
	assetCode := fmt.Sprintf("TST%s", h.RandString(3))
	createPayload := map[string]any{
		"name":     fmt.Sprintf("Test Asset %s", h.RandString(6)),
		"type":     "commodity",
		"code":     assetCode,
		"metadata": map[string]any{"version": 1},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var asset struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &asset); err != nil || asset.ID == "" {
		t.Fatalf("parse asset: %v body=%s", err, string(body))
	}
	t.Logf("Created asset: id=%s name=%s code=%s", asset.ID, asset.Name, assetCode)

	// UPDATE asset
	updatedName := fmt.Sprintf("Updated Asset %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"version": 2,
			"updated": true,
		},
	}

	updatePath := fmt.Sprintf("%s/%s", path, asset.ID)
	code, body, err = onboard.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("update asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify update
	code, body, err = onboard.Request(ctx, "GET", updatePath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get updated asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedAsset struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &updatedAsset); err != nil {
		t.Fatalf("parse updated asset: %v body=%s", err, string(body))
	}
	if updatedAsset.Name != updatedName {
		t.Fatalf("name not updated: want %s got %s", updatedName, updatedAsset.Name)
	}

	t.Log("Asset update test completed successfully")
}

// TestIntegration_Asset_Delete tests asset DELETE operation.
func TestIntegration_Asset_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, _ := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetDelete Org %s", h.RandString(6)))
	ledgerID, _ := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetDelete Ledger %s", h.RandString(6)))

	// Create custom asset (without accounts - can be deleted)
	assetCode := fmt.Sprintf("DEL%s", h.RandString(3))
	createPayload := map[string]any{
		"name": fmt.Sprintf("Delete Test Asset %s", h.RandString(6)),
		"type": "commodity",
		"code": assetCode,
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("create asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var asset struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &asset); err != nil || asset.ID == "" {
		t.Fatalf("parse asset: %v body=%s", err, string(body))
	}
	t.Logf("Created asset for deletion: id=%s code=%s", asset.ID, assetCode)

	// DELETE asset
	deletePath := fmt.Sprintf("%s/%s", path, asset.ID)
	code, body, err = onboard.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("delete asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Verify deletion
	code, body, err = onboard.Request(ctx, "GET", deletePath, headers, nil)
	if err != nil {
		t.Fatalf("get deleted asset request failed: %v", err)
	}
	if code != 404 {
		t.Fatalf("expected 404 for deleted asset, got: code=%d body=%s", code, string(body))
	}

	t.Log("Asset delete test completed successfully")
}
```

**Step 2: Run the test**

Run: `go test -v -run TestIntegration_Asset_Update -run TestIntegration_Asset_Delete ./tests/integration/ -count=1`

**Expected output:**
```
=== RUN   TestIntegration_Asset_Update
--- PASS: TestIntegration_Asset_Update (X.XXs)
=== RUN   TestIntegration_Asset_Delete
--- PASS: TestIntegration_Asset_Delete (X.XXs)
PASS
```

**If Task Fails:**
1. **Cannot delete:** Asset may have dependent accounts
2. **Rollback:** `rm tests/integration/asset_update_delete_test.go`

---

## Phase 7: Final Verification

### Task 7.1: Run All New Integration Tests

**Files:**
- None (verification only)

**Prerequisites:**
- All previous tasks completed
- All services running

**Step 1: Run all new integration tests**

Run: `go test -v ./tests/integration/ -run "TestIntegration_(Holder|Alias|OperationRoute|TransactionRoute|AssetRate|AccountType|Organization_Update|Organization_Delete|Ledger_Update|Ledger_Delete|Account_Update|Account_Delete|Asset_Update|Asset_Delete)" -count=1 -timeout 10m`

**Expected output:**
```
=== RUN   TestIntegration_Holder_CRUD
--- PASS: TestIntegration_Holder_CRUD (X.XXs)
=== RUN   TestIntegration_Holder_LegalPerson
--- PASS: TestIntegration_Holder_LegalPerson (X.XXs)
=== RUN   TestIntegration_Alias_CRUD
--- PASS: TestIntegration_Alias_CRUD (X.XXs)
=== RUN   TestIntegration_OperationRoute_CRUD
--- PASS: TestIntegration_OperationRoute_CRUD (X.XXs)
... (all tests should pass)
PASS
ok  	github.com/LerianStudio/midaz/v3/tests/integration	XX.XXs
```

**Step 2: Verify test count**

Run: `go test ./tests/integration/ -run "TestIntegration_(Holder|Alias|OperationRoute|TransactionRoute|AssetRate|AccountType|Organization_Update|Organization_Delete|Ledger_Update|Ledger_Delete|Account_Update|Account_Delete|Asset_Update|Asset_Delete)" -v -count=1 2>&1 | grep -c "^--- PASS"`

**Expected output:**
```
14
```

(Approximately 14 new test functions)

**If Task Fails:**
1. **Timeout:** Increase timeout or check service health
2. **Individual test failure:** Debug specific test
3. **No rollback needed:** This is verification only

---

### Task 7.2: Run Code Review

**Files:**
- None (review only)

**Step 1: Dispatch all 3 reviewers in parallel**

REQUIRED SUB-SKILL: Use requesting-code-review

**Step 2: Handle findings by severity**

- **Critical/High/Medium:** Fix immediately, re-run reviewers
- **Low:** Add `TODO(review):` comments
- **Cosmetic:** Add `FIXME(nitpick):` comments

**Step 3: Proceed when zero Critical/High/Medium issues remain**

---

## Summary

### New Files Created:
1. `/tests/helpers/crm.go` - CRM helper functions
2. `/tests/helpers/routing.go` - Routing helper functions
3. `/tests/integration/holder_crud_test.go` - Holder CRUD tests
4. `/tests/integration/alias_crud_test.go` - Alias CRUD tests
5. `/tests/integration/operation_route_crud_test.go` - Operation Route CRUD tests
6. `/tests/integration/transaction_route_crud_test.go` - Transaction Route CRUD tests
7. `/tests/integration/asset_rate_crud_test.go` - Asset Rate CRUD tests
8. `/tests/integration/account_type_crud_test.go` - Account Type CRUD tests
9. `/tests/integration/organization_update_delete_test.go` - Org PATCH/DELETE tests
10. `/tests/integration/ledger_update_delete_test.go` - Ledger PATCH/DELETE tests
11. `/tests/integration/account_update_delete_test.go` - Account PATCH/DELETE tests
12. `/tests/integration/asset_update_delete_test.go` - Asset PATCH/DELETE tests

### Files Modified:
1. `/tests/helpers/env.go` - Added CRMURL field

### Coverage Impact:
- **Before:** ~35-40 of 106 endpoints tested (33-38%)
- **After:** ~55-60 of 106 endpoints tested (52-57%)
- **Net Gain:** +20 additional endpoints covered

### Endpoints Now Covered:
- CRM: Holders (5 endpoints), Aliases (5 endpoints) - 10 total
- Routing: Operation Routes (5 endpoints), Transaction Routes (5 endpoints) - 10 total
- Asset Rates: 3 endpoints
- Account Types: 5 endpoints
- PATCH/DELETE: Organization, Ledger, Account, Asset - 8 total

### Execution Time Estimate:
- Infrastructure updates (Tasks 1.1-1.3): ~15 minutes
- CRM tests (Tasks 2.1-2.2): ~20 minutes
- Routing tests (Tasks 3.1-3.2): ~20 minutes
- Asset Rate tests (Task 4.1): ~10 minutes
- Account Type tests (Task 5.1): ~10 minutes
- PATCH/DELETE tests (Tasks 6.1-6.4): ~25 minutes
- Final verification (Tasks 7.1-7.2): ~15 minutes
- **Total:** ~2 hours

---

## Plan Checklist

- [x] **Historical precedent queried** (artifact-query --mode planning)
- [x] Historical Precedent section included in plan
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
