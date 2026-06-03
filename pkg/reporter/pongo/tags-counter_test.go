// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"testing"

	"github.com/flosch/pongo2/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestContext creates a pongo2.Context with counter storage for testing
func newTestContext() pongo2.Context {
	return pongo2.Context{
		CounterContextKey: NewCounterStorage(),
	}
}

func TestCounterTag_BasicIncrement(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% counter "1100" %}{% counter "1100" %}{% counter "1100" %}{% counter_show "1100" %}`)
	require.NoError(t, err)

	result, err := tpl.Execute(newTestContext())
	require.NoError(t, err)

	assert.Equal(t, "3", result)
}

func TestCounterTag_MultipleCounters(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% counter "1100" %}{% counter "1100" %}{% counter "1101" %}{% counter "1101" %}{% counter "1101" %}{% counter_show "1100" %}-{% counter_show "1101" %}`)
	require.NoError(t, err)

	result, err := tpl.Execute(newTestContext())
	require.NoError(t, err)

	assert.Equal(t, "2-3", result)
}

func TestCounterTag_SumMultipleCounters(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% counter "1100" %}{% counter "1100" %}{% counter "1101" %}{% counter "1101" %}{% counter "1101" %}{% counter_show "1100" "1101" %}`)
	require.NoError(t, err)

	result, err := tpl.Execute(newTestContext())
	require.NoError(t, err)

	assert.Equal(t, "5", result) // 2 + 3 = 5
}

func TestCounterTag_InLoop(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% for i in items %}|1100|{% counter "1100" %}{{ i.name }}|
{% endfor %}Total: {% counter_show "1100" %}`)
	require.NoError(t, err)

	ctx := newTestContext()
	ctx["items"] = []map[string]any{
		{"name": "Item1"},
		{"name": "Item2"},
		{"name": "Item3"},
	}

	result, err := tpl.Execute(ctx)
	require.NoError(t, err)

	expected := `|1100|Item1|
|1100|Item2|
|1100|Item3|
Total: 3`
	assert.Equal(t, expected, result)
}

func TestCounterTag_NestedLoops(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% for acc in accounts %}|1100|{% counter "1100" %}{{ acc.id }}|
{% for det in acc.details %}|1101|{% counter "1101" %}{{ det.value }}|
{% endfor %}{% endfor %}|9900|1100|{% counter_show "1100" %}|
|9900|1101|{% counter_show "1101" %}|
|9900|TOTAL|{% counter_show "1100" "1101" %}|`)
	require.NoError(t, err)

	ctx := newTestContext()
	ctx["accounts"] = []map[string]any{
		{
			"id": "ACC1",
			"details": []map[string]any{
				{"value": "D1"},
				{"value": "D2"},
			},
		},
		{
			"id": "ACC2",
			"details": []map[string]any{
				{"value": "D3"},
			},
		},
	}

	result, err := tpl.Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, result, "|9900|1100|2|")
	assert.Contains(t, result, "|9900|1101|3|")
	assert.Contains(t, result, "|9900|TOTAL|5|")
}

func TestCounterTag_ZeroCounter(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% counter_show "nonexistent" %}`)
	require.NoError(t, err)

	result, err := tpl.Execute(newTestContext())
	require.NoError(t, err)

	assert.Equal(t, "0", result)
}

func TestCounterTag_IsolatedBetweenRenders(t *testing.T) {
	t.Parallel()
	// Each render has its own counter storage, so counters don't leak between renders
	tpl, err := SafeFromString(`{% counter "test" %}{% counter "test" %}{% counter_show "test" %}`)
	require.NoError(t, err)

	// First render with its own context
	result1, err := tpl.Execute(newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "2", result1)

	// Second render with a fresh context - should also be "2", not "4"
	result2, err := tpl.Execute(newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "2", result2)
}

func TestCounterTag_DIMPExample(t *testing.T) {
	t.Parallel()
	// Simulates a DIMP report structure
	tpl, err := SafeFromString(`|0000|12345678901234|EMPRESA|
{% for acc in accounts %}|1100|{% counter "1100" %}SP|{{ acc.id }}|{{ acc.alias }}|
{% endfor %}{% for tx in transactions %}|1102|{% counter "1102" %}{{ tx.id }}|{{ tx.amount }}|
{% endfor %}|TOTAL_SP|1500.00|
|9900|1100|{% counter_show "1100" %}|
|9900|1102|{% counter_show "1102" %}|`)
	require.NoError(t, err)

	ctx := newTestContext()
	ctx["accounts"] = []map[string]any{
		{"id": "ACC001", "alias": "account/123"},
		{"id": "ACC002", "alias": "account/456"},
		{"id": "ACC003", "alias": "account/789"},
	}
	ctx["transactions"] = []map[string]any{
		{"id": "TX001", "amount": "500.00"},
		{"id": "TX002", "amount": "1000.00"},
	}

	result, err := tpl.Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, result, "|9900|1100|3|")
	assert.Contains(t, result, "|9900|1102|2|")
}

