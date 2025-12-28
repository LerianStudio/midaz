package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Routing_TransactionRouteCRUDLifecycle tests the complete CRUD lifecycle
// for Transaction Routes. Transaction routes link multiple operation routes together.
func TestIntegration_Routing_TransactionRouteCRUDLifecycle(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Operation Routes
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("TxRoute Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("TxRoute Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create source operation route
	sourceRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Source %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("create source operation route failed: %v", err)
	}
	t.Logf("Created source operation route: ID=%s", sourceRouteID)

	// Register cleanup for source route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceRouteID); err != nil {
			t.Logf("Warning: cleanup delete source route failed: %v", err)
		}
	})

	// Create destination operation route
	destRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Dest %s", h.RandString(4)), "destination")
	if err != nil {
		t.Fatalf("create destination operation route failed: %v", err)
	}
	t.Logf("Created destination operation route: ID=%s", destRouteID)

	// Register cleanup for destination route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, destRouteID); err != nil {
			t.Logf("Warning: cleanup delete destination route failed: %v", err)
		}
	})

	// Test data
	txRouteTitle := fmt.Sprintf("Transaction Route %s", h.RandString(6))
	updatedTitle := fmt.Sprintf("Updated TxRoute %s", h.RandString(6))

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - Transaction Route linking Operation Routes
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := h.CreateTransactionRoutePayload(txRouteTitle, []string{sourceRouteID, destRouteID})
	createPayload["description"] = "Test transaction route for integration testing"
	createPayload["metadata"] = map[string]any{"environment": "test", "type": "transfer"}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE transaction route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute h.TransactionRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created transaction route: %v body=%s", err, string(body))
	}

	txRouteID := createdRoute.ID
	t.Logf("Created transaction route: ID=%s Title=%s OperationRoutes=%d",
		txRouteID, createdRoute.Title, len(createdRoute.OperationRoutes))

	// Verify created route fields
	if createdRoute.Title != txRouteTitle {
		t.Errorf("route title mismatch: got %q, want %q", createdRoute.Title, txRouteTitle)
	}
	if createdRoute.OrganizationID != orgID {
		t.Errorf("route organization ID mismatch: got %q, want %q", createdRoute.OrganizationID, orgID)
	}
	if createdRoute.LedgerID != ledgerID {
		t.Errorf("route ledger ID mismatch: got %q, want %q", createdRoute.LedgerID, ledgerID)
	}
	if len(createdRoute.OperationRoutes) != 2 {
		t.Errorf("expected 2 operation routes, got %d", len(createdRoute.OperationRoutes))
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - Get Transaction Route by ID
	// ─────────────────────────────────────────────────────────────────────────
	fetchedRoute, err := h.GetTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID)
	if err != nil {
		t.Fatalf("GET transaction route by ID failed: %v", err)
	}

	if fetchedRoute.ID != txRouteID {
		t.Errorf("fetched route ID mismatch: got %q, want %q", fetchedRoute.ID, txRouteID)
	}
	if fetchedRoute.Title != txRouteTitle {
		t.Errorf("fetched route title mismatch: got %q, want %q", fetchedRoute.Title, txRouteTitle)
	}

	// Verify operation routes are populated
	if len(fetchedRoute.OperationRoutes) != 2 {
		t.Errorf("fetched route should have 2 operation routes, got %d", len(fetchedRoute.OperationRoutes))
	}

	t.Logf("Fetched transaction route: ID=%s Title=%s OperationRoutes=%d",
		fetchedRoute.ID, fetchedRoute.Title, len(fetchedRoute.OperationRoutes))

	// ─────────────────────────────────────────────────────────────────────────
	// 3) LIST - Get All Transaction Routes (verify our route appears)
	// ─────────────────────────────────────────────────────────────────────────
	routeList, err := h.ListTransactionRoutes(ctx, trans, headers, orgID, ledgerID)
	if err != nil {
		t.Fatalf("LIST transaction routes failed: %v", err)
	}

	found := false
	for _, route := range routeList.Items {
		if route.ID == txRouteID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created transaction route not found in list: ID=%s", txRouteID)
	}

	t.Logf("List transaction routes: found %d routes, target route found=%v", len(routeList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) UPDATE - Modify Transaction Route Title
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"title":       updatedTitle,
		"description": "Updated description for integration testing",
		"metadata": map[string]any{
			"environment": "test",
			"type":        "transfer",
			"updated":     true,
		},
	}

	updatedRoute, err := h.UpdateTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID, updatePayload)
	if err != nil {
		t.Fatalf("UPDATE transaction route failed: %v", err)
	}

	if updatedRoute.Title != updatedTitle {
		t.Errorf("updated route title mismatch: got %q, want %q", updatedRoute.Title, updatedTitle)
	}

	t.Logf("Updated transaction route: ID=%s NewTitle=%s", updatedRoute.ID, updatedRoute.Title)

	// Verify update persisted by fetching again
	verifyRoute, err := h.GetTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID)
	if err != nil {
		t.Fatalf("GET transaction route after update failed: %v", err)
	}
	if verifyRoute.Title != updatedTitle {
		t.Errorf("persisted title mismatch: got %q, want %q", verifyRoute.Title, updatedTitle)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 5) DELETE - Remove Transaction Route
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID)
	if err != nil {
		t.Fatalf("DELETE transaction route failed: %v", err)
	}

	t.Logf("Deleted transaction route: ID=%s", txRouteID)

	// Verify deletion - GET should fail
	_, err = h.GetTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID)
	if err == nil {
		t.Errorf("GET deleted transaction route should fail, but succeeded")
	}

	t.Log("Transaction Route CRUD lifecycle completed successfully")
}

