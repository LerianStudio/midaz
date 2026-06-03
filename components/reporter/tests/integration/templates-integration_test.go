//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"testing"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// GET /v1/templates — filters and pagination
func TestIntegration_Templates_ListWithFiltersAndPagination(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	path1 := "/v1/templates?limit=1&page=1&outputFormat=HTML"
	code, body, err := cli.Request(ctx, "GET", path1, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list page1 code=%d err=%v body=%s", code, err, string(body))
	}
	var page1 struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &page1); err != nil {
		t.Fatalf("failed to unmarshal page1: %v", err)
	}

	path2 := "/v1/templates?limit=1&page=2&outputFormat=HTML"
	code, body, err = cli.Request(ctx, "GET", path2, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list page2 code=%d err=%v body=%s", code, err, string(body))
	}
	var page2 struct {
		Items []map[string]any `json:"items"`
	}
	err = json.Unmarshal(body, &page2)
	if err != nil {
		t.Fatalf("failed to unmarshal page2: %v", err)
	}

	seen := map[string]bool{}
	for _, it := range page1.Items {
		if id, ok := it["id"].(string); ok {
			seen[id] = true
		}
	}
	for _, it := range page2.Items {
		if id, ok := it["id"].(string); ok {
			if seen[id] {
				t.Fatalf("duplicate across pages: %s", id)
			}
		}
	}
}

// POST /v1/templates — create template with invalid payload
func TestIntegration_Templates_Create_BadRequest(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	payload := map[string]any{"description": "x", "outputFormat": "HTML", "templateFile": "not-binary"}
	code, body, err := cli.Request(ctx, "POST", "/v1/templates", headers, payload)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if code < 400 || code >= 500 {
		t.Fatalf("expected 4xx, got %d body=%s", code, string(body))
	}
}
