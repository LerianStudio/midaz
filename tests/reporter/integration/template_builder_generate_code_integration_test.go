//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/template_builder"
	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
)

const generateCodePath = "/v1/templates/generate-code"

// newGenerateCodeClient returns a configured HTTP client, auth headers, and a
// background context suitable for generate-code integration tests.
func newGenerateCodeClient(t *testing.T) (*h.HTTPClient, map[string]string, context.Context) {
	t.Helper()

	env := h.LoadEnvironment()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	return cli, headers, context.Background()
}

// TestIntegration_GenerateCode_TextBlock verifies that a single text block
// produces output containing the literal text content. (IS-GC-1)
func TestIntegration_GenerateCode_TextBlock(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: "Hello World"},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.Contains(t, resp.Code, "Hello World", "generated code should contain the text content")
}

// TestIntegration_GenerateCode_VariableWithFilters verifies that a variable
// block with a filter chain produces the expected Pongo2 expression. (IS-GC-2)
func TestIntegration_GenerateCode_VariableWithFilters(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type:     "variable",
				Variable: "user.name",
				Filters: []template_builder.FilterChain{
					{Name: "replace", Args: "old,new"},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.Contains(t, resp.Code, "{{ user.name|replace:\"old,new\" }}", "generated code should contain variable with filter chain")
}

// TestIntegration_GenerateCode_LoopWithChildren verifies that a loop block
// with children produces a for/endfor structure with indented children. (IS-GC-3)
func TestIntegration_GenerateCode_LoopWithChildren(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type:       "loop",
				Iterator:   "item",
				Collection: "items",
				Children: []template_builder.TemplateBlock{
					{Type: "text", Content: "row content"},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.Contains(t, resp.Code, "{% for item in items %}", "generated code should contain for-loop opening tag")
	assert.Contains(t, resp.Code, "{% endfor %}", "generated code should contain endfor closing tag")
	assert.Contains(t, resp.Code, "row content", "generated code should contain child content")
}

// TestIntegration_GenerateCode_CounterIncrement verifies that a counter block
// with mode=increment produces a {% counter "name" %} tag. (IS-GC-4)
func TestIntegration_GenerateCode_CounterIncrement(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "increment",
					"counterNames": []interface{}{"invoice_count"},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.Contains(t, resp.Code, `{% counter "invoice_count" %}`, "generated code should contain counter increment tag")
}

// TestIntegration_GenerateCode_CounterShow verifies that a counter block
// with mode=show produces a {% counter_show "n1" "n2" %} tag. (IS-GC-5)
func TestIntegration_GenerateCode_CounterShow(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "show",
					"counterNames": []interface{}{"total", "subtotal"},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.Contains(t, resp.Code, `{% counter_show "total" "subtotal" %}`, "generated code should contain counter_show tag with both names")
}

// TestIntegration_GenerateCode_Section verifies that a section block with
// children generates code for the children without a section wrapper. (IS-GC-6)
func TestIntegration_GenerateCode_Section(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type: "section",
				Children: []template_builder.TemplateBlock{
					{Type: "text", Content: "child A"},
					{Type: "text", Content: "child B"},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.Contains(t, resp.Code, "child A", "generated code should contain first child content")
	assert.Contains(t, resp.Code, "child B", "generated code should contain second child content")
	assert.NotContains(t, resp.Code, "{% section", "section type should not emit a Pongo wrapper tag in output")
}

// TestIntegration_GenerateCode_MappedFieldsExtraction verifies that a variable
// in "table.field" format populates the mappedFields response. (IS-GC-7)
func TestIntegration_GenerateCode_MappedFieldsExtraction(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type:     "variable",
				Variable: "accounts.balance",
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	require.NotNil(t, resp.MappedFields, "mappedFields must not be nil")

	defaultDS, ok := resp.MappedFields["default"]
	require.True(t, ok, "mappedFields should contain 'default' datasource")

	fields, ok := defaultDS["accounts"]
	require.True(t, ok, "mappedFields['default'] should contain 'accounts' table")
	assert.Contains(t, fields, "balance", "accounts table should include 'balance' field")
}

// TestIntegration_GenerateCode_HTMLFormat verifies that format="html" wraps the
// generated code in a DOCTYPE html structure. (IS-GC-8)
func TestIntegration_GenerateCode_HTMLFormat(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: "wrapped content"},
		},
		Format: "html",
	}

	code, body, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.GenerateCodeResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.True(t, strings.HasPrefix(resp.Code, "<!DOCTYPE html>"), "html format should start with DOCTYPE declaration")
	assert.Contains(t, resp.Code, "<html>", "html format should contain <html> tag")
	assert.Contains(t, resp.Code, "</html>", "html format should contain closing </html> tag")
	assert.Contains(t, resp.Code, "wrapped content", "html format should contain the block content")
}

// TestIntegration_GenerateCode_EmptyBlocks verifies that an empty blocks array
// returns HTTP 400. (IS-GC-9)
func TestIntegration_GenerateCode_EmptyBlocks(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{},
	}

	code, _, err := cli.Request(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	assert.Equal(t, http.StatusBadRequest, code, "empty blocks should return 400")
}

// TestIntegration_GenerateCode_InvalidJSON verifies that a malformed request
// body returns HTTP 400. (IS-GC-10)
func TestIntegration_GenerateCode_InvalidJSON(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	// Use raw net/http because the test helper marshals body automatically;
	// we need to send a deliberately broken JSON payload.
	rawBody := strings.NewReader(`{not valid json`)

	req, err := http.NewRequestWithContext(ctx, "POST", env.ManagerURL+generateCodePath, rawBody)
	require.NoError(t, err, "create request")

	req.Header.Set("Content-Type", "application/json")

	authHeaders := h.AuthHeaders()
	for k, v := range authHeaders {
		if k != "Content-Type" {
			req.Header.Set(k, v)
		}
	}

	client := &http.Client{Timeout: env.HTTPTimeout}
	resp, err := client.Do(req)
	require.NoError(t, err, "execute request")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid JSON should return 400")
}

// TestIntegration_GenerateCode_ContentTypeJSON verifies that a successful
// generate-code response has Content-Type application/json. (IS-GC-11)
func TestIntegration_GenerateCode_ContentTypeJSON(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newGenerateCodeClient(t)

	input := template_builder.GenerateCodeInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: "content type test"},
		},
	}

	code, _, respHeaders, err := cli.RequestFull(ctx, "POST", generateCodePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200")

	ct := respHeaders.Get("Content-Type")
	require.NotEmpty(t, ct, "Content-Type header must be present")

	assert.True(t,
		ct == "application/json" || ct == "application/json; charset=utf-8",
		"expected Content-Type application/json, got %q", ct,
	)
}
