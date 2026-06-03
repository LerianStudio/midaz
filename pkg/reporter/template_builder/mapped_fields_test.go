// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMappedFields_VariableBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blocks   []TemplateBlock
		expected map[string]map[string][]string
	}{
		{
			name: "simple variable with dot notation",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "orders.total"},
			},
			expected: map[string]map[string][]string{
				"default": {
					"orders": {"total"},
				},
			},
		},
		{
			name: "simple variable without dot notation",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "total"},
			},
			expected: map[string]map[string][]string{
				"default": {
					"_root": {"total"},
				},
			},
		},
		{
			name: "variable with filter args referencing field",
			blocks: []TemplateBlock{
				{Type: "variable", Variable: "items", Filters: []FilterChain{
					{Name: "where", Args: "state:SP"},
					{Name: "sum", Args: "value"},
				}},
			},
			expected: map[string]map[string][]string{
				"default": {
					"_root": {"items"},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ExtractMappedFields(tt.blocks)
			require.NotNil(t, result)

			for ds, tables := range tt.expected {
				require.Contains(t, result, ds)

				for table, fields := range tables {
					require.Contains(t, result[ds], table)
					assert.ElementsMatch(t, fields, result[ds][table])
				}
			}
		})
	}
}

func TestExtractMappedFields_LoopBlock(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{
			Type:       "loop",
			Iterator:   "item",
			Collection: "transactions",
			Children: []TemplateBlock{
				{Type: "variable", Variable: "item.amount"},
			},
		},
	}

	result := ExtractMappedFields(blocks)
	require.NotNil(t, result)
	require.Contains(t, result, "default")
	require.Contains(t, result["default"], "transactions")
}

func TestExtractMappedFields_AggregationBlock(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{Type: "aggregation", Collection: "orders", Function: "sum", Field: "value"},
	}

	result := ExtractMappedFields(blocks)
	require.NotNil(t, result)
	require.Contains(t, result, "default")
	require.Contains(t, result["default"], "orders")
	assert.Contains(t, result["default"]["orders"], "value")
}

func TestExtractMappedFields_CalculationBlock(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{Type: "calculation", Variable: "total", Function: "percent_of"},
	}

	result := ExtractMappedFields(blocks)
	require.NotNil(t, result)
	require.Contains(t, result, "default")
	require.Contains(t, result["default"], "_root")
	assert.Contains(t, result["default"]["_root"], "total")
}

func TestExtractMappedFields_DateTimeBlock(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{Type: "date_time", Variable: "created_at", Format: "02/01/2006"},
	}

	result := ExtractMappedFields(blocks)
	require.NotNil(t, result)
	require.Contains(t, result, "default")
	require.Contains(t, result["default"], "_root")
	assert.Contains(t, result["default"]["_root"], "created_at")
}

func TestExtractMappedFields_NestedBlocks(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{
			Type: "section",
			Children: []TemplateBlock{
				{
					Type:       "loop",
					Iterator:   "tx",
					Collection: "transactions",
					Children: []TemplateBlock{
						{Type: "variable", Variable: "tx.description"},
						{
							Type:      "conditional",
							Condition: "tx.active",
							Children: []TemplateBlock{
								{Type: "variable", Variable: "tx.amount"},
							},
						},
					},
				},
				{Type: "aggregation", Collection: "transactions", Function: "sum", Field: "amount"},
			},
		},
	}

	result := ExtractMappedFields(blocks)
	require.NotNil(t, result)
	require.Contains(t, result, "default")
	require.Contains(t, result["default"], "transactions")
	assert.Contains(t, result["default"]["transactions"], "amount")
}

func TestExtractMappedFields_Deduplication(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{Type: "variable", Variable: "orders.total"},
		{Type: "variable", Variable: "orders.total"},
		{Type: "variable", Variable: "orders.count"},
	}

	result := ExtractMappedFields(blocks)
	require.NotNil(t, result)
	require.Contains(t, result, "default")
	require.Contains(t, result["default"], "orders")

	// Should be deduplicated
	totalCount := 0
	for _, f := range result["default"]["orders"] {
		if f == "total" {
			totalCount++
		}
	}

	assert.Equal(t, 1, totalCount, "total should appear only once after dedup")
}

func TestExtractMappedFields_EmptyBlocks(t *testing.T) {
	t.Parallel()

	result := ExtractMappedFields(nil)
	require.NotNil(t, result)
	assert.Empty(t, result)
}
