//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/pongo"
	h "github.com/LerianStudio/midaz/v3/tests/reporter/utils"
)

// TestIntegration_TemplateBuilder_GetBlocksConfig_ReturnsAllBlockTypes verifies that
// GET /v1/templates/blocks-config returns HTTP 200 with all 13 block types defined
// in the blocks registry. (IS-1)
func TestIntegration_TemplateBuilder_GetBlocksConfig_ReturnsAllBlockTypes(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/templates/blocks-config", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", code, string(body))
	}

	var resp pongo.BlocksConfigResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal blocks-config response: %v", err)
	}

	// Verify all 13 block types are present
	const expectedBlockCount = 13
	if len(resp.Blocks) != expectedBlockCount {
		t.Fatalf("expected %d block definitions, got %d", expectedBlockCount, len(resp.Blocks))
	}

	// Verify each expected block type exists
	expectedTypes := map[string]bool{
		"text":        false,
		"variable":    false,
		"loop":        false,
		"conditional": false,
		"aggregation": false,
		"calculation": false,
		"date_time":   false,
		"counter":     false,
		"comment":     false,
		"section":     false,
		"with":        false,
		"expression":  false,
		"custom_tag":  false,
	}

	for _, block := range resp.Blocks {
		if block.Type == "" {
			t.Error("found block with empty type")
		}

		if block.Label == "" {
			t.Errorf("block %q has empty label", block.Type)
		}

		if block.Category == "" {
			t.Errorf("block %q has empty category", block.Type)
		}

		if _, ok := expectedTypes[block.Type]; !ok {
			t.Errorf("unexpected block type %q", block.Type)
		}

		expectedTypes[block.Type] = true
	}

	for typ, found := range expectedTypes {
		if !found {
			t.Errorf("missing expected block type %q", typ)
		}
	}
}

// TestIntegration_TemplateBuilder_GetBlocksConfig_BlockStructure verifies that
// blocks with properties return correct property structure, specifically the
// counter block which has counterMode and counterNames properties. (IS-4 partial)
func TestIntegration_TemplateBuilder_GetBlocksConfig_BlockStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/templates/blocks-config", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", code, string(body))
	}

	var resp pongo.BlocksConfigResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal blocks-config response: %v", err)
	}

	// Find the counter block and validate its properties
	var counterBlock *pongo.BlockDefinition
	for i := range resp.Blocks {
		if resp.Blocks[i].Type == "counter" {
			counterBlock = &resp.Blocks[i]

			break
		}
	}

	if counterBlock == nil {
		t.Fatal("counter block not found in response")
	}

	if counterBlock.Category != "dimp" {
		t.Errorf("counter block category: expected %q, got %q", "dimp", counterBlock.Category)
	}

	if len(counterBlock.Properties) != 2 {
		t.Fatalf("counter block: expected 2 properties, got %d", len(counterBlock.Properties))
	}

	// Verify counterMode property
	propByName := make(map[string]pongo.BlockProperty)
	for _, p := range counterBlock.Properties {
		propByName[p.Name] = p
	}

	counterMode, ok := propByName["counterMode"]
	if !ok {
		t.Fatal("counter block missing counterMode property")
	}

	if counterMode.Type != "enum" {
		t.Errorf("counterMode type: expected %q, got %q", "enum", counterMode.Type)
	}

	if !counterMode.Required {
		t.Error("counterMode should be required")
	}

	if len(counterMode.Values) != 2 {
		t.Fatalf("counterMode values: expected 2, got %d", len(counterMode.Values))
	}

	counterNames, ok := propByName["counterNames"]
	if !ok {
		t.Fatal("counter block missing counterNames property")
	}

	if counterNames.Type != "string[]" {
		t.Errorf("counterNames type: expected %q, got %q", "string[]", counterNames.Type)
	}

	if !counterNames.Required {
		t.Error("counterNames should be required")
	}

	// Verify blocks with acceptsChildren flag
	childBlocks := map[string]bool{"loop": false, "conditional": false, "section": false}
	for _, block := range resp.Blocks {
		if _, expectsChildren := childBlocks[block.Type]; expectsChildren {
			if !block.AcceptsChildren {
				t.Errorf("block %q should have acceptsChildren=true", block.Type)
			}

			childBlocks[block.Type] = true
		}
	}

	for typ, verified := range childBlocks {
		if !verified {
			t.Errorf("block %q not found for acceptsChildren verification", typ)
		}
	}
}

// TestIntegration_TemplateBuilder_GetFiltersConfig_ReturnsDIMPFilters verifies that
// GET /v1/templates/filters returns HTTP 200 with all filters including the DIMP
// filters (replace, where, sum, count). (IS-2)
func TestIntegration_TemplateBuilder_GetFiltersConfig_ReturnsDIMPFilters(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/templates/filters", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", code, string(body))
	}

	var resp pongo.FiltersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal filters response: %v", err)
	}

	// Verify total filter count matches registry
	const expectedFilterCount = 10
	if len(resp.Filters) != expectedFilterCount {
		t.Fatalf("expected %d filter definitions, got %d", expectedFilterCount, len(resp.Filters))
	}

	// Verify DIMP filters are present
	dimpFilters := map[string]bool{
		"replace": false,
		"where":   false,
		"sum":     false,
		"count":   false,
	}

	for _, filter := range resp.Filters {
		if _, isDIMP := dimpFilters[filter.Name]; isDIMP {
			dimpFilters[filter.Name] = true
		}
	}

	for name, found := range dimpFilters {
		if !found {
			t.Errorf("missing DIMP filter %q in response", name)
		}
	}
}

