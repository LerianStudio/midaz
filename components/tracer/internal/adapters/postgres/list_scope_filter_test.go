// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func renderScope(t *testing.T, apply func(sq.SelectBuilder) sq.SelectBuilder) (string, []any) {
	t.Helper()

	query, args, err := apply(sq.Select("1").From("t")).PlaceholderFormat(sq.Dollar).ToSql()
	require.NoError(t, err)

	return query, args
}

func TestListScope_TransactionValidations(t *testing.T) {
	r := &TransactionValidationRepository{}
	account, segment, portfolio, merchant := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	t.Run("each allowed dimension narrows its field", func(t *testing.T) {
		query, args := renderScope(t, func(b sq.SelectBuilder) sq.SelectBuilder {
			return r.applyFilters(b, &model.TransactionValidationFilters{Scope: http.ScopeConfinement{
				"accountId": {account}, "segmentId": {segment}, "portfolioId": {portfolio}, "merchantId": {merchant},
			}})
		})

		assert.Equal(t, "SELECT 1 FROM t WHERE account->>'accountId' = ANY($1::text[]) AND merchant->>'merchantId' = ANY($2::text[])"+
			" AND portfolio->>'portfolioId' = ANY($3::text[]) AND segment->>'segmentId' = ANY($4::text[])", query)
		assert.Equal(t, []any{pq.StringArray{account.String()}, pq.StringArray{merchant.String()}, pq.StringArray{portfolio.String()}, pq.StringArray{segment.String()}}, args)
	})

	t.Run("an empty allowed list, or a dimension the list cannot apply, matches nothing", func(t *testing.T) {
		for _, scope := range []http.ScopeConfinement{{"accountId": {}}, {"ledgerId": {account}}} {
			query, _ := renderScope(t, func(b sq.SelectBuilder) sq.SelectBuilder {
				return r.applyFilters(b, &model.TransactionValidationFilters{Scope: scope})
			})
			assert.Equal(t, "SELECT 1 FROM t WHERE FALSE", query)
		}
	})

	t.Run("no confinement adds nothing", func(t *testing.T) {
		query, _ := renderScope(t, func(b sq.SelectBuilder) sq.SelectBuilder {
			return r.applyFilters(b, &model.TransactionValidationFilters{})
		})
		assert.Equal(t, "SELECT 1 FROM t", query)
	})
}

func TestListScope_AuditEvents(t *testing.T) {
	r := &AuditEventRepository{}
	account, merchant := uuid.New(), uuid.New()

	query, args := renderScope(t, func(b sq.SelectBuilder) sq.SelectBuilder {
		return r.applyFilters(b, &model.AuditEventFilters{Scope: http.ScopeConfinement{"accountId": {account}, "merchantId": {merchant}}})
	})

	assert.Equal(t, "SELECT 1 FROM t WHERE context->'request'->'account'->>'id' = ANY($1::text[])"+
		" AND context->'request'->'merchant'->>'merchantId' = ANY($2::text[])", query)
	assert.Equal(t, []any{pq.StringArray{account.String()}, pq.StringArray{merchant.String()}}, args)

	query, _ = renderScope(t, func(b sq.SelectBuilder) sq.SelectBuilder {
		return r.applyFilters(b, &model.AuditEventFilters{Scope: http.ScopeConfinement{"segmentId": {}}})
	})
	assert.Equal(t, "SELECT 1 FROM t WHERE FALSE", query)
}
