// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains balance tracking utilities for verifying transaction correctness.
package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// BalanceSnapshot captures the state of an account's balance at a specific point in time.
type BalanceSnapshot struct {
	Available decimal.Decimal
	Block     decimal.Decimal
	On_hold   decimal.Decimal
	Timestamp time.Time
}

// GetBalanceSnapshot captures the current balance state for a given account.
func GetBalanceSnapshot(ctx context.Context, client *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, headers map[string]string) (*BalanceSnapshot, error) {
	// Get current balance
	available, err := GetAvailableSumByAlias(ctx, client, orgID, ledgerID, accountAlias, assetCode, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance snapshot: %w", err)
	}

	// For now, we only track available balance
	// Could be extended to track block and on_hold if needed
	return &BalanceSnapshot{
		Available: available,
		Block:     decimal.Zero,
		On_hold:   decimal.Zero,
		Timestamp: time.Now(),
	}, nil
}

// WaitForBalanceChange polls an account's balance until it changes by an expected
// delta from a previous snapshot, or until a timeout is reached.
func WaitForBalanceChange(ctx context.Context, client *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, headers map[string]string, snapshot *BalanceSnapshot, expectedDelta decimal.Decimal, timeout time.Duration) (decimal.Decimal, error) {
	expectedFinal := snapshot.Available.Add(expectedDelta)

	deadline := time.Now().Add(timeout)
	lastSeen := snapshot.Available

	for time.Now().Before(deadline) {
		current, err := GetAvailableSumByAlias(ctx, client, orgID, ledgerID, accountAlias, assetCode, headers)
		if err != nil {
			return lastSeen, fmt.Errorf("failed to get current balance: %w", err)
		}

		lastSeen = current

		// Check if we've reached the expected value
		if current.Equal(expectedFinal) {
			return current, nil
		}

		// If we've exceeded the expected value (in case of concurrent operations)
		// and the delta is positive, that's acceptable
		if expectedDelta.IsPositive() && current.GreaterThan(expectedFinal) {
			return current, nil
		}

		// If we've undershot the expected value (in case of concurrent operations)
		// and the delta is negative, that's also acceptable
		if expectedDelta.IsNegative() && current.LessThan(expectedFinal) {
			return current, nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	actualDelta := lastSeen.Sub(snapshot.Available)

	return lastSeen, fmt.Errorf("timeout waiting for balance change; initial=%s expected_delta=%s actual_delta=%s last=%s expected_final=%s",
		snapshot.Available.String(), expectedDelta.String(), actualDelta.String(), lastSeen.String(), expectedFinal.String())
}

// OperationTracker provides a way to track balance changes for a specific account during a test.
type OperationTracker struct {
	OrgID           string
	LedgerID        string
	AccountAlias    string
	AssetCode       string
	Headers         map[string]string
	Client          *HTTPClient
	InitialSnapshot *BalanceSnapshot
}

// NewOperationTracker creates a new balance tracker for an account, taking an
// initial snapshot of its balance.
func NewOperationTracker(ctx context.Context, client *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, headers map[string]string) (*OperationTracker, error) {
	snapshot, err := GetBalanceSnapshot(ctx, client, orgID, ledgerID, accountAlias, assetCode, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to create operation tracker: %w", err)
	}

	return &OperationTracker{
		OrgID:           orgID,
		LedgerID:        ledgerID,
		AccountAlias:    accountAlias,
		AssetCode:       assetCode,
		Headers:         headers,
		Client:          client,
		InitialSnapshot: snapshot,
	}, nil
}

// VerifyDelta verifies that the account's balance has changed by the expected
// amount since the initial snapshot was taken.
func (ot *OperationTracker) VerifyDelta(ctx context.Context, expectedDelta decimal.Decimal, timeout time.Duration) (decimal.Decimal, error) {
	return WaitForBalanceChange(ctx, ot.Client, ot.OrgID, ot.LedgerID, ot.AccountAlias, ot.AssetCode, ot.Headers, ot.InitialSnapshot, expectedDelta, timeout)
}

// GetCurrentDelta returns the current difference between the account's live balance
// and the balance stored in the initial snapshot.
func (ot *OperationTracker) GetCurrentDelta(ctx context.Context) (decimal.Decimal, error) {
	current, err := GetBalanceSnapshot(ctx, ot.Client, ot.OrgID, ot.LedgerID, ot.AccountAlias, ot.AssetCode, ot.Headers)
	if err != nil {
		return decimal.Zero, err
	}

	return current.Available.Sub(ot.InitialSnapshot.Available), nil
}
