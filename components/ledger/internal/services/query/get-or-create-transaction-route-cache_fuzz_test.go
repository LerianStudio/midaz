// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

// FuzzGetOrCreateTransactionRouteCacheBytes exercises the byte-handling path of
// GetOrCreateTransactionRouteCache by injecting arbitrary bytes as the Redis cached
// value. The function must never panic regardless of what Redis returns.
//
// Three code paths are targeted:
//  1. NOT_FOUND sentinel bytes -> returns ErrDatabaseItemNotFound
//  2. Valid msgpack bytes -> deserializes to TransactionRouteCache
//  3. Any other bytes (corrupted/truncated/garbage) -> falls back to DB
func FuzzGetOrCreateTransactionRouteCacheBytes(f *testing.F) {
	// --- Seed corpus (9 entries, covering all 5 required categories) ---

	// Seed 1: empty byte slice (empty/nil category)
	f.Add([]byte{})

	// Seed 2: NOT_FOUND sentinel (valid sentinel category)
	f.Add([]byte("NOT_FOUND"))

	// Seed 3: valid msgpack from a fully-populated TransactionRouteCache
	validCache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source:        map[string]mmodel.OperationRouteCache{"op-1": {OperationType: "source", Account: &mmodel.AccountCache{RuleType: "alias", ValidIf: "@cash"}}},
				Destination:   map[string]mmodel.OperationRouteCache{"op-2": {OperationType: "destination"}},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	validBytes, err := validCache.ToMsgpack()
	if err != nil {
		f.Fatalf("failed to create valid msgpack seed: %v", err)
	}

	f.Add(validBytes)

	// Seed 4: corrupted bytes (random binary garbage)
	f.Add([]byte{0xFF, 0xFE, 0xAB, 0xCD, 0x00, 0x01, 0x99, 0x88})

	// Seed 5: truncated valid msgpack (first half of valid payload)
	if len(validBytes) > 2 {
		f.Add(validBytes[:len(validBytes)/2])
	}

	// Seed 6: JSON instead of msgpack (wrong format — unicode category)
	f.Add([]byte(`{"actions":{"direct":{"source":{}}}}`))

	// Seed 7: null bytes and control characters (security payload category)
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x80, 0x90, 0xa0, 0xc0, 0xd0})

	// Seed 8: large repeated msgpack-like pattern (boundary category)
	f.Add([]byte(strings.Repeat("\x92\x80\x80", 100)))

	// Seed 9: msgpack fixmap with nil values for all expected fields
	f.Add([]byte{
		0x84, 0xa6, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0xc0,
		0xab, 0x64, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0xc0,
		0xad, 0x62, 0x69, 0x64, 0x69, 0x72, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x61, 0x6c, 0xc0,
		0xa7, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0xc0,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Input bounding: prevent OOM from decompression bombs or oversized payloads
		if len(data) > 4096 {
			data = data[:4096]
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		organizationID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
		transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
			TransactionRouteRepo: mockTransactionRouteRepo,
		}

		expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

		// Redis returns the fuzzed bytes with no error (simulates a cache hit with arbitrary data)
		mockRedisRepo.EXPECT().
			GetBytes(gomock.Any(), expectedKey).
			Return(data, nil).
			Times(1)

		// For the DB fallback path (corrupted bytes or empty bytes trigger this):
		// provide a valid transaction route so the function can complete without error.
		fallbackRoute := &mmodel.TransactionRoute{
			ID:             transactionRouteID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Fuzz Fallback Route",
			OperationRoutes: []mmodel.OperationRoute{
				{
					ID:                uuid.New(),
					OperationType:     "source",
					AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
					Account: &mmodel.AccountRule{
						RuleType: "alias",
						ValidIf:  "@fuzz_account",
					},
				},
			},
		}

		fallbackCacheData := fallbackRoute.ToCache()
		fallbackCacheBytes, marshalErr := fallbackCacheData.ToMsgpack()

		if marshalErr != nil {
			t.Fatalf("failed to marshal fallback cache data: %v", marshalErr)
		}

		// Allow DB and Redis SetBytes calls (they may or may not be called depending on the path)
		mockTransactionRouteRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
			Return(fallbackRoute, nil).
			AnyTimes()

		mockRedisRepo.EXPECT().
			SetBytes(gomock.Any(), expectedKey, fallbackCacheBytes, gomock.Any()).
			Return(nil).
			AnyTimes()

		// Primary property: the function must NEVER panic
		result, fnErr := uc.GetOrCreateTransactionRouteCache(
			context.Background(),
			organizationID,
			ledgerID,
			transactionRouteID,
		)

		// Secondary property: if error is non-nil, result must be zero-value.
		// If error is nil, result was deserialized successfully (Actions may be nil
		// for legitimately empty caches) or came from the DB fallback path.
		if fnErr != nil {
			if result.Actions != nil {
				t.Error("function returned non-nil error with non-nil Actions map — invariant violation")
			}
		}
	})
}
