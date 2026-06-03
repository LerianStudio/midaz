// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBlockDefinitions(t *testing.T) {
	t.Parallel()

	expectedBlockTypes := []string{
		"text",
		"variable",
		"loop",
		"conditional",
		"aggregation",
		"calculation",
		"date_time",
		"counter",
		"comment",
		"section",
		"with",
		"expression",
		"custom_tag",
	}

	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Success - returns exactly 13 block types",
			test: func(t *testing.T) {
				t.Helper()

				blocks := GetBlockDefinitions()
				require.Len(t, blocks, 13, "must return exactly 13 block types")
			},
		},
		{
			name: "Success - contains all expected block types",
			test: func(t *testing.T) {
				t.Helper()

				blocks := GetBlockDefinitions()
				blockTypes := make([]string, len(blocks))

				for i, b := range blocks {
					blockTypes[i] = b.Type
				}

				for _, expected := range expectedBlockTypes {
					assert.Contains(t, blockTypes, expected, "must contain block type: %s", expected)
				}
			},
		},
		{
			name: "Success - counter block has counterMode enum property",
			test: func(t *testing.T) {
				t.Helper()

				blocks := GetBlockDefinitions()
				var counterBlock *BlockDefinition

				for i := range blocks {
					if blocks[i].Type == "counter" {
						counterBlock = &blocks[i]

						break
					}
				}

				require.NotNil(t, counterBlock, "counter block must exist")

				var counterModeProp *BlockProperty

				for i := range counterBlock.Properties {
					if counterBlock.Properties[i].Name == "counterMode" {
						counterModeProp = &counterBlock.Properties[i]

						break
					}
				}

				require.NotNil(t, counterModeProp, "counter block must have counterMode property")
				assert.Equal(t, "enum", counterModeProp.Type)
				assert.True(t, counterModeProp.Required)
				assert.Equal(t, []string{"increment", "show"}, counterModeProp.Values)
			},
		},
		{
			name: "Success - counter block has counterNames string array property",
			test: func(t *testing.T) {
				t.Helper()

				blocks := GetBlockDefinitions()
				var counterBlock *BlockDefinition

				for i := range blocks {
					if blocks[i].Type == "counter" {
						counterBlock = &blocks[i]

						break
					}
				}

				require.NotNil(t, counterBlock, "counter block must exist")

				var counterNamesProp *BlockProperty

				for i := range counterBlock.Properties {
					if counterBlock.Properties[i].Name == "counterNames" {
						counterNamesProp = &counterBlock.Properties[i]

						break
					}
				}

				require.NotNil(t, counterNamesProp, "counter block must have counterNames property")
				assert.Equal(t, "string[]", counterNamesProp.Type)
				assert.True(t, counterNamesProp.Required)
			},
		},
		{
			name: "Success - section block has acceptsChildren true",
			test: func(t *testing.T) {
				t.Helper()

				blocks := GetBlockDefinitions()
				var sectionBlock *BlockDefinition

				for i := range blocks {
					if blocks[i].Type == "section" {
						sectionBlock = &blocks[i]

						break
					}
				}

				require.NotNil(t, sectionBlock, "section block must exist")
				assert.True(t, sectionBlock.AcceptsChildren, "section block must accept children")
			},
		},
		{
			name: "Success - every block has non-empty type and label",
			test: func(t *testing.T) {
				t.Helper()

				blocks := GetBlockDefinitions()

				for _, block := range blocks {
					assert.NotEmpty(t, block.Type, "block type must not be empty")
					assert.NotEmpty(t, block.Label, "block label must not be empty for type: %s", block.Type)
					assert.NotEmpty(t, block.Category, "block category must not be empty for type: %s", block.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.test(t)
		})
	}
}

func TestGetFilterDefinitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Success - returns filters including DIMP filters",
			test: func(t *testing.T) {
				t.Helper()

				filters := GetFilterDefinitions()
				require.NotEmpty(t, filters, "must return at least one filter")

				filterNames := make([]string, len(filters))

				for i, f := range filters {
					filterNames[i] = f.Name
				}

				dimpFilters := []string{"replace", "where", "sum", "count"}

				for _, df := range dimpFilters {
					assert.Contains(t, filterNames, df, "must contain DIMP filter: %s", df)
				}
			},
		},
		{
			name: "Success - every filter has name description args and example",
			test: func(t *testing.T) {
				t.Helper()

				filters := GetFilterDefinitions()

				for _, filter := range filters {
					assert.NotEmpty(t, filter.Name, "filter name must not be empty")
					assert.NotEmpty(t, filter.Description, "filter description must not be empty for: %s", filter.Name)
					assert.NotNil(t, filter.Args, "filter args must not be nil for: %s", filter.Name)
					assert.NotEmpty(t, filter.Example, "filter example must not be empty for: %s", filter.Name)
				}
			},
		},
		{
			name: "Success - replace filter has correct args",
			test: func(t *testing.T) {
				t.Helper()

				filters := GetFilterDefinitions()
				var replaceFilter *FilterDefinition

				for i := range filters {
					if filters[i].Name == "replace" {
						replaceFilter = &filters[i]

						break
					}
				}

				require.NotNil(t, replaceFilter, "replace filter must exist")
				assert.Equal(t, []string{"search:replacement"}, replaceFilter.Args)
			},
		},
		{
			name: "Success - where filter has correct args",
			test: func(t *testing.T) {
				t.Helper()

				filters := GetFilterDefinitions()
				var whereFilter *FilterDefinition

				for i := range filters {
					if filters[i].Name == "where" {
						whereFilter = &filters[i]

						break
					}
				}

				require.NotNil(t, whereFilter, "where filter must exist")
				assert.Equal(t, []string{"field:value"}, whereFilter.Args)
			},
		},
		{
			name: "Success - sum filter has field arg",
			test: func(t *testing.T) {
				t.Helper()

				filters := GetFilterDefinitions()
				var sumFilter *FilterDefinition

				for i := range filters {
					if filters[i].Name == "sum" {
						sumFilter = &filters[i]

						break
					}
				}

				require.NotNil(t, sumFilter, "sum filter must exist")
				assert.Equal(t, []string{"field"}, sumFilter.Args)
			},
		},
		{
			name: "Success - count filter has field value arg",
			test: func(t *testing.T) {
				t.Helper()

				filters := GetFilterDefinitions()
				var countFilter *FilterDefinition

				for i := range filters {
					if filters[i].Name == "count" {
						countFilter = &filters[i]

						break
					}
				}

				require.NotNil(t, countFilter, "count filter must exist")
				assert.Equal(t, []string{"field:value"}, countFilter.Args)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.test(t)
		})
	}
}
