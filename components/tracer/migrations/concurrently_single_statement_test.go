// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentlyMigrationsHoldOneStatement refuses a migration that builds or
// drops an index CONCURRENTLY beside any other statement. The runner sends a
// file as one simple query, which PostgreSQL wraps in an implicit transaction
// whenever it carries more than one statement, and CONCURRENTLY refuses to run
// inside a transaction block, so such a file would fail at boot.
func TestConcurrentlyMigrationsHoldOneStatement(t *testing.T) {
	t.Parallel()

	matches, err := filepath.Glob(filepath.Join(migrationsDir(t), "*.sql"))
	require.NoError(t, err)

	checked := 0

	for _, path := range matches {
		name := filepath.Base(path)
		body := readMigration(t, name)

		if !strings.Contains(strings.ToUpper(stripLineComments(body)), "CONCURRENTLY") {
			continue
		}

		checked++

		assert.Equal(t, 1, statementCount(body), "%s uses CONCURRENTLY and must hold exactly one statement", name)
	}

	require.NotZero(t, checked, "no CONCURRENTLY migration found under %s: the guard is asserting nothing", migrationsDir(t))
}
