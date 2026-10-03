// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package scopefilter confines a list query to the instances a scoped
// credential may see.
package scopefilter

import (
	"sort"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// Predicate is the condition confining a list to ids on one dimension.
type Predicate func(ids pq.StringArray) squirrel.Sqlizer

// Column confines on a uuid column holding the dimension.
func Column(name string) Predicate {
	return func(ids pq.StringArray) squirrel.Sqlizer {
		return squirrel.Expr(name+" = ANY(?::uuid[])", ids)
	}
}

// Where narrows builder to scope. columns maps each dimension the list can
// confine to the uuid column holding it.
func Where(builder squirrel.SelectBuilder, scope http.ScopeConfinement, columns map[string]string) squirrel.SelectBuilder {
	predicates := make(map[string]Predicate, len(columns))
	for dimension, column := range columns {
		predicates[dimension] = Column(column)
	}

	return WherePredicates(builder, scope, predicates)
}

// WherePredicates narrows builder to scope with one predicate per dimension. A
// dimension confined to no instance, or one the list has no predicate for,
// matches nothing: an unapplied confinement must never widen the list.
func WherePredicates(builder squirrel.SelectBuilder, scope http.ScopeConfinement, predicates map[string]Predicate) squirrel.SelectBuilder {
	if len(scope) == 0 {
		return builder
	}

	dimensions := make([]string, 0, len(scope))
	for dimension := range scope {
		dimensions = append(dimensions, dimension)
	}

	sort.Strings(dimensions)

	for _, dimension := range dimensions {
		if _, applies := predicates[dimension]; !applies || len(scope[dimension]) == 0 {
			return builder.Where(squirrel.Expr("FALSE"))
		}
	}

	for _, dimension := range dimensions {
		builder = builder.Where(predicates[dimension](Array(scope[dimension])))
	}

	return builder
}

// Array is the uuid list as a text array, for a "= ANY(?::uuid[])" predicate.
func Array(ids []uuid.UUID) pq.StringArray {
	out := make(pq.StringArray, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}

	return out
}
