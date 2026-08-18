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

func TestMigration000036_RevertClaimContract(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000036_create_revert_claim.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000036_create_revert_claim.down.sql"))
	require.NoError(t, err)

	upSQL := strings.ToLower(string(up))
	for _, required := range []string{
		"create table if not exists transaction_revert_claim",
		"primary key (organization_id, ledger_id, origin_transaction_id)",
		"unique (organization_id, ledger_id, reverse_transaction_id)",
		"reconciliation_required",
		"check",
	} {
		assert.Contains(t, upSQL, required)
	}

	downSQL := strings.ToLower(string(down))
	for _, required := range []string{
		"select exists (select 1 from transaction_revert_claim limit 1)",
		"cannot remove transaction_revert_claim while reversal claims exist",
		"drop table if exists transaction_revert_claim",
	} {
		assert.Contains(t, downSQL, required)
	}
}
