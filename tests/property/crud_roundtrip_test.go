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

// Property: Find(Create(x).ID) == x
// This is the fundamental CRUD roundtrip property - what you create should be findable.
func TestProperty_OrganizationCRUDRoundtrip_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate test data
		name := fmt.Sprintf("CRUDOrg-%s-%d", h.RandString(6), rng.Intn(10000))
		legalName := fmt.Sprintf("Legal Entity %s", h.RandString(8))

		// CREATE
		createPayload := map[string]any{
			"legalName":       legalName,
			"doingBusinessAs": name,
		}

		code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, createPayload)
		if err != nil || code != 201 {
			t.Logf("create org: code=%d err=%v", code, err)
			return true
		}

		var createResp struct {
			ID              string `json:"id"`
			LegalName       string `json:"legalName"`
			DoingBusinessAs string `json:"doingBusinessAs"`
		}
		if err := json.Unmarshal(body, &createResp); err != nil {
			t.Logf("parse create response: %v", err)
			return true
		}

		if createResp.ID == "" {
			t.Logf("created org without ID")
			return true
		}

		// READ
		code, body, err = onboard.Request(ctx, "GET", "/v1/organizations/"+createResp.ID, headers, nil)
		if err != nil || code != 200 {
			t.Errorf("read org failed: code=%d err=%v", code, err)
			return false
		}

		var readResp struct {
			ID              string `json:"id"`
			LegalName       string `json:"legalName"`
			DoingBusinessAs string `json:"doingBusinessAs"`
		}
		if err := json.Unmarshal(body, &readResp); err != nil {
			t.Errorf("parse read response: %v", err)
			return false
		}

		// VERIFY: Find(Create(x).ID) == x
		if readResp.ID != createResp.ID {
			t.Errorf("ID mismatch: created=%s read=%s", createResp.ID, readResp.ID)
			return false
		}
		if readResp.LegalName != createResp.LegalName {
			t.Errorf("LegalName mismatch: created=%s read=%s", createResp.LegalName, readResp.LegalName)
			return false
		}
		if readResp.DoingBusinessAs != createResp.DoingBusinessAs {
			t.Errorf("DoingBusinessAs mismatch: created=%s read=%s", createResp.DoingBusinessAs, readResp.DoingBusinessAs)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("organization CRUD roundtrip failed: %v", err)
	}
}

// Property: Ledger CRUD roundtrip
func TestProperty_LedgerCRUDRoundtrip_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org once
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "CRUDLedgerOrg "+h.RandString(6))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		name := fmt.Sprintf("CRUDLedger-%s-%d", h.RandString(6), rng.Intn(10000))

		// CREATE
		createPayload := map[string]any{
			"name": name,
		}

		code, body, err := onboard.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers", orgID), headers, createPayload)
		if err != nil || code != 201 {
			t.Logf("create ledger: code=%d err=%v", code, err)
			return true
		}

		var createResp struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(body, &createResp); err != nil {
			t.Logf("parse create response: %v", err)
			return true
		}

		if createResp.ID == "" {
			t.Logf("created ledger without ID")
			return true
		}

		// READ
		code, body, err = onboard.Request(ctx, "GET",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, createResp.ID), headers, nil)
		if err != nil || code != 200 {
			t.Errorf("read ledger failed: code=%d", code)
			return false
		}

		var readResp struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(body, &readResp); err != nil {
			t.Errorf("parse read response: %v", err)
			return false
		}

		// VERIFY
		if readResp.ID != createResp.ID || readResp.Name != createResp.Name {
			t.Errorf("Ledger mismatch: created=%+v read=%+v", createResp, readResp)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("ledger CRUD roundtrip failed: %v", err)
	}
}

// Property: Account CRUD roundtrip
func TestProperty_AccountCRUDRoundtrip_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))

	orgID, err := h.SetupOrganization(ctx, onboard, headers, "CRUDAccOrg "+h.RandString(6))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, "L")
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		alias := fmt.Sprintf("crud-%s-%d", h.RandString(5), rng.Intn(10000))
		name := fmt.Sprintf("CRUD Account %d", rng.Intn(1000))

		// CREATE
		createPayload := map[string]any{
			"name":      name,
			"assetCode": "USD",
			"type":      "deposit",
			"alias":     alias,
		}

		code, body, err := onboard.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID),
			headers, createPayload)
		if err != nil || code != 201 {
			t.Logf("create account: code=%d err=%v", code, err)
			return true
		}

		var createResp struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Alias     string `json:"alias"`
			AssetCode string `json:"assetCode"`
		}
		if err := json.Unmarshal(body, &createResp); err != nil {
			t.Logf("parse create response: %v", err)
			return true
		}

		if createResp.ID == "" {
			t.Logf("created account without ID")
			return true
		}

		// READ
		code, body, err = onboard.Request(ctx, "GET",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, createResp.ID),
			headers, nil)
		if err != nil || code != 200 {
			t.Errorf("read account failed: code=%d", code)
			return false
		}

		var readResp struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Alias     string `json:"alias"`
			AssetCode string `json:"assetCode"`
		}
		if err := json.Unmarshal(body, &readResp); err != nil {
			t.Errorf("parse read response: %v", err)
			return false
		}

		// VERIFY
		if readResp.ID != createResp.ID {
			t.Errorf("Account ID mismatch: created=%s read=%s", createResp.ID, readResp.ID)
			return false
		}
		if readResp.Name != createResp.Name {
			t.Errorf("Account Name mismatch: created=%s read=%s", createResp.Name, readResp.Name)
			return false
		}
		if readResp.Alias != createResp.Alias {
			t.Errorf("Account Alias mismatch: created=%s read=%s", createResp.Alias, readResp.Alias)
			return false
		}
		if readResp.AssetCode != createResp.AssetCode {
			t.Errorf("Account AssetCode mismatch: created=%s read=%s", createResp.AssetCode, readResp.AssetCode)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("account CRUD roundtrip failed: %v", err)
	}
}
