// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMigrationVersionsAreUnique refuses two migrations sharing a version prefix.
// golang-migrate rejects the whole source on a duplicate, so a collision left by
// two branches numbering migrations in parallel would stop every service boot.
func TestMigrationVersionsAreUnique(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir(t))
	require.NoError(t, err)

	owners := make(map[string]string)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".sql" {
			continue
		}

		version, rest, found := strings.Cut(name, "_")
		require.True(t, found, "migration %s has no version prefix", name)

		description := strings.TrimSuffix(strings.TrimSuffix(rest, ".up.sql"), ".down.sql")
		if owner, exists := owners[version]; exists {
			require.Equal(t, owner, description, "migration version %s is used by %s and %s", version, owner, description)
		}

		owners[version] = description
	}
}
