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

// TestIntegration_OperationRoutes_CRUD tests the complete CRUD lifecycle for operation routes.
// This test verifies:
// 1. CREATE - POST creates a new operation route with all fields
// 2. READ - GET by ID retrieves and verifies all fields
// 3. UPDATE - PATCH updates title and description
// 4. LIST - GET all lists operation routes
// 5. DELETE - DELETE removes the route
// 6. Verify GET returns 404 after delete
func TestIntegration_OperationRoutes_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("OpRoute CRUD Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("OpRoute CRUD Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Test data
	routeTitle := fmt.Sprintf("CRUD Route %s", h.RandString(6))
	routeDescription := "Integration test operation route for CRUD lifecycle"
	routeCode := fmt.Sprintf("CRUD-%s", h.RandString(4))
	updatedTitle := fmt.Sprintf("Updated CRUD Route %s", h.RandString(6))
	updatedDescription := "Updated description for CRUD lifecycle test"

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - POST creates a new operation route
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := map[string]any{
		"title":         routeTitle,
		"description":   routeDescription,
		"code":          routeCode,
		"operationType": "source",
		"account": map[string]any{
			"ruleType": "alias",
			"validIf":  "@treasury",
		},
		"metadata": map[string]any{
			"environment": "integration-test",
			"testType":    "crud",
		},
	}

	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute operationRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created operation route: %v body=%s", err, string(body))
	}

	routeID := createdRoute.ID
	t.Logf("Created operation route: ID=%s Title=%s Code=%s Type=%s",
		routeID, createdRoute.Title, createdRoute.Code, createdRoute.OperationType)

	// Verify all created route fields
	if createdRoute.Title != routeTitle {
		t.Errorf("route title mismatch: got %q, want %q", createdRoute.Title, routeTitle)
	}
	if createdRoute.Description != routeDescription {
		t.Errorf("route description mismatch: got %q, want %q", createdRoute.Description, routeDescription)
	}
	if createdRoute.Code != routeCode {
		t.Errorf("route code mismatch: got %q, want %q", createdRoute.Code, routeCode)
	}
	if createdRoute.OperationType != "source" {
		t.Errorf("route operationType mismatch: got %q, want %q", createdRoute.OperationType, "source")
	}
	if createdRoute.OrganizationID != orgID {
		t.Errorf("route organizationId mismatch: got %q, want %q", createdRoute.OrganizationID, orgID)
	}
	if createdRoute.LedgerID != ledgerID {
		t.Errorf("route ledgerId mismatch: got %q, want %q", createdRoute.LedgerID, ledgerID)
	}
	if createdRoute.Account == nil {
		t.Errorf("route account rule should not be nil")
	} else {
		if createdRoute.Account.RuleType != "alias" {
			t.Errorf("account ruleType mismatch: got %q, want %q", createdRoute.Account.RuleType, "alias")
		}
	}
	if createdRoute.Metadata == nil {
		t.Errorf("route metadata should not be nil")
	} else {
		if createdRoute.Metadata["environment"] != "integration-test" {
			t.Errorf("metadata environment mismatch: got %v, want %q", createdRoute.Metadata["environment"], "integration-test")
		}
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - GET by ID retrieves and verifies fields
	// ─────────────────────────────────────────────────────────────────────────
	getPath := fmt.Sprintf("%s/%s", path, routeID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET operation route by ID failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetchedRoute operationRouteResponse
	if err := json.Unmarshal(body, &fetchedRoute); err != nil {
		t.Fatalf("parse fetched operation route: %v body=%s", err, string(body))
	}

	if fetchedRoute.ID != routeID {
		t.Errorf("fetched route ID mismatch: got %q, want %q", fetchedRoute.ID, routeID)
	}
	if fetchedRoute.Title != routeTitle {
		t.Errorf("fetched route title mismatch: got %q, want %q", fetchedRoute.Title, routeTitle)
	}
	if fetchedRoute.Description != routeDescription {
		t.Errorf("fetched route description mismatch: got %q, want %q", fetchedRoute.Description, routeDescription)
	}
	if fetchedRoute.Code != routeCode {
		t.Errorf("fetched route code mismatch: got %q, want %q", fetchedRoute.Code, routeCode)
	}

	t.Logf("Fetched operation route: ID=%s Title=%s", fetchedRoute.ID, fetchedRoute.Title)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) UPDATE - PATCH updates title and description
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"title":       updatedTitle,
		"description": updatedDescription,
		"metadata": map[string]any{
			"environment": "integration-test",
			"testType":    "crud",
			"updated":     true,
		},
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRoute operationRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated operation route: %v body=%s", err, string(body))
	}

	if updatedRoute.Title != updatedTitle {
		t.Errorf("updated route title mismatch: got %q, want %q", updatedRoute.Title, updatedTitle)
	}
	if updatedRoute.Description != updatedDescription {
		t.Errorf("updated route description mismatch: got %q, want %q", updatedRoute.Description, updatedDescription)
	}
	// operationType should remain unchanged
	if updatedRoute.OperationType != "source" {
		t.Errorf("updated route operationType should not change: got %q, want %q", updatedRoute.OperationType, "source")
	}

	t.Logf("Updated operation route: ID=%s NewTitle=%s NewDescription=%s", updatedRoute.ID, updatedRoute.Title, updatedRoute.Description)

	// Verify update persisted by fetching again
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET operation route after update failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var verifyRoute operationRouteResponse
	if err := json.Unmarshal(body, &verifyRoute); err != nil {
		t.Fatalf("parse verify operation route: %v body=%s", err, string(body))
	}

	if verifyRoute.Title != updatedTitle {
		t.Errorf("persisted title mismatch: got %q, want %q", verifyRoute.Title, updatedTitle)
	}
	if verifyRoute.Description != updatedDescription {
		t.Errorf("persisted description mismatch: got %q, want %q", verifyRoute.Description, updatedDescription)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 4) LIST - GET all lists operation routes
	// ─────────────────────────────────────────────────────────────────────────
	code, body, err = trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("LIST operation routes failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var routeList operationRoutesListResponse
	if err := json.Unmarshal(body, &routeList); err != nil {
		t.Fatalf("parse operation routes list: %v body=%s", err, string(body))
	}

	found := false
	for _, route := range routeList.Items {
		if route.ID == routeID {
			found = true
			if route.Title != updatedTitle {
				t.Errorf("list route title mismatch: got %q, want %q", route.Title, updatedTitle)
			}
			break
		}
	}
	if !found {
		t.Errorf("created operation route not found in list: ID=%s", routeID)
	}

	t.Logf("List operation routes: found %d routes, target route found=%v", len(routeList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 5) DELETE - DELETE removes the route
	// ─────────────────────────────────────────────────────────────────────────
	code, _, err = trans.Request(ctx, "DELETE", getPath, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE operation route failed: code=%d err=%v", code, err)
	}

	t.Logf("Deleted operation route: ID=%s", routeID)

	// ─────────────────────────────────────────────────────────────────────────
	// 6) Verify GET returns 404 after delete
	// ─────────────────────────────────────────────────────────────────────────
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Logf("GET after delete returned error (expected): %v", err)
	}
	if code != 404 {
		t.Errorf("GET deleted operation route should return 404, got %d: body=%s", code, string(body))
	}

	t.Log("Operation Routes CRUD lifecycle completed successfully")
}

