package integration

import (
    "context"
    "os"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Runs only when TEST_REQUIRE_AUTH=true (useful when PLUGIN_AUTH_ENABLED=true in services).
func TestIntegration_Security_UnauthorizedWithoutToken(t *testing.T) {
    if os.Getenv("TEST_REQUIRE_AUTH") != "true" {
        t.Skip("TEST_REQUIRE_AUTH not set; skipping authz checks")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

    // No Authorization header
    code, _, err := onboard.Request(ctx, "GET", "/v1/organizations", map[string]string{"X-Request-Id": h.RandHex(6)}, nil)
    if err == nil && code != 401 {
        t.Fatalf("expected 401 when missing Authorization, got %d", code)
    }
}

// When TEST_REQUIRE_AUTH=true and TEST_AUTH_HEADER is provided, request should be authorized.
func TestIntegration_Security_AuthorizedWithToken(t *testing.T) {
    if os.Getenv("TEST_REQUIRE_AUTH") != "true" {
        t.Skip("TEST_REQUIRE_AUTH not set; skipping authz checks")
    }
    if os.Getenv("TEST_AUTH_HEADER") == "" {
        t.Skip("TEST_AUTH_HEADER not provided; skipping")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(6))

    // Authorized request should not be 401
    code, _, err := onboard.Request(ctx, "GET", "/v1/organizations", headers, nil)
    if err != nil || code == 401 {
        t.Fatalf("unexpected unauthorized with TEST_AUTH_HEADER set: code=%d err=%v", code, err)
    }
}

