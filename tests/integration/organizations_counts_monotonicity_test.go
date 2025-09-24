package integration

import (
    "context"
    "encoding/json"
    "strconv"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Monotonicity of organization count via HEAD /v1/organizations/metrics/count.
func TestIntegration_Organizations_Counts_Monotonicity(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    getCount := func() int {
        c, _, hdr, e := onboard.RequestFull(ctx, "HEAD", "/v1/organizations/metrics/count", headers, nil)
        if e != nil || c != 204 { t.Fatalf("head orgs count: code=%d err=%v", c, e) }
        v, err := strconv.Atoi(hdr.Get("X-Total-Count"))
        if err != nil { t.Fatalf("parse orgs count: %v", err) }
        return v
    }

    last := getCount()

    // create 3 organizations and assert monotonic non-decreasing counts
    for i := 0; i < 3; i++ {
        code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("OrgCM "+h.RandString(4), h.RandString(10)))
        if err != nil || code != 201 { t.Fatalf("create org %d: %d %s", i, code, string(body)) }
        var org struct{ ID string `json:"id"` }
        _ = json.Unmarshal(body, &org)

        cur := getCount()
        if cur < last { t.Fatalf("org count decreased: last=%d cur=%d", last, cur) }
        if cur < last+1 { // allow slight lag; retry once via GET list length as fallback
            // optional: tolerate eventual consistency; still require >= last+1 by re-polling HEAD
            cur2 := getCount()
            if cur2 < last+1 { t.Fatalf("org count did not increase: last=%d got=%d", last, cur2) }
            cur = cur2
        }
        last = cur
    }
}
