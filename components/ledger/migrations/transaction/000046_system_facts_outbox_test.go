// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LerianStudio/lib-system-facts/outbox/outboxtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	migration000046Up   = "000046_system_facts_outbox.up.sql"
	migration000046Down = "000046_system_facts_outbox.down.sql"
)

func TestMigration000046_FilesExist(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	tests := []struct {
		name     string
		filename string
	}{
		{name: "up migration file exists", filename: migration000046Up},
		{name: "down migration file exists", filename: migration000046Down},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(filepath.Join(dir, tc.filename))
			require.NoError(t, err, "migration file %s must be readable", tc.filename)
			assert.NotEmpty(t, content, "migration file %s must not be empty", tc.filename)
		})
	}
}

// The kit forwarder reads system_facts_outbox exactly as outbox.SchemaSQL()
// shapes it, so the migration carries that DDL and outbox.DropSchemaSQL() byte
// for byte: a kit bump that changes either fails here until a new migration
// ships, and a down that dropped the shared enum would break the lib-commons
// outbox beside it.
func TestMigration000046_CarriesKitOutboxSchemaVerbatim(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	up, err := os.ReadFile(filepath.Join(dir, migration000046Up))
	require.NoError(t, err)

	down, err := os.ReadFile(filepath.Join(dir, migration000046Down))
	require.NoError(t, err)

	outboxtest.AssertMigrationCarriesSchema(t, string(up), string(down))
}
