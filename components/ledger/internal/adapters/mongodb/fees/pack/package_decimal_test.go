// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/stretchr/testify/assert"
)

func TestFromEntityCalculationArray_ReturnsErrorOnInvalidDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		calculations []model.Calculation
		wantErr      bool
	}{
		{
			name: "Error - invalid decimal value with comma",
			calculations: []model.Calculation{
				{Type: "percentage", Value: "10,50"},
			},
			wantErr: true,
		},
		{
			name: "Error - invalid decimal value with letters",
			calculations: []model.Calculation{
				{Type: "flat", Value: "abc"},
			},
			wantErr: true,
		},
		{
			name: "Success - valid decimal value",
			calculations: []model.Calculation{
				{Type: "percentage", Value: "10.50"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromEntityCalculationArray(tt.calculations)

			if tt.wantErr {
				assert.Error(t, err, "Expected error for invalid decimal value")
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, len(tt.calculations))
			}
		})
	}
}
