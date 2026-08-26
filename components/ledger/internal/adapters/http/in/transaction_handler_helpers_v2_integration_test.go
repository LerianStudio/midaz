// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegration_DecimalContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		msgAndArgs []any
		want       string
	}{
		{
			name:       "no context",
			msgAndArgs: nil,
			want:       "no context",
		},
		{
			name:       "plain message is returned verbatim",
			msgAndArgs: []any{"@dstA available (50% share leg)"},
			want:       "@dstA available (50% share leg)",
		},
		{
			name:       "plain message with several percent signs is returned verbatim",
			msgAndArgs: []any{"@dstA received 50% of its 60% share"},
			want:       "@dstA received 50% of its 60% share",
		},
		{
			name:       "format with arguments is substituted",
			msgAndArgs: []any{"operation[%d] amount", 3},
			want:       "operation[3] amount",
		},
		{
			name:       "non-string leader falls back to a value dump",
			msgAndArgs: []any{42, "tail"},
			want:       "[42 tail]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, decimalContext(tt.msgAndArgs))
		})
	}
}
