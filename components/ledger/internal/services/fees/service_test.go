// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewUseCase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)

	tests := []struct {
		name            string
		packageRepo     pack.Repository
		resolver        feeshared.MidazResolver
		defaultCurrency string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "Success - all dependencies provided",
			packageRepo:     mockPackRepo,
			resolver:        mockResolver,
			defaultCurrency: "BRL",
			wantErr:         false,
		},
		{
			name:            "Error - nil PackageRepo",
			packageRepo:     nil,
			resolver:        mockResolver,
			defaultCurrency: "BRL",
			wantErr:         true,
			errContains:     "PackageRepo",
		},
		{
			name:            "Error - nil MidazResolver",
			packageRepo:     mockPackRepo,
			resolver:        nil,
			defaultCurrency: "BRL",
			wantErr:         true,
			errContains:     "MidazResolver",
		},
		{
			name:            "Error - empty DefaultCurrency",
			packageRepo:     mockPackRepo,
			resolver:        mockResolver,
			defaultCurrency: "",
			wantErr:         true,
			errContains:     "DefaultCurrency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, err := NewUseCase(tt.packageRepo, tt.resolver, tt.defaultCurrency)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, uc)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, uc)
				assert.Equal(t, tt.packageRepo, uc.PackageRepo())
				assert.Equal(t, tt.resolver, uc.Resolver())
				assert.Equal(t, tt.defaultCurrency, uc.DefaultCurrency())
			}
		})
	}
}
