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

// Database success, full cache overlay.
// - Perform a credit inflow to the alias to ensure cache entry exists.
// - GET /balances and assert the available value eventually reflects the inflow amount.
func TestIntegration_GetAllBalances_FullCacheOverlay(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup separately: org -> ledger -> asset -> account
	orgID2, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID2, err := h.SetupLedger(ctx, onboard, headers, orgID2, iso.UniqueLedgerName("Ledger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID2, ledgerID2, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := iso.UniqueAccountAlias("acc")
	_, err = h.SetupAccount(ctx, onboard, headers, orgID2, ledgerID2, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// 4) Perform inflow to create/update cache entry with a known value
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID2, ledgerID2, alias, "USD", "123.45", headers)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 5) Poll GET /balances until the available for our alias default matches 123.45
	want, _ := decimal.NewFromString("123.45")
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances", orgID2, ledgerID2)
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

// Very large decimal magnitudes and precision preservation end-to-end.
// - Enable default balance.
// - Perform an inflow with a very large precision amount to seed cache.
// - GET /balances and assert the available value eventually equals the large amount exactly.
func TestIntegration_GetAllBalances_VeryLargePrecisionOverlay(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup separately: org -> ledger -> asset -> account
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
	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Perform inflow to create/update cache entry with a very large amount
	largeAmount := "123456789012345678901234567890.123456789012345678901234567890"
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", largeAmount, headers)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// Poll GET /balances until the available for our alias default matches the large amount
	want, _ := decimal.NewFromString(largeAmount)
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances", orgID, ledgerID)
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

// Filtering passthrough by dates.
// - Use a past-only window to expect zero items.
// - Use today's window to expect created balances.
func TestIntegration_GetAllBalances_FilteringByDate(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup org/ledger/USD asset
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

	// Create a few accounts to ensure balances exist today
	for i := 0; i < 3; i++ {
		alias := iso.UniqueAccountAlias("flt")
		if _, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD"); err != nil {
			t.Fatalf("create account %d: %v", i, err)
		}
	}

	// 1) Past-only window: expect zero items
	pastStart := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	pastEnd := time.Now().AddDate(0, 0, -9).Format("2006-01-02")
	pathPast := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances?start_date=%s&end_date=%s", orgID, ledgerID, pastStart, pastEnd)
	if code, b, err := trans.Request(ctx, "GET", pathPast, headers, nil); err != nil || code != 200 {
		t.Fatalf("past window request failed: code=%d err=%v body=%s", code, err, string(b))
	} else {
		var page struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal(b, &page)
		if len(page.Items) != 0 {
			t.Fatalf("expected 0 items for past-only window, got %d", len(page.Items))
		}
	}

	// 2) Today's window: expect items present
	today := time.Now().Format("2006-01-02")
	pathToday := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances?start_date=%s&end_date=%s", orgID, ledgerID, today, today)
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

// Pagination passthrough for GetAllBalances.
// - Seed more than limit balances across different aliases.
// - Page 1: verify limit items and non-empty next_cursor.
// - Page 2 (using next): verify subsequent items and non-empty prev_cursor.
func TestIntegration_GetAllBalances_PaginationPassthrough(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup org/ledger/USD asset
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

	// Create N accounts > limit and rely on eventual page materialization
	limit := 5
	total := 12
	for i := 0; i < total; i++ {
		alias := iso.UniqueAccountAlias("pg")
		if _, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD"); err != nil {
			t.Fatalf("create account %d: %v", i, err)
		}
	}

	// Page 1 with limit
	path1 := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances?limit=%d", orgID, ledgerID, limit)
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
	path2 := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances?limit=%d&cursor=%s", orgID, ledgerID, limit, url.QueryEscape(next))
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
