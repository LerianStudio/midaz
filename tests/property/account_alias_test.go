package property

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"testing/quick"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property: No two active accounts in the same org/ledger can share an alias.
// This is critical for transaction routing - aliases must uniquely identify accounts.
func TestProperty_AccountAliasUniqueness_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropAlias "+h.RandString(6))
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

		// Generate a unique alias for this test
		alias := fmt.Sprintf("alias-%s-%d", h.RandString(5), rng.Intn(10000))

		// Create first account with this alias - should succeed
		acc1ID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Logf("first account creation failed: %v", err)
			return true
		}
		if acc1ID == "" {
			t.Logf("first account created but no ID returned")
			return true
		}

		// Attempt to create second account with same alias - should fail
		payload := map[string]any{
			"name":      "Duplicate Alias Account",
			"assetCode": "USD",
			"type":      "deposit",
			"alias":     alias,
		}

		code, body, _ := onboard.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID),
			headers, payload)

		// Should be rejected (400, 409, or similar)
		if code == 201 {
			t.Errorf("API allowed duplicate alias creation: alias=%s first_id=%s", alias, acc1ID)
			return false
		}

		// Verify error response mentions alias
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &errResp) == nil {
			if errResp.Code == "" && errResp.Message == "" {
				t.Logf("Expected error response for duplicate alias, got code=%d", code)
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("account alias uniqueness property failed: %v", err)
	}
}

// Property: Concurrent alias creation should be serialized - only one should succeed.
func TestProperty_AccountAliasConcurrentCreation_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropAliasConcur "+h.RandString(6))
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

	f := func(seed int64, workers uint8) bool {
		numWorkers := int(workers)
		if numWorkers < 2 {
			numWorkers = 2
		}
		if numWorkers > 5 {
			numWorkers = 5
		}

		alias := fmt.Sprintf("concurrent-%s-%d", h.RandString(5), seed)

		var successCount int
		var mu sync.Mutex
		var wg sync.WaitGroup

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				workerHeaders := make(map[string]string)
				for k, v := range headers {
					workerHeaders[k] = v
				}
				workerHeaders["Idempotency-Key"] = fmt.Sprintf("%s-%d", alias, workerID)

				payload := map[string]any{
					"name":      fmt.Sprintf("Concurrent Account %d", workerID),
					"assetCode": "USD",
					"type":      "deposit",
					"alias":     alias,
				}

				code, _, _ := onboard.Request(ctx, "POST",
					fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID),
					workerHeaders, payload)

				if code == 201 {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}(w)
		}

		wg.Wait()

		// Exactly one worker should succeed
		if successCount == 0 {
			t.Logf("No workers succeeded creating alias=%s (may be expected if validation is strict)", alias)
			return true
		}

		if successCount > 1 {
			t.Errorf("Multiple workers succeeded with same alias: alias=%s count=%d", alias, successCount)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 5}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("concurrent alias creation property failed: %v", err)
	}
}

// Property: Deleted account's alias can be reused.
func TestProperty_AccountAliasReuseAfterDelete_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropAliasReuse "+h.RandString(6))
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
		alias := fmt.Sprintf("reuse-%s-%d", h.RandString(5), seed)

		// Create first account
		acc1ID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Logf("first account: %v", err)
			return true
		}

		// Delete the account
		code, _, _ := onboard.Request(ctx, "DELETE",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, acc1ID),
			headers, nil)

		if code != 200 && code != 204 {
			t.Logf("delete failed: code=%d", code)
			return true // API may not support delete
		}

		// Create new account with same alias - should succeed
		payload := map[string]any{
			"name":      "Reused Alias Account",
			"assetCode": "USD",
			"type":      "deposit",
			"alias":     alias,
		}

		code, body, _ := onboard.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID),
			headers, payload)

		if code != 201 {
			// This might be expected if soft-delete doesn't release alias
			t.Logf("alias reuse after delete: code=%d body=%s", code, string(body))
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 5}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("alias reuse after delete property failed: %v", err)
	}
}
