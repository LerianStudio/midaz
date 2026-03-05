// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

type replayObserver struct {
	replaySkippedReasons []string
}

func (o *replayObserver) ObserveAuthorizeLockWait(_, _ int, _ time.Duration) {}
func (o *replayObserver) ObserveAuthorizeLockHold(_, _ int, _ time.Duration) {}
func (o *replayObserver) ObserveWALAppendFailure(_ error)                    {}

func (o *replayObserver) ObserveWALReplaySkipped(reason, _ string, _ int) {
	o.replaySkippedReasons = append(o.replaySkippedReasons, reason)
}

func TestReplayEntriesEmptyBalanceKeyFallsBackToDefault(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10000,
		OnHold:         0,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	err := eng.ReplayEntries([]wal.Entry{{
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{{
			AccountAlias:    "@alice",
			BalanceKey:      "",
			Available:       9000,
			OnHold:          0,
			PreviousVersion: 1,
			NextVersion:     2,
		}},
	}})
	require.NoError(t, err)

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(9000), balance.Available)
	require.Equal(t, uint64(2), balance.Version)
}

func TestReplayEntriesEnforcesMutationLimitAndEmitsSkipReason(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	observer := &replayObserver{}
	eng.SetObserver(observer)
	eng.ConfigureReplayPolicy(1, 10, false)

	err := eng.ReplayEntries([]wal.Entry{{
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, PreviousVersion: 1, NextVersion: 2},
			{AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, PreviousVersion: 1, NextVersion: 2},
		},
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"mutation_limit_exceeded"}, observer.replaySkippedReasons)
}

