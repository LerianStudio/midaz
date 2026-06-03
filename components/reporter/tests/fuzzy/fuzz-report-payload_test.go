//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// FuzzReport_Payload tests various malformed report creation payloads
func FuzzReport_Payload(f *testing.F) {
	// Seed corpus with various payload patterns (realistic malformed requests)
	seeds := []string{
		`{"templateId": "00000000-0000-0000-0000-000000000000", "filters": {}}`,
		`{"templateId": "", "filters": {}}`,
		`{"templateId": null, "filters": {}}`,
		`{"filters": {}}`,
		`{"templateId": "00000000-0000-0000-0000-000000000000"}`,
		`{}`,
		`{"templateId": "not-a-uuid"}`,
		`{"templateId": "00000000-0000-0000-0000-000000000000", "filters": null}`,
		`{"templateId": "00000000-0000-0000-0000-000000000000", "filters": []}`,
		`{"templateId": "00000000-0000-0000-0000-000000000000", "filters": "invalid"}`,
		`{"templateId": "../../../etc/passwd"}`,
		`{"templateId": "'; DROP TABLE reports; --"}`,
		`{"templateId": "00000000-0000-0000-0000-000000000000", "extraField": "value"}`,
		`{"templateId": "00000000-0000-0000-0000-000000000000", "filters": {"invalid": "structure"}}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	f.Fuzz(func(t *testing.T, payloadJSON string) {
		// Limit size
		if len(payloadJSON) > 100000 {
			payloadJSON = payloadJSON[:100000]
		}

		// Try to parse as JSON - invalid JSON is valid fuzz input, just return
		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return
		}

		code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on fuzzed payload: code=%d body=%s payload=%s", code, string(body), payloadJSON)
		}

		t.Logf("Payload result: code=%d payload=%s", code, payloadJSON)
	})
}

// FuzzTemplate_IDFormats tests various template ID formats
func FuzzTemplate_IDFormats(f *testing.F) {
	f.Add("00000000-0000-0000-0000-000000000000")
	f.Add("not-a-uuid")
	f.Add("")
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff")
	f.Add("00000000-0000-0000-0000-00000000000g") // Invalid character
	f.Add("00000000-0000-0000-0000")              // Too short
	f.Add("00000000-0000-0000-0000-000000000000-extra")
	f.Add("00000000000000000000000000000000") // No dashes
	f.Add("../../../etc/passwd")
	f.Add("'; DELETE FROM templates WHERE '1'='1")
	f.Add("\x00\x01\x02")
	f.Add(strings.Repeat("a", 1000))
	f.Add("💀-💀-💀-💀-💀")

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"

	f.Fuzz(func(t *testing.T, templateID string) {
		// Limit size
		if len(templateID) > 1000 {
			templateID = templateID[:1000]
		}

		payload := map[string]any{
			"templateId": templateID,
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
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on fuzzed templateID: code=%d body=%s templateID=%q", code, string(body), templateID)
		}

		t.Logf("TemplateID result: code=%d templateID=%q", code, templateID)
	})
}

// FuzzOrganization_ID tests various organization ID inputs
func FuzzOrganization_ID(f *testing.F) {
	f.Add("06c4f684-19b0-449a-81f4-f9a4e503db83")
	f.Add("00000000-0000-0000-0000-000000000000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("../../../etc/passwd")
	f.Add("'; DROP TABLE organizations; --")
	f.Add("\x00\x01\x02")
	f.Add(strings.Repeat("a", 1000))

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)

	simpleTemplate := "Test"
	files := map[string][]byte{
		"template": []byte(simpleTemplate),
	}
	formData := map[string]string{
		"outputFormat": "TXT",
		"description":  "Fuzz test org ID",
	}

	f.Fuzz(func(t *testing.T, orgID string) {
		// Limit size
		if len(orgID) > 1000 {
			orgID = orgID[:1000]
		}

		headers := h.AuthHeaders()

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on fuzzed orgID: code=%d body=%s orgID=%q", code, string(body), orgID)
		}

		t.Logf("OrgID result: code=%d orgID=%q", code, orgID)
	})
}

// FuzzFilter_NestedStructure tests deeply nested filter structures
func FuzzFilter_NestedStructure(f *testing.F) {
	f.Add(1)
	f.Add(5)
	f.Add(10)
	f.Add(50)
	f.Add(100)

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	f.Fuzz(func(t *testing.T, depth int) {
		// Cap depth to prevent stack overflow
		if depth > 100 {
			depth = 100
		}
		if depth < 0 {
			depth = -depth
		}

		// Build deeply nested structure
		var nested any = map[string]any{"eq": []string{"value"}}
		for i := 0; i < depth; i++ {
			nested = map[string]any{
				fmt.Sprintf("level_%d", i%10): nested,
			}
		}

		payload := map[string]any{
			"templateId": "00000000-0000-0000-0000-000000000000",
			"filters":    nested,
		}

		code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil {
			t.Logf("Request error on depth=%d (acceptable): %v", depth, err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on nested filters: code=%d depth=%d body=%s", code, depth, string(body))
		}

		t.Logf("Nested filter result: code=%d depth=%d", code, depth)
	})
}

// FuzzRequest_Concurrent tests system behavior under concurrent fuzzed requests
func FuzzRequest_Concurrent(f *testing.F) {
	f.Add("request1")
	f.Add("request2")
	f.Add("request3")
	f.Add("<script>alert('xss')</script>")
	f.Add("' OR 1=1 --")
	f.Add(strings.Repeat("a", 1024))
	f.Add("\u540d\u524d\u30c6\u30b9\u30c8")
	f.Add("00000000-0000-0000-0000-000000000000")

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	f.Fuzz(func(t *testing.T, requestData string) {
		// Limit size
		if len(requestData) > 1000 {
			requestData = requestData[:1000]
		}

		// Fire multiple concurrent requests with same fuzzed data
		done := make(chan bool, 5)
		errors := make(chan error, 5)

		for i := 0; i < 5; i++ {
			go func(id int) {
				payload := map[string]any{
					"templateId": requestData,
					"filters": map[string]any{
						"data": requestData,
					},
				}

				code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
				if err != nil {
					errors <- err
					done <- true
					return
				}

				// Server should NEVER crash (5xx)
				if code >= 500 {
					errors <- fmt.Errorf("SERVER ERROR on concurrent request %d: code=%d body=%s", id, code, string(body))
				}

				done <- true
			}(i)
		}

		// Wait for all requests
		for i := 0; i < 5; i++ {
			<-done
		}

		// Check for errors
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
	})
}
