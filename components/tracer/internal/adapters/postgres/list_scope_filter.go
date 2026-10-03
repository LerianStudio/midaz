// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"sort"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// transactionValidationScopeFields are the JSONB fields a list of transaction
// validations is confined on, per scope dimension.
var transactionValidationScopeFields = map[string]string{
	"accountId":   "account->>'accountId'",
	"segmentId":   "segment->>'segmentId'",
	"portfolioId": "portfolio->>'portfolioId'",
	"merchantId":  "merchant->>'merchantId'",
}

// auditEventScopeFields are the JSONB fields of the validation request an audit
// event records, per scope dimension. An event that records no validation has
// none of them and is never in a confined list.
var auditEventScopeFields = map[string]string{
	"accountId":   "context->'request'->'account'->>'id'",
	"segmentId":   "context->'request'->'account'->>'segmentId'",
	"portfolioId": "context->'request'->'account'->>'portfolioId'",
	"merchantId":  "context->'request'->'merchant'->>'merchantId'",
}

// applyListScope narrows qb to the values scope allows. A dimension confined to
// no value, or one the list has no field for, matches nothing: an unapplied
// confinement must never widen the list.
func applyListScope(qb sq.SelectBuilder, scope http.ScopeConfinement, fields map[string]string) sq.SelectBuilder {
	if len(scope) == 0 {
		return qb
	}

	dimensions := make([]string, 0, len(scope))
	for dimension := range scope {
		dimensions = append(dimensions, dimension)
	}

	sort.Strings(dimensions)

	for _, dimension := range dimensions {
		if _, applies := fields[dimension]; !applies || len(scope[dimension]) == 0 {
			return qb.Where("FALSE")
		}
	}

	for _, dimension := range dimensions {
		values := make(pq.StringArray, 0, len(scope[dimension]))
		for _, id := range scope[dimension] {
			values = append(values, id.String())
		}

		qb = qb.Where(fields[dimension]+" = ANY(?::text[])", values)
	}

	return qb
}
