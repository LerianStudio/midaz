// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetParsedLedgerSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	ctx := context.Background()

	t.Run("returns defaults when GetLedgerSettings fails", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(nil, errors.New("connection error"))

		uc := &UseCase{
			LedgerRepo: mockLedgerRepo,
		}

		result := uc.GetParsedLedgerSettings(ctx, organizationID, ledgerID)

		assert.Equal(t, mmodel.DefaultLedgerSettings(), result)
	})

	t.Run("returns defaults when settings are empty", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{}, nil)

		uc := &UseCase{
			LedgerRepo: mockLedgerRepo,
		}

		result := uc.GetParsedLedgerSettings(ctx, organizationID, ledgerID)

		assert.Equal(t, mmodel.DefaultLedgerSettings(), result)
	})

	t.Run("returns parsed settings when accounting settings exist", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
				},
			}, nil)

		uc := &UseCase{
			LedgerRepo: mockLedgerRepo,
		}

		result := uc.GetParsedLedgerSettings(ctx, organizationID, ledgerID)

		assert.True(t, result.Accounting.ValidateAccountType)
		assert.True(t, result.Accounting.ValidateRoutes)
	})

	t.Run("returns partial settings when only some flags are set", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			}, nil)

		uc := &UseCase{
			LedgerRepo: mockLedgerRepo,
		}

		result := uc.GetParsedLedgerSettings(ctx, organizationID, ledgerID)

		assert.True(t, result.Accounting.ValidateAccountType)
		assert.False(t, result.Accounting.ValidateRoutes)
	})
}
