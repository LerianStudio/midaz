// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// --- Generator types for property-based tests ---

// bidirectionalInput generates N bidirectional routes.
type bidirectionalInput struct {
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
		RouteIDs: ids,
	})
}

// validCompleteInput generates a set of operation routes with at least one source
// and one destination, guaranteeing a valid configuration.
type validCompleteInput struct {
	SourceIDs       []uuid.UUID
	DestinationIDs  []uuid.UUID
	BidirectionalID []uuid.UUID
}

func (validCompleteInput) Generate(rng *rand.Rand, size int) reflect.Value {
	srcCount := rng.Intn(3) + 1
	dstCount := rng.Intn(3) + 1
	bidiCount := rng.Intn(2) // 0 or 1

	return reflect.ValueOf(validCompleteInput{
		SourceIDs:       randomUUIDs(rng, srcCount),
		DestinationIDs:  randomUUIDs(rng, dstCount),
		BidirectionalID: randomUUIDs(rng, bidiCount),
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

// --- Property Tests ---

// TestProperty_ValidateOperationRouteTypes_BidirectionalAlwaysValid verifies that
// if all routes are bidirectional, validateOperationRouteTypes returns nil
// (bidirectional counts as both source and destination).
func TestProperty_ValidateOperationRouteTypes_BidirectionalAlwaysValid(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	property := func(input bidirectionalInput) bool {
		opRoutes := make([]*mmodel.OperationRoute, len(input.RouteIDs))

		for i, id := range input.RouteIDs {
			opRoutes[i] = &mmodel.OperationRoute{
				ID:            id,
				OperationType: "bidirectional",
			}
		}

		err := validateOperationRouteTypes(opRoutes)

		return err == nil
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_ValidateOperationRouteTypes_ValidRoutesAlwaysAccept verifies that
// if operation routes have at least one source and one destination,
// the result is always nil regardless of how many routes.
func TestProperty_ValidateOperationRouteTypes_ValidRoutesAlwaysAccept(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	property := func(input validCompleteInput) bool {
		var opRoutes []*mmodel.OperationRoute

		for _, id := range input.SourceIDs {
			opRoutes = append(opRoutes, &mmodel.OperationRoute{
				ID:            id,
				OperationType: "source",
			})
		}

		for _, id := range input.DestinationIDs {
			opRoutes = append(opRoutes, &mmodel.OperationRoute{
				ID:            id,
				OperationType: "destination",
			})
		}

		for _, id := range input.BidirectionalID {
			opRoutes = append(opRoutes, &mmodel.OperationRoute{
				ID:            id,
				OperationType: "bidirectional",
			})
		}

		err := validateOperationRouteTypes(opRoutes)

		return err == nil
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_ValidateOperationRouteTypes_MissingSourceAlwaysRejects verifies that
// if operation routes have only destination routes (no source, no bidirectional),
// it always returns an error.
func TestProperty_ValidateOperationRouteTypes_MissingSourceAlwaysRejects(t *testing.T) {
	config := &quick.Config{MaxCount: 100}

	property := func(input bidirectionalInput) bool {
		// Use route IDs as destination-only routes
		opRoutes := make([]*mmodel.OperationRoute, len(input.RouteIDs))
		for i, id := range input.RouteIDs {
			opRoutes[i] = &mmodel.OperationRoute{
				ID:            id,
				OperationType: "destination",
			}
		}

		err := validateOperationRouteTypes(opRoutes)

		return err != nil
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}
