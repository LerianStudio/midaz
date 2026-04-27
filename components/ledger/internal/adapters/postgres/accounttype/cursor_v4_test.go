// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounttype

import (
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyCursorPagination mirrors the sibling cursor_v4 tests in /operation,
// /transaction, /balance, and /operationroute. The accounttype adapter is the only
// one that was missing this test; the function is a one-line forward to
// pagination.ApplyCursorPagination, but pinning the SQL it produces protects against
// accidental drift in the shared helper.
func TestApplyCursorPagination(t *testing.T) {
	t.Parallel()

	builder, err := applyCursorPagination(squirrel.Select("id").From("account_type"), libHTTP.Cursor{}, "asc", 10)
	require.NoError(t, err)

	sql, _, err := builder.ToSql()
	require.NoError(t, err)

	// LIMIT 11 = page size + 1 sentinel for has-next detection (the +1 lookahead
	// pattern shared across every cursor_v4 implementation).
	assert.Contains(t, sql, "ORDER BY id ASC")
	assert.Contains(t, sql, "LIMIT 11")
}

// TestApplyCursorPagination_DescendingOrder pins the descending order branch so a
// future change to the shared helper that flips defaults would surface here, not in a
// production endpoint regression test.
func TestApplyCursorPagination_DescendingOrder(t *testing.T) {
	t.Parallel()

	builder, err := applyCursorPagination(squirrel.Select("id").From("account_type"), libHTTP.Cursor{}, "desc", 25)
	require.NoError(t, err)

	sql, _, err := builder.ToSql()
	require.NoError(t, err)
	assert.Contains(t, sql, "ORDER BY id DESC")
	assert.Contains(t, sql, "LIMIT 26")
}
