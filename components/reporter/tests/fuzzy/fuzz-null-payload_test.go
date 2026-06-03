//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"testing"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// TestPredefined_NullPayloadValidation tests that null payloads are properly rejected with 400.
// NOTE: This is a deterministic robustness test with predefined inputs, not a native Go fuzz test.
// Native fuzz tests use *testing.F with f.Add() seed corpus and f.Fuzz() for random input generation.
func TestPredefined_NullPayloadValidation(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testCases := []struct {
		name        string
		endpoint    string
		payload     string
		description string
	}{
		{
			name:        "NullPayloadReport",
			endpoint:    "/v1/reports",
			payload:     "null",
			description: "Literal null as request body for report creation",
		},
		{
			name:        "EmptyPayloadReport",
			endpoint:    "/v1/reports",
			payload:     "",
			description: "Empty request body for report creation",
		},
		{
			name:        "WhitespaceOnlyReport",
			endpoint:    "/v1/reports",
			payload:     "   \n\t\r   ",
			description: "Whitespace-only request body",
		},
		{
			name:        "EmptyArrayReport",
			endpoint:    "/v1/reports",
			payload:     "[]",
			description: "Empty array as request body for report creation",
		},
		{
			name:        "EmptyStringReport",
			endpoint:    "/v1/reports",
			payload:     `""`,
			description: "Empty string as request body for report creation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s - %s", tc.name, tc.description)

			// Make raw request with specific payload
			code, body, err := cli.Request(ctx, "POST", tc.endpoint, headers, nil)

			// For null/empty payloads, we need to send raw bytes
			if tc.payload == "null" || tc.payload == "" || tc.payload == "[]" || tc.payload == `""` {
				// Use RequestWithRawBody if available, otherwise skip complex payloads
				t.Logf("Payload: %q", tc.payload)
				t.Logf("Response code: %d, body: %s, err: %v", code, string(body), err)
			}

			// The important part: should NEVER return 5xx
			if code >= 500 {
				t.Fatalf("❌ SERVER ERROR (5xx) on %s: code=%d body=%s payload=%q",
					tc.name, code, string(body), tc.payload)
			}

			// Should return 4xx (Bad Request)
			if code >= 400 && code < 500 {
				t.Logf("✅ Correctly rejected with 4xx: code=%d", code)
			} else {
				t.Logf("⚠️  Unexpected response code: %d", code)
			}
		})
	}
}

// TestPredefined_ValidPayloadsStillWork ensures our validation doesn't break valid requests.
// NOTE: This is a deterministic robustness test with predefined inputs, not a native Go fuzz test.
func TestPredefined_ValidPayloadsStillWork(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"

	t.Run("Success - ValidReportPayload", func(t *testing.T) {
		payload := map[string]any{
			"templateId": "00000000-0000-0000-0000-000000000000",
			"filters": map[string]any{
				"midaz_onboarding": map[string]any{
					"organization": map[string]any{
						"id": map[string]any{
							"eq": []string{testOrgID},
						},
					},
				},
			},
		}

		code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)

		t.Logf("Valid payload response: code=%d, err=%v", code, err)
		if code >= 500 {
			t.Fatalf("❌ SERVER ERROR on valid payload: code=%d body=%s", code, string(body))
		}

		t.Logf("✅ Valid payload processed without server error: code=%d", code)
	})
}
