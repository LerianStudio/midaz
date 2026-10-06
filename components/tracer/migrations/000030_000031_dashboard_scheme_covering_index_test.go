// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	dashboardSchemeIdxUp    = "000030_transaction_validations_dashboard_scheme_idx.up.sql"
	dashboardSchemeIdxDown  = "000030_transaction_validations_dashboard_scheme_idx.down.sql"
	dropDashboardIdxUp      = "000031_drop_transaction_validations_dashboard_idx.up.sql"
	dropDashboardIdxDown    = "000031_drop_transaction_validations_dashboard_idx.down.sql"
	dashboardCoveringKey    = "on transaction_validations (created_at)"
	dashboardIncludeColumns = "include (decision, transaction_type, asset, amount, processing_time_ms"
)

// TestMigration000030_BuildsSchemeAwareCoveringIndexConcurrently verifies the
// replacement covering index keeps the key and INCLUDE list of 000024 and adds
// scheme, so the dashboard reads stay index-only, and that each file holds one
// statement so CONCURRENTLY runs outside a transaction block.
func TestMigration000030_BuildsSchemeAwareCoveringIndexConcurrently(t *testing.T) {
	t.Parallel()

	upRaw := readMigration(t, dashboardSchemeIdxUp)
	downRaw := readMigration(t, dashboardSchemeIdxDown)

	up := normalizeSQL(upRaw)
	down := normalizeSQL(downRaw)

	assert.Contains(t, up, "create index concurrently if not exists idx_transaction_validations_dashboard_scheme "+
		dashboardCoveringKey+" "+dashboardIncludeColumns+", scheme);")
	assert.Contains(t, down, "drop index concurrently if exists idx_transaction_validations_dashboard_scheme;")

	assert.Equal(t, 1, statementCount(upRaw), "%s must hold exactly one statement", dashboardSchemeIdxUp)
	assert.Equal(t, 1, statementCount(downRaw), "%s must hold exactly one statement", dashboardSchemeIdxDown)
}

// TestMigration000031_DropsSupersededCoveringIndexConcurrently verifies the
// superseded 000024 index is dropped CONCURRENTLY and that the down recreates
// it with exactly the 000024 definition.
func TestMigration000031_DropsSupersededCoveringIndexConcurrently(t *testing.T) {
	t.Parallel()

	upRaw := readMigration(t, dropDashboardIdxUp)
	downRaw := readMigration(t, dropDashboardIdxDown)

	up := normalizeSQL(upRaw)
	down := normalizeSQL(downRaw)
	original := normalizeSQL(readMigration(t, "000024_add_dashboard_covering_index.up.sql"))

	assert.Contains(t, up, "drop index concurrently if exists idx_transaction_validations_dashboard;")
	assert.NotContains(t, up, "drop index concurrently if exists idx_transaction_validations_dashboard_scheme",
		"the up must drop only the superseded index")

	recreate := "create index concurrently if not exists idx_transaction_validations_dashboard " +
		dashboardCoveringKey + " " + dashboardIncludeColumns + ");"
	assert.Contains(t, down, recreate)
	assert.Contains(t, original, recreate, "the down must restore the index exactly as 000024 built it")

	assert.Equal(t, 1, statementCount(upRaw), "%s must hold exactly one statement", dropDashboardIdxUp)
	assert.Equal(t, 1, statementCount(downRaw), "%s must hold exactly one statement", dropDashboardIdxDown)
}
