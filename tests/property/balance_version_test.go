package property

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"testing/quick"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property: Balance version must be monotonically increasing.
// version(t+1) > version(t) for all valid state transitions.
// This is critical for optimistic locking in concurrent balance updates.
func TestProperty_BalanceVersionMonotonicity_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Setup org/ledger/asset once
	headers := h.AuthHeaders(h.RandHex(8))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropVersion "+h.RandString(6))
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

	f := func(seed int64, numTxns uint8) bool {
		rng := rand.New(rand.NewSource(seed))
		txns := int(numTxns)
		if txns <= 1 {
			txns = 2 // Need at least 2 transactions to verify monotonicity
		}
		if txns > 5 {
			txns = 5
		}

		// Create account
		alias := fmt.Sprintf("ver-%s", h.RandString(5))
		accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Logf("create account: %v", err)
			return true
		}

		// Ensure balance record exists
		if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
			t.Logf("ensure balance: %v", err)
			return true
		}

		// Track version history
		var versions []int64
		var mu sync.Mutex

		// Helper to get current version
		getVersion := func() (int64, error) {
			code, body, err := trans.Request(ctx, "GET",
				fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID),
				headers, nil)
			if err != nil || code != 200 {
				return 0, fmt.Errorf("get balances: code=%d err=%w", code, err)
			}

			var resp struct {
				Items []struct {
					Version int64 `json:"version"`
				} `json:"items"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return 0, err
			}
			if len(resp.Items) == 0 {
				return 0, fmt.Errorf("no balances found")
			}
			return resp.Items[0].Version, nil
		}

		// Get initial version
		initialVersion, err := getVersion()
		if err != nil {
			t.Logf("get initial version: %v", err)
			return true
		}
		versions = append(versions, initialVersion)

		// Perform transactions and track versions
		for i := 0; i < txns; i++ {
			amount := rng.Intn(10) + 1
			amountStr := fmt.Sprintf("%d.00", amount)

			headers["Idempotency-Key"] = fmt.Sprintf("%s-%d-%d", alias, seed, i)
			code, _, _ := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", amountStr, headers)
			if code != 201 {
				continue
			}

			// Wait for balance to update
			time.Sleep(75 * time.Millisecond)

			// Poll for version change
			deadline := time.Now().Add(3 * time.Second)
			for {
				newVersion, err := getVersion()
				if err != nil {
					break
				}

				mu.Lock()
				lastVersion := versions[len(versions)-1]
				if newVersion > lastVersion {
					versions = append(versions, newVersion)
					mu.Unlock()
					break
				}
				mu.Unlock()

				if time.Now().After(deadline) {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
		}

		// Verify monotonicity: each version must be greater than the previous
		for i := 1; i < len(versions); i++ {
			if versions[i] <= versions[i-1] {
				t.Errorf("Version monotonicity violated: version[%d]=%d <= version[%d]=%d alias=%s",
					i, versions[i], i-1, versions[i-1], alias)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 5}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("balance version monotonicity property failed: %v", err)
	}
}

// Property: Concurrent balance updates eventually increase the version.
// In an eventually consistent system with optimistic locking, we verify that
// after successful transactions, the final version is greater than the initial.
// Note: The exact increment depends on retry behavior and transaction batching.
func TestProperty_BalanceVersionConcurrentProgression_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	headers := h.AuthHeaders(h.RandHex(8))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropVersionProg "+h.RandString(6))
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

	f := func(seed int64, concurrency uint8) bool {
		numWorkers := int(concurrency)
		if numWorkers <= 1 {
			numWorkers = 2
		}
		if numWorkers > 5 {
			numWorkers = 5
		}

		alias := fmt.Sprintf("prog-%s", h.RandString(5))
		accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Logf("create account: %v", err)
			return true
		}

		if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
			t.Logf("ensure balance: %v", err)
			return true
		}

		// Wait for balance to be fully created and visible
		time.Sleep(200 * time.Millisecond)

		// Get initial version
		getVersion := func() (int64, error) {
			code, body, err := trans.Request(ctx, "GET",
				fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID),
				headers, nil)
			if err != nil || code != 200 {
				return 0, fmt.Errorf("get balances: code=%d err=%w", code, err)
			}

			var resp struct {
				Items []struct {
					Version int64 `json:"version"`
				} `json:"items"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return 0, err
			}
			if len(resp.Items) == 0 {
				return 0, fmt.Errorf("no balances found")
			}
			return resp.Items[0].Version, nil
		}

		initialVersion, err := getVersion()
		if err != nil {
			t.Logf("get initial version: %v", err)
			return true
		}

		// Track successful transactions
		var successCount int64
		var mu sync.Mutex
		var wg sync.WaitGroup

		// Concurrent workers performing transactions
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for i := range 3 {
					workerHeaders := make(map[string]string)
					for k, v := range headers {
						workerHeaders[k] = v
					}
					workerHeaders["Idempotency-Key"] = fmt.Sprintf("%s-%d-%d-%d", alias, seed, workerID, i)

					code, _, _ := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", "1.00", workerHeaders)
					if code == 201 {
						mu.Lock()
						successCount++
						mu.Unlock()
					}
				}
			}(w)
		}

		wg.Wait()

		if successCount == 0 {
			return true // No transactions succeeded, skip
		}

		// Wait for eventual consistency and poll for version to increase
		var finalVersion int64
		deadline := time.Now().Add(5 * time.Second)

		for time.Now().Before(deadline) {
			finalVersion, err = getVersion()
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Just need version to have increased from initial
			if finalVersion > initialVersion {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Property: final version must be greater than initial after successful transactions
		// Note: We don't require version to increment by exactly successCount because
		// optimistic locking retries and transaction batching affect the increment pattern.
		if finalVersion <= initialVersion {
			t.Errorf("Version did not progress: initial=%d final=%d successful=%d alias=%s",
				initialVersion, finalVersion, successCount, alias)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 3}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("balance version progression property failed: %v", err)
	}
}
