package helpers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)


// ErrBalanceChangeTimeout indicates timeout waiting for balance change
var ErrBalanceChangeTimeout = errors.New("timeout waiting for balance change")

// BalanceSnapshot captures the state of an account balance at a point in time
type BalanceSnapshot struct {
	Available decimal.Decimal
	Block     decimal.Decimal
	On_hold   decimal.Decimal
	Timestamp time.Time
}

// GetBalanceSnapshot captures the current balance state for an account
func GetBalanceSnapshot(ctx context.Context, client *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, headers map[string]string) (*BalanceSnapshot, error) {
	// Get current balance
	available, err := GetAvailableSumByAlias(ctx, client, orgID, ledgerID, accountAlias, assetCode, headers)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
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

// WaitForBalanceChange waits for the balance to change by the expected delta from a snapshot
func WaitForBalanceChange(ctx context.Context, client *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, headers map[string]string, snapshot *BalanceSnapshot, expectedDelta decimal.Decimal, timeout time.Duration) (decimal.Decimal, error) {
	expectedFinal := snapshot.Available.Add(expectedDelta)

	deadline := time.Now().Add(timeout)
	lastSeen := snapshot.Available

	for time.Now().Before(deadline) {
		current, err := GetAvailableSumByAlias(ctx, client, orgID, ledgerID, accountAlias, assetCode, headers)
		if err != nil {
			//nolint:wrapcheck // Error already wrapped with context for test helpers
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

		time.Sleep(PollIntervalFast)
	}

	actualDelta := lastSeen.Sub(snapshot.Available)

	//nolint:wrapcheck // Error already wrapped with context for test helpers
	return lastSeen, fmt.Errorf("%w; initial=%s expected_delta=%s actual_delta=%s last=%s expected_final=%s",
		ErrBalanceChangeTimeout, snapshot.Available.String(), expectedDelta.String(), actualDelta.String(), lastSeen.String(), expectedFinal.String())
}

// OperationTracker tracks balance changes during an operation.
type OperationTracker struct {
	OrgID           string
	LedgerID        string
	AccountAlias    string
	AssetCode       string
	Headers         map[string]string
	Client          *HTTPClient
	InitialSnapshot *BalanceSnapshot
}

// NewOperationTracker creates a new operation tracker for an account
func NewOperationTracker(ctx context.Context, client *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, headers map[string]string) (*OperationTracker, error) {
	snapshot, err := GetBalanceSnapshot(ctx, client, orgID, ledgerID, accountAlias, assetCode, headers)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
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

// VerifyDelta verifies that the balance changed by the expected amount
func (ot *OperationTracker) VerifyDelta(ctx context.Context, expectedDelta decimal.Decimal, timeout time.Duration) (decimal.Decimal, error) {
	return WaitForBalanceChange(ctx, ot.Client, ot.OrgID, ot.LedgerID, ot.AccountAlias, ot.AssetCode, ot.Headers, ot.InitialSnapshot, expectedDelta, timeout)
}

// GetCurrentDelta returns the current balance delta from the initial snapshot
func (ot *OperationTracker) GetCurrentDelta(ctx context.Context) (decimal.Decimal, error) {
	current, err := GetBalanceSnapshot(ctx, ot.Client, ot.OrgID, ot.LedgerID, ot.AccountAlias, ot.AssetCode, ot.Headers)
	if err != nil {
		return decimal.Zero, err
	}

	return current.Available.Sub(ot.InitialSnapshot.Available), nil
}
