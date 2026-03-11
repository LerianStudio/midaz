// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// validOperationTypes lists all valid operation type values for generated routes.
var validOperationTypes = []string{"source", "destination", "bidirectional"}

// --- Generator types for property-based tests ---

// bidirectionalInput generates a random valid action with N bidirectional routes.
type bidirectionalInput struct {
	Action   string
	RouteIDs []uuid.UUID
}

func (bidirectionalInput) Generate(rng *rand.Rand, size int) reflect.Value {
	n := rng.Intn(5) + 1 // 1..5 bidirectional routes
	ids := make([]uuid.UUID, n)

	for i := range ids {
		for j := range ids[i] {
			ids[i][j] = byte(rng.Intn(256))
		}
	}

	return reflect.ValueOf(bidirectionalInput{
		Action:   validActions[rng.Intn(len(validActions))],
		RouteIDs: ids,
	})
}

// duplicateInput generates an action input list that always contains at least one
// duplicate (action, routeID) pair, plus a random shuffle seed.
type duplicateInput struct {
	Action      string
	RouteID     uuid.UUID
	ExtraInputs []mmodel.OperationRouteActionInput
	ShuffleSeed int64
}

func (duplicateInput) Generate(rng *rand.Rand, size int) reflect.Value {
	var id uuid.UUID
	for j := range id {
		id[j] = byte(rng.Intn(256))
	}

	action := validActions[rng.Intn(len(validActions))]

	// Generate 0..5 additional unique entries (no duplicates among extras)
	extraCount := rng.Intn(6)
	extras := make([]mmodel.OperationRouteActionInput, 0, extraCount)

	seen := map[routeActionKeyFuzz]bool{{RouteID: id, Action: action}: true}

	for i := 0; i < extraCount; i++ {
		var eid uuid.UUID
		for j := range eid {
			eid[j] = byte(rng.Intn(256))
		}

		eaction := validActions[rng.Intn(len(validActions))]
		key := routeActionKeyFuzz{RouteID: eid, Action: eaction}

		if seen[key] {
			continue
		}

		seen[key] = true

		extras = append(extras, mmodel.OperationRouteActionInput{
			Action:           eaction,
			OperationRouteID: eid,
		})
	}

	return reflect.ValueOf(duplicateInput{
		Action:      action,
		RouteID:     id,
		ExtraInputs: extras,
		ShuffleSeed: rng.Int63(),
	})
}

// validCompleteInput generates a set of actions where each action has at least one
// source and one destination route, guaranteeing a valid configuration.
type validCompleteInput struct {
	// For each action: source IDs, destination IDs, and optional bidirectional IDs
	Actions []validActionGroup
}

type validActionGroup struct {
	Action          string
	SourceIDs       []uuid.UUID
	DestinationIDs  []uuid.UUID
	BidirectionalID []uuid.UUID
}

func (validCompleteInput) Generate(rng *rand.Rand, size int) reflect.Value {
	actionCount := rng.Intn(len(validActions)) + 1 // 1..5 actions

	// Pick actionCount distinct actions
	perm := rng.Perm(len(validActions))
	groups := make([]validActionGroup, actionCount)

	for i := 0; i < actionCount; i++ {
		action := validActions[perm[i]]

		// At least 1 source and 1 destination
		srcCount := rng.Intn(3) + 1
		dstCount := rng.Intn(3) + 1
		bidiCount := rng.Intn(2) // 0 or 1

		groups[i] = validActionGroup{
			Action:          action,
			SourceIDs:       randomUUIDs(rng, srcCount),
			DestinationIDs:  randomUUIDs(rng, dstCount),
			BidirectionalID: randomUUIDs(rng, bidiCount),
		}
	}

	return reflect.ValueOf(validCompleteInput{Actions: groups})
}

// invalidActionInput generates an action string not in ValidActions.
type invalidActionInput struct {
	InvalidAction string
	RouteID       uuid.UUID
}

