// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// FuzzToCache verifies that ToCache never panics regardless of input combinations.
func FuzzToCache(f *testing.F) {
	// Seed corpus with representative combinations:
	// (numRoutes, operationType index 0-3, action index 0-5, hasAccount bool)

	// Empty routes
	f.Add(0, 0, 0, false)

	// Single source route with direct action and account
	f.Add(1, 0, 0, true)

	// Single destination route with hold action, no account
	f.Add(1, 1, 1, false)

	// Multiple routes, bidirectional with commit action
	f.Add(3, 2, 2, true)

	// Many routes, unknown operation type
	f.Add(5, 3, 3, false)

	// Edge: large number of routes
	f.Add(50, 0, 4, true)

	operationTypes := []string{"source", "destination", "bidirectional", "unknown"}
	actions := []string{"direct", "hold", "commit", "cancel", "revert", ""}

	// Maps action name to the AccountingEntries that causes that action to appear in ToCache.
	actionToEntries := map[string]*AccountingEntries{
		"direct": {Direct: &AccountingEntry{}},
		"hold":   {Hold: &AccountingEntry{}},
		"commit": {Commit: &AccountingEntry{}},
		"cancel": {Cancel: &AccountingEntry{}},
		"revert": {Revert: &AccountingEntry{}},
		"":       nil, // empty action => no AccountingEntries => excluded from all buckets
	}

	f.Fuzz(func(t *testing.T, numRoutes int, opTypeIdx int, actionIdx int, hasAccount bool) {
		// Clamp inputs
		if numRoutes < 0 {
			numRoutes = 0
		}

		if numRoutes > 100 {
			numRoutes = 100
		}

		if opTypeIdx < 0 {
			opTypeIdx = 0
		}

		if actionIdx < 0 {
			actionIdx = 0
		}

		opType := operationTypes[opTypeIdx%len(operationTypes)]
		action := actions[actionIdx%len(actions)]

		routes := make([]OperationRoute, numRoutes)
		for i := range routes {
			route := OperationRoute{
				ID:                uuid.New(),
				OperationType:     opType,
				AccountingEntries: actionToEntries[action],
			}

			if hasAccount {
				route.Account = &AccountRule{
					RuleType: "alias",
					ValidIf:  "@fuzz_account",
				}
			}

			routes[i] = route
		}

		tr := &TransactionRoute{
			ID:              uuid.New(),
			OrganizationID:  uuid.New(),
			LedgerID:        uuid.New(),
			Title:           "Fuzz Test Route",
			OperationRoutes: routes,
		}

		// Must not panic
		cache := tr.ToCache()

		// Basic structural invariants
		if cache.Actions == nil {
			t.Error("Actions map should never be nil")
		}

		// Verify route count preservation: sum of all routes across all actions == input count
		// (only for known operation types with non-empty action)
		if (opType == "source" || opType == "destination" || opType == "bidirectional") && action != "" {
			totalInActions := 0
			for _, ac := range cache.Actions {
				totalInActions += len(ac.Source) + len(ac.Destination) + len(ac.Bidirectional)
			}
			// Note: routes with the same ID would collapse in the map, but since we use uuid.New()
			// they should all be unique
			if totalInActions != numRoutes {
				t.Errorf("actions route count %d != input count %d", totalInActions, numRoutes)
			}
		}
	})
}

