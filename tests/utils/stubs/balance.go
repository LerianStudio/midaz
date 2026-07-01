//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package stubs

import (
	"context"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
)

// BalanceRepoStub is a no-op implementation of the balance repository
// interface. It exists so handler integration tests that do NOT exercise
// balance behavior (e.g. organization/ledger/asset CRUD suites) can satisfy the
// command.UseCase BalanceRepo field without standing up balance persistence.
//
// Every method returns the zero value with no error. A test that depends on
// real balance behavior must use the concrete PostgreSQL repository, not this
// stub.
type BalanceRepoStub struct{}

// NewBalanceRepoStub returns a no-op balance repository stub.
func NewBalanceRepoStub() *BalanceRepoStub {
	return &BalanceRepoStub{}
}

func (s *BalanceRepoStub) Create(_ context.Context, balance *mmodel.Balance) (*mmodel.Balance, error) {
	return balance, nil
}

func (s *BalanceRepoStub) Find(_ context.Context, _, _, _ uuid.UUID) (*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) FindByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) ExistsByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}

func (s *BalanceRepoStub) ListAll(_ context.Context, _, _ uuid.UUID, _ http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *BalanceRepoStub) ListAllByAccountID(_ context.Context, _, _, _ uuid.UUID, _ http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *BalanceRepoStub) ListByAccountIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) ListByIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) ListByAliases(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) ListByAliasesWithKeys(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) BalancesUpdate(_ context.Context, _, _ uuid.UUID, _ []*mmodel.Balance) error {
	return nil
}

func (s *BalanceRepoStub) Update(_ context.Context, _, _, _ uuid.UUID, _ mmodel.UpdateBalance) (*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) Delete(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
}

func (s *BalanceRepoStub) DeleteAllByIDs(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}

func (s *BalanceRepoStub) UpdateMany(_ context.Context, _, _ uuid.UUID, _ []mmodel.BalanceRedis) (int64, error) {
	return 0, nil
}

func (s *BalanceRepoStub) UpdateAllByAccountID(_ context.Context, _, _, _ uuid.UUID, _ mmodel.UpdateBalance) error {
	return nil
}

func (s *BalanceRepoStub) ListByAccountID(_ context.Context, _, _, _ uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *BalanceRepoStub) ListByAccountIDAtTimestamp(_ context.Context, _, _, _ uuid.UUID, _ time.Time) ([]*mmodel.Balance, error) {
	return nil, nil
}
