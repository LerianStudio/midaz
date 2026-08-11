// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeeChainPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		specPath string
		expected string
	}{
		{
			name:     "organization scope",
			specPath: "/organizations/{organization_id}",
			expected: "/organizations/:organization_id",
		},
		{
			name:     "consecutive parameters keep their own names",
			specPath: "/organizations/{organization_id}/ledgers/{ledger_id}",
			expected: "/organizations/:organization_id/ledgers/:ledger_id",
		},
		{
			name:     "literal-only path is unchanged",
			specPath: "/billing/calculate",
			expected: "/billing/calculate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, feeChainPath(tt.specPath))
		})
	}
}