// TestIntegration_TemplateBuilder_GetFiltersConfig_FilterStructure verifies that
// each filter in the response has the required fields: name, description, args, and
// example. (IS-4 partial)
func TestIntegration_TemplateBuilder_GetFiltersConfig_FilterStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/templates/filters", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", code, string(body))
	}

	var resp pongo.FiltersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal filters response: %v", err)
	}

	if len(resp.Filters) == 0 {
		t.Fatal("filters response is empty")
	}

	// Verify every filter has required fields populated
	for _, filter := range resp.Filters {
		if filter.Name == "" {
			t.Error("found filter with empty name")
		}

		if filter.Description == "" {
			t.Errorf("filter %q has empty description", filter.Name)
		}

		if filter.Args == nil {
			t.Errorf("filter %q has nil args (expected empty slice or populated)", filter.Name)
		}

		if filter.Example == "" {
			t.Errorf("filter %q has empty example", filter.Name)
		}
	}

	// Verify specific filter details for deeper validation
	filterByName := make(map[string]pongo.FilterDefinition)
	for _, f := range resp.Filters {
		filterByName[f.Name] = f
	}

	// Verify percent_of has a denominator arg
	percentOf, ok := filterByName["percent_of"]
	if !ok {
		t.Fatal("missing filter percent_of")
	}

	if len(percentOf.Args) != 1 || percentOf.Args[0] != "denominator" {
		t.Errorf("percent_of args: expected [denominator], got %v", percentOf.Args)
	}

	// Verify sum has a field arg
	sumFilter, ok := filterByName["sum"]
	if !ok {
		t.Fatal("missing filter sum")
	}

	if len(sumFilter.Args) != 1 || sumFilter.Args[0] != "field" {
		t.Errorf("sum args: expected [field], got %v", sumFilter.Args)
	}

	// Verify strip_zeros has empty args
	stripZeros, ok := filterByName["strip_zeros"]
	if !ok {
		t.Fatal("missing filter strip_zeros")
	}

	if len(stripZeros.Args) != 0 {
		t.Errorf("strip_zeros args: expected empty, got %v", stripZeros.Args)
	}
}

// TestIntegration_TemplateBuilder_GetBlocksConfig_ContentTypeJSON verifies that
// GET /v1/templates/blocks-config returns Content-Type application/json. (IS-3 partial)
func TestIntegration_TemplateBuilder_GetBlocksConfig_ContentTypeJSON(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, _, respHeaders, err := cli.RequestFull(ctx, "GET", "/v1/templates/blocks-config", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", code)
	}

	ct := respHeaders.Get("Content-Type")
	if ct == "" {
		t.Fatal("Content-Type header is missing")
	}

	// Fiber returns "application/json" or "application/json; charset=utf-8"
	if ct != "application/json" && ct != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// TestIntegration_TemplateBuilder_GetFiltersConfig_ContentTypeJSON verifies that
// GET /v1/templates/filters returns Content-Type application/json. (IS-3 partial)
func TestIntegration_TemplateBuilder_GetFiltersConfig_ContentTypeJSON(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, _, respHeaders, err := cli.RequestFull(ctx, "GET", "/v1/templates/filters", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", code)
	}

	ct := respHeaders.Get("Content-Type")
	if ct == "" {
		t.Fatal("Content-Type header is missing")
	}

	if ct != "application/json" && ct != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// TestIntegration_TemplateBuilder_GetBlocksConfig_CategoryDistribution verifies that
// blocks are distributed across the expected categories: basic, control, data, dimp, layout.
// This is an edge case test ensuring no category is missing. (IS-4 edge case)
func TestIntegration_TemplateBuilder_GetBlocksConfig_CategoryDistribution(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/templates/blocks-config", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", code, string(body))
	}

	var resp pongo.BlocksConfigResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	expectedCategories := map[string]bool{
		"basic":    false,
		"control":  false,
		"data":     false,
		"dimp":     false,
		"layout":   false,
		"advanced": false,
	}

	for _, block := range resp.Blocks {
		if _, expected := expectedCategories[block.Category]; expected {
			expectedCategories[block.Category] = true
		} else {
			t.Errorf("unexpected category %q for block %q", block.Category, block.Type)
		}
	}

	for cat, found := range expectedCategories {
		if !found {
			t.Errorf("no blocks found for expected category %q", cat)
		}
	}
}

// TestIntegration_TemplateBuilder_GetFiltersConfig_AllFilterNamesUnique verifies that
// filter names are unique (no duplicates). (IS-4 edge case)
func TestIntegration_TemplateBuilder_GetFiltersConfig_AllFilterNamesUnique(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/templates/filters", headers, nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", code, string(body))
	}

	var resp pongo.FiltersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	seen := make(map[string]bool)
	for _, filter := range resp.Filters {
		if seen[filter.Name] {
			t.Errorf("duplicate filter name %q", filter.Name)
		}

		seen[filter.Name] = true
	}
}
