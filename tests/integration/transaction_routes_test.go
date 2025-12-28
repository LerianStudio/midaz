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
	ID              string                     `json:"id"`
	OrganizationID  string                     `json:"organizationId"`
	LedgerID        string                     `json:"ledgerId"`
	Title           string                     `json:"title"`
	Description     string                     `json:"description"`
	OperationRoutes []operationRouteEmbedded   `json:"operationRoutes"`
	Metadata        map[string]any             `json:"metadata"`
	CreatedAt       time.Time                  `json:"createdAt"`
	UpdatedAt       time.Time                  `json:"updatedAt"`
}

// operationRouteEmbedded represents an embedded operation route in transaction route response
type operationRouteEmbedded struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	OperationType string `json:"operationType"`
}

// transactionRoutesListResponse represents paginated transaction routes
type transactionRoutesListResponse struct {
	Items []transactionRouteResponse `json:"items"`
}

// TestIntegration_TransactionRoutes_CRUD tests the complete CRUD lifecycle for transaction routes.
// This test verifies:
// 1. CREATE operation routes as prerequisites (source and destination types)
// 2. CREATE transaction route referencing the operation routes
// 3. READ (GET by ID) retrieves and verifies all fields
// 4. UPDATE (PATCH) updates title and description
// 5. LIST (GET all) lists transaction routes
// 6. DELETE removes the transaction route
// 7. Verify GET returns 404 after delete
func TestIntegration_TransactionRoutes_CRUD(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("TxRoute CRUD Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("TxRoute CRUD Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE prerequisite operation routes (source and destination)
	// ─────────────────────────────────────────────────────────────────────────
	sourceRouteTitle := fmt.Sprintf("Source OpRoute %s", h.RandString(4))
	sourceRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceRouteTitle, "source")
	if err != nil {
		t.Fatalf("create source operation route failed: %v", err)
	}
	t.Logf("Created source operation route: ID=%s Title=%s", sourceRouteID, sourceRouteTitle)

	// Register cleanup for source route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceRouteID); err != nil {
			t.Logf("Warning: cleanup delete source route failed: %v", err)
		}
	})

	destRouteTitle := fmt.Sprintf("Dest OpRoute %s", h.RandString(4))
	destRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, destRouteTitle, "destination")
	if err != nil {
		t.Fatalf("create destination operation route failed: %v", err)
	}
	t.Logf("Created destination operation route: ID=%s Title=%s", destRouteID, destRouteTitle)

	// Register cleanup for destination route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, destRouteID); err != nil {
			t.Logf("Warning: cleanup delete destination route failed: %v", err)
		}
	})

	// Test data
	txRouteTitle := fmt.Sprintf("CRUD TxRoute %s", h.RandString(6))
	txRouteDescription := "Integration test transaction route for CRUD lifecycle"
	updatedTitle := fmt.Sprintf("Updated TxRoute %s", h.RandString(6))
	updatedDescription := "Updated description for CRUD lifecycle test"

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	// ─────────────────────────────────────────────────────────────────────────
	// 2) CREATE - POST creates a new transaction route
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := map[string]any{
		"title":           txRouteTitle,
		"description":     txRouteDescription,
		"operationRoutes": []string{sourceRouteID, destRouteID},
		"metadata": map[string]any{
			"environment": "integration-test",
			"testType":    "crud",
		},
	}

	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute transactionRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created transaction route: %v body=%s", err, string(body))
	}

	txRouteID := createdRoute.ID
	t.Logf("Created transaction route: ID=%s Title=%s OperationRoutes=%d",
		txRouteID, createdRoute.Title, len(createdRoute.OperationRoutes))

	// Verify all created route fields
	if createdRoute.Title != txRouteTitle {
		t.Errorf("route title mismatch: got %q, want %q", createdRoute.Title, txRouteTitle)
	}
	if createdRoute.Description != txRouteDescription {
		t.Errorf("route description mismatch: got %q, want %q", createdRoute.Description, txRouteDescription)
	}
	if createdRoute.OrganizationID != orgID {
		t.Errorf("route organizationId mismatch: got %q, want %q", createdRoute.OrganizationID, orgID)
	}
	if createdRoute.LedgerID != ledgerID {
		t.Errorf("route ledgerId mismatch: got %q, want %q", createdRoute.LedgerID, ledgerID)
	}
	if len(createdRoute.OperationRoutes) != 2 {
		t.Errorf("expected 2 operation routes, got %d", len(createdRoute.OperationRoutes))
	}
	if createdRoute.Metadata == nil {
		t.Errorf("route metadata should not be nil")
	} else {
		if createdRoute.Metadata["environment"] != "integration-test" {
			t.Errorf("metadata environment mismatch: got %v, want %q", createdRoute.Metadata["environment"], "integration-test")
		}
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 3) READ - GET by ID retrieves and verifies fields
	// ─────────────────────────────────────────────────────────────────────────
	getPath := fmt.Sprintf("%s/%s", path, txRouteID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET transaction route by ID failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetchedRoute transactionRouteResponse
	if err := json.Unmarshal(body, &fetchedRoute); err != nil {
		t.Fatalf("parse fetched transaction route: %v body=%s", err, string(body))
	}

	if fetchedRoute.ID != txRouteID {
		t.Errorf("fetched route ID mismatch: got %q, want %q", fetchedRoute.ID, txRouteID)
	}
	if fetchedRoute.Title != txRouteTitle {
		t.Errorf("fetched route title mismatch: got %q, want %q", fetchedRoute.Title, txRouteTitle)
	}
	if fetchedRoute.Description != txRouteDescription {
		t.Errorf("fetched route description mismatch: got %q, want %q", fetchedRoute.Description, txRouteDescription)
	}

	// Verify operation routes are embedded with their details
	if len(fetchedRoute.OperationRoutes) != 2 {
		t.Errorf("fetched route should have 2 operation routes, got %d", len(fetchedRoute.OperationRoutes))
	}

	// Verify each linked operation route
	foundSource, foundDest := false, false
	for _, opRoute := range fetchedRoute.OperationRoutes {
		switch opRoute.ID {
		case sourceRouteID:
			foundSource = true
			if opRoute.OperationType != "source" {
				t.Errorf("source route has wrong type: got %q, want %q", opRoute.OperationType, "source")
			}
		case destRouteID:
			foundDest = true
			if opRoute.OperationType != "destination" {
				t.Errorf("destination route has wrong type: got %q, want %q", opRoute.OperationType, "destination")
			}
		}
	}
	if !foundSource {
		t.Errorf("source operation route not found in transaction route")
	}
	if !foundDest {
		t.Errorf("destination operation route not found in transaction route")
	}

	t.Logf("Fetched transaction route: ID=%s Title=%s OperationRoutes=%d", fetchedRoute.ID, fetchedRoute.Title, len(fetchedRoute.OperationRoutes))

	// ─────────────────────────────────────────────────────────────────────────
	// 4) UPDATE - PATCH updates title and description
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
		t.Fatalf("PATCH transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRoute transactionRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated transaction route: %v body=%s", err, string(body))
	}

	if updatedRoute.Title != updatedTitle {
		t.Errorf("updated route title mismatch: got %q, want %q", updatedRoute.Title, updatedTitle)
	}
	if updatedRoute.Description != updatedDescription {
		t.Errorf("updated route description mismatch: got %q, want %q", updatedRoute.Description, updatedDescription)
	}
	// Operation routes should remain unchanged
	if len(updatedRoute.OperationRoutes) != 2 {
		t.Errorf("operation routes count should not change: got %d, want 2", len(updatedRoute.OperationRoutes))
	}

	t.Logf("Updated transaction route: ID=%s NewTitle=%s NewDescription=%s", updatedRoute.ID, updatedRoute.Title, updatedRoute.Description)

	// Verify update persisted by fetching again
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET transaction route after update failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var verifyRoute transactionRouteResponse
	if err := json.Unmarshal(body, &verifyRoute); err != nil {
		t.Fatalf("parse verify transaction route: %v body=%s", err, string(body))
	}

	if verifyRoute.Title != updatedTitle {
		t.Errorf("persisted title mismatch: got %q, want %q", verifyRoute.Title, updatedTitle)
	}
	if verifyRoute.Description != updatedDescription {
		t.Errorf("persisted description mismatch: got %q, want %q", verifyRoute.Description, updatedDescription)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 5) LIST - GET all lists transaction routes
	// ─────────────────────────────────────────────────────────────────────────
	code, body, err = trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("LIST transaction routes failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var routeList transactionRoutesListResponse
	if err := json.Unmarshal(body, &routeList); err != nil {
		t.Fatalf("parse transaction routes list: %v body=%s", err, string(body))
	}

	found := false
	for _, route := range routeList.Items {
		if route.ID == txRouteID {
			found = true
			if route.Title != updatedTitle {
				t.Errorf("list route title mismatch: got %q, want %q", route.Title, updatedTitle)
			}
			break
		}
	}
	if !found {
		t.Errorf("created transaction route not found in list: ID=%s", txRouteID)
	}

	t.Logf("List transaction routes: found %d routes, target route found=%v", len(routeList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 6) DELETE - DELETE removes the route
	// ─────────────────────────────────────────────────────────────────────────
	code, _, err = trans.Request(ctx, "DELETE", getPath, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE transaction route failed: code=%d err=%v", code, err)
	}

	t.Logf("Deleted transaction route: ID=%s", txRouteID)

	// ─────────────────────────────────────────────────────────────────────────
	// 7) Verify GET returns 404 after delete
	// ─────────────────────────────────────────────────────────────────────────
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Logf("GET after delete returned error (expected): %v", err)
	}
	if code != 404 {
		t.Errorf("GET deleted transaction route should return 404, got %d: body=%s", code, string(body))
	}

	t.Log("Transaction Routes CRUD lifecycle completed successfully")
}

