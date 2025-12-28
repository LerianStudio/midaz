package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_AssetRate_CreateAndRetrieve tests the asset rate creation and retrieval flow.
// Asset rates define currency conversion rates between two asset codes.
func TestIntegration_AssetRate_CreateAndRetrieve(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Assets
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetRate Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetRate Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Create BRL asset
	brlPayload := map[string]any{
		"name": "Brazilian Real",
		"type": "currency",
		"code": "BRL",
	}
	assetPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", assetPath, headers, brlPayload)
	if err != nil || (code != 201 && code != 409) {
		t.Fatalf("create BRL asset failed: code=%d err=%v body=%s", code, err, string(body))
	}
	t.Log("Created BRL asset")

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - Asset Rate (USD to BRL)
	// ─────────────────────────────────────────────────────────────────────────
	externalID := fmt.Sprintf("ext-%s", h.RandHex(8))
	createPayload := map[string]any{
		"from":       "USD",
		"to":         "BRL",
		"rate":       550, // 5.50 BRL per 1 USD (rate=550, scale=2 means 5.50)
		"scale":      2,
		"source":     "Integration Test",
		"externalId": externalID,
		"metadata": map[string]any{
			"environment": "test",
			"provider":    "test-system",
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)
	code, body, err = trans.Request(ctx, "PUT", path, headers, createPayload)
	if err != nil || code != 200 {
		t.Fatalf("CREATE asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRate h.AssetRateResponse
	if err := json.Unmarshal(body, &createdRate); err != nil || createdRate.ID == "" {
		t.Fatalf("parse created asset rate: %v body=%s", err, string(body))
	}

	t.Logf("Created asset rate: ID=%s From=%s To=%s Rate=%.2f Scale=%.0f",
		createdRate.ID, createdRate.From, createdRate.To, createdRate.Rate, createdRate.Scale)

	// Verify created rate fields
	if createdRate.From != "USD" {
		t.Errorf("asset rate 'from' mismatch: got %q, want %q", createdRate.From, "USD")
	}
	if createdRate.To != "BRL" {
		t.Errorf("asset rate 'to' mismatch: got %q, want %q", createdRate.To, "BRL")
	}
	if createdRate.Rate != 550 {
		t.Errorf("asset rate 'rate' mismatch: got %.2f, want %.2f", createdRate.Rate, 550.0)
	}
	if createdRate.Scale != 2 {
		t.Errorf("asset rate 'scale' mismatch: got %.0f, want %.0f", createdRate.Scale, 2.0)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - Get Asset Rate by External ID
	// ─────────────────────────────────────────────────────────────────────────
	fetchedRate, err := h.GetAssetRateByExternalID(ctx, trans, headers, orgID, ledgerID, externalID)
	if err != nil {
		t.Fatalf("GET asset rate by external ID failed: %v", err)
	}

	if fetchedRate.ExternalID != externalID {
		t.Errorf("fetched rate external ID mismatch: got %q, want %q", fetchedRate.ExternalID, externalID)
	}
	if fetchedRate.From != "USD" || fetchedRate.To != "BRL" {
		t.Errorf("fetched rate currency pair mismatch: got %s->%s, want USD->BRL", fetchedRate.From, fetchedRate.To)
	}

	t.Logf("Fetched asset rate: ExternalID=%s Rate=%.2f", fetchedRate.ExternalID, fetchedRate.Rate)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) LIST - Get All Asset Rates from USD
	// ─────────────────────────────────────────────────────────────────────────
	rateList, err := h.ListAssetRatesByAssetCode(ctx, trans, headers, orgID, ledgerID, "USD")
	if err != nil {
		t.Fatalf("LIST asset rates failed: %v", err)
	}

	found := false
	for _, rate := range rateList.Items {
		if rate.ExternalID == externalID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created asset rate not found in list: ExternalID=%s", externalID)
	}

	t.Logf("List asset rates from USD: found %d rates, target rate found=%v", len(rateList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) UPDATE - Modify Asset Rate (using PUT as upsert)
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"from":       "USD",
		"to":         "BRL",
		"rate":       560, // Updated rate: 5.60 BRL per 1 USD
		"scale":      2,
		"source":     "Integration Test - Updated",
		"externalId": externalID,
		"metadata": map[string]any{
			"environment": "test",
			"provider":    "test-system",
			"updated":     true,
		},
	}

	code, body, err = trans.Request(ctx, "PUT", path, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("UPDATE asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRate h.AssetRateResponse
	if err := json.Unmarshal(body, &updatedRate); err != nil {
		t.Fatalf("parse updated asset rate: %v", err)
	}

	if updatedRate.Rate != 560 {
		t.Errorf("updated rate mismatch: got %.2f, want %.2f", updatedRate.Rate, 560.0)
	}

	t.Logf("Updated asset rate: ExternalID=%s NewRate=%.2f", updatedRate.ExternalID, updatedRate.Rate)

	// Verify update persisted
	verifyRate, err := h.GetAssetRateByExternalID(ctx, trans, headers, orgID, ledgerID, externalID)
	if err != nil {
		t.Fatalf("GET asset rate after update failed: %v", err)
	}
	if verifyRate.Rate != 560 {
		t.Errorf("persisted rate mismatch: got %.2f, want %.2f", verifyRate.Rate, 560.0)
	}

	t.Log("Asset Rate CRUD lifecycle completed successfully")
}

// TestIntegration_AssetRate_MultipleCurrencyPairs tests creating multiple asset rates
// for different currency pairs in the same ledger.
func TestIntegration_AssetRate_MultipleCurrencyPairs(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("MultiRate Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("MultiRate Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create multiple currency assets
	currencies := []struct {
		name string
		code string
	}{
		{"US Dollar", "USD"},
		{"Euro", "EUR"},
		{"British Pound", "GBP"},
		{"Japanese Yen", "JPY"},
	}

	assetPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	for _, curr := range currencies {
		payload := map[string]any{"name": curr.name, "type": "currency", "code": curr.code}
		code, _, _ := onboard.Request(ctx, "POST", assetPath, headers, payload)
		if code != 201 && code != 409 {
			t.Fatalf("create %s asset failed: code=%d", curr.code, code)
		}
	}

	// Create asset rates from USD to other currencies
	ratePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)
	rates := []struct {
		to    string
		rate  int
		scale int
	}{
		{"EUR", 92, 2},    // 0.92 EUR per USD
		{"GBP", 79, 2},    // 0.79 GBP per USD
		{"JPY", 15000, 2}, // 150.00 JPY per USD
	}

	for _, r := range rates {
		payload := map[string]any{
			"from":       "USD",
			"to":         r.to,
			"rate":       r.rate,
			"scale":      r.scale,
			"externalId": fmt.Sprintf("ext-usd-%s-%s", r.to, h.RandHex(4)),
		}
		code, body, err := trans.Request(ctx, "PUT", ratePath, headers, payload)
		if err != nil || code != 200 {
			t.Fatalf("create USD->%s rate failed: code=%d err=%v body=%s", r.to, code, err, string(body))
		}
		t.Logf("Created rate: USD -> %s = %d (scale=%d)", r.to, r.rate, r.scale)
	}

	// List all rates from USD
	rateList, err := h.ListAssetRatesByAssetCode(ctx, trans, headers, orgID, ledgerID, "USD")
	if err != nil {
		t.Fatalf("list rates from USD failed: %v", err)
	}

	if len(rateList.Items) < 3 {
		t.Errorf("expected at least 3 rates from USD, got %d", len(rateList.Items))
	}

	// Verify each currency pair exists
	foundPairs := make(map[string]bool)
	for _, rate := range rateList.Items {
		foundPairs[rate.To] = true
	}

	for _, r := range rates {
		if !foundPairs[r.to] {
			t.Errorf("rate USD->%s not found in list", r.to)
		}
	}

	t.Logf("Multiple currency pairs test passed: created %d rates from USD", len(rateList.Items))
}

// TestIntegration_AssetRate_Validation tests validation errors for asset rate creation.
func TestIntegration_AssetRate_Validation(t *testing.T) {
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

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "missing from",
			payload: map[string]any{"to": "BRL", "rate": 550, "scale": 2},
		},
		{
			name:    "missing to",
			payload: map[string]any{"from": "USD", "rate": 550, "scale": 2},
		},
		{
			name:    "missing rate",
			payload: map[string]any{"from": "USD", "to": "BRL", "scale": 2},
		},
		{
			name:    "from code too short",
			payload: map[string]any{"from": "U", "to": "BRL", "rate": 550, "scale": 2},
		},
		{
			name:    "to code too long",
			payload: map[string]any{"from": "USD", "to": "VERYLONGCODE", "rate": 550, "scale": 2},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := trans.Request(ctx, "PUT", path, headers, tc.payload)
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
