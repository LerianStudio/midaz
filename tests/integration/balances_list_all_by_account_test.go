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

// Database success, full cache overlay for a single account.
// - Perform a credit inflow to the alias to ensure cache entry exists.
// - GET /accounts/{account_id}/balances and assert the available reflects the inflow amount.
func TestIntegration_GetAllBalancesByAccount_FullCacheOverlay(t *testing.T) {
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

	// Perform inflow to create/update cache entry with a known value
	want, _ := decimal.NewFromString("123.45")
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", want.String(), headers)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Poll GET /accounts/{account_id}/balances until the available for default equals want
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
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

// Very large decimal magnitudes and precision preservation end-to-end for a single account.
func TestIntegration_GetAllBalancesByAccount_VeryLargePrecisionOverlay(t *testing.T) {
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

	largeAmount := "123456789012345678901234567890.123456789012345678"
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", largeAmount, headers)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	want, _ := decimal.NewFromString(largeAmount)
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
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

// Filtering passthrough by dates for account-specific listing.
func TestIntegration_GetAllBalancesByAccount_FilteringByDate(t *testing.T) {
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
	alias := iso.UniqueAccountAlias("flt")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Past-only window: expect zero items
	pastStart := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")
	pastEnd := time.Now().UTC().AddDate(0, 0, -9).Format("2006-01-02")
	pathPast := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances?start_date=%s&end_date=%s", orgID, ledgerID, accountID, pastStart, pastEnd)
	if code, b, err := trans.Request(ctx, "GET", pathPast, headers, nil); err != nil || code != 200 {
		t.Fatalf("past window request failed: code=%d err=%v body=%s", code, err, string(b))
	} else {
		var page struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			t.Fatalf("parse past page: %v body=%s", err, string(b))
		}
		if page.Items == nil {
			t.Fatalf("expected items field in response for past-only window, got none: body=%s", string(b))
		}
		if len(page.Items) != 0 {
			t.Fatalf("expected 0 items for past-only window, got %d body=%s", len(page.Items), string(b))
		}
	}

	// Today's window: expect items present
	today := time.Now().UTC().Format("2006-01-02")
	pathToday := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances?start_date=%s&end_date=%s", orgID, ledgerID, accountID, today, today)
	code, b, err := trans.Request(ctx, "GET", pathToday, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("today window request failed: code=%d err=%v body=%s", code, err, string(b))
	}
	var pageToday struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(b, &pageToday); err != nil {
		t.Fatalf("parse today page: %v body=%s", err, string(b))
	}
	if len(pageToday.Items) == 0 {
		t.Fatalf("expected items for today's window")
	}
}

// Pagination passthrough for account-specific listing using additional balances.
func TestIntegration_GetAllBalancesByAccount_Pagination(t *testing.T) {
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
	alias := iso.UniqueAccountAlias("pg")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Create N additional balances > limit
	limit := 5
	total := 12
	for i := 0; i < total; i++ {
		payload := map[string]any{
			"key":            fmt.Sprintf("k-%d", i),
			"allowSending":   true,
			"allowReceiving": true,
		}
		p := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
		code, b, err := trans.Request(ctx, "POST", p, headers, payload)
		if err != nil || (code != 201 && code != 409) { // 409 if key already exists in rare race
			t.Fatalf("create additional balance %d failed: code=%d err=%v body=%s", i, code, err, string(b))
		}
	}

	// Page 1 with limit
	path1 := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances?limit=%d", orgID, ledgerID, accountID, limit)
	var next string
	deadline := time.Now().Add(20 * time.Second)
	for {
		code, b, err := trans.Request(ctx, "GET", path1, headers, nil)
		if err == nil && code == 200 {
			var page1 struct {
				Items      []map[string]any `json:"items"`
				NextCursor *string          `json:"next_cursor"`
				PrevCursor *string          `json:"prev_cursor"`
				Limit      int              `json:"limit"`
			}
			if json.Unmarshal(b, &page1) == nil {
				if len(page1.Items) == limit && page1.NextCursor != nil && *page1.NextCursor != "" {
					next = *page1.NextCursor
					break
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("did not observe page 1 with limit and next cursor")
		}
		time.Sleep(250 * time.Millisecond)
	}

	// Page 2 using next
	path2 := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances?limit=%d&cursor=%s", orgID, ledgerID, accountID, limit, url.QueryEscape(next))
	code, b, err := trans.Request(ctx, "GET", path2, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("page 2 failed: code=%d err=%v body=%s", code, err, string(b))
	}

	var page2 struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
		PrevCursor *string          `json:"prev_cursor"`
		Limit      int              `json:"limit"`
	}
	if err := json.Unmarshal(b, &page2); err != nil {
		t.Fatalf("parse page 2: %v body=%s", err, string(b))
	}
	if len(page2.Items) == 0 {
		t.Fatalf("expected items on page 2")
	}
	if page2.PrevCursor == nil || *page2.PrevCursor == "" {
		t.Fatalf("expected prev_cursor on page 2")
	}
}
