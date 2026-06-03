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

// GET /v1/reports — filters (status, templateId, createdAt)
func TestIntegration_Reports_ListWithFilters(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	q := "/v1/reports?status=Finished&limit=2&page=1"
	code, body, err := cli.Request(ctx, "GET", q, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list reports code=%d err=%v body=%s", code, err, string(body))
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(body, &page)
}

// POST /v1/reports - create report
func TestIntegration_Reports_Create_MinimalValidation(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	payload := map[string]any{
		"templateId": "00000000-0000-0000-0000-000000000000",
		"filters":    map[string]any{"status": map[string]any{"in": []any{"active"}}},
	}
	code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if code != 201 && (code < 400 || code >= 500) {
		t.Fatalf("expected 201 or 4xx, got %d body=%s", code, string(body))
	}
}