// TestIntegration_Routing_TransactionRouteOperationRouteLinkage tests that transaction routes
// correctly reference and embed their linked operation routes.
func TestIntegration_Routing_TransactionRouteOperationRouteLinkage(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Linkage Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Linkage Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create three operation routes
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

	// Create transaction route linking all three
	txRouteID, err := h.SetupTransactionRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Multi Link %s", h.RandString(4)), []string{sourceRouteID, dest1RouteID, dest2RouteID})
	if err != nil {
		t.Fatalf("create transaction route failed: %v", err)
	}
	t.Cleanup(func() {
		if err := h.DeleteTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID); err != nil {
			t.Logf("Warning: cleanup delete transaction route failed: %v", err)
		}
	})

	// Fetch and verify linkage
	txRoute, err := h.GetTransactionRoute(ctx, trans, headers, orgID, ledgerID, txRouteID)
	if err != nil {
		t.Fatalf("get transaction route failed: %v", err)
	}

	if len(txRoute.OperationRoutes) != 3 {
		t.Errorf("expected 3 linked operation routes, got %d", len(txRoute.OperationRoutes))
	}

	// Verify each linked operation route has correct data
	foundSource, foundDest1, foundDest2 := false, false, false
	for _, opRoute := range txRoute.OperationRoutes {
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

// TestIntegration_Routing_TransactionRouteValidation tests validation errors for transaction route creation.
func TestIntegration_Routing_TransactionRouteValidation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Validation Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Validation Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "missing title",
			payload: map[string]any{"operationRoutes": []string{"some-uuid"}},
		},
		{
			name:    "missing operation routes",
			payload: map[string]any{"title": "Test Route"},
		},
		{
			name:    "empty operation routes array",
			payload: map[string]any{"title": "Test Route", "operationRoutes": []string{}},
		},
		{
			name:    "title exceeds max (51 chars, max is 50)",
			payload: map[string]any{"title": h.RandString(51), "operationRoutes": []string{"some-uuid"}},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := trans.Request(ctx, "POST", path, headers, tc.payload)
			if err != nil {
				t.Logf("Request error (expected for validation): %v", err)
			}
			// Expect 400 Bad Request for validation errors
			if code != 400 {
				t.Errorf("expected 400 Bad Request for %s, but got %d: body=%s", tc.name, code, string(body))
			}
			t.Logf("Validation test %s: code=%d (expected 400)", tc.name, code)
		})
	}
}

// TestIntegration_Routing_TransactionRouteInvalidOperationRouteRef tests that creating a
// transaction route with non-existent operation route IDs fails appropriately.
func TestIntegration_Routing_TransactionRouteInvalidOperationRouteRef(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup organization and ledger
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Invalid OpRoute Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Invalid OpRoute Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create one valid operation route
	validRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Valid Route %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("create valid operation route failed: %v", err)
	}
	t.Logf("Created valid operation route: ID=%s", validRouteID)

	// Generate a non-existent UUID for testing
	nonExistentRouteID := "00000000-0000-0000-0000-000000000000"

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	testCases := []struct {
		name              string
		operationRouteIDs []string
		description       string
	}{
		{
			name:              "all non-existent operation routes",
			operationRouteIDs: []string{nonExistentRouteID, "11111111-1111-1111-1111-111111111111"},
			description:       "should fail when all operation route IDs are non-existent",
		},
		{
			name:              "mix of valid and non-existent operation routes",
			operationRouteIDs: []string{validRouteID, nonExistentRouteID},
			description:       "should fail when any operation route ID is non-existent",
		},
		{
			name:              "single non-existent operation route",
			operationRouteIDs: []string{nonExistentRouteID},
			description:       "should fail with single non-existent operation route",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			createPayload := h.CreateTransactionRoutePayload(
				fmt.Sprintf("TxRoute %s", h.RandString(6)),
				tc.operationRouteIDs,
			)
			createPayload["description"] = tc.description

			code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
			if err != nil {
				t.Logf("Request error (may be expected): %v", err)
			}

			// Should NOT succeed with non-existent operation route references
			if code == 201 {
				t.Errorf("%s: expected failure, but got 201 Created: body=%s", tc.description, string(body))
				// Cleanup if accidentally created
				var created h.TransactionRouteResponse
				if json.Unmarshal(body, &created) == nil && created.ID != "" {
					if delErr := h.DeleteTransactionRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
						t.Logf("Warning: cleanup delete accidental transaction route failed: %v", delErr)
					}
				}
			} else {
				t.Logf("%s: correctly rejected with code=%d", tc.description, code)
			}
		})
	}

	// Cleanup
	if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, validRouteID); err != nil {
		t.Logf("Warning: cleanup delete valid route failed: %v", err)
	}

	t.Log("Invalid operation route reference test completed")
}
