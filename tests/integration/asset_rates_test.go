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

// TestIntegration_AssetRates_CreateAndUpdate tests asset rate creation and update flow.
// It verifies that:
// 1. A new asset rate can be created via PUT
// 2. The response contains all expected fields
// 3. The same from/to pair can be updated with a new rate value via PUT (upsert behavior)
// 4. The updated rate is persisted correctly
func TestIntegration_AssetRates_CreateAndUpdate(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization -> Ledger -> Assets (USD and BRL)
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetRates Create Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetRates Create Test Ledger %s", h.RandString(5)))
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
	// TEST 1: Create Asset Rate (USD to BRL)
	// ─────────────────────────────────────────────────────────────────────────
	externalID := fmt.Sprintf("ext-create-update-%s", h.RandHex(8))
	source := "Integration Test - Create"
	scale := 2.0

	createPayload := map[string]any{
		"from":       "USD",
		"to":         "BRL",
		"rate":       550, // 5.50 BRL per 1 USD (rate=550, scale=2)
		"scale":      int(scale),
		"source":     source,
		"externalId": externalID,
		"metadata": map[string]any{
			"environment": "test",
			"testCase":    "create-and-update",
		},
	}

	ratePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)
	code, body, err = trans.Request(ctx, "PUT", ratePath, headers, createPayload)
	if err != nil || code != 200 {
		t.Fatalf("CREATE asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRate assetRateResponse
	if err := json.Unmarshal(body, &createdRate); err != nil {
		t.Fatalf("parse created asset rate: %v body=%s", err, string(body))
	}

	// Verify all response fields
	if createdRate.ID == "" {
		t.Error("created asset rate ID should not be empty")
	}
	if createdRate.From != "USD" {
		t.Errorf("asset rate 'from' mismatch: got %q, want %q", createdRate.From, "USD")
	}
	if createdRate.To != "BRL" {
		t.Errorf("asset rate 'to' mismatch: got %q, want %q", createdRate.To, "BRL")
	}
	if createdRate.Rate != 550 {
		t.Errorf("asset rate 'rate' mismatch: got %.2f, want %.2f", createdRate.Rate, 550.0)
	}
	if createdRate.Scale == nil || *createdRate.Scale != scale {
		t.Errorf("asset rate 'scale' mismatch: got %v, want %.0f", createdRate.Scale, scale)
	}
	if createdRate.ExternalID != externalID {
		t.Errorf("asset rate 'externalId' mismatch: got %q, want %q", createdRate.ExternalID, externalID)
	}
	if createdRate.Source == nil || *createdRate.Source != source {
		t.Errorf("asset rate 'source' mismatch: got %v, want %q", createdRate.Source, source)
	}

	t.Logf("Created asset rate: ID=%s From=%s To=%s Rate=%.2f ExternalID=%s",
		createdRate.ID, createdRate.From, createdRate.To, createdRate.Rate, createdRate.ExternalID)

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 2: Update Asset Rate (same from/to, new rate value)
	// ─────────────────────────────────────────────────────────────────────────
	updatedSource := "Integration Test - Updated"
	updatePayload := map[string]any{
		"from":       "USD",
		"to":         "BRL",
		"rate":       560, // Updated rate: 5.60 BRL per 1 USD
		"scale":      2,
		"source":     updatedSource,
		"externalId": externalID,
		"metadata": map[string]any{
			"environment": "test",
			"testCase":    "create-and-update",
			"updated":     true,
		},
	}

	code, body, err = trans.Request(ctx, "PUT", ratePath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("UPDATE asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedRate assetRateResponse
	if err := json.Unmarshal(body, &updatedRate); err != nil {
		t.Fatalf("parse updated asset rate: %v body=%s", err, string(body))
	}

	// Verify updated fields
	if updatedRate.Rate != 560 {
		t.Errorf("updated rate mismatch: got %.2f, want %.2f", updatedRate.Rate, 560.0)
	}
	if updatedRate.Source == nil || *updatedRate.Source != updatedSource {
		t.Errorf("updated source mismatch: got %v, want %q", updatedRate.Source, updatedSource)
	}

	t.Logf("Updated asset rate: ID=%s Rate=%.2f Source=%v", updatedRate.ID, updatedRate.Rate, updatedRate.Source)

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 3: Verify update persisted by fetching
	// ─────────────────────────────────────────────────────────────────────────
	fetchedRate, err := h.GetAssetRateByExternalID(ctx, trans, headers, orgID, ledgerID, externalID)
	if err != nil {
		t.Fatalf("GET asset rate after update failed: %v", err)
	}

	if fetchedRate.Rate != 560 {
		t.Errorf("persisted rate mismatch after update: got %.2f, want %.2f", fetchedRate.Rate, 560.0)
	}

	t.Log("Asset Rate create and update test completed successfully")
}

// TestIntegration_AssetRates_GetByExternalID tests retrieving an asset rate by its external ID.
// It verifies that:
// 1. An asset rate can be retrieved using its external ID
// 2. The retrieved data matches the created data
// 3. Non-existent external IDs return 404
func TestIntegration_AssetRates_GetByExternalID(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization -> Ledger -> Assets (USD and EUR)
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetRates GetByExtID Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetRates GetByExtID Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Create EUR asset
	eurPayload := map[string]any{
		"name": "Euro",
		"type": "currency",
		"code": "EUR",
	}
	assetPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", assetPath, headers, eurPayload)
	if err != nil || (code != 201 && code != 409) {
		t.Fatalf("create EUR asset failed: code=%d err=%v body=%s", code, err, string(body))
	}
	t.Log("Created EUR asset")

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 1: Create an asset rate with specific external ID
	// ─────────────────────────────────────────────────────────────────────────
	externalID := fmt.Sprintf("ext-get-by-id-%s", h.RandHex(8))
	source := "Integration Test - GetByExternalID"

	createPayload := map[string]any{
		"from":       "USD",
		"to":         "EUR",
		"rate":       92, // 0.92 EUR per 1 USD
		"scale":      2,
		"source":     source,
		"externalId": externalID,
		"metadata": map[string]any{
			"environment": "test",
			"testCase":    "get-by-external-id",
		},
	}

	ratePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)
	code, body, err = trans.Request(ctx, "PUT", ratePath, headers, createPayload)
	if err != nil || code != 200 {
		t.Fatalf("CREATE asset rate failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdRate assetRateResponse
	if err := json.Unmarshal(body, &createdRate); err != nil {
		t.Fatalf("parse created asset rate: %v body=%s", err, string(body))
	}
	t.Logf("Created asset rate: ID=%s ExternalID=%s", createdRate.ID, createdRate.ExternalID)

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 2: Retrieve asset rate by external ID
	// ─────────────────────────────────────────────────────────────────────────
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/%s", orgID, ledgerID, externalID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET asset rate by external ID failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetchedRate assetRateResponse
	if err := json.Unmarshal(body, &fetchedRate); err != nil {
		t.Fatalf("parse fetched asset rate: %v body=%s", err, string(body))
	}

	// Verify fetched data matches created data
	if fetchedRate.ExternalID != externalID {
		t.Errorf("fetched external ID mismatch: got %q, want %q", fetchedRate.ExternalID, externalID)
	}
	if fetchedRate.From != "USD" {
		t.Errorf("fetched 'from' mismatch: got %q, want %q", fetchedRate.From, "USD")
	}
	if fetchedRate.To != "EUR" {
		t.Errorf("fetched 'to' mismatch: got %q, want %q", fetchedRate.To, "EUR")
	}
	if fetchedRate.Rate != 92 {
		t.Errorf("fetched rate mismatch: got %.2f, want %.2f", fetchedRate.Rate, 92.0)
	}
	if fetchedRate.ID != createdRate.ID {
		t.Errorf("fetched ID mismatch: got %q, want %q", fetchedRate.ID, createdRate.ID)
	}

	t.Logf("Fetched asset rate: ID=%s ExternalID=%s From=%s To=%s Rate=%.2f",
		fetchedRate.ID, fetchedRate.ExternalID, fetchedRate.From, fetchedRate.To, fetchedRate.Rate)

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 3: Verify 404 for non-existent external ID
	// ─────────────────────────────────────────────────────────────────────────
	nonExistentID := fmt.Sprintf("non-existent-%s", h.RandHex(8))
	getPathNotFound := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/%s", orgID, ledgerID, nonExistentID)
	code, body, err = trans.Request(ctx, "GET", getPathNotFound, headers, nil)
	if err != nil {
		t.Logf("Request error for non-existent ID (may be expected): %v", err)
	}

	if code != 404 {
		t.Errorf("expected 404 for non-existent external ID, got %d: body=%s", code, string(body))
	}
	t.Logf("Non-existent external ID correctly returned 404")

	t.Log("Asset Rate GetByExternalID test completed successfully")
}

// TestIntegration_AssetRates_GetByAssetCode tests listing asset rates by source asset code.
// It verifies that:
// 1. Multiple rates FROM the same source asset can be created
// 2. All rates are returned when listing by source asset code
// 3. An asset with no rates returns an empty list
func TestIntegration_AssetRates_GetByAssetCode(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization -> Ledger -> Multiple Assets
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AssetRates GetByCode Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AssetRates GetByCode Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create multiple currency assets (USD, EUR, BRL, GBP)
	assets := []struct {
		name string
		code string
	}{
		{"US Dollar", "USD"},
		{"Euro", "EUR"},
		{"Brazilian Real", "BRL"},
		{"British Pound", "GBP"},
	}

	assetPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	for _, asset := range assets {
		payload := map[string]any{
			"name": asset.name,
			"type": "currency",
			"code": asset.code,
		}
		code, body, err := onboard.Request(ctx, "POST", assetPath, headers, payload)
		if err != nil || (code != 201 && code != 409) {
			t.Fatalf("create %s asset failed: code=%d err=%v body=%s", asset.code, code, err, string(body))
		}
		t.Logf("Created asset: %s", asset.code)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 1: Create multiple rates FROM USD
	// ─────────────────────────────────────────────────────────────────────────
	ratePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)

	rates := []struct {
		to         string
		rate       int
		scale      int
		externalID string
	}{
		{"EUR", 92, 2, fmt.Sprintf("ext-usd-eur-%s", h.RandHex(4))},  // 0.92 EUR per USD
		{"BRL", 550, 2, fmt.Sprintf("ext-usd-brl-%s", h.RandHex(4))}, // 5.50 BRL per USD
		{"GBP", 79, 2, fmt.Sprintf("ext-usd-gbp-%s", h.RandHex(4))},  // 0.79 GBP per USD
	}

	createdExternalIDs := make(map[string]bool)
	for _, r := range rates {
		payload := map[string]any{
			"from":       "USD",
			"to":         r.to,
			"rate":       r.rate,
			"scale":      r.scale,
			"externalId": r.externalID,
			"metadata": map[string]any{
				"testCase": "get-by-asset-code",
			},
		}

		code, body, err := trans.Request(ctx, "PUT", ratePath, headers, payload)
		if err != nil || code != 200 {
			t.Fatalf("create USD->%s rate failed: code=%d err=%v body=%s", r.to, code, err, string(body))
		}
		createdExternalIDs[r.externalID] = true
		t.Logf("Created rate: USD -> %s = %d (scale=%d) ExternalID=%s", r.to, r.rate, r.scale, r.externalID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 2: List all rates FROM USD
	// ─────────────────────────────────────────────────────────────────────────
	listPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/from/USD", orgID, ledgerID)
	code, body, err := trans.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("LIST asset rates from USD failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var listResponse assetRatesListResponse
	if err := json.Unmarshal(body, &listResponse); err != nil {
		t.Fatalf("parse asset rates list: %v body=%s", err, string(body))
	}

	// Verify we got at least 3 rates (the ones we created)
	if len(listResponse.Items) < 3 {
		t.Errorf("expected at least 3 rates from USD, got %d", len(listResponse.Items))
	}

	// Verify all created rates are present
	foundToCurrencies := make(map[string]bool)
	foundExternalIDs := make(map[string]bool)
	for _, item := range listResponse.Items {
		if item.From != "USD" {
			t.Errorf("unexpected 'from' in list: got %q, want %q", item.From, "USD")
		}
		foundToCurrencies[item.To] = true
		foundExternalIDs[item.ExternalID] = true
	}

	for _, r := range rates {
		if !foundToCurrencies[r.to] {
			t.Errorf("rate USD->%s not found in list", r.to)
		}
		if !foundExternalIDs[r.externalID] {
			t.Errorf("rate with externalID %s not found in list", r.externalID)
		}
	}

	t.Logf("Listed %d rates from USD, all expected currencies found", len(listResponse.Items))

	// ─────────────────────────────────────────────────────────────────────────
	// TEST 3: Verify empty result for asset with no rates (GBP has no outgoing rates)
	// ─────────────────────────────────────────────────────────────────────────
	listPathGBP := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/from/GBP", orgID, ledgerID)
	code, body, err = trans.Request(ctx, "GET", listPathGBP, headers, nil)
	if err != nil {
		t.Logf("Request error for GBP rates (may be expected): %v", err)
	}

	// Expect either 200 with empty items or 404 (depending on implementation)
	switch code {
	case 200:
		var emptyListResponse assetRatesListResponse
		if err := json.Unmarshal(body, &emptyListResponse); err != nil {
			t.Fatalf("parse empty asset rates list: %v body=%s", err, string(body))
		}
		if len(emptyListResponse.Items) != 0 {
			t.Errorf("expected empty list for GBP (no outgoing rates), got %d items", len(emptyListResponse.Items))
		}
		t.Log("GBP rates list returned empty as expected")
	case 404:
		t.Log("GBP rates list returned 404 (no rates found) as expected")
	default:
		t.Errorf("unexpected status code for GBP rates list: %d body=%s", code, string(body))
	}

	t.Log("Asset Rate GetByAssetCode test completed successfully")
}