func (invalidActionInput) Generate(rng *rand.Rand, size int) reflect.Value {
	// Generate random strings that are NOT valid actions
	invalidActions := []string{
		"", "DIRECT", "Hold", "COMMIT", "unknown", "transfer",
		"debit", "credit", "reverse", "rollback", "settle",
	}

	var id uuid.UUID
	for j := range id {
		id[j] = byte(rng.Intn(256))
	}

	return reflect.ValueOf(invalidActionInput{
		InvalidAction: invalidActions[rng.Intn(len(invalidActions))],
		RouteID:       id,
	})
}

// missingSourceInput generates an action group with only destination routes.
type missingSourceInput struct {
	Action         string
	DestinationIDs []uuid.UUID
}

func (missingSourceInput) Generate(rng *rand.Rand, size int) reflect.Value {
	n := rng.Intn(3) + 1 // 1..3 destination routes

	return reflect.ValueOf(missingSourceInput{
		Action:         validActions[rng.Intn(len(validActions))],
		DestinationIDs: randomUUIDs(rng, n),
	})
}

// --- Helper functions ---

func randomUUIDs(rng *rand.Rand, count int) []uuid.UUID {
	ids := make([]uuid.UUID, count)
	for i := range ids {
		for j := range ids[i] {
			ids[i][j] = byte(rng.Intn(256))
		}
	}

	return ids
}

// shuffleInputs shuffles a slice of OperationRouteActionInput in-place using a seed.
func shuffleInputs(inputs []mmodel.OperationRouteActionInput, seed int64) {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic shuffle for tests
	r.Shuffle(len(inputs), func(i, j int) {
		inputs[i], inputs[j] = inputs[j], inputs[i]
	})
}

// --- Property Tests ---

