// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package scopefilter

import (
	"testing"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func render(t *testing.T, scope http.ScopeConfinement) (string, []any) {
	t.Helper()

	query, args, err := Where(squirrel.Select("id").From("account"), scope, map[string]string{
		"accountId":   "id",
		"portfolioId": "portfolio_id",
	}).PlaceholderFormat(squirrel.Dollar).ToSql()
	require.NoError(t, err)

	return query, args
}

func TestWhere(t *testing.T) {
	a, b, p := uuid.New(), uuid.New(), uuid.New()

	t.Run("no confinement leaves the query as it was", func(t *testing.T) {
		query, args := render(t, nil)
		assert.Equal(t, "SELECT id FROM account", query)
		assert.Empty(t, args)
	})

	t.Run("each confined dimension narrows its column, in dimension order", func(t *testing.T) {
		query, args := render(t, http.ScopeConfinement{"portfolioId": {p}, "accountId": {a, b}})
		assert.Equal(t, "SELECT id FROM account WHERE id = ANY($1::uuid[]) AND portfolio_id = ANY($2::uuid[])", query)
		assert.Equal(t, []any{pq.StringArray{a.String(), b.String()}, pq.StringArray{p.String()}}, args)
	})

	t.Run("a dimension confined to nothing matches nothing", func(t *testing.T) {
		query, args := render(t, http.ScopeConfinement{"accountId": {}})
		assert.Equal(t, "SELECT id FROM account WHERE FALSE", query)
		assert.Empty(t, args)
	})

	t.Run("a dimension the list cannot apply matches nothing rather than everything", func(t *testing.T) {
		query, args := render(t, http.ScopeConfinement{"segmentId": {p}})
		assert.Equal(t, "SELECT id FROM account WHERE FALSE", query)
		assert.Empty(t, args)
	})
}
