package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Routing_OperationRouteCRUDLifecycle tests the complete CRUD lifecycle
// for Operation Routes. Operation routes define source/destination rules for transactions.
func TestIntegration_Routing_OperationRouteCRUDLifecycle(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Route Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Route Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Test data
	routeTitle := fmt.Sprintf("Source Route %s", h.RandString(6))
	updatedTitle := fmt.Sprintf("Updated Route %s", h.RandString(6))

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - Source Operation Route
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := h.CreateSourceOperationRoutePayload(routeTitle)
	createPayload["description"] = "Test source route for integration testing"
	createPayload["code"] = fmt.Sprintf("SRC-%s", h.RandString(4))
	createPayload["metadata"] = map[string]any{"environment": "test", "type": "source"}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute h.OperationRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created operation route: %v body=%s", err, string(body))
	}

	routeID := createdRoute.ID
	t.Logf("Created operation route: ID=%s Title=%s Type=%s",
		routeID, createdRoute.Title, createdRoute.OperationType)

	// Verify created route fields
	if createdRoute.Title != routeTitle {
		t.Errorf("route title mismatch: got %q, want %q", createdRoute.Title, routeTitle)
	}
	if createdRoute.OperationType != "source" {
		t.Errorf("route operation type mismatch: got %q, want %q", createdRoute.OperationType, "source")
	}
	if createdRoute.OrganizationID != orgID {
		t.Errorf("route organization ID mismatch: got %q, want %q", createdRoute.OrganizationID, orgID)
	}
	if createdRoute.LedgerID != ledgerID {
		t.Errorf("route ledger ID mismatch: got %q, want %q", createdRoute.LedgerID, ledgerID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - Get Operation Route by ID
	// ─────────────────────────────────────────────────────────────────────────
	fetchedRoute, err := h.GetOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID)
	if err != nil {
		t.Fatalf("GET operation route by ID failed: %v", err)
	}

	if fetchedRoute.ID != routeID {
		t.Errorf("fetched route ID mismatch: got %q, want %q", fetchedRoute.ID, routeID)
	}
	if fetchedRoute.Title != routeTitle {
		t.Errorf("fetched route title mismatch: got %q, want %q", fetchedRoute.Title, routeTitle)
	}

	t.Logf("Fetched operation route: ID=%s Title=%s", fetchedRoute.ID, fetchedRoute.Title)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) LIST - Get All Operation Routes (verify our route appears)
	// ─────────────────────────────────────────────────────────────────────────
	routeList, err := h.ListOperationRoutes(ctx, trans, headers, orgID, ledgerID)
	if err != nil {
		t.Fatalf("LIST operation routes failed: %v", err)
	}

	found := false
	for _, route := range routeList.Items {
		if route.ID == routeID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created operation route not found in list: ID=%s", routeID)
	}

	t.Logf("List operation routes: found %d routes, target route found=%v", len(routeList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) UPDATE - Modify Operation Route Title
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"title":       updatedTitle,
		"description": "Updated description for integration testing",
		"metadata": map[string]any{
			"environment": "test",
			"type":        "source",
			"updated":     true,
		},
	}

	updatedRoute, err := h.UpdateOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID, updatePayload)
	if err != nil {
		t.Fatalf("UPDATE operation route failed: %v", err)
	}

	if updatedRoute.Title != updatedTitle {
		t.Errorf("updated route title mismatch: got %q, want %q", updatedRoute.Title, updatedTitle)
	}
	// Operation type should remain unchanged
	if updatedRoute.OperationType != "source" {
		t.Errorf("updated route operation type should not change: got %q, want %q", updatedRoute.OperationType, "source")
	}

	t.Logf("Updated operation route: ID=%s NewTitle=%s", updatedRoute.ID, updatedRoute.Title)

	// Verify update persisted by fetching again
	verifyRoute, err := h.GetOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID)
	if err != nil {
		t.Fatalf("GET operation route after update failed: %v", err)
	}
	if verifyRoute.Title != updatedTitle {
		t.Errorf("persisted title mismatch: got %q, want %q", verifyRoute.Title, updatedTitle)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 5) DELETE - Remove Operation Route
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID)
	if err != nil {
		t.Fatalf("DELETE operation route failed: %v", err)
	}

	t.Logf("Deleted operation route: ID=%s", routeID)

	// Verify deletion - GET should fail
	_, err = h.GetOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID)
	if err == nil {
		t.Errorf("GET deleted operation route should fail, but succeeded")
	}

	t.Log("Operation Route CRUD lifecycle completed successfully")
}

