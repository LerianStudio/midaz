//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
)

// TestIntegration_Deadline_CreatePayload_InvalidSeeds verifies that known-invalid payloads
// are rejected with 4xx status codes. This complements the fuzz test which only asserts no 5xx.
func TestIntegration_Deadline_CreatePayload_InvalidSeeds(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services.
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	cases := []struct {
		name    string
		payload string
	}{
		{"empty object", `{}`},
		{"missing type and dueDate", `{"name":"Test"}`},
		{"missing name and dueDate", `{"type":"regulatory"}`},
		{"missing name and type", `{"frequency":"monthly"}`},
		{"invalid type", `{"name":"Test","type":"invalid","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733"}`},
		{"invalid frequency", `{"name":"Test","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"invalid","color":"#FF5733"}`},
		{"null values", `{"name":null,"type":null,"dueDate":null,"frequency":null,"color":null}`},
		{"invalid date format", `{"name":"Test","type":"regulatory","dueDate":"not-a-date","frequency":"monthly","color":"#FF5733"}`},
		{"empty date", `{"name":"Test","type":"regulatory","dueDate":"","frequency":"monthly","color":"#FF5733"}`},
		{"numeric types where strings expected", `{"name":12345,"type":true,"dueDate":0,"frequency":[],"color":{}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload any
			if err := json.Unmarshal([]byte(tc.payload), &payload); err != nil {
				t.Fatalf("invalid test case JSON: %v", err)
			}

			code, body, err := cli.Request(ctx, "POST", "/v1/deadlines", headers, payload)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}

			if code < 400 || code >= 500 {
				t.Fatalf("expected 4xx for invalid payload, got %d body=%s payload=%s",
					code, string(body), tc.payload)
			}

			t.Logf("correctly rejected with %d: %s", code, fmt.Sprintf("%.100s", string(body)))
		})
	}
}

func TestIntegration_Deadline_UpdatePayload_InvalidSeeds(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()
	created := createDeadlineViaAPI(t, cli, headers, createDeadlinePayload("IS-InvalidUpdate-"+h.RandString(8)))
	path := fmt.Sprintf("/v1/deadlines/%s", created.ID)

	cases := []struct {
		name    string
		payload string
	}{
		{"invalid type", `{"type":"invalid_type"}`},
		{"invalid frequency", `{"frequency":"invalid_freq"}`},
		{"null fields", `{"name":null,"type":null}`},
		{"wrong types", `{"name":123,"active":"not-bool","notifyDaysBefore":"not-int"}`},
		{"nested object", `{"name":{"nested":{"deep":"value"}}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload any
			if err := json.Unmarshal([]byte(tc.payload), &payload); err != nil {
				t.Fatalf("invalid test case JSON: %v", err)
			}

			code, body, err := cli.Request(ctx, "PATCH", path, headers, payload)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}

			if code < 400 || code >= 500 {
				t.Fatalf("expected 4xx for invalid update payload, got %d body=%s payload=%s",
					code, string(body), tc.payload)
			}
		})
	}
}

func TestIntegration_Deadline_DeliverPayload_InvalidSeeds(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()
	path := "/v1/deadlines/00000000-0000-0000-0000-000000000000/deliver"

	cases := []struct {
		name    string
		payload string
	}{
		{"empty object", `{}`},
		{"null delivered", `{"delivered":null}`},
		{"string delivered", `{"delivered":"yes"}`},
		{"numeric delivered", `{"delivered":1}`},
		{"array payload", `[]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload any
			if err := json.Unmarshal([]byte(tc.payload), &payload); err != nil {
				t.Fatalf("invalid test case JSON: %v", err)
			}

			code, body, err := cli.Request(ctx, "PATCH", path, headers, payload)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}

			if code < 400 || code >= 500 {
				t.Fatalf("expected 4xx for invalid deliver payload, got %d body=%s payload=%s",
					code, string(body), tc.payload)
			}
		})
	}
}