// TestIntegration_OperationRoutes_Validation tests validation error cases for operation routes.
// This test verifies:
// 1. Missing required title field returns 400
// 2. Missing required operationType field returns 400
// 3. Invalid operationType value returns 400
// 4. Title exceeds max length returns 400
// 5. GET non-existent route returns 404
func TestIntegration_OperationRoutes_Validation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("OpRoute Validation Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("OpRoute Validation Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	// ─────────────────────────────────────────────────────────────────────────
	// Validation Test Cases
	// ─────────────────────────────────────────────────────────────────────────
	testCases := []struct {
		name           string
		payload        map[string]any
		expectedStatus int
		description    string
	}{
		{
			name: "missing_required_title",
			payload: map[string]any{
				"operationType": "source",
				"description":   "Route without title",
			},
			expectedStatus: 400,
			description:    "Missing required title field should return 400",
		},
		{
			name: "missing_required_operationType",
			payload: map[string]any{
				"title":       "Route without operationType",
				"description": "Test route",
			},
			expectedStatus: 400,
			description:    "Missing required operationType field should return 400",
		},
		{
			name: "invalid_operationType_value",
			payload: map[string]any{
				"title":         "Route with invalid type",
				"operationType": "invalid_type",
				"description":   "Test route with invalid operationType",
			},
			expectedStatus: 400,
			description:    "Invalid operationType value should return 400",
		},
		{
			name: "title_exceeds_max_length",
			payload: map[string]any{
				"title":         h.RandString(256), // Exceeds typical max length
				"operationType": "source",
				"description":   "Route with excessively long title",
			},
			expectedStatus: 400,
			description:    "Title exceeding max length should return 400",
		},
		{
			name: "empty_title",
			payload: map[string]any{
				"title":         "",
				"operationType": "source",
				"description":   "Route with empty title",
			},
			expectedStatus: 400,
			description:    "Empty title should return 400",
		},
		{
			name: "empty_operationType",
			payload: map[string]any{
				"title":         "Route with empty operationType",
				"operationType": "",
				"description":   "Test route",
			},
			expectedStatus: 400,
			description:    "Empty operationType should return 400",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := trans.Request(ctx, "POST", path, headers, tc.payload)
			if err != nil {
				t.Logf("Request error (may be expected for validation): %v", err)
			}

			if code != tc.expectedStatus {
				t.Errorf("%s: expected %d, got %d: body=%s", tc.description, tc.expectedStatus, code, string(body))
			} else {
				t.Logf("%s: code=%d (expected %d) - PASS", tc.name, code, tc.expectedStatus)
			}
		})
	}

	// ─────────────────────────────────────────────────────────────────────────
	// GET non-existent route returns 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("get_non_existent_route", func(t *testing.T) {
		t.Parallel()
		nonExistentID := "00000000-0000-0000-0000-000000000000"
		getPath := fmt.Sprintf("%s/%s", path, nonExistentID)

		code, body, err := trans.Request(ctx, "GET", getPath, headers, nil)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		if code != 404 {
			t.Errorf("GET non-existent route should return 404, got %d: body=%s", code, string(body))
		} else {
			t.Logf("GET non-existent route: code=%d (expected 404) - PASS", code)
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// PATCH non-existent route returns 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("patch_non_existent_route", func(t *testing.T) {
		t.Parallel()
		nonExistentID := "00000000-0000-0000-0000-000000000001"
		patchPath := fmt.Sprintf("%s/%s", path, nonExistentID)

		updatePayload := map[string]any{
			"title": "Updated Title",
		}

		code, body, err := trans.Request(ctx, "PATCH", patchPath, headers, updatePayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		if code != 404 {
			t.Errorf("PATCH non-existent route should return 404, got %d: body=%s", code, string(body))
		} else {
			t.Logf("PATCH non-existent route: code=%d (expected 404) - PASS", code)
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// DELETE non-existent route returns 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("delete_non_existent_route", func(t *testing.T) {
		t.Parallel()
		nonExistentID := "00000000-0000-0000-0000-000000000002"
		deletePath := fmt.Sprintf("%s/%s", path, nonExistentID)

		code, body, err := trans.Request(ctx, "DELETE", deletePath, headers, nil)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		if code != 404 {
			t.Errorf("DELETE non-existent route should return 404, got %d: body=%s", code, string(body))
		} else {
			t.Logf("DELETE non-existent route: code=%d (expected 404) - PASS", code)
		}
	})

	t.Log("Operation Routes validation tests completed")
}

