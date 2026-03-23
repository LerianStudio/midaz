//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package stubs provides test doubles for integration tests.
//
// Stubs vs Mocks:
//   - Stubs: Simple implementations with fixed behavior (this package)
//   - Mocks: Configurable test doubles with expectation verification (use gomock)
//
// Use stubs when you need a dependency that "just works" without verifying interactions.
// Use mocks (gomock) when you need to verify specific method calls or vary behavior per test.
package stubs

import (
	"context"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	http "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// BalancePortStub is a stub implementation of mbootstrap.BalancePort for integration tests.
// It always succeeds and returns minimal valid data.
//
// Use this when:
//   - Testing flows that require a BalancePort but don't need to verify its behavior
//   - The external balance service is not the focus of the test
//
// Use gomock (pkg/mbootstrap/balance_mock.go) when:
//   - You need to verify specific calls were made
//   - You need different responses per test case
//   - You need to simulate errors
type BalancePortStub struct{}

// CreateBalanceSync returns a minimal valid balance for integration tests.
func (s *BalancePortStub) CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: input.OrganizationID.String(),
		LedgerID:       input.LedgerID.String(),
		AccountID:      input.AccountID.String(),
		AssetCode:      input.AssetCode,
	}, nil
}

// DeleteAllBalancesByAccountID is a no-op stub that always succeeds.
func (s *BalancePortStub) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, requestID string) error {
	return nil
}

// CheckHealth is a no-op stub that always succeeds.
func (s *BalancePortStub) CheckHealth(ctx context.Context) error {
	return nil
}

// balanceRepoStub is a stub implementation of balance.Repository for integration tests.
// It provides minimal behavior for flows that need a BalanceRepo without a real database.
type balanceRepoStub struct{}

// Compile-time interface verification
var _ balance.Repository = (*balanceRepoStub)(nil)

// NewBalanceRepoStub returns a new stub implementation of balance.Repository.
func NewBalanceRepoStub() balance.Repository {
	return &balanceRepoStub{}
}

func (s *balanceRepoStub) Create(_ context.Context, _ *mmodel.Balance) error {
	return nil
}

func (s *balanceRepoStub) Find(_ context.Context, _, _, _ uuid.UUID) (*mmodel.Balance, error) {
	return &mmodel.Balance{}, nil
}

func (s *balanceRepoStub) FindByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (*mmodel.Balance, error) {
	return &mmodel.Balance{}, nil
}

func (s *balanceRepoStub) ExistsByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}

func (s *balanceRepoStub) ListAll(_ context.Context, _, _ uuid.UUID, _ http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *balanceRepoStub) ListAllByAccountID(_ context.Context, _, _, _ uuid.UUID, _ http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *balanceRepoStub) ListByAccountIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ListByIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ListByAliases(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ListByAliasesWithKeys(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) BalancesUpdate(_ context.Context, _, _ uuid.UUID, _ []*mmodel.Balance) error {
	return nil
}

func (s *balanceRepoStub) Update(_ context.Context, _, _, _ uuid.UUID, _ mmodel.UpdateBalance) (*mmodel.Balance, error) {
	return &mmodel.Balance{}, nil
}

func (s *balanceRepoStub) Delete(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
}

func (s *balanceRepoStub) DeleteAllByIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}

func (s *balanceRepoStub) Sync(_ context.Context, _, _ uuid.UUID, _ mmodel.BalanceRedis) (bool, error) {
	return false, nil
}

func (s *balanceRepoStub) SyncBatch(_ context.Context, _, _ uuid.UUID, _ []mmodel.BalanceRedis) (int64, error) {
	return 0, nil
}

func (s *balanceRepoStub) UpdateAllByAccountID(_ context.Context, _, _, _ uuid.UUID, _ mmodel.UpdateBalance) error {
	return nil
}

func (s *balanceRepoStub) ListByAccountID(_ context.Context, _, _, _ uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ListByAccountIDAtTimestamp(_ context.Context, _, _, _ uuid.UUID, _ time.Time) ([]*mmodel.Balance, error) {
	return nil, nil
}
