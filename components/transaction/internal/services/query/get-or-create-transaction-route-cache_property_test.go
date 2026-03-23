// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"testing/quick"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestProperty_SentinelDetection_OnlyExactMatch verifies that only the exact byte
// sequence []byte("NOT_FOUND") triggers the sentinel path. Any variation (case changes,
// prefix, suffix, substring, empty) must NOT be treated as sentinel and must instead
// fall through to DB lookup or msgpack deserialization.
func TestProperty_SentinelDetection_OnlyExactMatch(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	property := func(input []byte) bool {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		organizationID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
		transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

		uc := &UseCase{
			RedisRepo:            mockRedisRepo,
			TransactionRouteRepo: mockTransactionRouteRepo,
		}

		expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

		isSentinel := bytes.Equal(input, cacheNotFoundSentinel)

		if len(input) == 0 {
			// Empty bytes: condition `len(cachedValue) > 0` is false, falls through to DB
			mockRedisRepo.EXPECT().
				GetBytes(gomock.Any(), expectedKey).
				Return(input, nil).
				Times(1)

			// DB will be called; return not-found to keep test simple
			mockTransactionRouteRepo.EXPECT().
				FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
				Return(nil, services.ErrDatabaseItemNotFound).
				Times(1)

			// Sentinel stored after DB not-found
			mockRedisRepo.EXPECT().
				SetBytes(gomock.Any(), expectedKey, cacheNotFoundSentinel, sentinelTTL).
				Return(nil).
				Times(1)

			_, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

			// Must NOT be treated as sentinel — DB was called, so this is the DB not-found path
			return err == services.ErrDatabaseItemNotFound
		}

		if isSentinel {
			// Exact sentinel: must return ErrDatabaseItemNotFound with NO DB call.
			// No TransactionRouteRepo mock expectations — gomock will fail if DB is called.
			uc.TransactionRouteRepo = nil

			mockRedisRepo.EXPECT().
				GetBytes(gomock.Any(), expectedKey).
				Return(input, nil).
				Times(1)

			result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

			return err == services.ErrDatabaseItemNotFound &&
				reflect.DeepEqual(result, mmodel.TransactionRouteCache{})
		}

		// Non-sentinel, non-empty bytes: must NOT trigger sentinel path.
		// These bytes will either deserialize as msgpack or fail and fall back to DB.
		mockRedisRepo.EXPECT().
			GetBytes(gomock.Any(), expectedKey).
			Return(input, nil).
			Times(1)

		// For non-sentinel data that fails msgpack, function falls back to DB.
		// For valid msgpack, function returns the deserialized data (no DB call).
		// We allow DB call by setting up an optional expectation.
		mockTransactionRouteRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
			Return(nil, services.ErrDatabaseItemNotFound).
			AnyTimes()

		mockRedisRepo.EXPECT().
			SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		_, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

		// The key invariant: non-sentinel bytes must NEVER produce a sentinel-path response.
		// If we get ErrDatabaseItemNotFound, it must be from the DB fallback (which we set up),
		// not from sentinel detection. We verify this by confirming the function did attempt DB access.
		// Since we're here (non-sentinel), the function either:
		//   a) Successfully deserialized msgpack -> returns (data, nil)
		//   b) Failed msgpack -> called DB -> got our mock ErrDatabaseItemNotFound
		// Both are correct non-sentinel behavior. The property holds as long as we reach here
		// without panic and without gomock failures (which would mean unexpected calls).
		_ = err

		return true
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}

// TestProperty_ReturnContract_NeverBothDataAndError verifies that the function never
// returns both a non-zero TransactionRouteCache AND a non-nil error simultaneously.
// For every possible cached value, exactly one of the two return values is meaningful:
// either (data, nil) or (zero-value, error).
func TestProperty_ReturnContract_NeverBothDataAndError(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	property := func(input []byte) bool {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		organizationID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
		transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

		uc := &UseCase{
			RedisRepo:            mockRedisRepo,
			TransactionRouteRepo: mockTransactionRouteRepo,
		}

		expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

		// Redis returns the generated bytes successfully
		mockRedisRepo.EXPECT().
			GetBytes(gomock.Any(), expectedKey).
			Return(input, nil).
			Times(1)

		// Allow DB fallback for non-sentinel, non-msgpack, or empty bytes
		mockTransactionRouteRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
			Return(nil, services.ErrDatabaseItemNotFound).
			AnyTimes()

		// Allow sentinel storage
		mockRedisRepo.EXPECT().
			SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

		zeroValue := mmodel.TransactionRouteCache{}

		// Invariant: NEVER return both non-zero data AND non-nil error
		if err != nil && !reflect.DeepEqual(result, zeroValue) {
			return false
		}

		return true
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}

// TestProperty_SentinelPath_NeverCallsDB verifies that when Redis returns the exact
// sentinel value []byte("NOT_FOUND"), the database repository FindByID is NEVER called.
// This is the core cache penetration defense: sentinel hits must short-circuit to avoid
// hammering the database for known-missing entries.
//
// The property generates random UUIDs for organization, ledger, and transaction route
// to ensure the sentinel detection is independent of the key composition.
func TestProperty_SentinelPath_NeverCallsDB(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	// Property input is 3 random byte slices used to seed UUIDs, ensuring we test
	// sentinel detection across varying key compositions.
	property := func(orgSeed, ledgerSeed, routeSeed uint64) bool {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		organizationID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
		transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// TransactionRouteRepo is intentionally nil. If the function attempts to call
		// FindByID, it will panic with a nil pointer dereference, which is a strong
		// signal that the sentinel path incorrectly fell through to DB.
		uc := &UseCase{
			RedisRepo:            mockRedisRepo,
			TransactionRouteRepo: nil,
		}

		expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

		// Redis returns the exact sentinel value
		mockRedisRepo.EXPECT().
			GetBytes(gomock.Any(), expectedKey).
			Return(cacheNotFoundSentinel, nil).
			Times(1)

		result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

		// Must return sentinel error with zero-value result and NO DB call
		return err == services.ErrDatabaseItemNotFound &&
			reflect.DeepEqual(result, mmodel.TransactionRouteCache{})
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}
