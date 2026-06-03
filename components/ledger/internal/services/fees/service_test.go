// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewUseCase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockMidazSvc := http.NewMockMidazClient(ctrl)

	tests := []struct {
		name            string
		packageRepo     pack.Repository
		midazClient     http.MidazClient
		defaultCurrency string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "Success - all dependencies provided",
			packageRepo:     mockPackRepo,
			midazClient:     mockMidazSvc,
			defaultCurrency: "BRL",
			wantErr:         false,
		},
		{
			name:            "Error - nil PackageRepo",
			packageRepo:     nil,
			midazClient:     mockMidazSvc,
			defaultCurrency: "BRL",
			wantErr:         true,
			errContains:     "PackageRepo",
		},
		{
			name:            "Error - nil MidazClient",
			packageRepo:     mockPackRepo,
			midazClient:     nil,
			defaultCurrency: "BRL",
			wantErr:         true,
			errContains:     "MidazClient",
		},
		{
			name:            "Error - empty DefaultCurrency",
			packageRepo:     mockPackRepo,
			midazClient:     mockMidazSvc,
			defaultCurrency: "",
			wantErr:         true,
			errContains:     "DefaultCurrency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, err := NewUseCase(tt.packageRepo, tt.midazClient, tt.defaultCurrency)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, uc)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, uc)
				assert.Equal(t, tt.packageRepo, uc.PackageRepo())
				assert.Equal(t, tt.midazClient, uc.MidazClient())
				assert.Equal(t, tt.defaultCurrency, uc.DefaultCurrency())
			}
		})
	}
}
