// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagination_SetItems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		items    any
		expected any
	}{
		{
			name:     "Set string slice",
			items:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "Set integer slice",
			items:    []int{1, 2, 3},
			expected: []int{1, 2, 3},
		},
		{
			name:     "Set empty slice",
			items:    []string{},
			expected: []string{},
		},
		{
			name:     "Set nil",
			items:    nil,
			expected: nil,
		},
		{
			name: "Set struct slice",
			items: []struct {
				ID   int
				Name string
			}{{1, "test"}, {2, "test2"}},
			expected: []struct {
				ID   int
				Name string
			}{{1, "test"}, {2, "test2"}},
		},
		{
			name:     "Set map slice",
			items:    []map[string]string{{"key": "value"}},
			expected: []map[string]string{{"key": "value"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Pagination{}
			p.SetItems(tt.items)
			assert.Equal(t, tt.expected, p.Items)
		})
	}
}

func TestPagination_SetTotal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    int
		expected int
	}{
		{
			name:     "Set positive total",
			total:    100,
			expected: 100,
		},
		{
			name:     "Set zero total",
			total:    0,
			expected: 0,
		},
		{
			name:     "Set large total",
			total:    1000000,
			expected: 1000000,
		},
		{
			name:     "Set negative total",
			total:    -1,
			expected: -1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Pagination{}
			p.SetTotal(tt.total)
			assert.Equal(t, tt.expected, p.Total)
		})
	}
}

func TestPagination_JSONMarshal(t *testing.T) {
	t.Parallel()
	p := Pagination{
		Items: []string{"item1", "item2"},
		Page:  1,
		Limit: 10,
		Total: 2,
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, float64(1), result["page"])
	assert.Equal(t, float64(10), result["limit"])
	assert.Equal(t, float64(2), result["total"])
	assert.NotNil(t, result["items"])
}

func TestPagination_JSONMarshal_OmitEmptyPage(t *testing.T) {
	t.Parallel()
	p := Pagination{
		Items: []string{"item1"},
		Page:  0, // Should be omitted due to omitempty
		Limit: 10,
		Total: 1,
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	// Page should be omitted when 0 due to omitempty tag
	_, hasPage := result["page"]
	assert.False(t, hasPage)
}

func TestPagination_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	jsonData := `{"items": ["a", "b"], "page": 2, "limit": 20, "total": 100}`

	var p Pagination
	err := json.Unmarshal([]byte(jsonData), &p)
	require.NoError(t, err)

	assert.Equal(t, 2, p.Page)
	assert.Equal(t, 20, p.Limit)
	assert.Equal(t, 100, p.Total)
	assert.NotNil(t, p.Items)
}

func TestPagination_Combined(t *testing.T) {
	t.Parallel()
	p := &Pagination{
		Limit: 10,
		Page:  1,
	}

	items := []map[string]string{
		{"id": "1", "name": "Item 1"},
		{"id": "2", "name": "Item 2"},
	}

	p.SetItems(items)
	p.SetTotal(50)

	assert.Equal(t, items, p.Items)
	assert.Equal(t, 50, p.Total)
	assert.Equal(t, 10, p.Limit)
	assert.Equal(t, 1, p.Page)
}
