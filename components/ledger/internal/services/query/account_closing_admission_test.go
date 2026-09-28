// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// expectAccountClosingSeedReread programs the second read of the seed, the one
// that runs under the ownership and whose rows are the ones actually used.
func expectAccountClosingSeedReread(m *admissionMocks) {
	m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return([]*mmodel.Balance{{
			ID:        uuid.NewString(),
			AccountID: admissionAccountID.String(),
			Alias:     "@closing_account",
			Key:       constant.DefaultBalanceKey,
		}}, nil)
}

// expectAccountClosingOpenAdmission programs the protection of an open account for
// one cache-miss load, without any release: the caller under test is expected to
// keep the ownership.
func expectAccountClosingOpenAdmission(m *admissionMocks) {
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountSeedAdmission(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
		Return(true, redis.AccountAdminHolderNone, nil)
	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return(time.Time{}, false, nil)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{admissionAccountID}).
		Return(map[uuid.UUID]*time.Time{}, nil)
	m.account.EXPECT().ListAccountsByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{admissionAccountID}).
		Return([]*mmodel.Account{}, nil)
	m.uc.OperationRepo.(*operation.MockRepository).EXPECT().
		ListLatestByBalances(gomock.Any(), admissionOrgID, admissionLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{}, nil)
}

// TestAccountClosingCacheMissHandsTheOwnershipToTheExecution covers the seam this
// epic moves: the seed is admitted inside the engine, so a caller that executes
// accounting keeps the ownership alive past the load instead of releasing it.
func TestAccountClosingCacheMissHandsTheOwnershipToTheExecution(t *testing.T) {
	m := newAdmissionMocks(t)
	m.expectCacheMiss()
	expectAccountClosingSeedReread(m)
	expectAccountClosingOpenAdmission(m)

	ctx, sink := accountprotection.ContextWithSink(context.Background())

	balances, err := m.uc.GetBalances(ctx, admissionOrgID, admissionLedgerID, []string{admissionAlias})
	require.NoError(t, err)
	require.Len(t, balances, 1)

	assert.NotEmpty(t, sink.TokenFor(admissionOrgID, admissionLedgerID, admissionAccountID),
		"the load leaves the ownership to the execution that admits its seed")
	assert.Empty(t, sink.TokenFor(admissionOrgID, admissionLedgerID, uuid.New()),
		"only the accounts of the load are owned")
}

// TestAccountClosingCacheMissReleasesWithoutAnExecution proves the ownership still
// ends with the load on every path that does not go on to execute accounting.
func TestAccountClosingCacheMissReleasesWithoutAnExecution(t *testing.T) {
	m := newAdmissionMocks(t)
	m.expectCacheMiss()
	expectAccountClosingSeedReread(m)
	expectAccountClosingOpenAdmission(m)

	released := make(chan struct{}, 1)
	m.redis.EXPECT().ReleaseAccountSeedAdmission(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (bool, error) {
			released <- struct{}{}

			return true, nil
		})

	balances, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})
	require.NoError(t, err)
	require.Len(t, balances, 1)

	assert.Len(t, released, 1)
}