// TestIntegration_OperationRoutes_DestinationType tests creating a destination operation route.
// This ensures both source and destination types work correctly.
func TestIntegration_OperationRoutes_DestinationType(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("OpRoute Dest Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("OpRoute Dest Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	// Create destination operation route
	createPayload := map[string]any{
		"title":         fmt.Sprintf("Destination Route %s", h.RandString(6)),
		"description":   "Destination operation route for testing",
		"operationType": "destination",
		"metadata": map[string]any{
			"type": "destination",
		},
	}

	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE destination operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute operationRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created operation route: %v body=%s", err, string(body))
	}

	t.Logf("Created destination operation route: ID=%s Title=%s Type=%s",
		createdRoute.ID, createdRoute.Title, createdRoute.OperationType)

	// Verify operationType is destination
	if createdRoute.OperationType != "destination" {
		t.Errorf("route operationType mismatch: got %q, want %q", createdRoute.OperationType, "destination")
	}

	// Cleanup
	t.Cleanup(func() {
		deletePath := fmt.Sprintf("%s/%s", path, createdRoute.ID)
		if _, _, err := trans.Request(ctx, "DELETE", deletePath, headers, nil); err != nil {
			t.Logf("Warning: cleanup delete route failed: %v", err)
		}
	})

	t.Log("Destination operation route test completed successfully")
}

