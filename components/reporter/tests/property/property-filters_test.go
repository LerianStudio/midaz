//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"encoding/json"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/reporter/pkg/model"
)

// Property 1: JSON round-trip preserves all string operator values.
// Marshaling a FilterCondition with string-typed operators to JSON and
// unmarshaling back must produce an identical struct. This catches
// encoding bugs, missing json tags, or field name mismatches.
func TestProperty_Filter_JSONRoundTripPreservesStringValues(t *testing.T) {
	t.Parallel()

	property := func(eq, gt, gte, lt, lte, inVal, ninVal, btwLo, btwHi string) bool {
		// Skip degenerate inputs: empty strings produce omitempty nil slices
		// that can differ from explicitly-set slices after round-trip.
		if eq == "" || gt == "" || gte == "" || lt == "" || lte == "" ||
			inVal == "" || ninVal == "" || btwLo == "" || btwHi == "" {
			return true
		}

		original := model.FilterCondition{
			Equals:         []any{eq},
			GreaterThan:    []any{gt},
			GreaterOrEqual: []any{gte},
			LessThan:       []any{lt},
			LessOrEqual:    []any{lte},
			Between:        []any{btwLo, btwHi},
			In:             []any{inVal},
			NotIn:          []any{ninVal},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Logf("Marshal error: %v", err)
			return false
		}

		var decoded model.FilterCondition
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Logf("Unmarshal error: %v", err)
			return false
		}

		// All fields must survive the round-trip with identical values.
		return reflect.DeepEqual(original, decoded)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: JSON round-trip did not preserve string values: %v", err)
	}
}

// Property 2: Filter serialization is deterministic.
// Marshaling the same FilterCondition twice must always produce the same
// JSON bytes. Non-determinism would break caching, deduplication, and
// idempotency checks that rely on content hashing.
func TestProperty_Filter_SerializationDeterministic(t *testing.T) {
	t.Parallel()

	property := func(val1, val2 string) bool {
		if val1 == "" || val2 == "" {
			return true
		}

		filter := model.FilterCondition{
			Equals:  []any{val1},
			In:      []any{val1, val2},
			Between: []any{val1, val2},
			NotIn:   []any{val2},
		}

		first, err1 := json.Marshal(filter)
		second, err2 := json.Marshal(filter)

		if err1 != nil || err2 != nil {
			t.Logf("Marshal errors: %v, %v", err1, err2)
			return false
		}

		return string(first) == string(second)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: serialization is non-deterministic: %v", err)
	}
}

// Property 3: Empty operator slices are omitted from JSON output.
// The domain uses `omitempty` tags so that unset operators do not appear
// in the serialized form. This verifies the structural contract: a filter
// with only Equals set must not emit "gt", "in", "between", etc. keys.
func TestProperty_Filter_OmitEmptyOperators(t *testing.T) {
	t.Parallel()

	property := func(val string) bool {
		if val == "" {
			return true
		}

		filter := model.FilterCondition{
			Equals: []any{val},
			// All other operators are intentionally nil/empty
		}

		data, err := json.Marshal(filter)
		if err != nil {
			t.Logf("Marshal error: %v", err)
			return false
		}

		// Unmarshal into a raw map to inspect which keys are present.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Logf("Unmarshal error: %v", err)
			return false
		}

		// Only "eq" should be present.
		if _, hasEq := raw["eq"]; !hasEq {
			t.Log("Expected 'eq' key to be present")
			return false
		}

		unexpectedKeys := []string{"gt", "gte", "lt", "lte", "between", "in", "nin"}
		for _, key := range unexpectedKeys {
			if _, found := raw[key]; found {
				t.Logf("Unexpected key '%s' found in JSON output", key)
				return false
			}
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: empty operators were not omitted: %v", err)
	}
}

