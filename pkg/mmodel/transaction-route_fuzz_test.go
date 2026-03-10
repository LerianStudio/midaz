// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
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
				ID:            uuid.New(),
				OperationType: opType,
				Action:        action,
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
		if cache.Source == nil {
			t.Error("Source map should never be nil")
		}

		if cache.Destination == nil {
			t.Error("Destination map should never be nil")
		}

		if cache.Bidirectional == nil {
			t.Error("Bidirectional map should never be nil")
		}

		if cache.Actions == nil {
			t.Error("Actions map should never be nil")
		}

		// Verify route count preservation: sum of all routes across all actions == input count
		// (only for known operation types)
		if opType == "source" || opType == "destination" || opType == "bidirectional" {
			totalInActions := 0
			for _, ac := range cache.Actions {
				totalInActions += len(ac.Source) + len(ac.Destination) + len(ac.Bidirectional)
			}
			// Note: routes with the same ID would collapse in the map, but since we use uuid.New()
			// they should all be unique
			totalLegacy := len(cache.Source) + len(cache.Destination) + len(cache.Bidirectional)

			if totalLegacy != numRoutes {
				t.Errorf("legacy fields route count %d != input count %d", totalLegacy, numRoutes)
			}

			if totalInActions != numRoutes {
				t.Errorf("actions route count %d != input count %d", totalInActions, numRoutes)
			}
		}
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
				ID:            uuid.New(),
				OperationType: "source",
				Action:        action,
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

		if len(restored.Source) != len(cache.Source) {
			t.Errorf("Source count mismatch: got %d, want %d", len(restored.Source), len(cache.Source))
		}
	})
}
