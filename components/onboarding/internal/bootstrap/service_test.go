// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceGetPGManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pgManager interface{}
		wantNil   bool
	}{
		{
			name:      "returns non-nil when pgManager is set",
			pgManager: "fake-pg-manager",
			wantNil:   false,
		},
		{
			name:      "returns nil when pgManager is not set (single-tenant)",
			pgManager: nil,
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &Service{
				pgManager: tt.pgManager,
			}

			got := svc.GetPGManager()

			if tt.wantNil {
				assert.Nil(t, got, "GetPGManager() should return nil in single-tenant mode")
			} else {
				assert.NotNil(t, got, "GetPGManager() should return non-nil when pgManager is set")
				assert.Equal(t, tt.pgManager, got)
			}
		})
	}
}

func TestServiceGetMongoManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mongoManager interface{}
		wantNil      bool
	}{
		{
			name:         "returns non-nil when mongoManager is set",
			mongoManager: "fake-mongo-manager",
			wantNil:      false,
		},
		{
			name:         "returns nil when mongoManager is not set (single-tenant)",
			mongoManager: nil,
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &Service{
				mongoManager: tt.mongoManager,
			}

			got := svc.GetMongoManager()

			if tt.wantNil {
				assert.Nil(t, got, "GetMongoManager() should return nil in single-tenant mode")
			} else {
				assert.NotNil(t, got, "GetMongoManager() should return non-nil when mongoManager is set")
				assert.Equal(t, tt.mongoManager, got)
			}
		})
	}
}
