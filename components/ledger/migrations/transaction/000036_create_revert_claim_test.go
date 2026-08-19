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

func TestMigration000036_RevertClaimContract(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)
	up, err := os.ReadFile(filepath.Join(dir, "000036_create_revert_claim.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000036_create_revert_claim.down.sql"))
	require.NoError(t, err)
	assert.Equal(t, "562ac8d0c6c91d1faf62f692d6f6d84ac2405023e28886608b5423453c981c0d",
		fmt.Sprintf("%x", sha256.Sum256(up)), "published migration up bytes must never be rewritten")
	assert.Equal(t, "d6fcdbef9278f8019f1f2bd19429939fc72631c2d42047be3ca2f592d0d2cb93",
		fmt.Sprintf("%x", sha256.Sum256(down)), "published migration down bytes must never be rewritten")

	upSQL := strings.ToLower(string(up))
	for _, required := range []string{
		"create table if not exists transaction_revert_claim",
		"primary key (organization_id, ledger_id, origin_transaction_id)",
		"unique (organization_id, ledger_id, reverse_transaction_id)",
		"legacy_fence_key",
		"legacy_fence_owner",
		"legacy_fence_owner = reverse_transaction_id::text",
		"btrim(legacy_fence_key) <> ''",
		"recovering",
		"reconciliation_required",
		"check",
	} {
		assert.Contains(t, upSQL, required)
	}

	downSQL := strings.ToLower(string(down))
	for _, required := range []string{
		"lock table transaction_revert_claim in access exclusive mode",
		"select exists (select 1 from transaction_revert_claim limit 1)",
		"cannot remove transaction_revert_claim while reversal claims exist",
		"drop table transaction_revert_claim",
	} {
		assert.Contains(t, downSQL, required)
	}
}
