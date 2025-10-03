package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Database success, full cache overlay for alias listing.
// - Enable default balance for the alias.
// - Perform a credit inflow to ensure cache entry exists.
// - GET /accounts/alias/{alias}/balances and assert the available reflects the inflow amount.
func TestIntegration_GetAllBalancesByAlias_FullCacheOverlay(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

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
	alias := iso.UniqueAccountAlias("acc")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
		t.Fatalf("ensure default ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, orgID, ledgerID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}

	// Perform inflow to create/update cache entry with a known value
	want, _ := decimal.NewFromString("123.45")
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", want.String(), headers)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Poll GET /accounts/alias/{alias}/balances until the available for default equals want
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, url.PathEscape(alias))
	deadline := time.Now().Add(8 * time.Second)
	for {
		code, b, err := trans.Request(ctx, "GET", path, headers, nil)
		if err == nil && code == 200 {
			var resp struct {
				Items []struct {
					Alias, Key, AssetCode string
					Available, OnHold     decimal.Decimal
					Version               int64 `json:"version"`
				} `json:"items"`
			}
			if json.Unmarshal(b, &resp) == nil {
				for _, it := range resp.Items {
					if it.Alias == alias && it.Key == "default" && it.AssetCode == "USD" {
						if it.Available.Equal(want) {
							return
						}
						break
					}
				}
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("overlay value not observed; want %s", want.String())
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// Very large decimal magnitudes and precision preservation end-to-end for alias listing.
func TestIntegration_GetAllBalancesByAlias_VeryLargePrecisionOverlay(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

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
	alias := iso.UniqueAccountAlias("acc")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, orgID, ledgerID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}

	largeAmount := "123456789012345678901234567890.123456789012345678901234567890"
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", largeAmount, headers)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	want, _ := decimal.NewFromString(largeAmount)
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, url.PathEscape(alias))
	deadline := time.Now().Add(12 * time.Second)
	for {
		code, b, err := trans.Request(ctx, "GET", path, headers, nil)
		if err == nil && code == 200 {
			var resp struct {
				Items []struct {
					Alias, Key, AssetCode string
					Available             decimal.Decimal
					Version               int64 `json:"version"`
				} `json:"items"`
			}
			if json.Unmarshal(b, &resp) == nil {
				for _, it := range resp.Items {
					if it.Alias == alias && it.Key == "default" && it.AssetCode == "USD" {
						if it.Available.Equal(want) {
							return
						}
						break
					}
				}
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("large-precision overlay not observed; want %s", want.String())
		}
		time.Sleep(150 * time.Millisecond)
	}
}

