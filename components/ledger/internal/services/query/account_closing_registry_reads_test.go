// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	txMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// A closed account stays fully consultable, and consulting it must not warm the
// transactional balance cache: the registry reads and the operation history answer
// from PostgreSQL and MongoDB alone.
//
// That separation is what keeps a read from undoing a closing. The closing evicts
// every synchronized balance blob under its marker, so any read that admitted a
// balance on the way past would put back exactly what the eviction removed — and,
// after the negative marker expires, with no protection to refuse it.
//
// The barrier is the Redis repository wired into the use case with NO
// expectations: any call reaching it fails the test, whatever the call is.
func TestAccountClosingRegistryReadsTouchNoTransactionalCache(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("cccccccc-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("cccccccc-0000-0000-0000-000000000002")
	accountID := uuid.MustParse("cccccccc-0000-0000-0000-000000000003")
	alias := "@closed-account"

	closedAt := admissionClosedAt

	closed := &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Closed Account",
		Alias:          &alias,
		ClosedAt:       &closedAt,
	}

	reads := []struct {
		name  string
		setup func(*account.MockRepository, *operation.MockRepository)
		read  func(*testing.T, *UseCase)
	}{
		{
			name: "by id",
			setup: func(accountRepo *account.MockRepository, _ *operation.MockRepository) {
				accountRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, nil, accountID, gomock.Any()).
					Return(closed, nil)
			},
			read: func(t *testing.T, uc *UseCase) {
				got, err := uc.GetAccountByID(context.Background(), organizationID, ledgerID, nil, accountID, mmodel.HolderOnV2)
				require.NoError(t, err)
				assert.Equal(t, &closedAt, got.ClosedAt)
			},
		},
		{
			name: "by alias",
			setup: func(accountRepo *account.MockRepository, _ *operation.MockRepository) {
				accountRepo.EXPECT().FindAlias(gomock.Any(), organizationID, ledgerID, nil, alias, gomock.Any()).
					Return(closed, nil)
			},
			read: func(t *testing.T, uc *UseCase) {
				got, err := uc.GetAccountByAlias(context.Background(), organizationID, ledgerID, nil, alias, mmodel.HolderOnV2)
				require.NoError(t, err)
				assert.Equal(t, &closedAt, got.ClosedAt)
			},
		},
		{
			name: "in a listing",
			setup: func(accountRepo *account.MockRepository, _ *operation.MockRepository) {
				accountRepo.EXPECT().FindAll(gomock.Any(), organizationID, ledgerID, nil, nil, gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{closed}, nil)
			},
			read: func(t *testing.T, uc *UseCase) {
				got, err := uc.GetAllAccount(context.Background(), organizationID, ledgerID, nil, nil, pkgHTTP.QueryHeader{Limit: 10, Page: 1}, mmodel.HolderOnV2)
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, &closedAt, got[0].ClosedAt)
			},
		},
		{
			name: "operation history",
			setup: func(_ *account.MockRepository, operationRepo *operation.MockRepository) {
				operationRepo.EXPECT().FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any(), gomock.Any()).
					Return([]*operation.Operation{{ID: uuid.NewString()}}, libHTTP.CursorPagination{}, nil)
			},
			read: func(t *testing.T, uc *UseCase) {
				got, _, err := uc.GetAllOperationsByAccount(context.Background(), organizationID, ledgerID, accountID, pkgHTTP.QueryHeader{Limit: 10, Page: 1})
				require.NoError(t, err)
				assert.Len(t, got, 1, "the history recorded before the closing is still readable")
			},
		},
	}

	for _, read := range reads {
		read := read

		t.Run(read.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			accountRepo := account.NewMockRepository(ctrl)
			operationRepo := operation.NewMockRepository(ctrl)
			onboardingMetadata := mongodb.NewMockRepository(ctrl)
			transactionMetadata := txMongo.NewMockRepository(ctrl)

			onboardingMetadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			onboardingMetadata.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			transactionMetadata.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			read.setup(accountRepo, operationRepo)

			uc := &UseCase{
				AccountRepo:             accountRepo,
				OperationRepo:           operationRepo,
				OnboardingMetadataRepo:  onboardingMetadata,
				TransactionMetadataRepo: transactionMetadata,
				// No expectations: the registry reads must never reach it.
				TransactionRedisRepo: txRedis.NewMockRedisRepository(ctrl),
			}

			read.read(t, uc)
		})
	}
}
