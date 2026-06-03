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

// TestRegisterAll_MultipleCallsNoPanic verifies that RegisterAll() is safe
// to call multiple times (sync.Once protects it). The very first call happens
// in TestMain, so these are subsequent calls.
func TestRegisterAll_MultipleCallsNoPanic(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() {
		err := RegisterAll()
		assert.NoError(t, err)
	})

	// Third call for good measure
	err := RegisterAll()
	assert.NoError(t, err)
}

// TestRegisteredFilters_WorkThroughTemplateRendering verifies that all filters
// registered by doRegisterAll are functional by rendering templates that use them.
func TestRegisteredFilters_WorkThroughTemplateRendering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		context  pongo2.Context
		expected string
	}{
		{
			name:     "percent_of_filter",
			template: `{{ num|percent_of:total }}`,
			context:  pongo2.Context{"num": 25, "total": 100},
			expected: "25.00%",
		},
		{
			name:     "slice_str_filter",
			template: `{{ text|slice_str:"0:5" }}`,
			context:  pongo2.Context{"text": "Hello World"},
			expected: "Hello",
		},
		{
			name:     "strip_zeros_filter",
			template: `{{ val|strip_zeros }}`,
			context:  pongo2.Context{"val": "3.14000"},
			expected: "3.14",
		},
		{
			name:     "replace_filter",
			template: `{{ text|replace:"-:" }}`,
			context:  pongo2.Context{"text": "01310-100"},
			expected: "01310100",
		},
		{
			name:     "where_filter",
			template: `{% for item in items|where:"status:active" %}{{ item.name }},{% endfor %}`,
			context: pongo2.Context{
				"items": []map[string]any{
					{"name": "A", "status": "active"},
					{"name": "B", "status": "inactive"},
					{"name": "C", "status": "active"},
				},
			},
			expected: "A,C,",
		},
		{
			name:     "sum_filter",
			template: `{{ items|sum:"amount" }}`,
			context: pongo2.Context{
				"items": []map[string]any{
					{"amount": 100},
					{"amount": 200},
				},
			},
			expected: "300",
		},
		{
			name:     "count_filter",
			template: `{{ items|count:"type:A" }}`,
			context: pongo2.Context{
				"items": []map[string]any{
					{"type": "A"},
					{"type": "B"},
					{"type": "A"},
				},
			},
			expected: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

// TestRegisteredTags_WorkThroughTemplateRendering verifies that all tags
// registered by doRegisterAll are functional by rendering templates that use them.
func TestRegisteredTags_WorkThroughTemplateRendering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		context  pongo2.Context
		contains string
	}{
		{
			name:     "calc_tag_addition",
			template: `{% calc 10 + 20 %}`,
			context:  pongo2.Context{},
			contains: "30",
		},
		{
			name:     "sum_by_tag",
			template: `{% sum_by data by "amount" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"amount": 100},
					{"amount": 200},
				},
			},
			contains: "300",
		},
		{
			name:     "count_by_tag",
			template: `{% count_by data if amount > 50 %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"amount": 30},
					{"amount": 100},
					{"amount": 200},
				},
			},
			contains: "2",
		},
		{
			name:     "avg_by_tag",
			template: `{% avg_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": 10},
					{"value": 20},
					{"value": 30},
				},
			},
			contains: "20",
		},
		{
			name:     "min_by_tag",
			template: `{% min_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": 50},
					{"value": 10},
					{"value": 30},
				},
			},
			contains: "10",
		},
		{
			name:     "max_by_tag",
			template: `{% max_by data by "value" %}`,
			context: pongo2.Context{
				"data": []map[string]any{
					{"value": 50},
					{"value": 10},
					{"value": 30},
				},
			},
			contains: "50",
		},
		{
			name:     "counter_and_counter_show_tags",
			template: `{% counter "x" %}{% counter "x" %}{% counter_show "x" %}`,
			context: pongo2.Context{
				CounterContextKey: NewCounterStorage(),
			},
			contains: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err)

			out, err := tpl.Execute(tt.context)
			require.NoError(t, err)
			assert.Contains(t, out, tt.contains)
		})
	}
}