func TestReplayEntriesStrictModeReturnsErrorOnSkippedEntry(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureReplayPolicy(10, 10, true)

	err := eng.ReplayEntries([]wal.Entry{{
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{{
			AccountAlias:    "@missing",
			BalanceKey:      constant.DefaultBalanceKey,
			Available:       1,
			OnHold:          0,
			PreviousVersion: 1,
			NextVersion:     2,
		}},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing_balance")
}

func TestReplayEntriesStrictModeReturnsErrorOnLockLimitExceeded(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@bob",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	eng.ConfigureReplayPolicy(10, 1, true)

	err := eng.ReplayEntries([]wal.Entry{{
		TransactionID:  "tx-lock-limit",
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, PreviousVersion: 1, NextVersion: 2},
			{AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, PreviousVersion: 1, NextVersion: 2},
		},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lock_limit_exceeded")
}

func TestReplayEntriesStrictModeReturnsErrorOnVersionMismatch(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10000,
		Scale:          2,
		Version:        5,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	eng.ConfigureReplayPolicy(10, 10, true)

	err := eng.ReplayEntries([]wal.Entry{{
		TransactionID:  "tx-version-mismatch",
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{{
			AccountAlias:    "@alice",
			BalanceKey:      constant.DefaultBalanceKey,
			Available:       9000,
			OnHold:          0,
			PreviousVersion: 4,
			NextVersion:     5,
		}},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "version_mismatch")
}

func TestEngineDefaultsReplayStrictModeToTrue(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	err := eng.ReplayEntries([]wal.Entry{{
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{{
			AccountAlias:    "@missing",
			BalanceKey:      constant.DefaultBalanceKey,
			Available:       1,
			OnHold:          0,
			PreviousVersion: 1,
			NextVersion:     2,
		}},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing_balance")
}

func TestAuthorizeRejectsRequestExceedingOperationLimit(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureAuthorizationLimits(1, 10)

	resp, err := eng.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-op-limit",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1", AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	})
	require.NoError(t, err)
	require.False(t, resp.GetAuthorized())
	require.Equal(t, RejectionRequestTooLarge, resp.GetRejectionCode())
	require.Contains(t, resp.GetRejectionMessage(), "operations exceed allowed request limit")
}

func TestPrepareAuthorizeRejectsRequestExceedingUniqueBalanceLimit(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureAuthorizationLimits(10, 1)
	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@bob",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-unique-limit",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1", AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	})
	require.NoError(t, err)
	require.Nil(t, ptx)
	require.NotNil(t, resp)
	require.False(t, resp.GetAuthorized())
	require.Equal(t, RejectionRequestTooLarge, resp.GetRejectionCode())
	require.Contains(t, resp.GetRejectionMessage(), "unique balances exceed allowed request limit")
}

func TestAuthorizeAllowsRequestAtExactOperationLimit(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureAuthorizationLimits(2, 10)

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@bob",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	resp, err := eng.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-op-boundary",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1", AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized())
}

func TestPrepareAuthorizeAllowsRequestAtExactUniqueBalanceLimit(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureAuthorizationLimits(10, 2)
	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@bob",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-unique-boundary",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1", AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())
	require.NoError(t, eng.AbortPrepared(ptx.ID))
}

func TestReplayEntriesSequentialAndIdempotentDoubleReplay(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureReplayPolicy(2048, 2048, false)
	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	entries := []wal.Entry{
		{
			TransactionID:  "tx-r1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			Mutations: []wal.BalanceMutation{{
				AccountAlias:    "@alice",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       9000,
				OnHold:          0,
				PreviousVersion: 1,
				NextVersion:     2,
			}},
		},
		{
			TransactionID:  "tx-r2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			Mutations: []wal.BalanceMutation{{
				AccountAlias:    "@alice",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       8000,
				OnHold:          0,
				PreviousVersion: 2,
				NextVersion:     3,
			}},
		},
	}

	require.NoError(t, eng.ReplayEntries(entries))

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(8000), balance.Available)
	require.Equal(t, uint64(3), balance.Version)

	require.NoError(t, eng.ReplayEntries(entries))

	balance, ok = eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(8000), balance.Available)
	require.Equal(t, uint64(3), balance.Version)
}

// TestReplayEntriesAndAuthorizeConcurrentStress exercises concurrent access between
// Authorize (which mutates balances) and ReplayEntries (which attempts to apply WAL
// mutations). The replay entries intentionally use PreviousVersion: 0 / NextVersion: 0,
// while balances start at Version 1. This guarantees every replay iteration hits the
// "version_mismatch" skip path rather than actually applying mutations. The purpose is
// to stress-test lock acquisition, contention, and safe skip behavior under concurrent
// load -- not to verify that mutations are applied correctly (other tests cover that).
func TestReplayEntriesAndAuthorizeConcurrentStress(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureReplayPolicy(10, 10, false)
	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      100000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	var wg sync.WaitGroup

	wg.Add(2)

	errCh := make(chan error, 1)

	go func() {
		defer wg.Done()

		for i := 0; i < 200; i++ {
			_, err := eng.Authorize(&authorizerv1.AuthorizeRequest{
				TransactionId:     fmt.Sprintf("tx-concurrent-%d", i),
				OrganizationId:    "org",
				LedgerId:          "ledger",
				Pending:           false,
				TransactionStatus: constant.CREATED,
				Operations: []*authorizerv1.BalanceOperation{
					{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 1, Scale: 2, Operation: constant.DEBIT},
				},
			})
			if err != nil {
				select {
				case errCh <- err:
				default:
				}

				return
			}
		}
	}()

	go func() {
		defer wg.Done()

		for i := 0; i < 200; i++ {
			err := eng.ReplayEntries([]wal.Entry{{
				TransactionID:  fmt.Sprintf("replay-concurrent-%d", i),
				OrganizationID: "org",
				LedgerID:       "ledger",
				Mutations: []wal.BalanceMutation{{
					AccountAlias:    "@alice",
					BalanceKey:      constant.DefaultBalanceKey,
					Available:       100000,
					OnHold:          0,
					PreviousVersion: 0,
					NextVersion:     0,
				}},
			}})
			if err != nil {
				select {
				case errCh <- err:
				default:
				}

				return
			}
		}
	}()

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent replay+authorize stress test timed out")
	}

	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}
}

