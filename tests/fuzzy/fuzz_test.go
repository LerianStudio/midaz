package fuzzy

import (
    "context"
    "encoding/json"
    "regexp"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// FuzzCreateOrganizationName fuzzes organization legalName to validate input handling.
// Run with: go test -v ./tests/fuzzy -fuzz=Fuzz -run=^$
func FuzzCreateOrganizationName(f *testing.F) {
    // Seed corpus
    f.Add("Acme, Inc.")
    f.Add("")
    f.Add("a")
    f.Add("Αθήνα") // non-ascii
    f.Add(h.RandString(300))

    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Precompile allowed chars (rough heuristic; server does full validation)
    allowed := regexp.MustCompile(`^[\p{L}\p{N} _.,\-]*$`)

    f.Fuzz(func(t *testing.T, name string) {
        // Bound name length to keep requests reasonable
        if len(name) > 512 {
            name = name[:512]
        }
        // Quick sanitation: replace control characters
        for _, c := range name {
            if c < 0x20 && c != '\n' && c != '\t' {
                name = allowed.ReplaceAllString(name, " ")
                break
            }
        }

        payload := h.OrgPayload(name, h.RandString(12))
        code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, payload)
        if err != nil {
            t.Fatalf("fuzz org request error: %v", err)
        }
        // Accept any non-5xx; 201 or 4xx depending on validation
        if code >= 500 {
            t.Fatalf("server 5xx on fuzz org name: %d body=%s len=%d", code, string(body), len(name))
        }
        // When accepted, ensure ID is present
        if code == 201 {
            var org struct{ ID string `json:"id"` }
            _ = json.Unmarshal(body, &org)
            if org.ID == "" {
                t.Fatalf("accepted org without ID: %s", string(body))
            }
            // tiny delay to avoid hammering too fast under -fuzz
            time.Sleep(10 * time.Millisecond)
        }
    })
}