// TestIntegration_TransactionRoutes_Validation tests validation error cases for transaction routes.
// This test verifies:
// 1. Missing required title field returns 400
// 2. Missing required operationRoutes field returns 400
// 3. Invalid UUID in operationRoutes returns 400
// 4. Non-existent operation route UUID returns 400 or 404
// 5. GET non-existent transaction route returns 404
func TestIntegration_TransactionRoutes_Validation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("TxRoute Validation Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("TxRoute Validation Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create one valid operation route for mixed tests
	validRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Valid Route %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("create valid operation route failed: %v", err)
	}
	t.Logf("Created valid operation route: ID=%s", validRouteID)

	// Register cleanup for valid route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, validRouteID); err != nil {
			t.Logf("Warning: cleanup delete valid route failed: %v", err)
		}
	})

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

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
				"operationRoutes": []string{validRouteID},
				"description":     "Route without title",
			},
			expectedStatus: 400,
			description:    "Missing required title field should return 400",
		},
		{
			name: "missing_required_operationRoutes",
			payload: map[string]any{
				"title":       "Route without operationRoutes",
				"description": "Test route without operation routes array",
			},
			expectedStatus: 400,
			description:    "Missing required operationRoutes field should return 400",
		},
		{
			name: "empty_operationRoutes_array",
			payload: map[string]any{
				"title":           "Route with empty operationRoutes",
				"operationRoutes": []string{},
				"description":     "Test route with empty operation routes array",
			},
			expectedStatus: 400,
			description:    "Empty operationRoutes array should return 400",
		},
		{
			name: "invalid_uuid_in_operationRoutes",
			payload: map[string]any{
				"title":           "Route with invalid UUID",
				"operationRoutes": []string{"not-a-valid-uuid", "also-invalid"},
				"description":     "Test route with invalid UUIDs in operationRoutes",
			},
			expectedStatus: 400,
			description:    "Invalid UUID in operationRoutes should return 400",
		},
		{
			name: "empty_title",
			payload: map[string]any{
				"title":           "",
				"operationRoutes": []string{validRouteID},
				"description":     "Route with empty title",
			},
			expectedStatus: 400,
			description:    "Empty title should return 400",
		},
		{
			name: "title_exceeds_max_length",
			payload: map[string]any{
				"title":           h.RandString(256), // Exceeds typical max length
				"operationRoutes": []string{validRouteID},
				"description":     "Route with excessively long title",
			},
			expectedStatus: 400,
			description:    "Title exceeding max length should return 400",
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
	// Non-existent operation route UUID should fail
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("non_existent_operation_route", func(t *testing.T) {
		t.Parallel()
		nonExistentOpRouteID := "00000000-0000-0000-0000-000000000000"
		createPayload := map[string]any{
			"title":           fmt.Sprintf("TxRoute %s", h.RandString(6)),
			"operationRoutes": []string{nonExistentOpRouteID},
			"description":     "Route referencing non-existent operation route",
		}

		code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		// Should NOT succeed with non-existent operation route reference
		if code == 201 {
			t.Errorf("expected failure for non-existent operation route, but got 201 Created: body=%s", string(body))
			// Cleanup if accidentally created
			var created transactionRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				deletePath := fmt.Sprintf("%s/%s", path, created.ID)
				if _, _, delErr := trans.Request(ctx, "DELETE", deletePath, headers, nil); delErr != nil {
					t.Logf("Warning: cleanup delete accidental transaction route failed: %v", delErr)
				}
			}
		} else {
			t.Logf("non_existent_operation_route: correctly rejected with code=%d", code)
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Mix of valid and non-existent operation routes should fail
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("mixed_valid_and_non_existent_operation_routes", func(t *testing.T) {
		t.Parallel()
		nonExistentOpRouteID := "11111111-1111-1111-1111-111111111111"
		createPayload := map[string]any{
			"title":           fmt.Sprintf("TxRoute %s", h.RandString(6)),
			"operationRoutes": []string{validRouteID, nonExistentOpRouteID},
			"description":     "Route with mix of valid and non-existent operation routes",
		}

		code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		// Should NOT succeed when any operation route is non-existent
		if code == 201 {
			t.Errorf("expected failure for mixed operation routes, but got 201 Created: body=%s", string(body))
			// Cleanup if accidentally created
			var created transactionRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				deletePath := fmt.Sprintf("%s/%s", path, created.ID)
				if _, _, delErr := trans.Request(ctx, "DELETE", deletePath, headers, nil); delErr != nil {
					t.Logf("Warning: cleanup delete accidental transaction route failed: %v", delErr)
				}
			}
		} else {
			t.Logf("mixed_valid_and_non_existent_operation_routes: correctly rejected with code=%d", code)
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// GET non-existent transaction route returns 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("get_non_existent_transaction_route", func(t *testing.T) {
		t.Parallel()
		nonExistentID := "00000000-0000-0000-0000-000000000000"
		getPath := fmt.Sprintf("%s/%s", path, nonExistentID)

		code, body, err := trans.Request(ctx, "GET", getPath, headers, nil)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		if code != 404 {
			t.Errorf("GET non-existent transaction route should return 404, got %d: body=%s", code, string(body))
		} else {
			t.Logf("GET non-existent transaction route: code=%d (expected 404) - PASS", code)
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// PATCH non-existent transaction route returns 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("patch_non_existent_transaction_route", func(t *testing.T) {
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
			t.Errorf("PATCH non-existent transaction route should return 404, got %d: body=%s", code, string(body))
		} else {
			t.Logf("PATCH non-existent transaction route: code=%d (expected 404) - PASS", code)
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// DELETE non-existent transaction route returns 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("delete_non_existent_transaction_route", func(t *testing.T) {
		t.Parallel()
		nonExistentID := "00000000-0000-0000-0000-000000000002"
		deletePath := fmt.Sprintf("%s/%s", path, nonExistentID)

		code, body, err := trans.Request(ctx, "DELETE", deletePath, headers, nil)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		if code != 404 {
			t.Errorf("DELETE non-existent transaction route should return 404, got %d: body=%s", code, string(body))
		} else {
			t.Logf("DELETE non-existent transaction route: code=%d (expected 404) - PASS", code)
		}
	})

	t.Log("Transaction Routes validation tests completed")
}

// TestIntegration_TransactionRoutes_MetadataHandling tests metadata operations on transaction routes.
func TestIntegration_TransactionRoutes_MetadataHandling(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("TxRoute Meta Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("TxRoute Meta Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create operation route as prerequisite
	opRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Meta OpRoute %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("create operation route failed: %v", err)
	}
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, opRouteID); err != nil {
			t.Logf("Warning: cleanup delete operation route failed: %v", err)
		}
	})

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	// Create route with complex metadata
	complexMetadata := map[string]any{
		"string_value": "test",
		"int_value":    42,
		"float_value":  3.14,
		"bool_value":   true,
		"nested": map[string]any{
			"key1": "value1",
			"key2": 123,
		},
		"array_value": []any{"a", "b", "c"},
	}

	createPayload := map[string]any{
		"title":           fmt.Sprintf("Metadata TxRoute %s", h.RandString(6)),
		"operationRoutes": []string{opRouteID},
		"metadata":        complexMetadata,
	}

	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE transaction route with metadata failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute transactionRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created transaction route: %v body=%s", err, string(body))
	}

	txRouteID := createdRoute.ID
	t.Logf("Created transaction route with complex metadata: ID=%s", txRouteID)

	// Cleanup
	t.Cleanup(func() {
		deletePath := fmt.Sprintf("%s/%s", path, txRouteID)
		if _, _, err := trans.Request(ctx, "DELETE", deletePath, headers, nil); err != nil {
			t.Logf("Warning: cleanup delete transaction route failed: %v", err)
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
	getPath := fmt.Sprintf("%s/%s", path, txRouteID)
	updatePayload := map[string]any{
		"metadata": map[string]any{
			"updated":      true,
			"new_field":    "new_value",
			"string_value": "updated_test",
		},
	}

	code, body, err = trans.Request(ctx, "PATCH", getPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH transaction route metadata failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRoute transactionRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated transaction route: %v body=%s", err, string(body))
	}

	if updatedRoute.Metadata == nil {
		t.Fatalf("updated metadata should not be nil")
	}

	if updatedRoute.Metadata["updated"] != true {
		t.Errorf("metadata updated flag mismatch: got %v, want %v", updatedRoute.Metadata["updated"], true)
	}

	t.Log("Metadata handling test completed successfully")
}

// TestIntegration_TransactionRoutes_MultipleOperationRoutes tests transaction routes with multiple operation routes.
func TestIntegration_TransactionRoutes_MultipleOperationRoutes(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("TxRoute Multi Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("TxRoute Multi Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create multiple operation routes (1 source, 2 destinations)
	sourceTitle := fmt.Sprintf("Source %s", h.RandString(4))
	sourceRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceTitle, "source")
	if err != nil {
		t.Fatalf("create source route failed: %v", err)
	}
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceRouteID); err != nil {
			t.Logf("Warning: cleanup delete source route failed: %v", err)
		}
	})

	dest1Title := fmt.Sprintf("Dest1 %s", h.RandString(4))
	dest1RouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, dest1Title, "destination")
	if err != nil {
		t.Fatalf("create dest1 route failed: %v", err)
	}
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, dest1RouteID); err != nil {
			t.Logf("Warning: cleanup delete dest1 route failed: %v", err)
		}
	})

	dest2Title := fmt.Sprintf("Dest2 %s", h.RandString(4))
	dest2RouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, dest2Title, "destination")
	if err != nil {
		t.Fatalf("create dest2 route failed: %v", err)
	}
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, dest2RouteID); err != nil {
			t.Logf("Warning: cleanup delete dest2 route failed: %v", err)
		}
	})

	// Create transaction route linking all three operation routes
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)
	createPayload := map[string]any{
		"title":           fmt.Sprintf("Multi Link TxRoute %s", h.RandString(4)),
		"operationRoutes": []string{sourceRouteID, dest1RouteID, dest2RouteID},
		"description":     "Transaction route with multiple operation routes",
	}

	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute transactionRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created transaction route: %v body=%s", err, string(body))
	}

	txRouteID := createdRoute.ID
	t.Logf("Created transaction route with 3 operation routes: ID=%s", txRouteID)

	// Cleanup
	t.Cleanup(func() {
		deletePath := fmt.Sprintf("%s/%s", path, txRouteID)
		if _, _, err := trans.Request(ctx, "DELETE", deletePath, headers, nil); err != nil {
			t.Logf("Warning: cleanup delete transaction route failed: %v", err)
		}
	})

	// Fetch and verify all linked operation routes
	getPath := fmt.Sprintf("%s/%s", path, txRouteID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetchedRoute transactionRouteResponse
	if err := json.Unmarshal(body, &fetchedRoute); err != nil {
		t.Fatalf("parse fetched transaction route: %v body=%s", err, string(body))
	}

	if len(fetchedRoute.OperationRoutes) != 3 {
		t.Errorf("expected 3 linked operation routes, got %d", len(fetchedRoute.OperationRoutes))
	}

	// Verify each linked operation route
	foundSource, foundDest1, foundDest2 := false, false, false
	for _, opRoute := range fetchedRoute.OperationRoutes {
		switch opRoute.ID {
		case sourceRouteID:
			foundSource = true
			if opRoute.OperationType != "source" {
				t.Errorf("source route has wrong type: got %q", opRoute.OperationType)
			}
			if opRoute.Title != sourceTitle {
				t.Errorf("source route title mismatch: got %q, want %q", opRoute.Title, sourceTitle)
			}
		case dest1RouteID:
			foundDest1 = true
			if opRoute.OperationType != "destination" {
				t.Errorf("dest1 route has wrong type: got %q", opRoute.OperationType)
			}
		case dest2RouteID:
			foundDest2 = true
			if opRoute.OperationType != "destination" {
				t.Errorf("dest2 route has wrong type: got %q", opRoute.OperationType)
			}
		}
	}

	if !foundSource || !foundDest1 || !foundDest2 {
		t.Errorf("not all operation routes found in linkage: source=%v, dest1=%v, dest2=%v", foundSource, foundDest1, foundDest2)
	}

	t.Logf("Transaction route linkage verified: 3 operation routes correctly embedded")
}