// TestIntegration_OperationRoutes_MetadataHandling tests metadata operations on operation routes.
func TestIntegration_OperationRoutes_MetadataHandling(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("OpRoute Meta Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("OpRoute Meta Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	// Create route with flat metadata (nested objects and arrays are not allowed)
	complexMetadata := map[string]any{
		"string_value": "test",
		"int_value":    42,
		"float_value":  3.14,
		"bool_value":   true,
		"nested_key1":  "value1",
		"nested_key2":  123,
		"array_value":  "a,b,c",
	}

	createPayload := map[string]any{
		"title":         fmt.Sprintf("Metadata Route %s", h.RandString(6)),
		"operationType": "source",
		"metadata":      complexMetadata,
	}

	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE operation route with metadata failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute operationRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created operation route: %v body=%s", err, string(body))
	}

	routeID := createdRoute.ID
	t.Logf("Created operation route with complex metadata: ID=%s", routeID)

	// Cleanup
	t.Cleanup(func() {
		deletePath := fmt.Sprintf("%s/%s", path, routeID)
		if _, _, err := trans.Request(ctx, "DELETE", deletePath, headers, nil); err != nil {
			t.Logf("Warning: cleanup delete route failed: %v", err)
		}
	})

	// Verify metadata was stored
	if createdRoute.Metadata == nil {
		t.Fatalf("metadata should not be nil")
	}

	if createdRoute.Metadata["string_value"] != "test" {
		t.Errorf("metadata string_value mismatch: got %v, want %q", createdRoute.Metadata["string_value"], "test")
	}

	// Update metadata
	getPath := fmt.Sprintf("%s/%s", path, routeID)
	updatePayload := map[string]any{
		"metadata": map[string]any{
			"updated":     true,
			"new_field":   "new_value",
			"string_value": "updated_test",
		},
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH operation route metadata failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRoute operationRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated operation route: %v body=%s", err, string(body))
	}

	if updatedRoute.Metadata == nil {
		t.Fatalf("updated metadata should not be nil")
	}

	if updatedRoute.Metadata["updated"] != true {
		t.Errorf("metadata updated flag mismatch: got %v, want %v", updatedRoute.Metadata["updated"], true)
	}

	t.Log("Metadata handling test completed successfully")
}