func TestCounterTag_SumThreeCounters(t *testing.T) {
	t.Parallel()
	tpl, err := SafeFromString(`{% counter "A" %}{% counter "A" %}{% counter "B" %}{% counter "B" %}{% counter "B" %}{% counter "C" %}{% counter_show "A" "B" "C" %}`)
	require.NoError(t, err)

	result, err := tpl.Execute(newTestContext())
	require.NoError(t, err)

	assert.Equal(t, "6", result) // 2 + 3 + 1 = 6
}

func TestCounterTag_ConcurrentRendersSafe(t *testing.T) {
	t.Parallel()
	// This test verifies that concurrent renders don't interfere with each other
	tpl, err := SafeFromString(`{% counter "x" %}{% counter "x" %}{% counter "x" %}{% counter_show "x" %}`)
	require.NoError(t, err)

	// Run many concurrent renders
	done := make(chan string, 100)
	for i := 0; i < 100; i++ {
		go func() {
			result, err := tpl.Execute(newTestContext())
			if err != nil {
				done <- "error"
				return
			}
			done <- result
		}()
	}

	// All results should be "3" (isolated counters)
	for i := 0; i < 100; i++ {
		result := <-done
		assert.Equal(t, "3", result, "concurrent render produced wrong result")
	}
}

// ---------------------------------------------------------------------------
// getCounterStorage fallback path
// ---------------------------------------------------------------------------

// TestGetCounterStorage_FallbackWhenMissing verifies that getCounterStorage
// creates a new map when CounterContextKey is NOT present in the pongo2 context.
// This exercises the fallback branch (line 37 of tags-counter.go).
func TestGetCounterStorage_FallbackWhenMissing(t *testing.T) {
	t.Parallel()

	// Execute a template with counter tags but WITHOUT pre-setting counter storage.
	// The counter tag will call getCounterStorage which should create a fresh map.
	tpl, err := SafeFromString(`{% counter "test" %}{% counter "test" %}{% counter_show "test" %}`)
	require.NoError(t, err)

	// Empty context: no CounterContextKey set
	ctx := pongo2.Context{}
	result, err := tpl.Execute(ctx)
	require.NoError(t, err)

	// Because getCounterStorage falls back to a NEW map each time it's called,
	// the counter increments are lost between calls. The counter_show will see 0.
	assert.Equal(t, "0", result)
}

// TestGetCounterStorage_FallbackWithWrongType verifies that getCounterStorage
// creates a new map when CounterContextKey is present but holds a wrong type.
func TestGetCounterStorage_FallbackWithWrongType(t *testing.T) {
	t.Parallel()

	tpl, err := SafeFromString(`{% counter "abc" %}{% counter_show "abc" %}`)
	require.NoError(t, err)

	// Set CounterContextKey to a wrong type (string instead of map[string]int)
	ctx := pongo2.Context{
		CounterContextKey: "not a map",
	}
	result, err := tpl.Execute(ctx)
	require.NoError(t, err)

	// Fallback creates new empty map each time, so counter_show sees 0
	assert.Equal(t, "0", result)
}

// TestCounterShowTag_MultipleNamesWithFallback verifies counter_show with
// multiple counter names where some counters have been incremented.
func TestCounterShowTag_MultipleNamesWithFallback(t *testing.T) {
	t.Parallel()

	tpl, err := SafeFromString(`{% counter "a" %}{% counter "a" %}{% counter "b" %}{% counter_show "a" "b" "c" %}`)
	require.NoError(t, err)

	result, err := tpl.Execute(newTestContext())
	require.NoError(t, err)

	// a=2, b=1, c=0 (never incremented, so storage[c] = 0 which is Go zero value)
	assert.Equal(t, "3", result)
}

// ---------------------------------------------------------------------------
// counter/counter_show tag parser error paths
// ---------------------------------------------------------------------------

// TestCounterTag_NoArguments verifies the tag parser error when no argument is provided.
func TestCounterTag_NoArguments(t *testing.T) {
	t.Parallel()
	_, err := SafeFromString(`{% counter %}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter tag requires a counter name")
}

// TestCounterTag_NonStringArgument verifies the tag parser error when a non-string
// argument is provided (e.g. a variable name rather than a quoted string).
func TestCounterTag_NonStringArgument(t *testing.T) {
	t.Parallel()
	_, err := SafeFromString(`{% counter myvar %}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter tag requires a string argument")
}

// TestCounterShowTag_NoArguments verifies the tag parser error when no argument is provided.
func TestCounterShowTag_NoArguments(t *testing.T) {
	t.Parallel()
	_, err := SafeFromString(`{% counter_show %}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter_show tag requires at least one counter name")
}

// TestCounterShowTag_NonStringArgument verifies the parser error for non-string argument.
func TestCounterShowTag_NonStringArgument(t *testing.T) {
	t.Parallel()
	_, err := SafeFromString(`{% counter_show myvar %}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter_show tag requires string arguments")
}
