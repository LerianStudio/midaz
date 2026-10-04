//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"sync"
	"testing"
	"time"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	http "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"

	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// interleavingRepo runs between once, after the overlap guard's first read and
// before the write it guards: the window a concurrent request lands in.
type interleavingRepo struct {
	pack.Repository
	once    sync.Once
	between func()
}

func (r *interleavingRepo) FindList(ctx context.Context, filter http.QueryHeader) ([]*pack.Package, error) {
	found, err := r.Repository.FindList(ctx, filter)
	r.once.Do(r.between)

	return found, err
}

// staleAmountsRepo runs between once, after an update's first read of the package
// it edits: the read a concurrent mutation can leave stale before the lock is held.
type staleAmountsRepo struct {
	pack.Repository
	once    sync.Once
	between func()
}

func (r *staleAmountsRepo) FindFeesAndAmountDataByPackageID(ctx context.Context, organizationID, packageID uuid.UUID) (*model.AmountData, error) {
	found, err := r.Repository.FindFeesAndAmountDataByPackageID(ctx, organizationID, packageID)
	r.once.Do(r.between)

	return found, err
}

func packageInput(minAmount, maxAmount string) *model.CreatePackageInput {
	return &model.CreatePackageInput{
		FeeGroupLabel: "Pacote " + minAmount,
		MinAmount:     minAmount,
		MaxAmount:     maxAmount,
		Enable:        boolPtr(true),
	}
}

// Two replicas mutate the packages of one ledger at once. The second creates
// [500, 1500] while the first sits between its overlap read and its write, so an
// unguarded pair both persist; guarded, the first lands and the second is refused
// with the range-overlap error.
func TestIntegration_PackageOverlapGuard_ConcurrentMutationsKeepOnePackage(t *testing.T) {
	container := mongotestutil.SetupContainer(t)

	repo, err := pack.NewPackageMongoDBRepository(&feesmongo.MongoConnection{
		ConnectionStringSource: container.URI,
		Database:               container.DBName,
		Logger:                 &libLog.NopLogger{},
		MaxPoolSize:            8,
		DB:                     container.Client,
	}, &libLog.NopLogger{})
	require.NoError(t, err)

	lock, err := libRedis.NewRedisLockManager(redistestutil.CreateConnection(t, redistestutil.SetupContainer(t).Addr))
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name  string
		first func(t *testing.T, replicaA, replicaB *UseCase, organizationID, ledgerID uuid.UUID) error
	}{
		{
			name: "create racing create",
			first: func(_ *testing.T, replicaA, _ *UseCase, organizationID, ledgerID uuid.UUID) error {
				_, errCreate := replicaA.CreatePackage(ctx, packageInput("100", "1000"), organizationID, ledgerID, uuid.Nil)

				return errCreate
			},
		},
		{
			name: "range update racing create",
			first: func(t *testing.T, replicaA, replicaB *UseCase, organizationID, ledgerID uuid.UUID) error {
				stored, errSeed := replicaB.CreatePackage(ctx, packageInput("0", "100"), organizationID, ledgerID, uuid.Nil)
				require.NoError(t, errSeed)

				newMaximum := "10000"

				return replicaA.UpdatePackageByID(ctx, stored.ID, organizationID, ledgerID, &model.UpdatePackageInput{MaxAmount: &newMaximum})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			organizationID, ledgerID := uuid.New(), uuid.New()
			replicaB := &UseCase{packageRepo: repo, PackageLock: lock}

			finished := make(chan struct{})

			var secondErr error

			replicaA := &UseCase{packageRepo: &interleavingRepo{Repository: repo, between: func() {
				go func() {
					defer close(finished)

					_, secondErr = replicaB.CreatePackage(ctx, packageInput("500", "1500"), organizationID, ledgerID, uuid.Nil)
				}()

				select {
				case <-finished:
				case <-time.After(500 * time.Millisecond):
				}
			}}, PackageLock: lock}

			firstErr := tt.first(t, replicaA, replicaB, organizationID, ledgerID)
			<-finished

			require.NoError(t, firstErr)

			var conflict pkg.EntityConflictError

			require.ErrorAs(t, secondErr, &conflict, "the second mutation must be refused")
			require.Equal(t, constant.ErrPackageRange.Error(), conflict.Code)

			stored, errFind := repo.FindList(ctx, http.QueryHeader{OrganizationID: organizationID, LedgerID: ledgerID, Limit: 10, Page: 1})
			require.NoError(t, errFind)
			require.Len(t, stored, 1, "overlapping packages must never both be stored")
		})
	}

	// Replica A raises only the maximum of [1000, 2000] to 5000. Between its first
	// read and its lock, replica B moves the package to [0, 500] and creates
	// [600, 900]. Judged against the minimum A first read, [1000, 5000] clears
	// [600, 900] and the write would store [0, 5000] over it; judged against the
	// package under the lock, A is refused.
	t.Run("single-bound update racing a move and a create", func(t *testing.T) {
		organizationID, ledgerID := uuid.New(), uuid.New()
		replicaB := &UseCase{packageRepo: repo, PackageLock: lock}

		moved, errSeed := replicaB.CreatePackage(ctx, packageInput("1000", "2000"), organizationID, ledgerID, uuid.Nil)
		require.NoError(t, errSeed)

		var moveErr, createErr error

		replicaA := &UseCase{packageRepo: &staleAmountsRepo{Repository: repo, between: func() {
			newMinimum, newMaximum := "0", "500"
			moveErr = replicaB.UpdatePackageByID(ctx, moved.ID, organizationID, ledgerID, &model.UpdatePackageInput{MinAmount: &newMinimum, MaxAmount: &newMaximum})
			_, createErr = replicaB.CreatePackage(ctx, packageInput("600", "900"), organizationID, ledgerID, uuid.Nil)
		}}, PackageLock: lock}

		raisedMaximum := "5000"
		raiseErr := replicaA.UpdatePackageByID(ctx, moved.ID, organizationID, ledgerID, &model.UpdatePackageInput{MaxAmount: &raisedMaximum})

		require.NoError(t, moveErr)
		require.NoError(t, createErr)

		var conflict pkg.EntityConflictError

		require.ErrorAs(t, raiseErr, &conflict, "the update must be judged against the package under the lock")
		require.Equal(t, constant.ErrPackageRange.Error(), conflict.Code)

		stored, errFind := repo.FindFeesAndAmountDataByPackageID(ctx, organizationID, moved.ID)
		require.NoError(t, errFind)
		require.Equal(t, "500", stored.MaxAmount.String(), "the moved package must keep its maximum")
	})
}
