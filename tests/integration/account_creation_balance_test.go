package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Verifies that creating an account results in a default balance record that can be listed by account ID.
func TestIntegration_AccountCreation_DefaultBalanceExistsByAccountID(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// 1) Setup org → ledger → asset (USD)
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("Ledger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// 2) Create account (deposit, USD) with unique alias
	alias := iso.UniqueAccountAlias("acc")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// 3) GET balances by account ID and assert presence of default with asset USD
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("balances by account id failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var byAccount struct {
		Items []struct {
			ID        string `json:"id"`
			Key       string `json:"key"`
			AssetCode string `json:"assetCode"`
			Alias     string `json:"alias"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &byAccount); err != nil {
		t.Fatalf("parse balances by account id: %v body=%s", err, string(body))
	}
	if len(byAccount.Items) == 0 {
		t.Fatalf("no balances returned for account %s", accountID)
	}
	foundDefault := false
	for _, it := range byAccount.Items {
		if it.Key == "default" && it.AssetCode == "USD" {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Fatalf("default USD balance not found for account %s", accountID)
	}
}

// Verifies that the default balance created with the account is also visible by alias.
func TestIntegration_AccountCreation_DefaultBalanceVisibleByAlias(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// 1) Setup org → ledger → asset (USD)
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("Ledger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// 2) Create account (deposit, USD) with unique alias
	alias := iso.UniqueAccountAlias("acc")
	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// 3) GET balances by alias and assert presence of default with asset USD
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias)
	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("balances by alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var byAlias struct {
		Items []struct {
			ID        string `json:"id"`
			Key       string `json:"key"`
			AssetCode string `json:"assetCode"`
			Alias     string `json:"alias"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &byAlias); err != nil {
		t.Fatalf("parse balances by alias: %v body=%s", err, string(body))
	}
	if len(byAlias.Items) == 0 {
		t.Fatalf("no balances returned for alias %s", alias)
	}
	foundDefault := false
	for _, it := range byAlias.Items {
		if it.Key == "default" && it.AssetCode == "USD" {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Fatalf("default USD balance not found for alias %s", alias)
	}
}
