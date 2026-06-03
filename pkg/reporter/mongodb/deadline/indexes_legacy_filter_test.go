// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package deadline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestIsLegacyTemplateScopedFilter_BsonDDecodeTrap locks the mongo-driver v2
// nested-document decode behavior: v2 default-decodes nested documents to
// bson.D, whereas v1 yielded bson.M. The previous implementation asserted
// bson.M on nested values, so under v2 it would silently return false for a
// genuinely legacy index spec and skip the required cleanup. This test feeds
// the bson.D shape a real v2 decode produces and asserts the legacy filter is
// still detected; it would fail against the pre-migration .(bson.M) code.
func TestIsLegacyTemplateScopedFilter_BsonDDecodeTrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec bson.M
		want bool
	}{
		{
			name: "legacy shape decoded as bson.D (v2 default) is detected",
			spec: bson.M{
				"partialFilterExpression": bson.D{
					{Key: "template_id", Value: bson.D{
						{Key: "$type", Value: "string"},
					}},
				},
			},
			want: true,
		},
		{
			name: "legacy shape decoded as bson.M (v1 / explicit map) still detected",
			spec: bson.M{
				"partialFilterExpression": bson.M{
					"template_id": bson.M{
						"$type": "string",
					},
				},
			},
			want: true,
		},
		{
			name: "mixed bson.M outer + bson.D inner",
			spec: bson.M{
				"partialFilterExpression": bson.M{
					"template_id": bson.D{
						{Key: "$type", Value: "string"},
					},
				},
			},
			want: true,
		},
		{
			name: "new $exists shape (bson.D) is not legacy",
			spec: bson.M{
				"partialFilterExpression": bson.D{
					{Key: "template_id", Value: bson.D{
						{Key: "$exists", Value: true},
					}},
				},
			},
			want: false,
		},
		{
			name: "missing partialFilterExpression is not legacy",
			spec: bson.M{
				"key": bson.D{{Key: "_id", Value: 1}},
			},
			want: false,
		},
		{
			name: "partialFilterExpression without template_id is not legacy",
			spec: bson.M{
				"partialFilterExpression": bson.D{
					{Key: "deleted_at", Value: nil},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isLegacyTemplateScopedFilter(tt.spec))
		})
	}
}
