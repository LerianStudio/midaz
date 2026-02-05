// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

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

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
