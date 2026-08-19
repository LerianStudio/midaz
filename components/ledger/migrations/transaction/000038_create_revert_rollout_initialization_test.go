// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration000038_RevertRolloutInitializationContract(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000038_create_revert_rollout_initialization.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000038_create_revert_rollout_initialization.down.sql"))
	require.NoError(t, err)

	upSQL := strings.ToLower(string(up))
	for _, required := range []string{
		"create table if not exists transaction_revert_rollout_initialization",
		"redis_generation uuid not null",
		"initialization_request_id uuid not null",
		"add column if not exists redis_generation uuid",
		"primary key (singleton)",
		"check (singleton)",
		"state in ('preparing', 'prepared')",
	} {
		assert.Contains(t, upSQL, required)
	}

	downSQL := strings.ToLower(string(down))
	for _, required := range []string{
		"lock table transaction_revert_rollout_initialization in access exclusive mode",
		"select exists (select 1 from transaction_revert_rollout_initialization limit 1)",
		"rollback requires an uninitialized rollout",
		"drop table transaction_revert_rollout_initialization",
	} {
		assert.Contains(t, downSQL, required)
	}
}
