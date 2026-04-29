// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/stretchr/testify/assert"
)

func TestExpectedBackupStatusForCleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		transactionStatus string
		validate          *mtransaction.Responses
		expected          string
	}{
		{
			name:              "created promoted to approved maps back to CREATED",
			transactionStatus: constant.APPROVED,
			validate:          &mtransaction.Responses{Pending: false},
			expected:          constant.CREATED,
		},
		{
			name:              "pending transition keeps APPROVED",
			transactionStatus: constant.APPROVED,
			validate:          &mtransaction.Responses{Pending: true},
			expected:          constant.APPROVED,
		},
		{
			name:              "pending transition keeps CANCELED",
			transactionStatus: constant.CANCELED,
			validate:          &mtransaction.Responses{Pending: true},
			expected:          constant.CANCELED,
		},
		{
			name:              "nil validate keeps original status",
			transactionStatus: constant.APPROVED,
			validate:          nil,
			expected:          constant.APPROVED,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ExpectedBackupStatusForCleanup(tt.transactionStatus, tt.validate)
			assert.Equal(t, tt.expected, got)
		})
	}
}
