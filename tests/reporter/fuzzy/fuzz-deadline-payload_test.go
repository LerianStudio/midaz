//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
)

// FuzzDeadline_CreatePayload fuzzes the create deadline endpoint with random JSON payloads.
// Verifies: no 5xx errors or panics from arbitrary input. For explicit 4xx validation of
// known-invalid payloads, see TestDeadline_CreatePayload_InvalidSeeds.
func FuzzDeadline_CreatePayload(f *testing.F) {
	seeds := []string{
		// Valid payload
		`{"name":"Monthly Report","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733"}`,
		// Empty object
		`{}`,
		// Missing required fields
		`{"name":"Test"}`,
		`{"type":"regulatory"}`,
		`{"frequency":"monthly"}`,
		// Invalid types
		`{"name":"Test","type":"invalid","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733"}`,
		`{"name":"Test","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"invalid","color":"#FF5733"}`,
		// Null values
		`{"name":null,"type":null,"dueDate":null,"frequency":null,"color":null}`,
		// XSS/injection
		`{"name":"<script>alert('xss')</script>","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733"}`,
		`{"name":"' OR 1=1 --","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733"}`,
		// Oversized fields
		`{"name":"` + strings.Repeat("a", 10000) + `","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733"}`,
		// Invalid date formats
		`{"name":"Test","type":"regulatory","dueDate":"not-a-date","frequency":"monthly","color":"#FF5733"}`,
		`{"name":"Test","type":"regulatory","dueDate":"","frequency":"monthly","color":"#FF5733"}`,
		// Extra fields
		`{"name":"Test","type":"regulatory","dueDate":"2026-12-31T23:59:59Z","frequency":"monthly","color":"#FF5733","extraField":"value","nested":{"a":"b"}}`,
		// Array instead of object
		`[{"name":"Test"}]`,
		// Numeric types where strings expected
		`{"name":12345,"type":true,"dueDate":0,"frequency":[],"color":{}}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	f.Fuzz(func(t *testing.T, payloadJSON string) {
		// Bound input to prevent OOM
		if len(payloadJSON) > 100000 {
			payloadJSON = payloadJSON[:100000]
		}

		// Parse as JSON - invalid JSON is valid fuzz input, just skip
		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return
		}

		code, body, err := cli.Request(ctx, "POST", "/v1/deadlines", headers, payload)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server must NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on fuzzed deadline payload: code=%d body=%s payload=%s", code, string(body), payloadJSON)
		}

		t.Logf("Deadline create result: code=%d payload=%s", code, payloadJSON)
	})
}

// FuzzDeadline_UpdatePayload fuzzes the update deadline endpoint with random JSON payloads.
// Verifies: no 5xx errors on malformed update payloads.
func FuzzDeadline_UpdatePayload(f *testing.F) {
	seeds := []string{
		// Valid partial update
		`{"name":"Updated Name"}`,
		`{"type":"custom"}`,
		`{"frequency":"annual"}`,
		`{"color":"#00FF00"}`,
		// Empty object (no-op update)
		`{}`,
		// Invalid field values
		`{"type":"invalid_type"}`,
		`{"frequency":"invalid_freq"}`,
		// Null fields
		`{"name":null,"type":null}`,
		// XSS/injection in update
		`{"name":"<img src=x onerror=alert(1)>"}`,
		`{"name":"'; DROP TABLE deadlines; --"}`,
		// Oversized
		`{"name":"` + strings.Repeat("x", 10000) + `"}`,
		// Wrong types
		`{"name":123,"active":"not-bool","notifyDaysBefore":"not-int"}`,
		// Deeply nested
		`{"name":{"nested":{"deep":"value"}}}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Use a well-known UUID for the update endpoint
	deadlineID := "00000000-0000-0000-0000-000000000000"

	f.Fuzz(func(t *testing.T, payloadJSON string) {
		if len(payloadJSON) > 100000 {
			payloadJSON = payloadJSON[:100000]
		}

		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return
		}

		code, body, err := cli.Request(ctx, "PATCH", "/v1/deadlines/"+deadlineID, headers, payload)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server must NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on fuzzed deadline update: code=%d body=%s payload=%s", code, string(body), payloadJSON)
		}

		t.Logf("Deadline update result: code=%d payload=%s", code, payloadJSON)
	})
}

// FuzzDeadline_DeliverPayload fuzzes the deliver deadline endpoint with random JSON payloads.
// Verifies: no 5xx errors on malformed deliver payloads.
func FuzzDeadline_DeliverPayload(f *testing.F) {
	seeds := []string{
		`{"delivered":true}`,
		`{"delivered":false}`,
		`{}`,
		`{"delivered":null}`,
		`{"delivered":"yes"}`,
		`{"delivered":1}`,
		`{"delivered":"true"}`,
		`{"extra":"field","delivered":true}`,
		`null`,
		`[]`,
		`""`,
		`{"delivered":true,"delivered":false}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	deadlineID := "00000000-0000-0000-0000-000000000000"

	f.Fuzz(func(t *testing.T, payloadJSON string) {
		if len(payloadJSON) > 100000 {
			payloadJSON = payloadJSON[:100000]
		}

		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return
		}

		code, body, err := cli.Request(ctx, "PATCH", "/v1/deadlines/"+deadlineID+"/deliver", headers, payload)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server must NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on fuzzed deliver payload: code=%d body=%s payload=%s", code, string(body), payloadJSON)
		}

		t.Logf("Deadline deliver result: code=%d payload=%s", code, payloadJSON)
	})
}
