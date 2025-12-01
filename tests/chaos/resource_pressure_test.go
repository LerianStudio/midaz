package chaos

import (
    "context"
    "fmt"
    "sync"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Best-effort: push many concurrent requests to stress pools; assert no widespread 5xx.
func TestChaos_ResourcePressure_NoCrashes(t *testing.T) {
    shouldRunChaos(t)

    env := h.LoadEnvironment()
    onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))
    ctx := context.Background()

    // Ensure at least one org exists to list
    _, _, _ = onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Load Org "+h.RandString(5), h.RandString(12)))

    var wg sync.WaitGroup
    errs := make(chan error, 1)

    worker := func(id int) {
        defer wg.Done()
        deadline := time.Now().Add(3 * time.Second)
        for time.Now().Before(deadline) {
            code, body, err := onboard.Request(ctx, "GET", "/v1/organizations", headers, nil)
            if err != nil {
                errs <- fmt.Errorf("worker %d request err: %v", id, err)
                return
            }
            if code >= 500 {
                errs <- fmt.Errorf("worker %d got 5xx: %d body=%s", id, code, string(body))
                return
            }
        }
    }

    n := 100
    for i := 0; i < n; i++ { wg.Add(1); go worker(i) }
    done := make(chan struct{})
    go func(){ wg.Wait(); close(done) }()

    select {
    case err := <-errs:
        t.Fatalf("resource pressure failure: %v", err)
    case <-done:
        // ok
    case <-time.After(10 * time.Second):
        t.Fatalf("resource pressure test timed out")
    }
}

