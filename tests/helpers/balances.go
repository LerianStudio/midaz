package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

const (
	balanceDefaultKey         = "default"
	balanceCheckTimeout       = 120 * time.Second
	balanceCheckPollInterval  = 150 * time.Millisecond
	balanceEnableTimeout      = 60 * time.Second
	balanceEnablePollInterval = 100 * time.Millisecond
	balanceHTTPStatusOK       = 200
)

var (
	// ErrDefaultBalanceNotReady indicates the default balance is not ready within timeout
	ErrDefaultBalanceNotReady = errors.New("default balance not ready for account")
	// ErrDefaultBalanceNotFound indicates the default balance was not found
	ErrDefaultBalanceNotFound = errors.New("default balance not found for alias")
	// ErrBalancePatchFailed indicates the balance PATCH request failed
	ErrBalancePatchFailed = errors.New("patch default balance failed")
	// ErrBalancesByAliasFailed indicates fetching balances by alias failed
	ErrBalancesByAliasFailed = errors.New("balances by alias request failed")
	// ErrBalanceBecameNegative indicates a balance became negative unexpectedly
	ErrBalanceBecameNegative = errors.New("available balance became negative")
	// ErrBalanceTimeoutWaiting indicates timeout waiting for expected balance
	ErrBalanceTimeoutWaiting = errors.New("timeout waiting for available sum")
)

type balanceItem struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	AssetCode string `json:"assetCode"`
}

// EnsureDefaultBalanceRecord waits until the default balance exists for the given account ID.
// It no longer attempts to create the default, as the system creates it asynchronously upon account creation.
func EnsureDefaultBalanceRecord(ctx context.Context, trans *HTTPClient, orgID, ledgerID, accountID string, headers map[string]string) error {
	deadline := time.Now().Add(balanceCheckTimeout)

	for {
		c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID), headers, nil)
		if e == nil && c == balanceHTTPStatusOK {
			var paged struct {
				Items []balanceItem `json:"items"`
			}

			_ = json.Unmarshal(b, &paged)
			for _, it := range paged.Items {
				if it.Key == balanceDefaultKey {
					return nil
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s", ErrDefaultBalanceNotReady, accountID)
		}

		time.Sleep(balanceCheckPollInterval)
	}
}

// EnableDefaultBalance sets AllowSending/AllowReceiving to true on the default balance for an account alias.
func EnableDefaultBalance(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias string, headers map[string]string) error {
	defID, err := findDefaultBalanceID(ctx, trans, orgID, ledgerID, alias, headers)
	if err != nil {
		return err
	}

	return patchDefaultBalance(ctx, trans, orgID, ledgerID, defID, headers)
}

// findDefaultBalanceID locates the default balance ID for the given alias
func findDefaultBalanceID(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias string, headers map[string]string) (string, error) {
	deadline := time.Now().Add(balanceEnableTimeout)

	for {
		c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias), headers, nil)
		if e == nil && c == balanceHTTPStatusOK {
			var paged struct {
				Items []balanceItem `json:"items"`
			}

			_ = json.Unmarshal(b, &paged)
			for _, it := range paged.Items {
				if it.Key == balanceDefaultKey {
					return it.ID, nil
				}
			}
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("%w: %s", ErrDefaultBalanceNotFound, alias)
		}

		time.Sleep(balanceEnablePollInterval)
	}
}

// patchDefaultBalance updates the default balance to allow sending and receiving
func patchDefaultBalance(ctx context.Context, trans *HTTPClient, orgID, ledgerID, defID string, headers map[string]string) error {
	payload := map[string]any{"allowSending": true, "allowReceiving": true}

	c2, b2, e2 := trans.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, defID), headers, payload)
	if e2 != nil {
		return e2
	}

	if c2 != balanceHTTPStatusOK {
		return fmt.Errorf("%w: status %d body=%s", ErrBalancePatchFailed, c2, string(b2))
	}

	return nil
}

// GetAvailableSumByAlias returns the sum of Available across all balances for the given alias and asset code.
func GetAvailableSumByAlias(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias, asset string, headers map[string]string) (decimal.Decimal, error) {
	code, body, err := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias), headers, nil)
	if err != nil {
		return decimal.Zero, err
	}

	if code != balanceHTTPStatusOK {
		return decimal.Zero, fmt.Errorf("%w: status=%d body=%s", ErrBalancesByAliasFailed, code, string(body))
	}

	var paged struct {
		Items []struct {
			AssetCode string          `json:"assetCode"`
			Available decimal.Decimal `json:"available"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &paged); err != nil {
		return decimal.Zero, fmt.Errorf("failed to unmarshal balances response: %w", err)
	}

	sum := decimal.Zero

	for _, it := range paged.Items {
		if it.AssetCode == asset {
			sum = sum.Add(it.Available)
		}
	}

	return sum, nil
}

// WaitForAvailableSumByAlias polls until the available sum for alias equals expected, or timeout.
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
				return cur, fmt.Errorf("%w for alias %s: %s", ErrBalanceBecameNegative, alias, cur.String())
			}
		}

		if time.Now().After(deadline) {
			return last, fmt.Errorf("%w; last=%s expected=%s", ErrBalanceTimeoutWaiting, last.String(), expected.String())
		}

		time.Sleep(balanceCheckPollInterval)
	}
}
