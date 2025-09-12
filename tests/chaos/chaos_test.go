package chaos

import (
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestChaos_RestartDatabase demonstrates a chaos experiment scaffold.
// It will restart Postgres and then verify the APIs recover gracefully.
func TestChaos_RestartDatabase(t *testing.T) {
    t.Skip("implementation pending: orchestrate chaos and health checks")

    // Example containers (from components/infra/docker-compose.yml)
    const pgPrimary = "midaz-postgres-primary"

    if err := h.RestartWithWait(pgPrimary, 5*time.Second); err != nil {
        t.Fatalf("failed to restart %s: %v", pgPrimary, err)
    }
}