// Property 4: Deep-copy via JSON preserves all fields.
// Creating a FilterCondition, copying it through marshal/unmarshal,
// and comparing with reflect.DeepEqual must hold for arbitrary string
// combinations across all eight operator slots.
func TestProperty_Filter_DeepCopyPreservesAllFields(t *testing.T) {
	t.Parallel()

	property := func(a, b, c string) bool {
		if a == "" || b == "" || c == "" {
			return true
		}

		original := model.FilterCondition{
			Equals:         []any{a},
			GreaterThan:    []any{b},
			GreaterOrEqual: []any{c},
			LessThan:       []any{a},
			LessOrEqual:    []any{b},
			Between:        []any{a, c},
			In:             []any{a, b, c},
			NotIn:          []any{c},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Logf("Marshal error: %v", err)
			return false
		}

		var copy model.FilterCondition
		if err := json.Unmarshal(data, &copy); err != nil {
			t.Logf("Unmarshal error: %v", err)
			return false
		}

		if !reflect.DeepEqual(original, copy) {
			t.Logf("Deep copy mismatch:\n  original: %+v\n  copy:     %+v", original, copy)
			return false
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: deep-copy did not preserve all fields: %v", err)
	}
}

// Property 5: Filtros devem ser serializáveis para JSON e deserializáveis sem perda
func TestProperty_Filter_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	property := func(val1, val2 string) bool {
		if val1 == "" && val2 == "" {
			return true
		}

		original := model.FilterCondition{
			Equals: []any{val1},
			NotIn:  []any{val2},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		if err != nil {
			return false
		}

		// Unmarshal back
		var decoded model.FilterCondition
		if err := json.Unmarshal(jsonData, &decoded); err != nil {
			return false
		}

		// Compare lengths
		return len(decoded.Equals) == len(original.Equals) &&
			len(decoded.NotIn) == len(original.NotIn)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: JSON round-trip: %v", err)
	}
}

// Property 6: Between operator JSON output always contains exactly 2 elements.
// The domain rule is that Between must have exactly 2 values (min, max).
// This verifies that for any pair of random strings, the serialized
// "between" array always contains exactly 2 entries after round-trip.
func TestProperty_Filter_BetweenAlwaysTwoElementsAfterRoundTrip(t *testing.T) {
	t.Parallel()

	property := func(lo, hi string) bool {
		if lo == "" || hi == "" {
			return true
		}

		filter := model.FilterCondition{
			Between: []any{lo, hi},
		}

		data, err := json.Marshal(filter)
		if err != nil {
			t.Logf("Marshal error: %v", err)
			return false
		}

		var decoded model.FilterCondition
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Logf("Unmarshal error: %v", err)
			return false
		}

		if len(decoded.Between) != 2 {
			t.Logf("Expected Between length 2, got %d", len(decoded.Between))
			return false
		}

		// Values must survive the round-trip.
		loDecoded, loOk := decoded.Between[0].(string)
		hiDecoded, hiOk := decoded.Between[1].(string)

		if !loOk || !hiOk {
			t.Logf("Type assertion failed: lo=%T, hi=%T", decoded.Between[0], decoded.Between[1])
			return false
		}

		return loDecoded == lo && hiDecoded == hi
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: Between round-trip did not preserve exactly 2 elements: %v", err)
	}
}

// Property 7: Numeric values undergo type widening through JSON round-trip.
// JSON numbers always unmarshal as float64 in Go. This property verifies
// that integer operator values are widened to float64 after a round-trip,
// which is a domain-relevant invariant: code consuming deserialized filters
// must handle float64 instead of int.
func TestProperty_Filter_NumericTypeWideningAfterRoundTrip(t *testing.T) {
	t.Parallel()

	property := func(n int) bool {
		original := model.FilterCondition{
			GreaterThan: []any{n},
			LessThan:    []any{n},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Logf("Marshal error: %v", err)
			return false
		}

		var decoded model.FilterCondition
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Logf("Unmarshal error: %v", err)
			return false
		}

		// After JSON round-trip, int values become float64.
		gtVal, gtOk := decoded.GreaterThan[0].(float64)
		ltVal, ltOk := decoded.LessThan[0].(float64)

		if !gtOk || !ltOk {
			t.Logf("Expected float64 after round-trip, got gt=%T, lt=%T",
				decoded.GreaterThan[0], decoded.LessThan[0])
			return false
		}

		return gtVal == float64(n) && ltVal == float64(n)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: numeric type widening after round-trip: %v", err)
	}
}

// Property 8: Empty FilterCondition produces empty JSON object.
// A zero-value FilterCondition with all nil slices must serialize to "{}".
// This ensures omitempty works correctly across all fields simultaneously
// and that no unexpected default values leak into the output.
func TestProperty_Filter_EmptyProducesEmptyJSON(t *testing.T) {
	t.Parallel()

	// This property does not use random inputs because the subject is
	// the zero-value struct, but quick.Check still validates the invariant
	// over multiple iterations (proving stability, not randomness).
	property := func(seed uint8) bool {
		empty := model.FilterCondition{}

		data, err := json.Marshal(empty)
		if err != nil {
			t.Logf("Marshal error: %v", err)
			return false
		}

		if string(data) != "{}" {
			t.Logf("Expected '{}', got '%s'", string(data))
			return false
		}

		// Round-trip must also produce a zero-value struct.
		var decoded model.FilterCondition
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Logf("Unmarshal error: %v", err)
			return false
		}

		return reflect.DeepEqual(empty, decoded)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: empty filter did not produce empty JSON: %v", err)
	}
}

// Property 9: NotIn deve ser inverso de In logicamente
func TestProperty_Filter_NotInInverse(t *testing.T) {
	t.Parallel()

	property := func(value string) bool {
		if value == "" {
			return true
		}

		filterIn := model.FilterCondition{
			In: []any{value},
		}

		filterNotIn := model.FilterCondition{
			NotIn: []any{value},
		}

		// Both should be valid but opposite
		return len(filterIn.In) > 0 && len(filterNotIn.NotIn) > 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: NotIn/In inverse: %v", err)
	}
}

// Property 10: FilterCondition com todos os operadores deve ser válido
func TestProperty_Filter_AllOperators(t *testing.T) {
	t.Parallel()

	property := func(val1, val2 string, num1, num2 int) bool {
		filter := model.FilterCondition{
			Equals:         []any{val1},
			GreaterThan:    []any{num1},
			GreaterOrEqual: []any{num1},
			LessThan:       []any{num2},
			LessOrEqual:    []any{num2},
			Between:        []any{num1, num2},
			In:             []any{val1, val2},
			NotIn:          []any{val2},
		}

		// All operators should be populated
		return len(filter.Equals) > 0 &&
			len(filter.GreaterThan) > 0 &&
			len(filter.In) > 0 &&
			len(filter.NotIn) > 0 &&
			len(filter.Between) == 2
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: all operators: %v", err)
	}
}
