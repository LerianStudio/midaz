// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCursorPagination(t *testing.T) {
	t.Parallel()

	builder, err := applyCursorPagination(squirrel.Select("id").From("balance"), libHTTP.Cursor{}, "asc", 10)
	require.NoError(t, err)

	sql, _, err := builder.ToSql()
	require.NoError(t, err)
	assert.Contains(t, sql, "ORDER BY id ASC")
	assert.Contains(t, sql, "LIMIT 11")
}
