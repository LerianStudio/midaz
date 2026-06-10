// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFiltersForDatasource_DirectShape proves the named-type shape (the DIRECT
// QueryStream path the connector's own tests use) is accepted verbatim.
func TestFiltersForDatasource_DirectShape(t *testing.T) {
	t.Parallel()

	condition := model.FilterCondition{Equals: []any{"ACTIVE"}}
	raw := map[string]any{
		"onboarding": datasourceFilters{
			"public.accounts": {"status": condition},
		},
	}

	got, err := filtersForDatasource("onboarding", raw)
	require.NoError(t, err)
	assert.Equal(t, condition, got.tableFilters("public.accounts")["status"])
}

// TestFiltersForDatasource_PlannerShape proves the nested map[string]any shape
// the planner/runner produce (the PLAN path: PlanExtraction -> ExecuteExtraction)
// is structurally decoded back to the same datasourceFilters the cursors consume.
// This is the path the generate-report handler drives; the engine's pre-Phase-3
// integration tests never exercised it.
func TestFiltersForDatasource_PlannerShape(t *testing.T) {
	t.Parallel()

	condition := model.FilterCondition{Equals: []any{"World"}}
	raw := map[string]any{
		"onboarding": map[string]any{
			"public.organization": map[string]any{
				"name": condition,
			},
		},
	}

	got, err := filtersForDatasource("onboarding", raw)
	require.NoError(t, err)
	assert.Equal(t, condition, got.tableFilters("public.organization")["name"])
}

// TestFiltersForDatasource_MissingDatasource proves an absent or nil datasource
// entry yields the unfiltered case (nil, no error).
func TestFiltersForDatasource_MissingDatasource(t *testing.T) {
	t.Parallel()

	got, err := filtersForDatasource("onboarding", map[string]any{"other": map[string]any{}})
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = filtersForDatasource("onboarding", map[string]any{"onboarding": nil})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestFiltersForDatasource_MisShapedFailsClosed proves a mis-shaped payload is a
// loud validation error rather than a silent full-table read: a financial
// reporter must never widen scope because a filter payload was malformed.
func TestFiltersForDatasource_MisShapedFailsClosed(t *testing.T) {
	t.Parallel()

	t.Run("datasource entry not a map", func(t *testing.T) {
		t.Parallel()

		_, err := filtersForDatasource("onboarding", map[string]any{"onboarding": "nonsense"})
		require.Error(t, err)
	})

	t.Run("table entry not a map", func(t *testing.T) {
		t.Parallel()

		raw := map[string]any{"onboarding": map[string]any{"public.accounts": "nonsense"}}
		_, err := filtersForDatasource("onboarding", raw)
		require.Error(t, err)
	})

	t.Run("field value not a FilterCondition", func(t *testing.T) {
		t.Parallel()

		raw := map[string]any{
			"onboarding": map[string]any{
				"public.accounts": map[string]any{"status": "nonsense"},
			},
		}
		_, err := filtersForDatasource("onboarding", raw)
		require.Error(t, err)
	})
}
