// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/tests/reporter/e2e/shared"
)

// ############################################################################
// Blocks Config Tests (TC-TB-001 to TC-TB-003)
// ############################################################################

func TestTemplateBuilder_GetBlocksConfig(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GetBlocksConfig(ctx)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	blocks, ok := body["blocks"].([]any)
	require.True(t, ok, "response should contain 'blocks' array")
	assert.GreaterOrEqual(t, len(blocks), 10, "should have at least 10 block definitions")

	// Validate expected categories are present
	categories := make(map[string]bool)
	blockTypes := make(map[string]bool)

	for _, b := range blocks {
		block, ok := b.(map[string]any)
		require.True(t, ok)

		cat, _ := block["category"].(string)
		categories[cat] = true

		bt, _ := block["type"].(string)
		blockTypes[bt] = true
	}

	expectedCategories := []string{"basic", "control", "data"}
	for _, cat := range expectedCategories {
		assert.True(t, categories[cat], "should have category %q", cat)
	}

	// Validate core block types exist
	expectedTypes := []string{"text", "variable", "loop", "conditional", "comment"}
	for _, bt := range expectedTypes {
		assert.True(t, blockTypes[bt], "should have block type %q", bt)
	}
}

func TestTemplateBuilder_BlocksConfigHasProperties(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GetBlocksConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	blocks := body["blocks"].([]any)

	for _, b := range blocks {
		block := b.(map[string]any)

		// Every block must have type, label, category
		assert.NotEmpty(t, block["type"], "block should have 'type'")
		assert.NotEmpty(t, block["label"], "block should have 'label'")
		assert.NotEmpty(t, block["category"], "block should have 'category'")

		// acceptsChildren should be present
		_, hasAcceptsChildren := block["acceptsChildren"]
		assert.True(t, hasAcceptsChildren, "block %v should have 'acceptsChildren'", block["type"])
	}
}

// ############################################################################
// Filters Config Tests (TC-TB-004 to TC-TB-005)
// ############################################################################

func TestTemplateBuilder_GetFiltersConfig(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GetFiltersConfig(ctx)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	filters, ok := body["filters"].([]any)
	require.True(t, ok, "response should contain 'filters' array")
	assert.GreaterOrEqual(t, len(filters), 5, "should have at least 5 filter definitions")

	filterNames := make(map[string]bool)

	for _, f := range filters {
		filter, ok := f.(map[string]any)
		require.True(t, ok)

		name, _ := filter["name"].(string)
		filterNames[name] = true

		// Each filter should have name, description, example
		assert.NotEmpty(t, filter["name"], "filter should have 'name'")
		assert.NotEmpty(t, filter["description"], "filter %v should have 'description'", name)
		assert.NotEmpty(t, filter["example"], "filter %v should have 'example'", name)
	}

	// Validate key filters exist
	expectedFilters := []string{"replace", "floatformat", "length", "date"}
	for _, name := range expectedFilters {
		assert.True(t, filterNames[name], "should have filter %q", name)
	}
}
