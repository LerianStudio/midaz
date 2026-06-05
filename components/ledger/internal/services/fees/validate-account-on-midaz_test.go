// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkg "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidateExistenceOfAccountOnMidaz_DeduplicatesAliases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := pkg.NewMockMidazResolver(ctrl)

	svc := &UseCase{
		packageRepo: mockPackRepo,
		resolver:    mockResolver,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name              string
		fees              map[string]model.Fee
		expectedCallCount int
		mockSetup         func()
		expectErr         bool
	}{
		{
			name: "Three fees with same alias - only one call",
			fees: map[string]model.Fee{
				"fee1": {CreditAccount: "account_a"},
				"fee2": {CreditAccount: "account_a"},
				"fee3": {CreditAccount: "account_a"},
			},
			expectedCallCount: 1,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "account_a").
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name: "Three fees with two unique aliases - two calls",
			fees: map[string]model.Fee{
				"fee1": {CreditAccount: "account_a"},
				"fee2": {CreditAccount: "account_b"},
				"fee3": {CreditAccount: "account_a"},
			},
			expectedCallCount: 2,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "account_a").
					Return(nil).
					Times(1)
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "account_b").
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name: "Five fees with all same alias - only one call",
			fees: map[string]model.Fee{
				"fee1": {CreditAccount: "shared_account"},
				"fee2": {CreditAccount: "shared_account"},
				"fee3": {CreditAccount: "shared_account"},
				"fee4": {CreditAccount: "shared_account"},
				"fee5": {CreditAccount: "shared_account"},
			},
			expectedCallCount: 1,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "shared_account").
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name: "Invalid alias returns error",
			fees: map[string]model.Fee{
				"fee1": {CreditAccount: "valid_account"},
				"fee2": {CreditAccount: "invalid_account"},
			},
			expectedCallCount: 2,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "valid_account").
					Return(nil).
					MaxTimes(1)
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), orgID, ledgerID, "invalid_account").
					Return(constant.ErrFindAccountOnMidaz).
					MaxTimes(1)
			},
			expectErr: true,
		},
		{
			name:              "Empty fees - no calls",
			fees:              map[string]model.Fee{},
			expectedCallCount: 0,
			mockSetup:         func() {},
			expectErr:         false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			cpi := model.CreatePackageInput{
				Fee: tt.fees,
			}

			ctx := context.Background()
			err := svc.validateExistenceOfAccountOnMidaz(ctx, cpi, orgID, ledgerID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "FEE-0014")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
