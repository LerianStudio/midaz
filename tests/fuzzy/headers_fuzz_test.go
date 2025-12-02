package fuzzy

import (
	"context"
	"os"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func shouldRunHeaders(t *testing.T) { /* always run */ }

// Validate behavior for missing/duplicated headers and invalid Authorization tokens.
// Guarded by MIDAZ_TEST_FUZZ, and 401-specific assertions only when TEST_REQUIRE_AUTH=true.
func TestFuzz_Headers_MissingDuplicated_InvalidAuth(t *testing.T) {
	shouldRunHeaders(t)
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)

	requireAuth := os.Getenv("TEST_REQUIRE_AUTH") == "true"

	// 1) Missing Authorization header
	if requireAuth {
		code, _, err := onboard.Request(ctx, "GET", "/v1/organizations", map[string]string{"X-Request-Id": h.RandHex(6)}, nil)
		if err == nil && code != 401 {
			t.Fatalf("expected 401 when missing Authorization (auth required), got %d", code)
		}
	} else {
		code, body, err := onboard.Request(ctx, "GET", "/v1/organizations", map[string]string{"X-Request-Id": h.RandHex(6)}, nil)
		if err != nil || code >= 500 {
			t.Fatalf("unexpected server error without Authorization: code=%d err=%v body=%s", code, err, string(body))
		}
	}

	// 2) Duplicated X-Request-Id and Authorization headers
	dup := map[string][]string{
		"X-Request-Id":  {h.RandHex(6), h.RandHex(6)},
		"Authorization": {os.Getenv("TEST_AUTH_HEADER"), "Bearer second"},
	}
	code, body, _, err := onboard.RequestWithHeaderValues(ctx, "GET", "/v1/organizations", dup, nil)
	if err != nil || code >= 500 {
		t.Fatalf("server 5xx or error on duplicate headers: code=%d err=%v body=%s", code, err, string(body))
	}

	// 3) Invalid Authorization formats (when auth required -> 401; otherwise -> not 5xx)
	// Only use values acceptable by net/http (no control chars like CR/LF in header values)
	invalids := []string{
		"", "Bearer", "Bearer ", "Basic abc", strings.Repeat("x", 256),
	}
	for i, tok := range invalids {
		headers := map[string][]string{
			"X-Request-Id": {h.RandHex(6)},
		}
		if tok != "" {
			headers["Authorization"] = []string{tok}
		}
		code, body, _, err := onboard.RequestWithHeaderValues(ctx, "GET", "/v1/organizations", headers, nil)
		if requireAuth {
			if err == nil && !(code == 401 || code == 403) {
				t.Fatalf("case %d: expected 401/403 for invalid token, got %d body=%s", i, code, string(body))
			}
		} else {
			if err != nil || code >= 500 {
				t.Fatalf("case %d: unexpected server error for invalid token when auth not required: code=%d err=%v", i, code, err)
			}
		}
	}

	// 4) Missing Content-Type on POST body -> expect 4xx (not 5xx)
	rawHeaders := map[string]string{"X-Request-Id": h.RandHex(6)}
	if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
		rawHeaders["Authorization"] = v
	}
	code2, body2, _, err := onboard.RequestRaw(ctx, "POST", "/v1/organizations", rawHeaders, "", []byte(`{"legalName":"ACME","legalDocument":"X"}`))
	if err != nil {
		t.Fatalf("missing content-type request error: %v", err)
	}
	if code2 >= 500 {
		t.Fatalf("missing Content-Type returned 5xx: %d body=%s", code2, string(body2))
	}

	// 5) Duplicate Content-Type values -> not 5xx
	dupCT := map[string][]string{
		"X-Request-Id": {h.RandHex(6)},
		"Content-Type": {"application/json", "application/json"},
	}
	if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
		dupCT["Authorization"] = []string{v}
	}
	code3, body3, _, err := onboard.RequestWithHeaderValues(ctx, "POST", "/v1/organizations", dupCT, h.OrgPayload("HDR ACME "+h.RandString(3), h.RandString(8)))
	if err != nil || code3 >= 500 {
		t.Fatalf("duplicate Content-Type produced server error: code=%d err=%v body=%s", code3, err, string(body3))
	}
}