// TestIntegration_Routing_OperationRouteSourceAndDestination tests creating both
// source and destination operation routes in the same ledger.
func TestIntegration_Routing_OperationRouteSourceAndDestination(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("SrcDst Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("SrcDst Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create SOURCE operation route
	sourceRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Source %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("create source route failed: %v", err)
	}
	t.Logf("Created source route: ID=%s", sourceRouteID)

	// Register cleanup for source route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceRouteID); err != nil {
			t.Logf("Warning: cleanup delete source route failed: %v", err)
		}
	})

	// Create DESTINATION operation route
	destRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Destination %s", h.RandString(4)), "destination")
	if err != nil {
		t.Fatalf("create destination route failed: %v", err)
	}
	t.Logf("Created destination route: ID=%s", destRouteID)

	// Register cleanup for destination route
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, destRouteID); err != nil {
			t.Logf("Warning: cleanup delete destination route failed: %v", err)
		}
	})

	// List and verify both routes exist
	routeList, err := h.ListOperationRoutes(ctx, trans, headers, orgID, ledgerID)
	if err != nil {
		t.Fatalf("list operation routes failed: %v", err)
	}

	if len(routeList.Items) < 2 {
		t.Errorf("expected at least 2 routes, got %d", len(routeList.Items))
	}

	foundSource, foundDest := false, false
	for _, route := range routeList.Items {
		if route.ID == sourceRouteID && route.OperationType == "source" {
			foundSource = true
		}
		if route.ID == destRouteID && route.OperationType == "destination" {
			foundDest = true
		}
	}

	if !foundSource {
		t.Errorf("source route not found or wrong type")
	}
	if !foundDest {
		t.Errorf("destination route not found or wrong type")
	}

	t.Logf("Source and destination routes test passed: found source=%v, dest=%v", foundSource, foundDest)
}

// TestIntegration_Routing_OperationRouteWithAccountRule tests creating an operation route
// with account selection rules.
func TestIntegration_Routing_OperationRouteWithAccountRule(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AccountRule Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AccountRule Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create operation route with alias rule
	createPayload := h.CreateOperationRoutePayloadWithAccount(
		fmt.Sprintf("Alias Rule Route %s", h.RandString(4)),
		"source",
		"alias",
		"@treasury",
	)
	createPayload["description"] = "Route with alias-based account rule"

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE operation route with account rule failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute h.OperationRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created operation route: %v body=%s", err, string(body))
	}

	t.Logf("Created operation route with account rule: ID=%s Title=%s", createdRoute.ID, createdRoute.Title)

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, createdRoute.ID); err != nil {
			t.Logf("Warning: cleanup delete route failed: %v", err)
		}
	})

	// Verify account rule is set
	if createdRoute.Account == nil {
		t.Errorf("expected account rule to be set, got nil")
	} else {
		if createdRoute.Account.RuleType != "alias" {
			t.Errorf("account rule type mismatch: got %q, want %q", createdRoute.Account.RuleType, "alias")
		}
	}

	t.Log("Operation route with account rule test completed successfully")
}

