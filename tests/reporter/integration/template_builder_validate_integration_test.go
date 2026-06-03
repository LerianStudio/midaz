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

	"github.com/LerianStudio/midaz/v3/pkg/reporter/template_builder"
	h "github.com/LerianStudio/midaz/v3/tests/reporter/utils"
)

const validatePath = "/v1/templates/validate"

// newValidateClient returns a configured HTTP client, auth headers, and a
// background context suitable for validate integration tests.
func newValidateClient(t *testing.T) (*h.HTTPClient, map[string]string, context.Context) {
	t.Helper()

	env := h.LoadEnvironment()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	return cli, headers, context.Background()
}

// TestIntegration_Validate_ValidTextBlock verifies that a valid text block
// returns HTTP 200 with valid=true and no errors. (IS-V-1)
func TestIntegration_Validate_ValidTextBlock(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: "Hello World"},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.True(t, resp.Valid, "valid text block should return valid=true")
	assert.Empty(t, resp.Errors, "valid text block should return no errors")
}

// TestIntegration_Validate_InvalidTextBlock verifies that a text block without
// content returns HTTP 200 with valid=false and an error referencing the
// content field. (IS-V-2)
func TestIntegration_Validate_InvalidTextBlock(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: ""},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.False(t, resp.Valid, "text block without content should return valid=false")
	require.NotEmpty(t, resp.Errors, "text block without content should return errors")
	assert.Equal(t, "content", resp.Errors[0].Field, "error should reference the content field")
}

// TestIntegration_Validate_CounterValidation verifies that a counter block
// with an invalid counterMode returns HTTP 200 with valid=false and an error
// referencing the counterMode field. (IS-V-3)
func TestIntegration_Validate_CounterValidation(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "invalid_mode",
					"counterNames": []interface{}{"test_counter"},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.False(t, resp.Valid, "counter with invalid mode should return valid=false")
	require.NotEmpty(t, resp.Errors, "counter with invalid mode should return errors")

	foundCounterModeError := false
	for _, e := range resp.Errors {
		if e.Field == "counterMode" {
			foundCounterModeError = true

			break
		}
	}

	assert.True(t, foundCounterModeError, "errors should include a counterMode field error")
}

// TestIntegration_Validate_LoopWithoutChildren verifies that a loop block
// without children returns HTTP 200 with valid=false and an error referencing
// the children field. (IS-V-4)
func TestIntegration_Validate_LoopWithoutChildren(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type:       "loop",
				Iterator:   "item",
				Collection: "items",
				Children:   []template_builder.TemplateBlock{},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.False(t, resp.Valid, "loop without children should return valid=false")
	require.NotEmpty(t, resp.Errors, "loop without children should return errors")

	foundChildrenError := false
	for _, e := range resp.Errors {
		if e.Field == "children" {
			foundChildrenError = true

			break
		}
	}

	assert.True(t, foundChildrenError, "errors should include a children field error")
}

// TestIntegration_Validate_RecursiveValidation verifies that a loop with an
// invalid child block returns errors for the child block, proving that
// validation recurses into children. (IS-V-5)
func TestIntegration_Validate_RecursiveValidation(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{
				Type:       "loop",
				Iterator:   "item",
				Collection: "items",
				Children: []template_builder.TemplateBlock{
					{Type: "text", Content: ""},
				},
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.False(t, resp.Valid, "loop with invalid child should return valid=false")
	require.NotEmpty(t, resp.Errors, "loop with invalid child should return errors")

	foundChildError := false
	for _, e := range resp.Errors {
		if e.Field == "content" {
			foundChildError = true

			break
		}
	}

	assert.True(t, foundChildError, "errors should include the child text block content error")
}

// TestIntegration_Validate_MultipleErrors verifies that posting multiple
// invalid blocks returns multiple validation errors, one per invalid block.
// (IS-V-6)
func TestIntegration_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: ""},
			{Type: "variable", Variable: ""},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.False(t, resp.Valid, "multiple invalid blocks should return valid=false")
	assert.GreaterOrEqual(t, len(resp.Errors), 2, "should return at least 2 errors for 2 invalid blocks")

	fields := make(map[string]bool)
	for _, e := range resp.Errors {
		fields[e.Field] = true
	}

	assert.True(t, fields["content"], "errors should include a content field error for text block")
	assert.True(t, fields["variable"], "errors should include a variable field error for variable block")
}

// TestIntegration_Validate_EmptyBlocks verifies that posting an empty blocks
// array returns HTTP 200 with a structured validation failure. (IS-V-7)
func TestIntegration_Validate_EmptyBlocks(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	assert.Equal(t, http.StatusOK, code, "empty blocks should return 200 with validation errors")

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Valid)
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "blocks", resp.Errors[0].Field)
	assert.Equal(t, "blocks must not be empty", resp.Errors[0].Message)
}

// TestIntegration_Validate_InvalidJSON verifies that a malformed request body
// returns HTTP 400. (IS-V-8)
func TestIntegration_Validate_InvalidJSON(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	// Use raw net/http because the test helper marshals body automatically;
	// we need to send a deliberately broken JSON payload.
	rawBody := strings.NewReader(`{not valid json`)

	req, err := http.NewRequestWithContext(ctx, "POST", env.ManagerURL+validatePath, rawBody)
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

// TestIntegration_Validate_ContentTypeJSON verifies that a successful validate
// response has Content-Type application/json. (IS-V-9)
func TestIntegration_Validate_ContentTypeJSON(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{Type: "text", Content: "content type test"},
		},
	}

	code, _, respHeaders, err := cli.RequestFull(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200")

	ct := respHeaders.Get("Content-Type")
	require.NotEmpty(t, ct, "Content-Type header must be present")

	assert.True(t,
		ct == "application/json" || ct == "application/json; charset=utf-8",
		"expected Content-Type application/json, got %q", ct,
	)
}

// TestIntegration_Validate_BlockIDPreserved verifies that when a block
// includes a blockId, validation errors reference that same blockId rather
// than a generated one. (IS-V-10)
func TestIntegration_Validate_BlockIDPreserved(t *testing.T) {
	t.Parallel()

	cli, headers, ctx := newValidateClient(t)

	const customBlockID = "my-custom-block-42"

	input := template_builder.ValidateBlocksInput{
		Blocks: []template_builder.TemplateBlock{
			{
				BlockID: customBlockID,
				Type:    "text",
				Content: "",
			},
		},
	}

	code, body, err := cli.Request(ctx, "POST", validatePath, headers, input)
	require.NoError(t, err, "request error")
	require.Equal(t, http.StatusOK, code, "expected status 200, got %d body=%s", code, string(body))

	var resp template_builder.ValidateBlocksResponse
	require.NoError(t, json.Unmarshal(body, &resp), "unmarshal response")

	assert.False(t, resp.Valid, "invalid block should return valid=false")
	require.NotEmpty(t, resp.Errors, "invalid block should return errors")
	assert.Equal(t, customBlockID, resp.Errors[0].BlockID, "error blockId should match the custom blockId provided in the request")
}
