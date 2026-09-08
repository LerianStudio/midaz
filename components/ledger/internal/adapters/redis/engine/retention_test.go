// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveRetentionSecondsMatchesHTTPBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested int64
		want      int64
		wantErr   bool
	}{
		{name: "default", requested: 0, want: 300},
		{name: "one week", requested: 604800, want: 604800},
		{name: "negative", requested: -1, wantErr: true},
		{name: "one week plus one", requested: 604801, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := effectiveRetentionSeconds(test.requested)
			if test.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
