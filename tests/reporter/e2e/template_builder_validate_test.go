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
// Valid Block Tests (TC-TBV-001 to TC-TBV-005)
// ############################################################################

func TestTemplateBuilder_ValidateTextBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "text", "content": "Hello, World!"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

func TestTemplateBuilder_ValidateVariableWithFilters(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":     "variable",
				"variable": "accounts.name",
				"filters": []map[string]any{
					{"name": "length"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

func TestTemplateBuilder_ValidateLoopWithChildren(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":       "loop",
				"iterator":   "item",
				"collection": "accounts",
				"children": []map[string]any{
					{"type": "variable", "variable": "item.name"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

func TestTemplateBuilder_ValidateConditionalWithElifElse(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":      "conditional",
				"condition": "item.active",
				"children": []map[string]any{
					{"type": "text", "content": "Active"},
				},
				"elifBranches": []map[string]any{
					{
						"condition": "item.pending",
						"children": []map[string]any{
							{"type": "text", "content": "Pending"},
						},
					},
				},
				"elseChildren": []map[string]any{
					{"type": "text", "content": "Inactive"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

func TestTemplateBuilder_ValidateNestedBlocks(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":       "loop",
				"iterator":   "item",
				"collection": "users",
				"children": []map[string]any{
					{
						"type":      "conditional",
						"condition": "item.active",
						"children": []map[string]any{
							{"type": "variable", "variable": "item.email"},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

// ############################################################################
// Invalid Block Tests (TC-TBV-006 to TC-TBV-016)
// ############################################################################

func TestTemplateBuilder_ValidateInvalidBlockType(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "nonexistent_block", "content": "test"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateTextWithoutContent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "text", "content": ""},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateVariableWithoutName(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "variable", "variable": ""},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateLoopWithoutChildren(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":       "loop",
				"iterator":   "item",
				"collection": "items",
				"children":   []map[string]any{},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateUnknownFilter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":     "variable",
				"variable": "accounts.name",
				"filters": []map[string]any{
					{"name": "nonexistent_filter"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateTemplateDelimitersInCondition(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Template delimiters are banned in condition fields (SSTI prevention).
	// Text blocks allow literal content including delimiters, but condition/assignment
	// fields must not contain {{ }}, {% %}, or {# #} sequences.
	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":      "conditional",
				"condition": "{{ malicious }}",
				"children": []map[string]any{
					{"type": "text", "content": "test"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateEmptyBlocks(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	// Empty blocks should return valid=false with an error
	valid, ok := body["valid"].(bool)
	require.True(t, ok)
	assert.False(t, valid, "empty blocks should not be valid")
}

func TestTemplateBuilder_ValidateCounterBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Valid counter
	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type": "counter",
				"properties": map[string]any{
					"counterMode":  "increment",
					"counterNames": []string{"myCounter"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

func TestTemplateBuilder_ValidateCounterInvalidMode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type": "counter",
				"properties": map[string]any{
					"counterMode":  "invalid_mode",
					"counterNames": []string{"myCounter"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}

func TestTemplateBuilder_ValidateCustomTagValid(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":    "custom_tag",
				"tagName": "sum_by",
				"tagArgs": "items amount category",
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, true, 0)
}

func TestTemplateBuilder_ValidateCustomTagInvalidName(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.ValidateBlocks(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":    "custom_tag",
				"tagName": "invalid_tag_name",
				"tagArgs": "items amount",
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertValidationResponse(t, body, false, 1)
}
