// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// FuzzValidateOperationRouteTypes_OperationTypes fuzzes the operation type validation
// in validateOperationRouteTypes with random operation type values. This ensures that arbitrary
// strings never cause panics and always return either nil or a known error.
func FuzzValidateOperationRouteTypes_OperationTypes(f *testing.F) {
	// Seed corpus: valid operation types
	f.Add("source", "destination")
	// Seed corpus: bidirectional only
	f.Add("bidirectional", "bidirectional")
	// Seed corpus: unknown types
	f.Add("unknown", "invalid")
	// Seed corpus: empty strings
	f.Add("", "")
	// Seed corpus: unicode characters
	f.Add("\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e\xf0\x9f\x92\xb0", "source")
	// Seed corpus: SQL injection payload
	f.Add("'; DROP TABLE routes;--", "destination")

	f.Fuzz(func(t *testing.T, opType1, opType2 string) {
		// Bound input to prevent excessive memory usage
		if len(opType1) > 1024 {
			opType1 = opType1[:1024]
		}

		if len(opType2) > 1024 {
			opType2 = opType2[:1024]
		}

		opRoutes := []*mmodel.OperationRoute{
			{ID: uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a"), OperationType: opType1},
			{ID: uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b"), OperationType: opType2},
		}

		// Must not panic; must return nil or error
		err := validateOperationRouteTypes(opRoutes)

		hasSource := opType1 == "source" || opType1 == "bidirectional" || opType2 == "source" || opType2 == "bidirectional"
		hasDest := opType1 == "destination" || opType1 == "bidirectional" || opType2 == "destination" || opType2 == "bidirectional"

		if hasSource && hasDest {
			assert.NoError(t, err, "should succeed with source and destination")
		}
	})
}

// FuzzValidateOperationRouteTypes_EmptyInputs fuzzes validateOperationRouteTypes with
// varying combinations of empty and populated slices. This verifies the function handles
// nil slices, empty slices without panics.
func FuzzValidateOperationRouteTypes_EmptyInputs(f *testing.F) {
	// Seed corpus
	f.Add(false, "source")
	f.Add(true, "destination")
	f.Add(true, "bidirectional")
	f.Add(false, "unknown_type")

	routeID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

	f.Fuzz(func(t *testing.T, hasRoutes bool, opType string) {
		if len(opType) > 64 {
			opType = opType[:64]
		}

		var opRoutes []*mmodel.OperationRoute

		if hasRoutes {
			opRoutes = []*mmodel.OperationRoute{
				{ID: routeID, OperationType: opType},
			}
		}

		// Must not panic
		result := validateOperationRouteTypes(opRoutes)

		// Empty inputs should always succeed
		if !hasRoutes {
			assert.NoError(t, result, "empty operation routes should return nil")
		}
	})
}