// FuzzFromMsgpack_ArbitraryBytes feeds random bytes into FromMsgpack to verify it never
// panics on any input. This is the highest-priority fuzz target for cache deserialization
// because FromMsgpack receives data from Redis, which may contain corrupted, truncated,
// pre-migration (old schema), or completely arbitrary bytes.
func FuzzFromMsgpack_ArbitraryBytes(f *testing.F) {
	// Seed 1: valid msgpack from a fully-populated TransactionRouteCache
	validCache := TransactionRouteCache{
		Actions: map[string]ActionRouteCache{
			"direct": {
				Source:        map[string]OperationRouteCache{"route-1": {OperationType: "source", Account: &AccountCache{RuleType: "alias", ValidIf: "@cash"}}},
				Destination:   map[string]OperationRouteCache{"route-2": {OperationType: "destination"}},
				Bidirectional: map[string]OperationRouteCache{},
			},
		},
	}

	validBytes, err := validCache.ToMsgpack()
	if err != nil {
		f.Fatalf("failed to create valid msgpack seed: %v", err)
	}

	f.Add(validBytes)

	// Seed 2: empty byte slice
	f.Add([]byte{})

	// Seed 3: single byte (boundary - minimal input)
	f.Add([]byte{0x00})

	// Seed 4: truncated valid msgpack (first half of a valid payload)
	if len(validBytes) > 2 {
		f.Add(validBytes[:len(validBytes)/2])
	}

	// Seed 5: cache format with nil Actions to simulate pre-migration cache
	legacyCache := TransactionRouteCache{}

	legacyBytes, err := legacyCache.ToMsgpack()
	if err != nil {
		f.Fatalf("failed to create legacy msgpack seed: %v", err)
	}

	f.Add(legacyBytes)

	// Seed 6: JSON instead of msgpack (wrong format entirely)
	f.Add([]byte(`{"source":{"r1":{"operationType":"source"}},"destination":{}}`))

	// Seed 7: null bytes and control characters (binary garbage)
	f.Add([]byte{0xff, 0xfe, 0xfd, 0x00, 0x01, 0x80, 0x90, 0xa0, 0xc0, 0xd0})

	// Seed 8: large repeated byte pattern (boundary - memory pressure)
	f.Add([]byte(strings.Repeat("\x92\x80\x80", 100)))

	// Seed 9: msgpack fixmap with unexpected nested types
	f.Add([]byte{
		0x84, 0xa6, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0xc0,
		0xab, 0x64, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0xc0,
		0xad, 0x62, 0x69, 0x64, 0x69, 0x72, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x61, 0x6c, 0xc0,
		0xa7, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0xc0,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Bound input to prevent excessive memory usage from decompression bombs
		if len(data) > 4096 {
			data = data[:4096]
		}

		var cache TransactionRouteCache

		// Primary property: FromMsgpack must never panic on any input
		err := cache.FromMsgpack(data)
		if err != nil {
			// Error is acceptable -- corrupted data should return an error, not panic
			return
		}

		// Secondary property: if deserialization succeeds, re-serialization must not panic
		roundtripped, err := cache.ToMsgpack()
		if err != nil {
			// Serialization failure after successful deserialization is noteworthy but not a crash
			return
		}

		// Tertiary property: roundtrip deserialization must not panic
		var cache2 TransactionRouteCache

		_ = cache2.FromMsgpack(roundtripped)
	})
}

// FuzzToCacheMsgpackRoundTrip verifies that ToCache output survives msgpack serialization.
func FuzzToCacheMsgpackRoundTrip(f *testing.F) {
	f.Add(0, 0, false)
	f.Add(1, 0, true)
	f.Add(3, 1, true)
	f.Add(5, 2, false)
	f.Add(2, 4, true)

	actions := []string{"direct", "hold", "commit", "cancel", "revert"}
	roundtripActionToEntries := map[string]*AccountingEntries{
		"direct": {Direct: &AccountingEntry{}},
		"hold":   {Hold: &AccountingEntry{}},
		"commit": {Commit: &AccountingEntry{}},
		"cancel": {Cancel: &AccountingEntry{}},
		"revert": {Revert: &AccountingEntry{}},
	}

	f.Fuzz(func(t *testing.T, numRoutes int, actionIdx int, hasAccount bool) {
		if numRoutes < 0 {
			numRoutes = 0
		}

		if numRoutes > 20 {
			numRoutes = 20
		}

		if actionIdx < 0 {
			actionIdx = 0
		}

		action := actions[actionIdx%len(actions)]

		routes := make([]OperationRoute, numRoutes)
		for i := range routes {
			route := OperationRoute{
				ID:                uuid.New(),
				OperationType:     "source",
				AccountingEntries: roundtripActionToEntries[action],
			}

			if hasAccount {
				route.Account = &AccountRule{
					RuleType: "alias",
					ValidIf:  "@roundtrip",
				}
			}

			routes[i] = route
		}

		tr := &TransactionRoute{
			ID:              uuid.New(),
			OrganizationID:  uuid.New(),
			LedgerID:        uuid.New(),
			Title:           "Fuzz Roundtrip",
			OperationRoutes: routes,
		}

		cache := tr.ToCache()

		data, err := cache.ToMsgpack()
		if err != nil {
			t.Fatalf("ToMsgpack failed: %v", err)
		}

		var restored TransactionRouteCache

		err = restored.FromMsgpack(data)
		if err != nil {
			t.Fatalf("FromMsgpack failed: %v", err)
		}

		if len(restored.Actions) != len(cache.Actions) {
			t.Errorf("Actions count mismatch: got %d, want %d", len(restored.Actions), len(cache.Actions))
		}
	})
}
