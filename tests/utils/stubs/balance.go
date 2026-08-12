// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package stubs provides test doubles for tests.
//
// This file contains BalanceRepoStub, a no-op implementation of the ledger
// balance repository contract. Like logger.go it carries no build tag so it is
// available in both unit and integration test contexts.
package stubs

import (
	"context"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	netHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// BalanceRepoStub is a no-op test double for the ledger balance repository.
//
// The concrete balance repository interface lives under the ledger internal
// tree, which the shared test-utils module may not import; conformance is
// verified structurally at the assignment site inside the ledger package.
//
// Use it when a UseCase under integration test needs a BalanceRepo dependency
// but the scenario does not exercise real balance persistence (e.g. asset and
// account handler tests that only assert on the onboarding side effects).
//
// Reads return empty results and writes succeed silently, so account creation
// paths that auto-create a default balance complete without a live balance
// store. Create echoes the supplied balance back so callers that use the
// returned value keep working.
type BalanceRepoStub struct{}

// NewBalanceRepoStub returns a BalanceRepoStub ready for use as a
// balance.Repository test double.
func NewBalanceRepoStub() *BalanceRepoStub {
	return &BalanceRepoStub{}
}

// Create echoes the supplied balance back with no error.
func (s *BalanceRepoStub) Create(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
	return b, nil
}

// Find returns no balance and no error.
func (s *BalanceRepoStub) Find(_ context.Context, _, _, _ uuid.UUID) (*mmodel.Balance, error) {
	return nil, nil
}

// FindByAccountIDAndKey returns no balance and no error.
func (s *BalanceRepoStub) FindByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (*mmodel.Balance, error) {
	return nil, nil
}

// ExistsByAccountIDAndKey reports that no balance exists, so default-balance
// creation paths proceed to Create.
func (s *BalanceRepoStub) ExistsByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}

// ListAll returns an empty page.
func (s *BalanceRepoStub) ListAll(_ context.Context, _, _ uuid.UUID, _ netHTTP.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

// ListAllByAccountID returns an empty page.
func (s *BalanceRepoStub) ListAllByAccountID(_ context.Context, _, _, _ uuid.UUID, _ netHTTP.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

// ListByAccountIDs returns no balances and no error.
func (s *BalanceRepoStub) ListByAccountIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

// ListByIDs returns no balances and no error.
func (s *BalanceRepoStub) ListByIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

// ListByAliases returns no balances and no error.
func (s *BalanceRepoStub) ListByAliases(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

// ListByAliasesWithKeys returns no balances and no error.
func (s *BalanceRepoStub) ListByAliasesWithKeys(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

// BalancesUpdate succeeds silently.
func (s *BalanceRepoStub) BalancesUpdate(_ context.Context, _, _ uuid.UUID, _ []*mmodel.Balance) error {
	return nil
}

// Update returns no balance and no error.
func (s *BalanceRepoStub) Update(_ context.Context, _, _, _ uuid.UUID, _ mmodel.UpdateBalance) (*mmodel.Balance, error) {
	return nil, nil
}

// Delete succeeds silently.
func (s *BalanceRepoStub) Delete(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
}

// DeleteAllByIDs succeeds silently.
func (s *BalanceRepoStub) DeleteAllByIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}

// UpdateMany reports zero rows updated and no error.
func (s *BalanceRepoStub) UpdateMany(_ context.Context, _, _ uuid.UUID, _ []mmodel.BalanceRedis) (int64, error) {
	return 0, nil
}

// UpdateAllByAccountID succeeds silently.
func (s *BalanceRepoStub) UpdateAllByAccountID(_ context.Context, _, _, _ uuid.UUID, _ mmodel.UpdateBalance) error {
	return nil
}

// ListByAccountID returns no balances and no error.
func (s *BalanceRepoStub) ListByAccountID(_ context.Context, _, _, _ uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

// ListByAccountIDAtTimestamp returns no balances and no error.
func (s *BalanceRepoStub) ListByAccountIDAtTimestamp(_ context.Context, _, _, _ uuid.UUID, _ time.Time) ([]*mmodel.Balance, error) {
	return nil, nil
}
