package chaos

import (
    "context"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Pause/unpause containers; and network disconnect/connect to simulate disruptions; verify /health recovers.
func TestChaos_NetworkDisruptions(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-ledger"}, "NetworkDisruptions")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
    trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Pause/unpause transaction container
    if err := h.DockerAction("pause", "midaz-ledger"); err != nil { t.Fatalf("pause transaction: %v", err) }
    time.Sleep(2 * time.Second)
    if err := h.DockerAction("unpause", "midaz-ledger"); err != nil { t.Fatalf("unpause transaction: %v", err) }

    // Wait for health to return
    deadline := time.Now().Add(30 * time.Second)
    for {
        code, _, _ := trans.Request(context.Background(), "GET", "/health", headers, nil)
        if code == 200 { break }
        if time.Now().After(deadline) { t.Fatalf("transaction health did not recover after pause/unpause") }
        time.Sleep(300 * time.Millisecond)
    }

    // Network disconnect/connect transaction from infra-network
    if err := h.DockerNetwork("disconnect", "infra-network", "midaz-ledger"); err != nil {
        t.Fatalf("network disconnect transaction: %v", err)
    }
    time.Sleep(2 * time.Second)
    if err := h.DockerNetwork("connect", "infra-network", "midaz-ledger"); err != nil {
        t.Fatalf("network connect transaction: %v", err)
    }

    deadline = time.Now().Add(30 * time.Second)
    for {
        code, _, _ := trans.Request(context.Background(), "GET", "/health", headers, nil)
        if code == 200 { break }
        if time.Now().After(deadline) { t.Fatalf("transaction health did not recover after network reconnect") }
        time.Sleep(300 * time.Millisecond)
    }
}
