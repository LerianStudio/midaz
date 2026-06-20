// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

// TestBuildMongoFilter_FieldNameCharset proves the CRM fan-out path
// (buildMongoFilter -> convertFilterConditionToMongoFilter) rejects an
// injection-shaped field BEFORE it becomes a verbatim BSON map key, while
// passing legitimate plain columns and dotted document paths through. Each case
// carries a non-empty condition so the charset gate is actually reached rather
// than short-circuited by the empty-condition skip.
func TestBuildMongoFilter_FieldNameCharset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		wantErr bool
	}{
		{name: "plain column", field: "id", wantErr: false},
		{name: "dotted document path", field: "metadata.foo", wantErr: false},
		{name: "deep dotted path", field: "nested.a.b", wantErr: false},
		{name: "sql injection via paren", field: "id) OR 1=1", wantErr: true},
		{name: "mongo operator field", field: "$where", wantErr: true},
		{name: "whitespace in field", field: "a b", wantErr: true},
		{name: "quote and comment breakout", field: "x';--", wantErr: true},
		{name: "empty field", field: "", wantErr: true},
	}

	ds := &ExternalDataSource{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter := map[string]model.FilterCondition{
				tc.field: {Equals: []any{"value"}},
			}

			got, err := ds.buildMongoFilter(filter)
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)
			assert.Contains(t, got, tc.field)
		})
	}
}
