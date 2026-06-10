// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package multitenant

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTenantID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tenantID string
		wantErr  error
	}{
		{name: "valid tenant id", tenantID: "tenant-123", wantErr: nil},
		{name: "empty is required", tenantID: "", wantErr: ErrTenantIDRequired},
		{name: "leading underscore is invalid", tenantID: "_bad", wantErr: ErrTenantIDInvalid},
		{name: "space is invalid", tenantID: "bad tenant", wantErr: ErrTenantIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateTenantID(tt.tenantID)
			if tt.wantErr == nil {
				assert.NoError(t, err)

				return
			}

			assert.True(t, errors.Is(err, tt.wantErr),
				"expected errors.Is(%v, %v)", err, tt.wantErr)
		})
	}
}
