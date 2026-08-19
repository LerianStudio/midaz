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

func TestMigration000039_RevertClaimArmContract(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000039_arm_revert_claim.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000039_arm_revert_claim.down.sql"))
	require.NoError(t, err)

	upSQL := strings.ToLower(string(up))
	for _, required := range []string{
		"lock table transaction_revert_claim in access exclusive mode",
		"select exists",
		"set state = 'armed'",
		"state in ('claimed', 'recovering')",
		"'claimed', 'armed', 'recovering', 'mutated', 'completed', 'reconciliation_required'",
	} {
		assert.Contains(t, upSQL, required)
	}

	downSQL := strings.ToLower(string(down))
	for _, required := range []string{
		"lock table transaction_revert_claim in access exclusive mode",
		"select exists (select 1 from transaction_revert_claim limit 1)",
		"rollback requires an empty claim table",
		"'claimed', 'recovering', 'mutated', 'completed', 'reconciliation_required'",
	} {
		assert.Contains(t, downSQL, required)
	}
}