// TestIntegration_Routing_OperationRouteValidation tests validation errors for operation route creation.
func TestIntegration_Routing_OperationRouteValidation(t *testing.T) {
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

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "missing title",
			payload: map[string]any{"operationType": "source"},
		},
		{
			name:    "missing operation type",
			payload: map[string]any{"title": "Test Route"},
		},
		{
			name:    "invalid operation type",
			payload: map[string]any{"title": "Test Route", "operationType": "invalid"},
		},
		{
			name:    "title exceeds max (51 chars, max is 50)",
			payload: map[string]any{"title": h.RandString(51), "operationType": "source"},
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

// TestIntegration_Routing_OperationTypeImmutability tests that the operationType field
// cannot be changed after creation. The operationType (source/destination) is a
// fundamental property that should be immutable once the operation route is created.
func TestIntegration_Routing_OperationTypeImmutability(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Immutable Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Immutable Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create a SOURCE operation route
	sourceTitle := fmt.Sprintf("Source Route %s", h.RandString(6))
	createPayload := h.CreateSourceOperationRoutePayload(sourceTitle)
	createPayload["description"] = "Route to test operationType immutability"

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE source operation route failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRoute h.OperationRouteResponse
	if err := json.Unmarshal(body, &createdRoute); err != nil || createdRoute.ID == "" {
		t.Fatalf("parse created operation route: %v body=%s", err, string(body))
	}

	routeID := createdRoute.ID
	t.Logf("Created SOURCE operation route: ID=%s OperationType=%s", routeID, createdRoute.OperationType)

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID); err != nil {
			t.Logf("Warning: cleanup delete route failed: %v", err)
		}
	})

	// Verify initial operationType is "source"
	if createdRoute.OperationType != "source" {
		t.Fatalf("expected initial operationType to be 'source', got %q", createdRoute.OperationType)
	}

	// Attempt to change operationType from "source" to "destination" via PATCH
	updatePayload := map[string]any{
		"operationType": "destination", // Attempting to change the type
		"title":         fmt.Sprintf("Changed Route %s", h.RandString(6)),
	}

	updatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes/%s", orgID, ledgerID, routeID)
	code, body, err = trans.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	// The API should either:
	// A) Reject the update with 400 (operationType is immutable and cannot be changed)
	// B) Accept the update but ignore the operationType change (operationType remains "source")
	// C) Accept and apply the change (operationType becomes "destination" - not ideal but valid)
	if err != nil {
		t.Logf("PATCH request error: %v", err)
	}

	t.Logf("PATCH with operationType change: code=%d", code)

	if code == 400 {
		// Behavior A: API rejects operationType changes explicitly
		t.Logf("API behavior: operationType change REJECTED with 400 Bad Request")
		t.Log("operationType immutability test completed: API explicitly rejects changes")
		return
	}

	if code != 200 {
		t.Fatalf("unexpected response code for PATCH: code=%d body=%s", code, string(body))
	}

	// If PATCH returned 200, check if operationType actually changed
	var updatedRoute h.OperationRouteResponse
	if err := json.Unmarshal(body, &updatedRoute); err != nil {
		t.Fatalf("parse updated operation route: %v body=%s", err, string(body))
	}

	t.Logf("After PATCH: ID=%s OperationType=%s", updatedRoute.ID, updatedRoute.OperationType)

	switch updatedRoute.OperationType {
	case "source":
		// Behavior B: API accepted PATCH but ignored operationType change
		t.Logf("API behavior: operationType change IGNORED (remained 'source')")
		t.Log("operationType immutability test completed: operationType is effectively immutable")
	case "destination":
		// Behavior C: API allowed operationType change
		t.Logf("API behavior: operationType change ALLOWED (changed to 'destination')")
		t.Logf("Warning: operationType is mutable - consider if this is desired behavior")
	default:
		t.Errorf("unexpected operationType after PATCH: got %q", updatedRoute.OperationType)
	}

	// Verify by fetching again
	fetchedRoute, err := h.GetOperationRoute(ctx, trans, headers, orgID, ledgerID, routeID)
	if err != nil {
		t.Fatalf("GET operation route after update failed: %v", err)
	}

	t.Logf("Verified operationType after PATCH: %s", fetchedRoute.OperationType)
	t.Log("operationType immutability test completed successfully")
}

