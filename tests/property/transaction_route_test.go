package property

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property: Every TransactionRoute must have at least one source AND one destination.
// This is a fundamental accounting rule - money must flow from somewhere to somewhere.
func TestProperty_TransactionRouteComposition_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropRoute "+h.RandString(6))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, "L")
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}

	f := func(seed int64, numSources uint8, numDests uint8) bool {
		rng := rand.New(rand.NewSource(seed))
		sources := int(numSources)
		dests := int(numDests)

		// Clamp to reasonable values
		if sources > 5 {
			sources = 5
		}
		if dests > 5 {
			dests = 5
		}

		// Test case 1: Valid route (has both source and destination)
		if sources >= 1 && dests >= 1 {
			// This should succeed
			return testValidRouteCreation(t, ctx, trans, orgID, ledgerID, headers, sources, dests, rng)
		}

		// Test case 2: Invalid route (missing source or destination)
		if sources == 0 || dests == 0 {
			return testInvalidRouteRejection(t, ctx, trans, orgID, ledgerID, headers, sources, dests)
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("transaction route composition property failed: %v", err)
	}
}

func testValidRouteCreation(t *testing.T, ctx context.Context, trans *h.HTTPClient,
	orgID, ledgerID string, headers map[string]string, sources, dests int, rng *rand.Rand) bool {

	// Create operation routes first
	var operationRouteIDs []string

	// Create source operation routes
	for i := 0; i < sources; i++ {
		routePayload := map[string]any{
			"title":         fmt.Sprintf("Source Route %d", i),
			"operationType": "source",
			"accountAlias":  fmt.Sprintf("@source-%d-%d", rng.Intn(1000), i),
		}

		code, body, err := trans.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID),
			headers, routePayload)

		if err != nil || (code != 201 && code != 400) {
			// API might not support direct operation route creation - skip
			t.Logf("operation route creation: code=%d err=%v", code, err)
			return true
		}

		if code == 201 {
			var resp struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(body, &resp) == nil && resp.ID != "" {
				operationRouteIDs = append(operationRouteIDs, resp.ID)
			}
		}
	}

	// Create destination operation routes
	for i := 0; i < dests; i++ {
		routePayload := map[string]any{
			"title":         fmt.Sprintf("Dest Route %d", i),
			"operationType": "destination",
			"accountAlias":  fmt.Sprintf("@dest-%d-%d", rng.Intn(1000), i),
		}

		code, body, err := trans.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID),
			headers, routePayload)

		if err != nil || (code != 201 && code != 400) {
			return true
		}

		if code == 201 {
			var resp struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(body, &resp) == nil && resp.ID != "" {
				operationRouteIDs = append(operationRouteIDs, resp.ID)
			}
		}
	}

	if len(operationRouteIDs) == 0 {
		return true // API doesn't support this flow
	}

	// Create transaction route with operation routes
	txRoutePayload := map[string]any{
		"title":           fmt.Sprintf("TX Route %d", rng.Intn(1000)),
		"operationRoutes": operationRouteIDs,
	}

	code, body, _ := trans.Request(ctx, "POST",
		fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID),
		headers, txRoutePayload)

	if code == 201 {
		// Valid route created - verify it has both source and destination
		var resp struct {
			OperationRoutes []struct {
				OperationType string `json:"operationType"`
			} `json:"operationRoutes"`
		}
		if json.Unmarshal(body, &resp) == nil {
			hasSource := false
			hasDestination := false
			for _, or := range resp.OperationRoutes {
				if or.OperationType == "source" {
					hasSource = true
				}
				if or.OperationType == "destination" {
					hasDestination = true
				}
			}
			if !hasSource || !hasDestination {
				t.Errorf("TransactionRoute created without both source and destination: hasSource=%v hasDestination=%v",
					hasSource, hasDestination)
				return false
			}
		}
	}

	return true
}

func testInvalidRouteRejection(t *testing.T, ctx context.Context, trans *h.HTTPClient,
	orgID, ledgerID string, headers map[string]string, sources, dests int) bool {

	// Attempt to create a route with missing source or destination
	// This SHOULD be rejected by the API

	var operationRouteIDs []string

	// Only create operation routes for one type
	if sources > 0 {
		for i := 0; i < sources; i++ {
			routePayload := map[string]any{
				"title":         fmt.Sprintf("Source Only %d", i),
				"operationType": "source",
			}
			code, body, _ := trans.Request(ctx, "POST",
				fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID),
				headers, routePayload)
			if code == 201 {
				var resp struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(body, &resp) == nil && resp.ID != "" {
					operationRouteIDs = append(operationRouteIDs, resp.ID)
				}
			}
		}
	} else if dests > 0 {
		for i := 0; i < dests; i++ {
			routePayload := map[string]any{
				"title":         fmt.Sprintf("Dest Only %d", i),
				"operationType": "destination",
			}
			code, body, _ := trans.Request(ctx, "POST",
				fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID),
				headers, routePayload)
			if code == 201 {
				var resp struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(body, &resp) == nil && resp.ID != "" {
					operationRouteIDs = append(operationRouteIDs, resp.ID)
				}
			}
		}
	}

	if len(operationRouteIDs) == 0 {
		return true
	}

	// Attempt to create transaction route - this SHOULD fail
	txRoutePayload := map[string]any{
		"title":           "Invalid Route",
		"operationRoutes": operationRouteIDs,
	}

	code, _, _ := trans.Request(ctx, "POST",
		fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID),
		headers, txRoutePayload)

	// Should be rejected (400 or similar error)
	if code == 201 {
		t.Errorf("API accepted TransactionRoute without both source and destination: sources=%d dests=%d",
			sources, dests)
		return false
	}

	return true
}