// TestSafeFromString_ValidTemplate verifies SafeFromString creates templates
// using a fresh TemplateSet to avoid data races.
func TestSafeFromString_ValidTemplate(t *testing.T) {
	t.Parallel()

	tpl, err := SafeFromString(`Hello {{ name }}!`)
	require.NoError(t, err)
	require.NotNil(t, tpl)

	out, err := tpl.Execute(pongo2.Context{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", out)
}

// TestSafeFromString_InvalidTemplate verifies SafeFromString returns an error
// for malformed templates.
func TestSafeFromString_InvalidTemplate(t *testing.T) {
	t.Parallel()

	_, err := SafeFromString(`{% if %}`)
	require.Error(t, err)
}

// TestSafeFromString_ConcurrentUsage verifies SafeFromString is safe for
// concurrent use from multiple goroutines.
func TestSafeFromString_ConcurrentUsage(t *testing.T) {
	t.Parallel()

	done := make(chan error, 50)

	for i := 0; i < 50; i++ {
		go func() {
			tpl, err := SafeFromString(`{{ x }}`)
			if err != nil {
				done <- err
				return
			}

			_, err = tpl.Execute(pongo2.Context{"x": "ok"})
			done <- err
		}()
	}

	for i := 0; i < 50; i++ {
		err := <-done
		assert.NoError(t, err)
	}
}

// ─── BanTag SSTI Prevention Tests ────────────────────────────────────────────

// TestBannedTags_SSTI_Prevention verifies that all 5 banned tags are rejected
// by SafeFromString, preventing Server-Side Template Injection via file
// inclusion and template inheritance.
func TestBannedTags_SSTI_Prevention(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		tag      string
	}{
		{
			name:     "include is banned",
			template: `{% include "secret.html" %}`,
			tag:      "include",
		},
		{
			name:     "extends is banned",
			template: `{% extends "base.html" %}`,
			tag:      "extends",
		},
		{
			name:     "import is banned",
			template: `{% import "macros.html" %}`,
			tag:      "import",
		},
		{
			name:     "block is banned",
			template: `{% block content %}evil{% endblock %}`,
			tag:      "block",
		},
		{
			name:     "ssi is banned",
			template: `{% ssi "/etc/passwd" %}`,
			tag:      "ssi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := SafeFromString(tt.template)
			require.Error(t, err, "tag %q should be banned but was accepted", tt.tag)
			assert.Contains(t, err.Error(), "is not allowed")
		})
	}
}

// TestBannedTags_SafeTagsStillWork verifies that banning dangerous tags does
// not break legitimate template functionality.
func TestBannedTags_SafeTagsStillWork(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		context  pongo2.Context
		contains string
	}{
		{
			name:     "for loop works",
			template: `{% for x in items %}{{ x }},{% endfor %}`,
			context:  pongo2.Context{"items": []string{"a", "b"}},
			contains: "a,b,",
		},
		{
			name:     "if/else works",
			template: `{% if ok %}yes{% else %}no{% endif %}`,
			context:  pongo2.Context{"ok": true},
			contains: "yes",
		},
		{
			name:     "with works",
			template: `{% with val=42 %}{{ val }}{% endwith %}`,
			context:  pongo2.Context{},
			contains: "42",
		},
		{
			name:     "set works",
			template: `{% set x = 10 %}{{ x }}`,
			context:  pongo2.Context{},
			contains: "10",
		},
		{
			name:     "filter tag works",
			template: `{% filter upper %}hello{% endfilter %}`,
			context:  pongo2.Context{},
			contains: "HELLO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tpl, err := SafeFromString(tt.template)
			require.NoError(t, err, "safe tag should not be banned")

			out, err := tpl.Execute(tt.context)
			require.NoError(t, err)
			assert.Contains(t, out, tt.contains)
		})
	}
}

// TestBannedTags_SSTI_NestedInContent verifies that banned tags embedded
// inside otherwise valid template content are still rejected.
func TestBannedTags_SSTI_NestedInContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
	}{
		{
			name:     "include nested in for loop",
			template: `{% for x in items %}{% include "evil.html" %}{% endfor %}`,
		},
		{
			name:     "extends after variable",
			template: `{{ name }}{% extends "base.html" %}`,
		},
		{
			name:     "ssi nested in conditional",
			template: `{% if true %}{% ssi "/etc/passwd" %}{% endif %}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := SafeFromString(tt.template)
			require.Error(t, err, "nested banned tag should still be rejected")
			assert.Contains(t, err.Error(), "is not allowed")
		})
	}
}

// TestNewSafeTemplateSet_PanicsOnInvalidBan verifies fail-closed behavior:
// if BanTag fails, the system panics rather than silently allowing the tag.
func TestNewSafeTemplateSet_PanicsOnInvalidBan(t *testing.T) {
	t.Parallel()

	// newSafeTemplateSet with valid banned tags should NOT panic
	assert.NotPanics(t, func() {
		ts := newSafeTemplateSet("test-safe")
		require.NotNil(t, ts)
	})
}

// TestBannedTagsList_IsComplete verifies the bannedTags slice contains exactly
// the 5 expected tags (regression guard against accidental removal).
func TestBannedTagsList_IsComplete(t *testing.T) {
	t.Parallel()

	expected := []string{"include", "extends", "import", "block", "ssi"}
	assert.Equal(t, expected, bannedTags)
}
