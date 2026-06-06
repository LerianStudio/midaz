package buildinfo

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestVersionHandler(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantVersion string
	}{
		{name: "empty version falls back to 0.0.0", version: "", wantVersion: "0.0.0"},
		{name: "non-empty version is echoed", version: "1.2.3", wantVersion: "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/version", VersionHandler(tt.version))

			req := httptest.NewRequest(fiber.MethodGet, "/version", nil)

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("unmarshal body %q: %v", string(body), err)
			}

			for _, key := range []string{"version", "requestDate", "commit", "buildTime", "dirty"} {
				if _, ok := got[key]; !ok {
					t.Errorf("response missing key %q; body=%s", key, string(body))
				}
			}

			if got["version"] != tt.wantVersion {
				t.Errorf("version = %v, want %q", got["version"], tt.wantVersion)
			}
		})
	}
}
