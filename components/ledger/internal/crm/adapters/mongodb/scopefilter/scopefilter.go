// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package scopefilter confines a document listing to the instances a scoped
// credential may see.
package scopefilter

import (
	"sort"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// Field is how one dimension's ids are matched on a document field.
type Field struct {
	// Name is the document field holding the dimension.
	Name string
	// AsString matches the ids in their text form, for a field stored as a
	// string; otherwise the ids are matched as uuid values.
	AsString bool
}

// Apply appends to filter the confinement of scope, under one $and so it never
// collides with another condition on the same field. A dimension confined to no
// instance, or one the listing has no field for, matches nothing: an unapplied
// confinement must never widen the listing.
func Apply(filter bson.D, scope http.ScopeConfinement, fields map[string]Field) bson.D {
	if len(scope) == 0 {
		return filter
	}

	dimensions := make([]string, 0, len(scope))
	for dimension := range scope {
		dimensions = append(dimensions, dimension)
	}

	sort.Strings(dimensions)

	conditions := bson.A{}

	for _, dimension := range dimensions {
		ids := scope[dimension]

		field, applies := fields[dimension]
		if !applies || len(ids) == 0 {
			return append(filter, bson.E{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{}}}})
		}

		conditions = append(conditions, bson.D{{Key: field.Name, Value: bson.D{{Key: "$in", Value: values(ids, field.AsString)}}}})
	}

	return append(filter, bson.E{Key: "$and", Value: conditions})
}

func values(ids []uuid.UUID, asString bool) bson.A {
	out := make(bson.A, 0, len(ids))

	for _, id := range ids {
		if asString {
			out = append(out, id.String())
		} else {
			out = append(out, id)
		}
	}

	return out
}
