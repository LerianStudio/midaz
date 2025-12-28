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

	// Create DESTINATION operation route
	destRouteID, err := h.SetupOperationRoute(ctx, trans, headers, orgID, ledgerID, fmt.Sprintf("Destination %s", h.RandString(4)), "destination")
	if err != nil {
		t.Fatalf("create destination route failed: %v", err)
	}
	t.Logf("Created destination route: ID=%s", destRouteID)

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

	// Cleanup
	_ = h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, sourceRouteID)
	_ = h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, destRouteID)
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

	// Verify account rule is set
	if createdRoute.Account == nil {
		t.Errorf("expected account rule to be set, got nil")
	} else {
		if createdRoute.Account.RuleType != "alias" {
			t.Errorf("account rule type mismatch: got %q, want %q", createdRoute.Account.RuleType, "alias")
		}
	}

	// Cleanup
	_ = h.DeleteOperationRoute(ctx, trans, headers, orgID, ledgerID, createdRoute.ID)

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
		t.Run(tc.name, func(t *testing.T) {
			code, body, err := trans.Request(ctx, "POST", path, headers, tc.payload)
			if err != nil {
				t.Logf("Request error (expected for validation): %v", err)
			}
			// Expect non-201 for validation errors
			if code == 201 {
				t.Errorf("expected validation error for %s, but got 201 Created: body=%s", tc.name, string(body))
			}
			t.Logf("Validation test %s: code=%d (expected non-201)", tc.name, code)
		})
	}
}
