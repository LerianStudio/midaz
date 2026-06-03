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

func fetchDataSources(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string) []map[string]any {
	t.Helper()

	code, body, err := cli.Request(ctx, "GET", "/v1/data-sources", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("failed to get data sources: code=%d err=%v body=%s", code, err, string(body))
	}

	var dataSources []map[string]any
	if err := json.Unmarshal(body, &dataSources); err != nil {
		t.Fatalf("failed to unmarshal data sources: %v", err)
	}

	return dataSources
}

func extractIDs(dataSources []map[string]any) map[string]bool {
	ids := make(map[string]bool)

	for _, ds := range dataSources {
		if id, ok := ds["id"].(string); ok {
			ids[id] = true
		}
	}

	return ids
}

func assertNoNewIDs(t *testing.T, finalDataSources []map[string]any, initialIDs map[string]bool, forbiddenIDs []string) {
	t.Helper()

	forbiddenSet := make(map[string]bool, len(forbiddenIDs))
	for _, id := range forbiddenIDs {
		forbiddenSet[id] = true
	}

	for _, ds := range finalDataSources {
		id, ok := ds["id"].(string)
		if !ok {
			continue
		}

		if !initialIDs[id] {
			t.Errorf("data source map was corrupted: unexpected ID %q appeared after invalid request", id)
		}

		if forbiddenSet[id] {
			t.Errorf("data source map was corrupted with template ID fragment: %q", id)
		}
	}
}

// TestIntegration_DataSources_InvalidFilterKeysShouldNotCorruptMap tests that sending
func TestIntegration_DataSources_InvalidFilterKeysShouldNotCorruptMap(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Step 1: Get initial data sources to establish baseline
	initialDataSources := fetchDataSources(t, ctx, cli, headers)
	initialIDs := extractIDs(initialDataSources)

	// Step 2: Try to create a report with invalid datasource name in filters
	// Using a fake template ID as the datasource name (simulating the frontend bug)
	fakeTemplateID := "019abd3d-13c8-7692-8067-a9a9d42d9b41"
	invalidPayload := map[string]any{
		"templateId": "00000000-0000-0000-0000-000000000000", // Invalid template, will fail
		"filters": map[string]any{
			fakeTemplateID: map[string]any{ // This is the bug: template ID as datasource name
				"some_table": map[string]any{
					"some_field": map[string]any{
						"eq": []any{"value"},
					},
				},
			},
		},
	}

	// This request should fail with a validation error (missing datasource)
	code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, invalidPayload)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	// We expect a 4xx error because the datasource doesn't exist
	if code < 400 || code >= 500 {
		t.Logf("request returned code=%d body=%s", code, string(body))
	}

	// Step 3: Verify the data sources were NOT corrupted
	finalDataSources := fetchDataSources(t, ctx, cli, headers)
	assertNoNewIDs(t, finalDataSources, initialIDs, []string{fakeTemplateID, "abd3d-13c8-7", "abd3d-13c8-7692-"})

	// Verify count hasn't changed unexpectedly
	if len(finalDataSources) != len(initialDataSources) {
		t.Errorf("data source count changed: initial=%d, final=%d", len(initialDataSources), len(finalDataSources))
	}
}

// TestIntegration_DataSources_MultipleInvalidRequestsShouldNotAccumulate tests that
// multiple requests with invalid datasource names don't accumulate invalid entries
// in the ExternalDataSources map.
func TestIntegration_DataSources_MultipleInvalidRequestsShouldNotAccumulate(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Get initial state
	code, body, err := cli.Request(ctx, "GET", "/v1/data-sources", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("failed to get initial data sources: code=%d err=%v body=%s", code, err, string(body))
	}

	var initialDataSources []map[string]any
	_ = json.Unmarshal(body, &initialDataSources)
	initialCount := len(initialDataSources)

	// Send multiple requests with different invalid datasource names
	invalidDatasourceNames := []string{
		"fake-uuid-1234-5678-90ab-cdef12345678",
		"another-invalid-datasource",
		"019zzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		"not-a-real-database",
	}

	for _, invalidName := range invalidDatasourceNames {
		payload := map[string]any{
			"templateId": "00000000-0000-0000-0000-000000000000",
			"filters": map[string]any{
				invalidName: map[string]any{
					"table": map[string]any{
						"field": map[string]any{"eq": []any{"value"}},
					},
				},
			},
		}

		code, _, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil {
			t.Logf("request with invalid datasource %q failed: %v", invalidName, err)
		}
		// We expect these to fail, but they shouldn't corrupt the map
		_ = code
	}

	// Verify final state
	code, body, err = cli.Request(ctx, "GET", "/v1/data-sources", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("failed to get final data sources: code=%d err=%v body=%s", code, err, string(body))
	}

	var finalDataSources []map[string]any
	_ = json.Unmarshal(body, &finalDataSources)

	if len(finalDataSources) != initialCount {
		t.Errorf("data source count changed after multiple invalid requests: initial=%d, final=%d",
			initialCount, len(finalDataSources))

		// Log the extra entries for debugging
		initialIDs := make(map[string]bool)
		for _, ds := range initialDataSources {
			if id, ok := ds["id"].(string); ok {
				initialIDs[id] = true
			}
		}

		for _, ds := range finalDataSources {
			if id, ok := ds["id"].(string); ok {
				if !initialIDs[id] {
					t.Errorf("unexpected datasource appeared: %v", ds)
				}
			}
		}
	}
}
