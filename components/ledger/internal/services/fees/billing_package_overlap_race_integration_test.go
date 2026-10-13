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
	billing_package "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"

	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// interleavingBillingRepo runs between once, as the guarded write starts: the
// window a concurrent request lands in.
type interleavingBillingRepo struct {
	billing_package.Repository
	once    sync.Once
	between func()
}

func (r *interleavingBillingRepo) Create(ctx context.Context, bp *model.BillingPackage) (*model.BillingPackage, error) {
	r.once.Do(r.between)

	return r.Repository.Create(ctx, bp)
}

func (r *interleavingBillingRepo) Update(ctx context.Context, id, organizationID, ledgerID string, updateFields *bson.M) (*model.BillingPackage, error) {
	r.once.Do(r.between)

	return r.Repository.Update(ctx, id, organizationID, ledgerID, updateFields)
}

// anyAccount answers every alias as an existing account.
type anyAccount struct{ feeshared.MidazResolver }

func (anyAccount) AccountExistsByAlias(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

func volumePackage(organizationID uuid.UUID, route string, enable bool) *model.BillingPackage {
	bp := validVolumeBillingPackage()
	bp.OrganizationID = organizationID.String()
	bp.EventFilter.TransactionRoute = route
	bp.Enable = &enable

	return bp
}

// billingWrite is one replica's write on a route.
type billingWrite func(*BillingPackageService) error

// Replica A writes a volume package on one billing route; inside A's write,
// replica B races it with its own. Unguarded, both store an enabled package on
// the route; guarded, A lands and B is judged against it and refused.
func TestIntegration_BillingRouteOverlapGuard_ConcurrentWritesKeepOnePackage(t *testing.T) {
	container := mongotestutil.SetupContainer(t)

	repo, err := billing_package.NewBillingPackageMongoDBRepository(&feesmongo.MongoConnection{
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
	seeder := &BillingPackageService{billingPackageRepo: repo, resolver: anyAccount{}}

	createEnabled := func(organizationID, ledgerID uuid.UUID, route string) billingWrite {
		return func(s *BillingPackageService) error {
			_, errCreate := s.CreateBillingPackage(ctx, ledgerID, volumePackage(organizationID, route, true))

			return errCreate
		}
	}

	enableDisabled := func(t *testing.T, organizationID, ledgerID uuid.UUID, route string) billingWrite {
		disabled, errSeed := seeder.CreateBillingPackage(ctx, ledgerID, volumePackage(organizationID, route, false))
		require.NoError(t, errSeed)

		return func(s *BillingPackageService) error {
			_, errUpdate := s.UpdateBillingPackage(ctx, uuid.MustParse(disabled.ID), organizationID, ledgerID, map[string]any{"enable": true})

			return errUpdate
		}
	}

	tests := []struct {
		name   string
		writes func(t *testing.T, organizationID, ledgerID uuid.UUID, route string) (first, second billingWrite)
	}{
		{
			name: "create racing create",
			writes: func(_ *testing.T, organizationID, ledgerID uuid.UUID, route string) (billingWrite, billingWrite) {
				return createEnabled(organizationID, ledgerID, route), createEnabled(organizationID, ledgerID, route)
			},
		},
		{
			name: "enable racing create",
			writes: func(t *testing.T, organizationID, ledgerID uuid.UUID, route string) (billingWrite, billingWrite) {
				return createEnabled(organizationID, ledgerID, route), enableDisabled(t, organizationID, ledgerID, route)
			},
		},
		{
			name: "create racing enable",
			writes: func(t *testing.T, organizationID, ledgerID uuid.UUID, route string) (billingWrite, billingWrite) {
				return enableDisabled(t, organizationID, ledgerID, route), createEnabled(organizationID, ledgerID, route)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			organizationID, ledgerID, route := uuid.New(), uuid.New(), uuid.NewString()
			first, second := tt.writes(t, organizationID, ledgerID, route)
			replicaB := &BillingPackageService{billingPackageRepo: repo, resolver: anyAccount{}, RouteLock: lock}

			finished := make(chan struct{})

			var secondErr error

			replicaA := &BillingPackageService{billingPackageRepo: &interleavingBillingRepo{Repository: repo, between: func() {
				go func() {
					defer close(finished)

					secondErr = second(replicaB)
				}()

				select {
				case <-finished:
				case <-time.After(500 * time.Millisecond):
				}
			}}, resolver: anyAccount{}, RouteLock: lock}

			firstErr := first(replicaA)
			<-finished

			require.NoError(t, firstErr)

			var conflict pkg.EntityConflictError

			require.ErrorAs(t, secondErr, &conflict, "the second write must be refused")
			require.Equal(t, constant.ErrBillingRouteOverlap.Error(), conflict.Code)

			enabled, errFind := repo.FindMatchingPackages(ctx, organizationID.String(), ledgerID.String(), route)
			require.NoError(t, errFind)
			require.Len(t, enabled, 1, "a route must never be billed by two enabled packages")
		})
	}

	t.Run("held lock answers 0086", func(t *testing.T) {
		organizationID, ledgerID, route := uuid.New(), uuid.New(), uuid.NewString()
		writer := &BillingPackageService{billingPackageRepo: repo, resolver: anyAccount{}, RouteLock: lock}

		unlock, errLock := writer.lockBillingRoute(ctx, organizationID.String(), ledgerID.String(), route)
		require.NoError(t, errLock)

		defer unlock()

		_, errCreate := writer.CreateBillingPackage(ctx, ledgerID, volumePackage(organizationID, route, true))

		var contended pkg.UnprocessableOperationError

		require.ErrorAs(t, errCreate, &contended, "a writer behind a held lock must give up")
		require.Equal(t, constant.ErrLockVersionAccountBalance.Error(), contended.Code)
		require.Equal(t, constant.EntityBillingPackage, contended.EntityType)

		enabled, errFind := repo.FindMatchingPackages(ctx, organizationID.String(), ledgerID.String(), route)
		require.NoError(t, errFind)
		require.Empty(t, enabled, "a refused writer must store nothing")
	})
}
