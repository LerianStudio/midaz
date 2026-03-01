// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"errors"
	"sync"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/require"
)

type failingWALWriter struct{}

func (failingWALWriter) Append(_ wal.Entry) error {
	return errors.New("wal append failed")
}

func (failingWALWriter) Close() error {
	return nil
}

func TestAuthorizeSingleDebitCredit(t *testing.T) {
	e := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      10000,
			OnHold:         0,
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
			AssetCode:      "BRL",
			Available:      0,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx1",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 1000, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@bob#default", AccountAlias: "@bob", BalanceKey: "default", Amount: 1000, Scale: 2, Operation: constant.CREDIT},
		},
	})

	require.NoError(t, err)
	require.True(t, resp.Authorized)
	require.Len(t, resp.Balances, 2)

	alice, ok := e.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(9000), alice.Available)
}

func TestAuthorizeInsufficientFunds(t *testing.T) {
	e := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      100,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx2",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 1000, Scale: 2, Operation: constant.DEBIT},
		},
	})

	require.NoError(t, err)
	require.False(t, resp.Authorized)
	require.Equal(t, engine.RejectionInsufficientFunds, resp.RejectionCode)

	alice, ok := e.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(100), alice.Available)
}

func TestAuthorizeVersionMonotonic(t *testing.T) {
	e := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      10000,
			Scale:          2,
			Version:        5,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	for i := 0; i < 10; i++ {
		resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
			TransactionId:     "tx",
			OrganizationId:    "org",
			LedgerId:          "ledger",
			Pending:           false,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			},
		})

		require.NoError(t, err)
		require.True(t, resp.Authorized)
	}

	balance, ok := e.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, uint64(15), balance.Version)
	require.Equal(t, int64(9000), balance.Available)
}

func TestAuthorizeConcurrentSameShard(t *testing.T) {
	e := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      20000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	var wg sync.WaitGroup
	errCh := make(chan error, 1000)

	for i := 0; i < 1000; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
				TransactionId:     "tx",
				OrganizationId:    "org",
				LedgerId:          "ledger",
				Pending:           false,
				TransactionStatus: constant.CREATED,
				Operations: []*authorizerv1.BalanceOperation{
					{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 10, Scale: 2, Operation: constant.DEBIT},
				},
			})
			if err != nil {
				errCh <- err
				return
			}

			if !resp.Authorized {
				errCh <- errors.New("authorization rejected during concurrent test")
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	balance, ok := e.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(10000), balance.Available)
	require.Equal(t, uint64(1001), balance.Version)
}

func TestAuthorizePreSplitExternalAccount(t *testing.T) {
	router := shard.NewRouter(8)
	externalKey := router.ResolveExternalBalanceKey("@alice")

	e := engine.New(router, wal.NewNoopWriter())
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      1000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@external/BRL",
			BalanceKey:     externalKey,
			AssetCode:      "BRL",
			Available:      -500,
			Scale:          2,
			Version:        1,
			AccountType:    constant.ExternalAccountType,
			IsExternal:     true,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx4",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@external/BRL#default", AccountAlias: "@external/BRL", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT, IsExternal: true},
		},
	})

	require.NoError(t, err)
	require.True(t, resp.Authorized)

	external, ok := e.GetBalance("org", "ledger", "@external/BRL", externalKey)
	require.True(t, ok)
	require.Equal(t, int64(-400), external.Available)

	foundCanonicalKey := false
	for _, snapshot := range resp.Balances {
		if snapshot.GetAccountAlias() == "@external/BRL" {
			require.Equal(t, externalKey, snapshot.GetBalanceKey())
			foundCanonicalKey = true
		}
	}
	require.True(t, foundCanonicalKey)
}

func TestAuthorizeExternalCreditCannotBecomePositive(t *testing.T) {
	e := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@external/BRL",
			BalanceKey:     shard.ExternalBalanceKey(0),
			AssetCode:      "BRL",
			Available:      -100,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
			AccountType:    constant.ExternalAccountType,
			IsExternal:     true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      1000,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx3",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@external/BRL#default", AccountAlias: "@external/BRL", BalanceKey: shard.ExternalBalanceKey(0), Amount: 200, Scale: 2, Operation: constant.CREDIT, IsExternal: true},
			{OperationAlias: "1#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 200, Scale: 2, Operation: constant.DEBIT},
		},
	})

	require.NoError(t, err)
	require.False(t, resp.Authorized)
	require.Equal(t, engine.RejectionInsufficientFunds, resp.RejectionCode)

	alice, ok := e.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(1000), alice.Available)

	external, ok := e.GetBalance("org", "ledger", "@external/BRL", shard.ExternalBalanceKey(0))
	require.True(t, ok)
	require.Equal(t, int64(-100), external.Available)
}

func TestAuthorizeFailClosedWhenWALAppendFails(t *testing.T) {
	e := engine.New(shard.NewRouter(8), failingWALWriter{})
	defer e.Close()
	e.UpsertBalances([]*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-fail-closed",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})

	require.Error(t, err)
	require.Nil(t, resp)

	balance, ok := e.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(10000), balance.Available)
	require.Equal(t, uint64(1), balance.Version)
}
