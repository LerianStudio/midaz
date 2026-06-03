// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"

	"github.com/flosch/pongo2/v6"
)

// CounterContextKey is the key used to store counters in the pongo2 context
const CounterContextKey = "_counters"

// counterNode represents the counter tag that increments a named counter
type counterNode struct {
	name string
}

// counterShowNode represents the counter_show tag that displays counter value(s)
type counterShowNode struct {
	names []string
}

// NewCounterStorage creates a new counter storage map for a render context
func NewCounterStorage() map[string]int {
	return make(map[string]int)
}

// getCounterStorage retrieves the counter storage from the execution context
func getCounterStorage(ctx *pongo2.ExecutionContext) map[string]int {
	if storage, ok := ctx.Public[CounterContextKey].(map[string]int); ok {
		return storage
	}
	// Fallback: create new storage if not found (shouldn't happen in normal use)
	return make(map[string]int)
}

// makeCounterTag creates the counter tag parser with expression support
// Usage: {% counter "1100" %}
func makeCounterTag() pongo2.TagParser {
	return func(_ *pongo2.Parser, _ *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
		if arguments.Remaining() == 0 {
			return nil, arguments.Error("counter tag requires a counter name", nil)
		}

		// Get the counter name (should be a string)
		token := arguments.Current()
		if token.Typ != pongo2.TokenString {
			return nil, arguments.Error("counter tag requires a string argument", nil)
		}

		name := token.Val

		arguments.Consume()

		return &counterNode{name: name}, nil
	}
}

// Execute increments the named counter using the render-scoped storage
func (node *counterNode) Execute(ctx *pongo2.ExecutionContext, _ pongo2.TemplateWriter) *pongo2.Error {
	storage := getCounterStorage(ctx)
	storage[node.name]++

	return nil
}

// makeCounterShowTag creates the counter_show tag parser
// Usage: {% counter_show "1100" %} or {% counter_show "1100" "1101" "1102" %}
func makeCounterShowTag() pongo2.TagParser {
	return func(_ *pongo2.Parser, _ *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
		if arguments.Remaining() == 0 {
			return nil, arguments.Error("counter_show tag requires at least one counter name", nil)
		}

		var names []string

		// Parse all string arguments
		for arguments.Remaining() > 0 {
			token := arguments.Current()
			if token.Typ != pongo2.TokenString {
				break
			}

			names = append(names, token.Val)

			arguments.Consume()
		}

		if len(names) == 0 {
			return nil, arguments.Error("counter_show tag requires string arguments", nil)
		}

		return &counterShowNode{names: names}, nil
	}
}

// Execute displays the sum of all named counters using render-scoped storage
func (node *counterShowNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	storage := getCounterStorage(ctx)

	total := 0
	for _, name := range node.names {
		total += storage[name]
	}

	if _, err := fmt.Fprintf(writer, "%d", total); err != nil {
		return &pongo2.Error{
			Sender:    "counter_show",
			OrigError: err,
		}
	}

	return nil
}
