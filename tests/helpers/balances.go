// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains balance assertion and verification utilities.
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type balanceItem struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	AssetCode string `json:"assetCode"`
}

// EnsureDefaultBalanceRecord polls until the default balance for a given account
// is created, as this process is asynchronous.
func EnsureDefaultBalanceRecord(ctx context.Context, trans *HTTPClient, orgID, ledgerID, accountID string, headers map[string]string) error {
	deadline := time.Now().Add(120 * time.Second)

	for {
		c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID), headers, nil)
		if e == nil && c == 200 {
			var paged struct {
				Items []balanceItem `json:"items"`
			}

			_ = json.Unmarshal(b, &paged)
			for _, it := range paged.Items {
				if it.Key == "default" {
					return nil
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("default balance not ready for account %s", accountID)
		}

		time.Sleep(150 * time.Millisecond)
	}
}

// EnableDefaultBalance enables sending and receiving on the default balance for
// an account, identified by its alias.
func EnableDefaultBalance(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias string, headers map[string]string) error {
	// Get balances by alias
	var defID string

	deadline := time.Now().Add(60 * time.Second)

	for {
		c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias), headers, nil)
		if e == nil && c == 200 {
			var paged struct {
				Items []balanceItem `json:"items"`
			}

			_ = json.Unmarshal(b, &paged)
			for _, it := range paged.Items {
				if it.Key == "default" {
					defID = it.ID
					break
				}
			}

			if defID != "" {
				break
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("default balance not found for alias %s", alias)
		}

		time.Sleep(100 * time.Millisecond)
	}

	if defID == "" {
		return fmt.Errorf("default balance not found for alias %s", alias)
	}
	// PATCH update
	payload := map[string]any{"allowSending": true, "allowReceiving": true}

	c2, b2, e2 := trans.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, defID), headers, payload)
	if e2 != nil {
		return e2
	}

	if c2 != 200 {
		return fmt.Errorf("patch default balance: status %d body=%s", c2, string(b2))
	}

	return nil
}

// GetAvailableSumByAlias calculates the total available balance for a specific
// asset by summing up the `available` fields across all balances of an account.
func GetAvailableSumByAlias(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias, asset string, headers map[string]string) (decimal.Decimal, error) {
	code, body, err := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias), headers, nil)
	if err != nil {
		return decimal.Zero, err
	}

	if code != 200 {
		return decimal.Zero, fmt.Errorf("balances by alias status=%d body=%s", code, string(body))
	}

	var paged struct {
		Items []struct {
			AssetCode string          `json:"assetCode"`
			Available decimal.Decimal `json:"available"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &paged); err != nil {
		return decimal.Zero, err
	}

	sum := decimal.Zero

	for _, it := range paged.Items {
		if it.AssetCode == asset {
			sum = sum.Add(it.Available)
		}
	}

	return sum, nil
}

// WaitForAvailableSumByAlias polls an account's balances until the total available
// sum for a specific asset reaches an expected value, or until a timeout occurs.
func WaitForAvailableSumByAlias(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias, asset string, headers map[string]string, expected decimal.Decimal, timeout time.Duration) (decimal.Decimal, error) {
	deadline := time.Now().Add(timeout)

	var last decimal.Decimal

	for {
		cur, err := GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, asset, headers)
		if err == nil {
			last = cur
			if cur.Equal(expected) {
				return cur, nil
			}
			// guard that it never becomes negative
			if cur.IsNegative() {
				return cur, fmt.Errorf("available for alias %s became negative: %s", alias, cur.String())
			}
		}

		if time.Now().After(deadline) {
			return last, fmt.Errorf("timeout waiting for available sum; last=%s expected=%s", last.String(), expected.String())
		}

		time.Sleep(150 * time.Millisecond)
	}
}