// TestProperty_ValidateOperationRouteTypes_BidirectionalAlwaysValid verifies that
// for any valid action, if all routes for that action are bidirectional,
// validateOperationRouteTypes returns nil (bidirectional counts as both source and destination).
func TestProperty_ValidateOperationRouteTypes_BidirectionalAlwaysValid(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	property := func(input bidirectionalInput) bool {
		actionInputs := make([]mmodel.OperationRouteActionInput, len(input.RouteIDs))
		opRoutes := make([]*mmodel.OperationRoute, len(input.RouteIDs))

		for i, id := range input.RouteIDs {
			actionInputs[i] = mmodel.OperationRouteActionInput{
				Action:           input.Action,
				OperationRouteID: id,
			}
			opRoutes[i] = &mmodel.OperationRoute{
				ID:            id,
				OperationType: "bidirectional",
			}
		}

		err := validateOperationRouteTypes(actionInputs, opRoutes)

		return err == nil
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_ValidateOperationRouteTypes_DuplicateDetectionOrderIndependent verifies that
// for any list of (action, routeID) pairs containing a duplicate, shuffling the input order
// always produces an error (duplicate detection does not depend on position).
func TestProperty_ValidateOperationRouteTypes_DuplicateDetectionOrderIndependent(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	property := func(input duplicateInput) bool {
		// Build base list: duplicate entry + extras
		base := []mmodel.OperationRouteActionInput{
			{Action: input.Action, OperationRouteID: input.RouteID},
			{Action: input.Action, OperationRouteID: input.RouteID}, // duplicate
		}
		base = append(base, input.ExtraInputs...)

		// Collect all unique route IDs for opRoutes
		idSet := make(map[uuid.UUID]bool)
		for _, ai := range base {
			idSet[ai.OperationRouteID] = true
		}

		opRoutes := make([]*mmodel.OperationRoute, 0, len(idSet))
		for id := range idSet {
			opRoutes = append(opRoutes, &mmodel.OperationRoute{
				ID:            id,
				OperationType: "bidirectional",
			})
		}

		// Test with original order
		inputs1 := make([]mmodel.OperationRouteActionInput, len(base))
		copy(inputs1, base)

		err1 := validateOperationRouteTypes(inputs1, opRoutes)
		if err1 == nil {
			return false // must detect duplicate
		}

		// Test with shuffled order
		inputs2 := make([]mmodel.OperationRouteActionInput, len(base))
		copy(inputs2, base)
		shuffleInputs(inputs2, input.ShuffleSeed)

		err2 := validateOperationRouteTypes(inputs2, opRoutes)

		return err2 != nil // shuffled order must also detect duplicate
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_ValidateOperationRouteTypes_ValidActionsAlwaysAccept verifies that
// if all actions are from ValidActions and each action group has at least one source
// and one destination, the result is always nil regardless of how many actions/routes.
func TestProperty_ValidateOperationRouteTypes_ValidActionsAlwaysAccept(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	property := func(input validCompleteInput) bool {
		var actionInputs []mmodel.OperationRouteActionInput

		var opRoutes []*mmodel.OperationRoute

		for _, group := range input.Actions {
			for _, id := range group.SourceIDs {
				actionInputs = append(actionInputs, mmodel.OperationRouteActionInput{
					Action:           group.Action,
					OperationRouteID: id,
				})
				opRoutes = append(opRoutes, &mmodel.OperationRoute{
					ID:            id,
					OperationType: "source",
				})
			}

			for _, id := range group.DestinationIDs {
				actionInputs = append(actionInputs, mmodel.OperationRouteActionInput{
					Action:           group.Action,
					OperationRouteID: id,
				})
				opRoutes = append(opRoutes, &mmodel.OperationRoute{
					ID:            id,
					OperationType: "destination",
				})
			}

			for _, id := range group.BidirectionalID {
				actionInputs = append(actionInputs, mmodel.OperationRouteActionInput{
					Action:           group.Action,
					OperationRouteID: id,
				})
				opRoutes = append(opRoutes, &mmodel.OperationRoute{
					ID:            id,
					OperationType: "bidirectional",
				})
			}
		}

		err := validateOperationRouteTypes(actionInputs, opRoutes)

		return err == nil
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_ValidateOperationRouteTypes_InvalidActionAlwaysRejects verifies that
// any action string not in ValidActions always produces an error,
// regardless of other input.
func TestProperty_ValidateOperationRouteTypes_InvalidActionAlwaysRejects(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	entityType := reflect.TypeOf(mmodel.TransactionRoute{}).Name()

	property := func(input invalidActionInput) bool {
		actionInputs := []mmodel.OperationRouteActionInput{
			{Action: input.InvalidAction, OperationRouteID: input.RouteID},
		}

		opRoutes := []*mmodel.OperationRoute{
			{ID: input.RouteID, OperationType: "bidirectional"},
		}

		err := validateOperationRouteTypes(actionInputs, opRoutes)
		if err == nil {
			return false
		}

		expectedErr := pkg.ValidateBusinessError(constant.ErrInvalidRouteAction, entityType, input.InvalidAction)

		return err.Error() == expectedErr.Error()
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_ValidateOperationRouteTypes_MissingSourceAlwaysRejects verifies that
// if an action group has only destination routes (no source, no bidirectional),
// it always returns ErrNoSourceForAction.
func TestProperty_ValidateOperationRouteTypes_MissingSourceAlwaysRejects(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	entityType := reflect.TypeOf(mmodel.TransactionRoute{}).Name()

	property := func(input missingSourceInput) bool {
		actionInputs := make([]mmodel.OperationRouteActionInput, len(input.DestinationIDs))
		opRoutes := make([]*mmodel.OperationRoute, len(input.DestinationIDs))

		for i, id := range input.DestinationIDs {
			actionInputs[i] = mmodel.OperationRouteActionInput{
				Action:           input.Action,
				OperationRouteID: id,
			}
			opRoutes[i] = &mmodel.OperationRoute{
				ID:            id,
				OperationType: "destination",
			}
		}

		err := validateOperationRouteTypes(actionInputs, opRoutes)
		if err == nil {
			return false
		}

		expectedErr := pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, input.Action)

		return err.Error() == expectedErr.Error()
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}
