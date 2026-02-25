// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAccountingSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	ctx := context.Background()

	t.Run("returns defaults when SettingsPort is nil", func(t *testing.T) {
		uc := &UseCase{
			SettingsPort: nil,
		}

		result := uc.GetAccountingSettings(ctx, organizationID, ledgerID)

		assert.Equal(t, mmodel.DefaultAccountingSettings(), result)
	})

	t.Run("returns defaults when GetLedgerSettings fails", func(t *testing.T) {
		mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
		mockSettingsPort.EXPECT().
			GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
			Return(nil, errors.New("connection error"))

		uc := &UseCase{
			SettingsPort: mockSettingsPort,
		}

		result := uc.GetAccountingSettings(ctx, organizationID, ledgerID)

		assert.Equal(t, mmodel.DefaultAccountingSettings(), result)
	})

	t.Run("returns defaults when settings are empty", func(t *testing.T) {
		mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
		mockSettingsPort.EXPECT().
			GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{}, nil)

		uc := &UseCase{
			SettingsPort: mockSettingsPort,
		}

		result := uc.GetAccountingSettings(ctx, organizationID, ledgerID)

		assert.Equal(t, mmodel.DefaultAccountingSettings(), result)
	})

	t.Run("returns parsed settings when accounting settings exist", func(t *testing.T) {
		mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
		mockSettingsPort.EXPECT().
			GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
				},
			}, nil)

		uc := &UseCase{
			SettingsPort: mockSettingsPort,
		}

		result := uc.GetAccountingSettings(ctx, organizationID, ledgerID)

		assert.True(t, result.ValidateAccountType)
		assert.True(t, result.ValidateRoutes)
	})

	t.Run("returns partial settings when only some flags are set", func(t *testing.T) {
		mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
		mockSettingsPort.EXPECT().
			GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			}, nil)

		uc := &UseCase{
			SettingsPort: mockSettingsPort,
		}

		result := uc.GetAccountingSettings(ctx, organizationID, ledgerID)

		assert.True(t, result.ValidateAccountType)
		assert.False(t, result.ValidateRoutes)
	})
}
