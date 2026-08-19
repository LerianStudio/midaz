// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration000037_RevertRolloutGenerationContract(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000037_add_revert_rollout_generation.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000037_add_revert_rollout_generation.down.sql"))
	require.NoError(t, err)
	assert.Equal(t, "245098bedd90716323597ac79260331ebfef12421ae515b6cc90de69244d3884",
		fmt.Sprintf("%x", sha256.Sum256(up)), "published migration up bytes must never be rewritten")
	assert.Equal(t, "fb0abd315a29465e0276666a6358ae65e5c9edfda084c477265bc8eb468d41a4",
		fmt.Sprintf("%x", sha256.Sum256(down)), "published migration down bytes must never be rewritten")

	upSQL := strings.ToLower(string(up))
	for _, required := range []string{
		"add column if not exists rollout_mode",
		"add column if not exists rollout_token",
		"add column if not exists redis_generation",
		"rollout_mode in ('legacy', 'bridge')",
		"rollout_token is not null",
		"rollout_mode is null or redis_generation is not null",
		"btrim(rollout_token) <> ''",
		"redis_generation is null or btrim(redis_generation) <> ''",
	} {
		assert.Contains(t, upSQL, required)
	}

	downSQL := strings.ToLower(string(down))
	for _, required := range []string{
		"lock table transaction_revert_claim in access exclusive mode",
		"select exists (select 1 from transaction_revert_claim limit 1)",
		"rollback requires an empty claim table",
		"drop column if exists redis_generation",
		"drop column if exists rollout_token",
		"drop column if exists rollout_mode",
	} {
		assert.Contains(t, downSQL, required)
	}
}
