// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)

func TestProperty_ValidActions_CountIsAlwaysFive(t *testing.T) {
	t.Parallel()

	// Property: ValidActions always has exactly 5 elements
	f := func() bool {
		return len(constant.ValidActions) == 5
	}

	err := quick.Check(f, &quick.Config{MaxCount: 100})
	assert.NoError(t, err, "ValidActions count must always be 5")
}

func TestProperty_ValidActions_AllLowercase(t *testing.T) {
	t.Parallel()

	// Property: every action in ValidActions is lowercase
	f := func() bool {
		for _, action := range constant.ValidActions {
			if action != strings.ToLower(action) {
				return false
			}
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 100})
	assert.NoError(t, err, "all actions must be lowercase")
}

func TestProperty_ValidActions_NonEmpty(t *testing.T) {
	t.Parallel()

	// Property: no action in ValidActions is empty
	f := func() bool {
		for _, action := range constant.ValidActions {
			if action == "" {
				return false
			}
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 100})
	assert.NoError(t, err, "no action should be empty")
}

func TestProperty_ValidActions_NoDuplicates(t *testing.T) {
	t.Parallel()

	// Property: no duplicates in ValidActions
	f := func() bool {
		seen := make(map[string]bool, len(constant.ValidActions))
		for _, action := range constant.ValidActions {
			if seen[action] {
				return false
			}

			seen[action] = true
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 100})
	assert.NoError(t, err, "no duplicates allowed in ValidActions")
}

// randomTransactionRoute generates a TransactionRoute with random operation routes
// using only valid actions and operation types.
type randomTransactionRoute struct {
	Route *TransactionRoute
}

func (randomTransactionRoute) Generate(r *rand.Rand, size int) reflect.Value {
	operationTypes := []string{"source", "destination", "bidirectional"}

	// Maps action name to AccountingEntries with the corresponding field set.
	actionToEntries := map[string]*AccountingEntries{
		"direct": {Direct: &AccountingEntry{}},
		"hold":   {Hold: &AccountingEntry{}},
		"commit": {Commit: &AccountingEntry{}},
		"cancel": {Cancel: &AccountingEntry{}},
		"revert": {Revert: &AccountingEntry{}},
	}

	numRoutes := r.Intn(size + 1)
	routes := make([]OperationRoute, numRoutes)

	for i := range routes {
		actionIdx := r.Intn(len(constant.ValidActions))
		opTypeIdx := r.Intn(len(operationTypes))
		action := constant.ValidActions[actionIdx]

		route := OperationRoute{
			ID:                uuid.New(),
			OperationType:     operationTypes[opTypeIdx],
			AccountingEntries: actionToEntries[action],
		}

		if r.Intn(2) == 0 {
			route.Account = &AccountRule{
				RuleType: "alias",
				ValidIf:  "@prop_test",
			}
		}

		routes[i] = route
	}

	tr := &TransactionRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		LedgerID:        uuid.New(),
		Title:           "Property Test",
		OperationRoutes: routes,
	}

	return reflect.ValueOf(randomTransactionRoute{Route: tr})
}

func TestProperty_ToCache_ActionKeysAreValid(t *testing.T) {
	t.Parallel()

	// Property: all keys in cache.Actions are valid actions (from ValidActions)
	validSet := make(map[string]bool, len(constant.ValidActions))
	for _, a := range constant.ValidActions {
		validSet[a] = true
	}

	f := func(rtr randomTransactionRoute) bool {
		cache := rtr.Route.ToCache()

		for key := range cache.Actions {
			if !validSet[key] {
				return false
			}
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "all action keys in cache must be valid actions")
}

func TestProperty_ToCache_PreservesRouteCount(t *testing.T) {
	t.Parallel()

	// Property: total routes across all actions == input route count
	f := func(rtr randomTransactionRoute) bool {
		cache := rtr.Route.ToCache()
		inputCount := len(rtr.Route.OperationRoutes)

		// Count routes in Actions
		actionsCount := 0
		for _, ac := range cache.Actions {
			actionsCount += len(ac.Source) + len(ac.Destination) + len(ac.Bidirectional)
		}

		return actionsCount == inputCount
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "ToCache must preserve total route count")
}

func TestProperty_ToCache_MapsNeverNil(t *testing.T) {
	t.Parallel()

	// Property: all maps in cache output are never nil
	f := func(rtr randomTransactionRoute) bool {
		cache := rtr.Route.ToCache()

		if cache.Actions == nil {
			return false
		}

		for _, ac := range cache.Actions {
			if ac.Source == nil || ac.Destination == nil || ac.Bidirectional == nil {
				return false
			}
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "all maps in cache must be initialized (never nil)")
}

// TestProperty_ToCache_ResultAlwaysHasActions verifies that ToCache() always produces
// a TransactionRouteCache with Actions != nil.
// This must hold for any TransactionRoute, including those with zero operation routes.
func TestProperty_ToCache_ResultAlwaysHasActions(t *testing.T) {
	t.Parallel()

	f := func(rtr randomTransactionRoute) bool {
		cache := rtr.Route.ToCache()

		return cache.Actions != nil
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "ToCache() result must always have non-nil Actions")
}

// TestProperty_TransactionRouteCache_MsgpackRoundtripPreservesActions verifies
// that msgpack serialization followed by deserialization preserves the Actions
// field: if a cache has non-nil Actions before serialization, it must have
// non-nil Actions after deserialization.
func TestProperty_TransactionRouteCache_MsgpackRoundtripPreservesActions(t *testing.T) {
	t.Parallel()

	f := func(rtr randomTransactionRoute) bool {
		original := rtr.Route.ToCache()

		// Pre-condition: ToCache always produces result with Actions
		if original.Actions == nil {
			return false
		}

		// Serialize to msgpack
		data, err := msgpack.Marshal(original)
		if err != nil {
			return false
		}

		// Deserialize from msgpack
		var restored TransactionRouteCache

		err = msgpack.Unmarshal(data, &restored)
		if err != nil {
			return false
		}

		// Property: Actions is preserved across roundtrip
		return restored.Actions != nil
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "msgpack roundtrip must preserve non-nil Actions")
}

// TestProperty_ToCache_EveryRouteInExactlyOneActionGroup verifies that every
// operation route from the input appears in exactly one action group in the cache.
// No route should be duplicated across action groups, and no route should be missing.
func TestProperty_ToCache_EveryRouteInExactlyOneActionGroup(t *testing.T) {
	t.Parallel()

	f := func(rtr randomTransactionRoute) bool {
		cache := rtr.Route.ToCache()

		// Build a map counting how many action groups each route ID appears in
		routeOccurrences := make(map[string]int)

		for _, ac := range cache.Actions {
			for id := range ac.Source {
				routeOccurrences[id]++
			}

			for id := range ac.Destination {
				routeOccurrences[id]++
			}

			for id := range ac.Bidirectional {
				routeOccurrences[id]++
			}
		}

		// Every input route must appear exactly once in action groups
		for _, route := range rtr.Route.OperationRoutes {
			routeID := route.ID.String()
			count, exists := routeOccurrences[routeID]

			if !exists || count != 1 {
				return false
			}
		}

		// Total routes in action groups must equal input routes
		totalInActions := 0
		for _, count := range routeOccurrences {
			totalInActions += count
		}

		return totalInActions == len(rtr.Route.OperationRoutes)
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "every operation route must appear in exactly one action group")
}
