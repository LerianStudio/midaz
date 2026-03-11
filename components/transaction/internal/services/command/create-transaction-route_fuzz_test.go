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

// FuzzValidateOperationRouteTypes_ActionStrings fuzzes the action string validation
// in validateOperationRouteTypes with random action values. This ensures that arbitrary
// strings (empty, long, unicode, special characters, SQL injection payloads) never cause
// panics and always return either nil or a known error.
func FuzzValidateOperationRouteTypes_ActionStrings(f *testing.F) {
	// Seed corpus: valid action
	f.Add("direct")
	// Seed corpus: empty string
	f.Add("")
	// Seed corpus: boundary - long string
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	// Seed corpus: unicode characters
	f.Add("\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e\xf0\x9f\x92\xb0")
	// Seed corpus: SQL injection payload
	f.Add("'; DROP TABLE routes;--")
	// Seed corpus: XSS payload
	f.Add("<script>alert('xss')</script>")
	// Seed corpus: null bytes and control characters
	f.Add("action\x00with\x00nulls\t\n\r")

	sourceRouteID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	destRouteID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

	opRoutes := []*mmodel.OperationRoute{
		{ID: sourceRouteID, OperationType: "source"},
		{ID: destRouteID, OperationType: "destination"},
	}

	f.Fuzz(func(t *testing.T, action string) {
		// Bound input to prevent excessive memory usage
		if len(action) > 1024 {
			action = action[:1024]
		}

		actionInputs := []mmodel.OperationRouteActionInput{
			{Action: action, OperationRouteID: sourceRouteID},
			{Action: action, OperationRouteID: destRouteID},
		}

		// Must not panic; must return nil or error
		err := validateOperationRouteTypes(actionInputs, opRoutes)

		// Valid actions should succeed (source + destination present)
		validActions := map[string]bool{
			"direct": true, "hold": true, "commit": true, "cancel": true, "revert": true,
		}

		if validActions[action] {
			assert.NoError(t, err, "valid action %q with source+destination should succeed", action)
		} else {
			assert.Error(t, err, "invalid action %q should return error", action)
		}
	})
}

// FuzzValidateOperationRouteTypes_Combinations fuzzes validateOperationRouteTypes with
// random combinations of route IDs (via UUID bytes), action strings, and operation types.
// This tests duplicate detection, source/destination coverage validation, and ensures
// no panics on any combination of inputs.
func FuzzValidateOperationRouteTypes_Combinations(f *testing.F) {
	id1Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	id2Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")
	zeroBytes := make([]byte, 16)

	// Seed corpus: valid - source + destination with "direct"
	f.Add(id1Bytes[:], "direct", "source", id2Bytes[:], "direct", "destination")
	// Seed corpus: duplicate route+action pair
	f.Add(id1Bytes[:], "hold", "source", id1Bytes[:], "hold", "destination")
	// Seed corpus: bidirectional covers both source and destination
	f.Add(id1Bytes[:], "commit", "bidirectional", id2Bytes[:], "commit", "source")
	// Seed corpus: zero UUID with empty action and unknown op type
	f.Add(zeroBytes, "", "unknown", zeroBytes, "", "unknown")
	// Seed corpus: unicode action with mixed op types
	f.Add(id1Bytes[:], "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", "source", id2Bytes[:], "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", "destination")
	// Seed corpus: security payload actions
	f.Add(id1Bytes[:], "'; DROP TABLE--", "source", id2Bytes[:], "<script>", "destination")
	// Seed corpus: only sources, no destination
	f.Add(id1Bytes[:], "direct", "source", id2Bytes[:], "direct", "source")

	f.Fuzz(func(t *testing.T, id1Bytes []byte, action1, opType1 string, id2Bytes []byte, action2, opType2 string) {
		// Bound inputs to prevent excessive memory usage
		if len(action1) > 512 {
			action1 = action1[:512]
		}

		if len(action2) > 512 {
			action2 = action2[:512]
		}

		if len(opType1) > 64 {
			opType1 = opType1[:64]
		}

		if len(opType2) > 64 {
			opType2 = opType2[:64]
		}

		// Pad or truncate UUID bytes to exactly 16 bytes
		uuid1Bytes := padToUUID(id1Bytes)
		uuid2Bytes := padToUUID(id2Bytes)

		routeID1, err := uuid.FromBytes(uuid1Bytes)
		if err != nil {
			t.Skip("invalid UUID bytes for route 1")
		}

		routeID2, err := uuid.FromBytes(uuid2Bytes)
		if err != nil {
			t.Skip("invalid UUID bytes for route 2")
		}

		actionInputs := []mmodel.OperationRouteActionInput{
			{Action: action1, OperationRouteID: routeID1},
			{Action: action2, OperationRouteID: routeID2},
		}

		opRoutes := []*mmodel.OperationRoute{
			{ID: routeID1, OperationType: opType1},
			{ID: routeID2, OperationType: opType2},
		}

		// Must not panic; must return nil or error
		result := validateOperationRouteTypes(actionInputs, opRoutes)
		_ = result
	})
}

// FuzzValidateOperationRouteTypes_EmptyInputs fuzzes validateOperationRouteTypes with
// varying combinations of empty and populated slices. This verifies the function handles
// nil slices, empty slices, and mismatched lengths without panics.
func FuzzValidateOperationRouteTypes_EmptyInputs(f *testing.F) {
	// Seed corpus: both empty
	f.Add(false, false, "direct", "source")
	// Seed corpus: inputs only, no routes
	f.Add(true, false, "hold", "destination")
	// Seed corpus: routes only, no inputs
	f.Add(false, true, "commit", "bidirectional")
	// Seed corpus: both populated
	f.Add(true, true, "cancel", "source")
	// Seed corpus: empty action string
	f.Add(true, true, "", "source")
	// Seed corpus: unicode action
	f.Add(true, true, "\xf0\x9f\x92\xb0", "destination")
	// Seed corpus: unknown op type
	f.Add(true, true, "direct", "unknown_type")

	routeID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

	f.Fuzz(func(t *testing.T, hasInputs, hasRoutes bool, action, opType string) {
		if len(action) > 512 {
			action = action[:512]
		}

		if len(opType) > 64 {
			opType = opType[:64]
		}

		var actionInputs []mmodel.OperationRouteActionInput

		var opRoutes []*mmodel.OperationRoute

		if hasInputs {
			actionInputs = []mmodel.OperationRouteActionInput{
				{Action: action, OperationRouteID: routeID},
			}
		}

		if hasRoutes {
			opRoutes = []*mmodel.OperationRoute{
				{ID: routeID, OperationType: opType},
			}
		}

		// Must not panic
		result := validateOperationRouteTypes(actionInputs, opRoutes)

		// Empty inputs should always succeed
		if !hasInputs {
			assert.NoError(t, result, "empty action inputs should return nil")
		}
	})
}
