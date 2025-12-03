package integration

import (
	"context"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
)

// TestSmoke_LedgerHealthAndVersion verifies that the Ledger service is reachable
// and responds correctly to health and version endpoints.
func TestSmoke_LedgerHealthAndVersion(t *testing.T) {
	env := h.LoadEnvironment()
	host, err := h.URLHostPort(env.LedgerURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if err := h.WaitForTCP(host, 30*time.Second); err != nil {
		t.Fatalf("ledger not reachable: %v", err)
	}

	c := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	ctx := context.Background()
	if code, _, err := c.Request(ctx, "GET", "/health", nil, nil); err != nil || code != 200 {
		t.Fatalf("/health failed: code=%d err=%v", code, err)
	}
	if code, body, err := c.Request(ctx, "GET", "/version", nil, nil); err != nil || code != 200 {
		t.Fatalf("/version failed: code=%d err=%v", code, err)
	} else if len(body) == 0 {
		t.Fatalf("/version empty body")
	}
}