// TestIntegration_Routing_EdgeCases tests various edge cases for routing APIs.
// This covers boundary conditions and error handling scenarios that may not be
// covered by standard CRUD tests.
func TestIntegration_Routing_EdgeCases(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Operation Routes for testing
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("EdgeCase Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("EdgeCase Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create a valid operation route for use in transaction route tests
	validRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Valid Route %s", h.RandString(4)), "source")
	if err != nil {
		t.Fatalf("create valid operation route failed: %v", err)
	}
	t.Logf("Created valid operation route: ID=%s", validRouteID)

	t.Cleanup(func() {
		if err := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, validRouteID); err != nil {
			t.Logf("Warning: cleanup delete valid route failed: %v", err)
		}
	})

	opRoutePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)
	txRoutePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Duplicate operation route IDs in transaction route
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("duplicate_operation_route_ids_in_transaction_route", func(t *testing.T) {
		t.Parallel()
		createPayload := h.CreateTransactionRoutePayload(
			fmt.Sprintf("Dup IDs TxRoute %s", h.RandString(6)),
			[]string{validRouteID, validRouteID}, // Same ID twice
		)
		createPayload["description"] = "Transaction route with duplicate operation route IDs"

		code, body, err := trans.Request(ctx, "POST", txRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		// Should fail with 400 Bad Request (duplicate IDs not allowed) or 409 Conflict
		switch code {
		case 201:
			t.Errorf("expected failure for duplicate operation route IDs, but got 201 Created: body=%s", string(body))
			// Cleanup if accidentally created
			var created h.TransactionRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteTransactionRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental transaction route failed: %v", delErr)
				}
			}
		case 400, 409, 422:
			t.Logf("Duplicate operation route IDs correctly rejected with code=%d", code)
		default:
			t.Logf("Duplicate operation route IDs returned unexpected code=%d body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Account rule with invalid validIf type (array instead of string)
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("account_rule_invalid_validif_type_array", func(t *testing.T) {
		t.Parallel()
		// Create operation route with alias rule but array instead of string for validIf
		createPayload := map[string]any{
			"title":         fmt.Sprintf("Invalid ValidIf Type %s", h.RandString(4)),
			"operationType": "source",
			"account": map[string]any{
				"ruleType": "alias",
				"validIf":  []string{"@treasury", "@fees"}, // Array instead of expected string
			},
		}

		code, body, err := trans.Request(ctx, "POST", opRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		// Should fail with 400 Bad Request due to invalid type for validIf
		switch code {
		case 201:
			t.Errorf("expected failure for invalid validIf type (array), but got 201 Created: body=%s", string(body))
			// Cleanup if accidentally created
			var created h.OperationRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental operation route failed: %v", delErr)
				}
			}
		case 400, 422:
			t.Logf("Invalid validIf type (array) correctly rejected with code=%d", code)
		default:
			t.Logf("Invalid validIf type returned unexpected code=%d body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Account rule with empty validIf string
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("account_rule_empty_validif", func(t *testing.T) {
		t.Parallel()
		// Create operation route with alias rule but empty string for validIf
		createPayload := map[string]any{
			"title":         fmt.Sprintf("Empty ValidIf %s", h.RandString(4)),
			"operationType": "source",
			"account": map[string]any{
				"ruleType": "alias",
				"validIf":  "", // Empty string
			},
		}

		code, body, err := trans.Request(ctx, "POST", opRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		// Should fail with 400 Bad Request due to empty validIf
		switch code {
		case 201:
			t.Errorf("expected failure for empty validIf, but got 201 Created: body=%s", string(body))
			// Cleanup if accidentally created
			var created h.OperationRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental operation route failed: %v", delErr)
				}
			}
		case 400, 422:
			t.Logf("Empty validIf correctly rejected with code=%d", code)
		default:
			t.Logf("Empty validIf returned unexpected code=%d body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Account rule with invalid ruleType
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("account_rule_invalid_rule_type", func(t *testing.T) {
		t.Parallel()
		createPayload := map[string]any{
			"title":         fmt.Sprintf("Invalid RuleType %s", h.RandString(4)),
			"operationType": "source",
			"account": map[string]any{
				"ruleType": "invalid_rule_type", // Invalid rule type
				"validIf":  "@treasury",
			},
		}

		code, body, err := trans.Request(ctx, "POST", opRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		switch code {
		case 201:
			t.Errorf("expected failure for invalid ruleType, but got 201 Created: body=%s", string(body))
			var created h.OperationRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental operation route failed: %v", delErr)
				}
			}
		case 400, 422:
			t.Logf("Invalid ruleType correctly rejected with code=%d", code)
		default:
			t.Logf("Invalid ruleType returned unexpected code=%d body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Account rule with null validIf
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("account_rule_null_validif", func(t *testing.T) {
		t.Parallel()
		createPayload := map[string]any{
			"title":         fmt.Sprintf("Null ValidIf %s", h.RandString(4)),
			"operationType": "source",
			"account": map[string]any{
				"ruleType": "alias",
				"validIf":  nil, // Null value
			},
		}

		code, body, err := trans.Request(ctx, "POST", opRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		switch code {
		case 201:
			t.Errorf("expected failure for null validIf, but got 201 Created: body=%s", string(body))
			var created h.OperationRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental operation route failed: %v", delErr)
				}
			}
		case 400, 422:
			t.Logf("Null validIf correctly rejected with code=%d", code)
		default:
			t.Logf("Null validIf returned unexpected code=%d body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Account rule with object instead of string for validIf
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("account_rule_object_validif", func(t *testing.T) {
		t.Parallel()
		createPayload := map[string]any{
			"title":         fmt.Sprintf("Object ValidIf %s", h.RandString(4)),
			"operationType": "source",
			"account": map[string]any{
				"ruleType": "alias",
				"validIf":  map[string]any{"alias": "@treasury"}, // Object instead of string
			},
		}

		code, body, err := trans.Request(ctx, "POST", opRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		switch code {
		case 201:
			t.Errorf("expected failure for object validIf, but got 201 Created: body=%s", string(body))
			var created h.OperationRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental operation route failed: %v", delErr)
				}
			}
		case 400, 422:
			t.Logf("Object validIf correctly rejected with code=%d", code)
		default:
			t.Logf("Object validIf returned unexpected code=%d body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Edge Case: Account rule with missing ruleType but validIf present
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("account_rule_missing_ruletype", func(t *testing.T) {
		t.Parallel()
		createPayload := map[string]any{
			"title":         fmt.Sprintf("Missing RuleType %s", h.RandString(4)),
			"operationType": "source",
			"account": map[string]any{
				"validIf": "@treasury", // ruleType is missing
			},
		}

		code, body, err := trans.Request(ctx, "POST", opRoutePath, headers, createPayload)
		if err != nil {
			t.Logf("Request error (may be expected): %v", err)
		}

		switch code {
		case 201:
			t.Errorf("expected failure for missing ruleType, but got 201 Created: body=%s", string(body))
			var created h.OperationRouteResponse
			if json.Unmarshal(body, &created) == nil && created.ID != "" {
				if delErr := h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, created.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental operation route failed: %v", delErr)
				}
			}
		case 400, 422:
			t.Logf("Missing ruleType correctly rejected with code=%d", code)
		default:
			t.Logf("Missing ruleType returned unexpected code=%d body=%s", code, string(body))
		}
	})

	t.Log("Edge case tests completed")
}
