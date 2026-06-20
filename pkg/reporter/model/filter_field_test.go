// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateFieldName asserts the shared charset whitelist accepts legitimate
// plain columns and dotted JSONB/document paths while rejecting any field
// carrying injection-shaped escapes or whitespace.
func TestValidateFieldName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		wantErr bool
	}{
		{name: "plain column", field: "id", wantErr: false},
		{name: "dotted jsonb path", field: "metadata.foo", wantErr: false},
		{name: "deep dotted path", field: "nested.a.b", wantErr: false},
		{name: "leading underscore root", field: "_internal", wantErr: false},
		{name: "digit after root", field: "field1", wantErr: false},
		{name: "sql injection via paren", field: "id) OR 1=1", wantErr: true},
		{name: "mongo operator field", field: "$where", wantErr: true},
		{name: "whitespace in field", field: "a b", wantErr: true},
		{name: "quote and comment breakout", field: "x';--", wantErr: true},
		{name: "empty field", field: "", wantErr: true},
		{name: "leading digit root", field: "1status", wantErr: true},
		{name: "trailing dot", field: "metadata.", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateFieldName(tc.field)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