func TestNormalizeExternalOperationsDistributesOverflowCounterparties(t *testing.T) {
	router := shard.NewRouter(8)
	ops := []*authorizerv1.BalanceOperation{
		{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
		{AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey},
		{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
		{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
		{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
	}

	normalized := normalizeExternalOperations(ops, router)
	require.Len(t, normalized, len(ops))

	key1 := normalized[2].GetBalanceKey()
	key2 := normalized[3].GetBalanceKey()
	key3 := normalized[4].GetBalanceKey()

	require.Equal(t, router.ResolveExternalBalanceKey("@alice"), key1)
	require.Equal(t, router.ResolveExternalBalanceKey("@bob"), key2)
	require.Equal(t, router.ResolveExternalBalanceKey("@alice"), key3)
}

func TestNormalizeExternalOperationsIgnoresNilIndicesForCounterpartySelection(t *testing.T) {
	router := shard.NewRouter(8)
	ops := []*authorizerv1.BalanceOperation{
		nil,
		{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
		nil,
		{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
		{AccountAlias: "@external/USD", BalanceKey: constant.DefaultBalanceKey},
	}

	normalized := normalizeExternalOperations(ops, router)
	require.Len(t, normalized, 3)
	require.Equal(t, "@alice", normalized[0].GetAccountAlias())
	require.Equal(t, router.ResolveExternalBalanceKey("@alice"), normalized[1].GetBalanceKey())
	require.Equal(t, router.ResolveExternalBalanceKey("@alice"), normalized[2].GetBalanceKey())
}

func TestApplyOperationPendingBranches(t *testing.T) {
	tests := []struct {
		name       string
		available  int64
		onHold     int64
		pending    bool
		status     string
		op         string
		amount     int64
		wantAvail  int64
		wantOnHold int64
	}{
		{name: "pending onhold", available: 100, onHold: 0, pending: true, status: constant.PENDING, op: constant.ONHOLD, amount: 10, wantAvail: 90, wantOnHold: 10},
		{name: "pending canceled release", available: 90, onHold: 10, pending: true, status: constant.CANCELED, op: constant.RELEASE, amount: 5, wantAvail: 95, wantOnHold: 5},
		{name: "approved compensate debit", available: 90, onHold: 10, pending: true, status: "APPROVED_COMPENSATE", op: constant.DEBIT, amount: 5, wantAvail: 90, wantOnHold: 15},
		{name: "approved compensate credit", available: 90, onHold: 10, pending: true, status: "APPROVED_COMPENSATE", op: constant.CREDIT, amount: 5, wantAvail: 85, wantOnHold: 10},
		{name: "approved compensate release", available: 90, onHold: 10, pending: true, status: "APPROVED_COMPENSATE", op: constant.RELEASE, amount: 5, wantAvail: 85, wantOnHold: 15},
		{name: "approved compensate onhold", available: 90, onHold: 10, pending: true, status: "APPROVED_COMPENSATE", op: constant.ONHOLD, amount: 5, wantAvail: 95, wantOnHold: 5},
		{name: "approved debit", available: 90, onHold: 10, pending: true, status: constant.APPROVED, op: constant.DEBIT, amount: 5, wantAvail: 90, wantOnHold: 5},
		{name: "approved release", available: 90, onHold: 10, pending: true, status: constant.APPROVED, op: constant.RELEASE, amount: 5, wantAvail: 95, wantOnHold: 5},
		{name: "approved default", available: 90, onHold: 10, pending: true, status: constant.APPROVED, op: constant.CREDIT, amount: 5, wantAvail: 95, wantOnHold: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAvail, gotOnHold, err := applyOperation(tt.available, tt.onHold, tt.pending, tt.status, tt.op, tt.amount)
			require.NoError(t, err)
			require.Equal(t, tt.wantAvail, gotAvail)
			require.Equal(t, tt.wantOnHold, gotOnHold)
		})
	}
}

func TestApplyOperationNonPendingBranches(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		available  int64
		onHold     int64
		amount     int64
		wantAvail  int64
		wantOnHold int64
	}{
		{name: "debit subtracts from available", operation: constant.DEBIT, available: 100, onHold: 7, amount: 5, wantAvail: 95, wantOnHold: 7},
		{name: "credit adds to available", operation: constant.CREDIT, available: 100, onHold: 7, amount: 5, wantAvail: 105, wantOnHold: 7},
		{name: "zero amount no-op", operation: constant.DEBIT, available: 100, onHold: 7, amount: 0, wantAvail: 100, wantOnHold: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAvail, gotOnHold, err := applyOperation(tt.available, tt.onHold, false, constant.CREATED, tt.operation, tt.amount)
			require.NoError(t, err)
			require.Equal(t, tt.wantAvail, gotAvail)
			require.Equal(t, tt.wantOnHold, gotOnHold)
		})
	}
}

func TestApplyOperationExtremeValues(t *testing.T) {
	t.Run("max int64 debit does not panic", func(t *testing.T) {
		require.NotPanics(t, func() {
			gotAvail, gotOnHold, err := applyOperation(math.MaxInt64, 0, false, constant.CREATED, constant.DEBIT, math.MaxInt64)
			require.NoError(t, err)
			require.Equal(t, int64(0), gotAvail)
			require.Equal(t, int64(0), gotOnHold)
		})
	})

	t.Run("overflow returns error", func(t *testing.T) {
		_, _, err := applyOperation(math.MaxInt64, 0, false, constant.CREATED, constant.CREDIT, 1)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrIntegerOverflow)
	})

	t.Run("underflow returns error", func(t *testing.T) {
		_, _, err := applyOperation(math.MinInt64, 0, false, constant.CREATED, constant.DEBIT, 1)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrIntegerUnderflow)
	})
}

func TestRescaleAmountEdgeCases(t *testing.T) {
	rescaled, err := rescaleAmount(123, 2, 2)
	require.NoError(t, err)
	require.Equal(t, int64(123), rescaled)

	rescaled, err = rescaleAmount(123, 2, 3)
	require.NoError(t, err)
	require.Equal(t, int64(1230), rescaled)

	rescaled, err = rescaleAmount(123, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(123), rescaled)

	_, err = rescaleAmount(1234, 3, 2)
	require.ErrorIs(t, err, pkgTransaction.ErrPrecisionLoss)
}
