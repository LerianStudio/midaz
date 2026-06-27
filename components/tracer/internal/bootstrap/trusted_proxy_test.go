// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTrustedProxyCIDRs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty input yields no trusted proxies",
			input:     "",
			wantCount: 0,
		},
		{
			name:      "whitespace-only input yields no trusted proxies",
			input:     "   ",
			wantCount: 0,
		},
		{
			name:      "single IPv4 CIDR",
			input:     "10.0.0.0/8",
			wantCount: 1,
		},
		{
			name:      "multiple CIDRs with surrounding whitespace",
			input:     "10.0.0.0/8, 172.16.0.0/12 , 192.168.0.0/16",
			wantCount: 3,
		},
		{
			name:      "IPv6 CIDR",
			input:     "fd00::/8",
			wantCount: 1,
		},
		{
			name:      "trailing comma is tolerated",
			input:     "10.0.0.0/8,",
			wantCount: 1,
		},
		{
			name:      "malformed CIDR fails with actionable error",
			input:     "10.0.0.0/8, not-a-cidr",
			wantErr:   true,
			errSubstr: "TRUSTED_PROXY_CIDRS",
		},
		{
			name:      "bare IP without mask fails",
			input:     "10.0.0.1",
			wantErr:   true,
			errSubstr: "TRUSTED_PROXY_CIDRS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nets, err := parseTrustedProxyCIDRs(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, nets)

				return
			}

			require.NoError(t, err)
			assert.Len(t, nets, tt.wantCount)
		})
	}
}
