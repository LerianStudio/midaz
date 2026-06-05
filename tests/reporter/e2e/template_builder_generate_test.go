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

	"github.com/LerianStudio/midaz/v4/tests/reporter/e2e/shared"
)

// ############################################################################
// Code Generation Tests (TC-TBG-001 to TC-TBG-011)
// ############################################################################

func TestTemplateBuilder_GenerateHTML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"format": "html",
		"blocks": []map[string]any{
			{"type": "text", "content": "Report Title"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertGeneratedCode(t, body, []string{"<!DOCTYPE html>", "Report Title"})
}

func TestTemplateBuilder_GenerateXML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"format": "xml",
		"blocks": []map[string]any{
			{"type": "text", "content": "<root>data</root>"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertGeneratedCode(t, body, []string{"<?xml version"})
}

func TestTemplateBuilder_GeneratePlainText(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "text", "content": "Plain text output"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	code, ok := body["code"].(string)
	require.True(t, ok)
	assert.Contains(t, code, "Plain text output")
	assert.NotContains(t, code, "<!DOCTYPE", "plain text should not have HTML wrapper")
	assert.NotContains(t, code, "<?xml", "plain text should not have XML wrapper")
}

func TestTemplateBuilder_GenerateLoopBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
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
	shared.AssertGeneratedCode(t, body, []string{"{% for item in accounts %}", "{% endfor %}"})
}

func TestTemplateBuilder_GenerateVariableWithFilters(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":     "variable",
				"variable": "amount",
				"filters": []map[string]any{
					{"name": "floatformat", "args": "2"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertGeneratedCode(t, body, []string{"{{ amount|floatformat:\"2\" }}"})
}

func TestTemplateBuilder_GenerateMappedFields(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "variable", "variable": "accounts.name"},
			{"type": "variable", "variable": "accounts.balance"},
			{"type": "variable", "variable": "users.email"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	mappedFields, ok := body["mappedFields"].(map[string]any)
	require.True(t, ok, "response should contain 'mappedFields' map")
	assert.NotEmpty(t, mappedFields, "mappedFields should not be empty")
}

func TestTemplateBuilder_GenerateConditionalWithElif(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":      "conditional",
				"condition": "status == \"active\"",
				"children": []map[string]any{
					{"type": "text", "content": "Active"},
				},
				"elifBranches": []map[string]any{
					{
						"condition": "status == \"pending\"",
						"children": []map[string]any{
							{"type": "text", "content": "Pending"},
						},
					},
				},
				"elseChildren": []map[string]any{
					{"type": "text", "content": "Other"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertGeneratedCode(t, body, []string{"{% if", "{% elif", "{% else %}", "{% endif %}"})
}

func TestTemplateBuilder_GenerateComment(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"blocks": []map[string]any{
			{"type": "comment", "content": "This is a comment"},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertGeneratedCode(t, body, []string{"{#", "#}"})
}

func TestTemplateBuilder_GenerateWithBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"blocks": []map[string]any{
			{
				"type":       "with",
				"variable":   "total",
				"assignment": "items|sum:\"amount\"",
				"children": []map[string]any{
					{"type": "variable", "variable": "total"},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertGeneratedCode(t, body, []string{"{% with", "{% endwith %}"})
}

func TestTemplateBuilder_GenerateEmptyBlocksError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.GenerateCodeRaw(ctx, map[string]any{
		"blocks": []map[string]any{},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "empty blocks should return 400")
}

func TestTemplateBuilder_GenerateAndCreateReport(t *testing.T) {
	// This test does a round-trip: generate code → create template → create report → download
	// Not parallel because it's resource-intensive
	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout+30*time.Second)
	defer cancel()

	// Step 1: Generate code
	genStatus, genBody, err := apiClient.GenerateCode(ctx, map[string]any{
		"format": "html",
		"blocks": []map[string]any{
			{"type": "text", "content": "<h1>Generated Report</h1>"},
			{
				"type":       "loop",
				"iterator":   "org",
				"collection": "midaz_onboarding.organization",
				"children": []map[string]any{
					{"type": "variable", "variable": "org.legal_name"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, genStatus)

	code, ok := genBody["code"].(string)
	require.True(t, ok, "should have generated code")
	require.NotEmpty(t, code)

	// Step 2: Create template from generated code
	desc := shared.UniqueID("tpl") + " Generated template"

	tplStatus, tplBody, err := apiClient.CreateTemplate(ctx, []byte(code), "generated.tpl", shared.FormatHTML, desc)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, tplStatus)

	tplID, ok := tplBody["id"].(string)
	require.True(t, ok)

	// Step 3: Create report from template
	rptStatus, rptBody, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, rptStatus)

	reportID, ok := rptBody["id"].(string)
	require.True(t, ok)

	// Step 4: Wait for report to complete
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Step 5: Download and verify
	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)
	shared.AssertContentType(t, headers, "text/html")
	shared.AssertHTMLContent(t, data, []string{"Generated Report"})
}

// ############################################################################
// Security Tests (TC-TBG-SEC-001)
// ############################################################################

func TestTemplateBuilder_GenerateCode_EscapesFilterInjection(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.GenerateCode(ctx, map[string]any{
		"format": "html",
		"blocks": []map[string]any{
			{
				"type":     "variable",
				"variable": "accounts.name",
				"filters": []map[string]any{
					{"name": "replace", "args": `x" |safe`},
				},
			},
		},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	code, ok := body["code"].(string)
	require.True(t, ok, "response should contain 'code' string field")

	// The injection attempt should be escaped — the payload must remain inside the quoted filter arg
	assert.Contains(t, code, `replace:"x\" |safe"`, "payload should remain inside the quoted filter arg")
	assert.NotContains(t, code, `replace:"x" |safe`, "must not allow quote breakout into a separate |safe filter")
}
