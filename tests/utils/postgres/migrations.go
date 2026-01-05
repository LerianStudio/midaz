//go:build integration

package postgres

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// FindMigrationsPath locates a migrations directory by traversing up from the current directory.
// It looks for the pattern: components/{component}/migrations
//
// Example:
//
//	path := FindMigrationsPath(t, "onboarding")  // finds components/onboarding/migrations
//	path := FindMigrationsPath(t, "transaction") // finds components/transaction/migrations
func FindMigrationsPath(t *testing.T, component string) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")

	for {
		candidate := filepath.Join(dir, "components", component, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find migrations directory for component %q", component)
		}

		dir = parent
	}
}
