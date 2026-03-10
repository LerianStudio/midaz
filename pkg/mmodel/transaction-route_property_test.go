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

	numRoutes := r.Intn(size + 1)
	routes := make([]OperationRoute, numRoutes)

	for i := range routes {
		actionIdx := r.Intn(len(constant.ValidActions))
		opTypeIdx := r.Intn(len(operationTypes))

		route := OperationRoute{
			ID:            uuid.New(),
			OperationType: operationTypes[opTypeIdx],
			Action:        constant.ValidActions[actionIdx],
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

		// Count routes in legacy fields
		legacyCount := len(cache.Source) + len(cache.Destination) + len(cache.Bidirectional)
		if legacyCount != inputCount {
			return false
		}

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

		if cache.Source == nil || cache.Destination == nil || cache.Bidirectional == nil || cache.Actions == nil {
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

func TestProperty_ToCache_LegacyAndActionsConsistent(t *testing.T) {
	t.Parallel()

	// Property: every route in legacy Source/Destination/Bidirectional also appears in exactly one action group
	f := func(rtr randomTransactionRoute) bool {
		cache := rtr.Route.ToCache()

		// Collect all route IDs from action groups
		actionRouteIDs := make(map[string]bool)
		for _, ac := range cache.Actions {
			for id := range ac.Source {
				actionRouteIDs[id] = true
			}

			for id := range ac.Destination {
				actionRouteIDs[id] = true
			}

			for id := range ac.Bidirectional {
				actionRouteIDs[id] = true
			}
		}

		// Every route in legacy fields must be in action groups
		for id := range cache.Source {
			if !actionRouteIDs[id] {
				return false
			}
		}

		for id := range cache.Destination {
			if !actionRouteIDs[id] {
				return false
			}
		}

		for id := range cache.Bidirectional {
			if !actionRouteIDs[id] {
				return false
			}
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 200})
	assert.NoError(t, err, "legacy and action fields must be consistent")
}
